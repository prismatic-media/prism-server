// Package transcoder implements a worker-pool-based DASH transcode engine.
//
// Architecture:
//   - Pool manages N worker goroutines polling the DB-backed queue.
//   - Workers claim jobs atomically from the DB, call ffmpeg, and update DB state.
//   - Hub broadcasts progress events to WebSocket subscribers.
package transcoder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/artifact"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/storage"
	"github.com/ringmaster217/prism/internal/store/sqlite"
	"github.com/ringmaster217/prism/pkg/dash"
	"github.com/ringmaster217/prism/pkg/events"
	"github.com/ringmaster217/prism/pkg/ffmpeg"
	"github.com/ringmaster217/prism/pkg/fingerprint"
)

// ProgressEvent is emitted during a transcode job.
type ProgressEvent struct {
	JobID    uuid.UUID `json:"job_id"`
	Progress float64   `json:"progress"` // 0–100
	Done     bool      `json:"done"`
	Error    string    `json:"error,omitempty"`
}

// Hub broadcasts ProgressEvents to registered WebSocket subscribers.
// Subscribers register a channel via Subscribe and deregister via Unsubscribe.
type Hub struct {
	mu   sync.RWMutex
	subs map[uuid.UUID][]chan ProgressEvent
}

func newHub() *Hub { return &Hub{subs: make(map[uuid.UUID][]chan ProgressEvent)} }

// Subscribe returns a channel that receives events for jobID. The caller must
// call Unsubscribe with the returned channel when done.
func (h *Hub) Subscribe(jobID uuid.UUID) <-chan ProgressEvent {
	ch := make(chan ProgressEvent, 16)
	h.mu.Lock()
	h.subs[jobID] = append(h.subs[jobID], ch)
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes ch from subscribers for jobID.
func (h *Hub) Unsubscribe(jobID uuid.UUID, ch <-chan ProgressEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.subs[jobID]
	for i, s := range list {
		if s == ch {
			h.subs[jobID] = append(list[:i], list[i+1:]...)
			close(s)
			return
		}
	}
}

func (h *Hub) Publish(evt ProgressEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subs[evt.JobID] {
		select {
		case ch <- evt:
		default:
			// Drop if subscriber is slow — non-blocking.
		}
	}
}

// Pool manages a fixed number of transcode worker goroutines.
type Pool struct {
	db           *sql.DB
	mpdCache     *dash.Cache
	eventBus     *events.Bus
	hub          *Hub
	workers      int
	pollInterval atomic.Int64 // nanoseconds; read atomically by workers
	wg           sync.WaitGroup
	stopCh       chan struct{}
	stopOnce     sync.Once
}

// NewPool creates a Pool. Call Start to begin processing.
func NewPool(db *sql.DB, workers int, mpdCache *dash.Cache, bus *events.Bus) *Pool {
	p := &Pool{
		db:       db,
		mpdCache: mpdCache,
		eventBus: bus,
		hub:      newHub(),
		workers:  workers,
		stopCh:   make(chan struct{}),
	}
	p.pollInterval.Store(int64(15 * time.Second))
	return p
}

// Hub returns the event hub for WebSocket subscriptions.
func (p *Pool) Hub() *Hub { return p.hub }

// MPDCache returns the in-process MPD path cache.
func (p *Pool) MPDCache() *dash.Cache { return p.mpdCache }

// Start launches worker goroutines.
// It is non-blocking; workers run until ctx is cancelled or Stop is called.
func (p *Pool) Start(ctx context.Context) error {
	if err := sqlite.RecoverStaleJobs(ctx, p.db); err != nil {
		return fmt.Errorf("recovering stale jobs: %w", err)
	}

	p.pollInterval.Store(int64(p.loadPollInterval(ctx)))

	if p.eventBus != nil {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.runAutoEnqueueListener(ctx)
		}()
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.runWorkerMonitor(ctx)
	}()

	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.runWorker(ctx)
		}()
	}

	return nil
}

// Stop signals workers to stop and waits for them to finish.
func (p *Pool) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})
	p.wg.Wait()
}

// Enqueue creates a new transcode job for mediaItemID, or resets/reuses an existing one.
func (p *Pool) Enqueue(ctx context.Context, mediaItemID uuid.UUID) (*models.TranscodeJob, error) {
	item, err := sqlite.GetMediaItemByID(ctx, p.db, mediaItemID)
	if err != nil {
		return nil, err
	}
	if item.BundleStatus == models.BundleStatusAvailable && item.TranscodeStatus == models.TranscodeStatusDone {
		return nil, fmt.Errorf("media item already transcoded and has bundle available")
	}

	existingJob, err := sqlite.GetTranscodeJobByMediaItem(ctx, p.db, mediaItemID)
	if err != nil && !errors.Is(err, sqlite.ErrNotFound) {
		return nil, err
	}

	var j *models.TranscodeJob
	if existingJob != nil {
		// If the job is pending or processing, return it as-is (do not reset it or disrupt the current worker).
		if existingJob.Status == models.TranscodeStatusPending || existingJob.Status == models.TranscodeStatusProcessing {
			return existingJob, nil
		}

		// Otherwise, it was done or failed. Reset it.
		// Update created_at to now, preserve priority.
		now := time.Now().UTC()
		existingJob.Status = models.TranscodeStatusPending
		existingJob.Progress = 0
		existingJob.WorkerID = nil
		existingJob.ErrorMsg = nil
		existingJob.StartedAt = nil
		existingJob.FinishedAt = nil
		existingJob.CreatedAt = now

		// Update in DB
		if err := sqlite.ResetTranscodeJob(ctx, p.db, existingJob); err != nil {
			return nil, err
		}
		j = existingJob
	} else {
		// Create new job
		j = &models.TranscodeJob{MediaItemID: mediaItemID}
		if err := sqlite.CreateTranscodeJob(ctx, p.db, j); err != nil {
			return nil, err
		}
	}

	if err := sqlite.SetMediaTranscodeStatus(ctx, p.db, mediaItemID, models.TranscodeStatusPending); err != nil {
		return nil, err
	}
	p.mpdCache.Invalidate(mediaItemID)
	return j, nil
}


func (p *Pool) runWorker(ctx context.Context) {
	for {
		if p.shouldStop(ctx) {
			return
		}

		j, err := sqlite.ClaimNextJob(ctx, p.db, nil)
		if err != nil {
			slog.Warn("claiming next transcode job failed", "error", err)
			if !p.sleepOrStop(ctx, time.Duration(p.pollInterval.Load())) {
				return
			}
			continue
		}

		if j == nil {
			// Reload poll interval from DB so changes take effect without restart.
			p.pollInterval.Store(int64(p.loadPollInterval(ctx)))
			if !p.sleepOrStop(ctx, time.Duration(p.pollInterval.Load())) {
				return
			}
			continue
		}

		p.process(ctx, j)
	}
}

func (p *Pool) runWorkerMonitor(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			requeued, err := sqlite.RecoverFailedWorkers(ctx, p.db, 30*time.Second)
			if err != nil {
				slog.Error("failed to recover failed remote workers", "error", err)
				continue
			}

			for _, job := range requeued {
				slog.Info("Requeued transcode job due to worker heartbeat timeout", "job_id", job.JobID, "media_item_id", job.MediaItemID)

				// Publish to WebSocket hub
				p.hub.Publish(ProgressEvent{
					JobID:    job.JobID,
					Progress: 0,
					Done:     false,
				})

				// Publish events to the system bus
				if p.eventBus != nil {
					p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
						JobID:       job.JobID,
						MediaItemID: job.MediaItemID,
						Progress:    0,
						Done:        false,
					})
					p.eventBus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
						MediaItemID:     job.MediaItemID,
						LibraryID:       job.LibraryID,
						TranscodeStatus: string(models.TranscodeStatusPending),
					})
				}
			}
		}
	}
}

func (p *Pool) runAutoEnqueueListener(ctx context.Context) {
	subID, ch := p.eventBus.Subscribe()
	defer p.eventBus.Unsubscribe(subID)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if evt.Type != events.EventMediaCreated {
				continue
			}

			var payload events.MediaCreatedPayload
			if err := json.Unmarshal(evt.Payload, &payload); err != nil {
				slog.Warn("could not decode media.created payload", "error", err)
				continue
			}

			enabled, err := sqlite.GetSetting(ctx, p.db, "auto_transcode_on_discovery")
			if err != nil || enabled != "true" {
				continue
			}

			item, err := sqlite.GetMediaItemByID(ctx, p.db, payload.MediaItemID)
			if err != nil {
				continue
			}
			if item.TranscodeStatus != models.TranscodeStatusNone {
				continue
			}

			hasJob, err := sqlite.HasTranscodeJobForMediaItem(ctx, p.db, payload.MediaItemID)
			if err != nil || hasJob {
				continue
			}

			if _, err := p.Enqueue(ctx, payload.MediaItemID); err != nil {
				slog.Warn("auto-enqueue on discovery failed", "media_item_id", payload.MediaItemID, "error", err)
			}
		}
	}
}

func (p *Pool) shouldStop(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	case <-p.stopCh:
		return true
	default:
		return false
	}
}

func (p *Pool) sleepOrStop(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-p.stopCh:
		return false
	case <-t.C:
		return true
	}
}

func (p *Pool) loadPollInterval(ctx context.Context) time.Duration {
	const defaultInterval = 15

	raw, err := sqlite.GetSetting(ctx, p.db, "transcode_poll_interval")
	if err != nil {
		return time.Duration(defaultInterval) * time.Second
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return time.Duration(defaultInterval) * time.Second
	}

	return time.Duration(seconds) * time.Second
}

// process runs a single transcode job.
func (p *Pool) process(ctx context.Context, j *models.TranscodeJob) {
	log := slog.With("job_id", j.ID, "media_item_id", j.MediaItemID)
	log.Info("starting transcode job")

	// Job status is set to processing by ClaimNextJob; only update media status here.
	if err := sqlite.SetMediaTranscodeStatus(ctx, p.db, j.MediaItemID, models.TranscodeStatusProcessing); err != nil {
		log.Warn("could not set media processing status", "error", err)
	}

	item, err := sqlite.GetMediaItemByID(ctx, p.db, j.MediaItemID)
	if err != nil {
		p.fail(ctx, j, fmt.Sprintf("fetching media item: %v", err))
		return
	}

	if p.eventBus != nil {
		p.eventBus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
			MediaItemID:     j.MediaItemID,
			LibraryID:       item.LibraryID,
			TranscodeStatus: string(models.TranscodeStatusProcessing),
		})
	}

	// Probe to get duration for progress estimation.
	ffmpegPath := "ffmpeg"
	ffprobePath := "ffprobe"

	var duration float64
	if ffprobePath != "" {
		if probe, err := ffmpeg.Probe(ctx, ffprobePath, item.FilePath); err == nil {
			duration = probe.Duration
		}
	}

	outputDir, err := p.SelectSegmentsOutputDir(ctx, j.MediaItemID)
	if err != nil {
		p.fail(ctx, j, err.Error())
		return
	}
	mpdPath := filepath.Join(outputDir, "manifest.mpd")

	profiles := ffmpeg.DefaultProfiles()
	// Filter out profiles that would require upscaling. A profile is kept when:
	//   a) the source height is >= the profile height (standard check), OR
	//   b) the source width is >= the profile's reference width — this allows
	//      wide-format sources (e.g. 1920x800) to receive the 1080p rendition
	//      because their horizontal resolution is sufficient.
	if item.Height > 0 && item.Width > 0 {
		var filtered []ffmpeg.RenditionProfile
		for _, prof := range profiles {
			if prof.Height <= item.Height || (prof.Width > 0 && item.Width >= prof.Width) {
				filtered = append(filtered, prof)
			}
		}
		if len(filtered) > 0 {
			profiles = filtered
		}
	}

	// progressFn receives an overall 0-100 percent computed by TranscodeDASH.
	progressFn := func(pct float64) {
		if pct > 99 {
			pct = 99
		}
		_ = sqlite.UpdateJobProgress(ctx, p.db, j.ID, pct)
		p.hub.Publish(ProgressEvent{JobID: j.ID, Progress: pct})
		if p.eventBus != nil {
			p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
				JobID:       j.ID,
				MediaItemID: j.MediaItemID,
				Progress:    pct,
			})
		}
	}

	// Probe for subtitle streams.
	var subtitleStreams []ffmpeg.SubtitleStream
	if ffprobePath != "" {
		if probe, err := ffmpeg.Probe(ctx, ffprobePath, item.FilePath); err == nil {
			subtitleStreams = probe.SubtitleStreams
		}
	}

	hwaccel, err := sqlite.GetSetting(ctx, p.db, "ffmpeg_hwaccel")
	if err != nil {
		hwaccel = "none"
	}

	opts := ffmpeg.TranscodeOptions{
		InputPath:       item.FilePath,
		OutputDir:       outputDir,
		Profiles:        profiles,
		Duration:        duration,
		SourceWidth:     item.Width,
		SourceHeight:    item.Height,
		SubtitleStreams: subtitleStreams,
		ProgressFn:      progressFn,
		HWAccelType:     hwaccel,
	}

	if err := ffmpeg.TranscodeDASH(ctx, ffmpegPath, opts); err != nil {
		p.fail(ctx, j, err.Error())
		return
	}

	// Build rendition infos for MPD generation.
	renditions := make([]dash.RenditionInfo, len(profiles))
	for i, prof := range profiles {
		renditions[i] = dash.RenditionInfo{
			Name:          prof.Name,
			Height:        prof.Height,
			VideoBitrateK: prof.VideoBitrateK,
			AudioBitrateK: prof.AudioBitrateK,
		}
	}

	// Collect extracted subtitle VTT files (embedded streams).
	var subs []dash.SubtitleInfo
	for _, s := range subtitleStreams {
		lang := s.Language
		if lang == "" {
			lang = fmt.Sprintf("%d", s.Index)
		}
		vttPath := filepath.Join(outputDir, "sub_"+lang+".vtt")
		subs = append(subs, dash.SubtitleInfo{Language: lang, VTTPath: vttPath})
	}

	// Also collect any sidecar .srt/.vtt files next to the source video.
	sidecars := ffmpeg.FindSidecarSubtitles(item.FilePath)
	for _, sc := range sidecars {
		// Avoid overwriting a track already extracted from the container.
		lang := sc.Language
		if lang == "" {
			lang = "default"
		}
		alreadyHave := false
		for _, existing := range subs {
			if existing.Language == lang {
				alreadyHave = true
				break
			}
		}
		if alreadyHave {
			continue
		}
		vttPath, err := ffmpeg.CopySidecarSubtitle(ctx, ffmpegPath, sc, outputDir)
		if err != nil {
			slog.Warn("sidecar subtitle processing failed", "path", sc.Path, "error", err)
			continue
		}
		subs = append(subs, dash.SubtitleInfo{Language: lang, VTTPath: vttPath})
	}

	if err := dash.GenerateMPD(outputDir, mpdPath, renditions, subs, duration); err != nil {
		p.fail(ctx, j, fmt.Sprintf("generating MPD: %v", err))
		return
	}

	// Persist results.
	if err := sqlite.SetMediaMPDPath(ctx, p.db, j.MediaItemID, mpdPath); err != nil {
		log.Warn("could not set mpd path", "error", err)
	}
	p.mpdCache.Set(j.MediaItemID, mpdPath)

	// Fetch the latest version of the media item to capture any metadata
	// written by the Enricher (which runs concurrently during transcoding).
	if latestItem, err := sqlite.GetMediaItemByID(ctx, p.db, j.MediaItemID); err == nil {
		item = latestItem
	} else {
		log.Warn("could not reload latest media item for sidecar", "error", err)
	}

	// Write artifact metadata sidecar for recovery. This is migration-safe:
	// it writes only to the filesystem (no database queries) so it works
	// before and after migration 00006 is applied.
	if err := writeArtifactSidecar(ctx, p.db, item, outputDir, profiles, duration); err != nil {
		log.Warn("could not write artifact sidecar", "error", err)
	}

	now := time.Now()
	j.FinishedAt = &now
	if err := sqlite.UpdateJobStatus(ctx, p.db, j.ID, models.TranscodeStatusDone, nil); err != nil {
		log.Warn("could not mark job done", "error", err)
	}
	if err := sqlite.UpdateJobProgress(ctx, p.db, j.ID, 100); err != nil {
		log.Warn("could not set final progress", "error", err)
	}

	p.hub.Publish(ProgressEvent{JobID: j.ID, Progress: 100, Done: true})
	if p.eventBus != nil {
		p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
			JobID:       j.ID,
			MediaItemID: j.MediaItemID,
			Progress:    100,
			Done:        true,
		})
		p.eventBus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
			MediaItemID:     j.MediaItemID,
			LibraryID:       item.LibraryID,
			TranscodeStatus: string(models.TranscodeStatusDone),
		})
	}
	log.Info("transcode job complete", "mpd", mpdPath)
}

func (p *Pool) loadStorageMinFreeBytes(ctx context.Context) uint64 {
	const defaultMinFree uint64 = 20 * 1024 * 1024 * 1024
	raw, err := sqlite.GetSetting(ctx, p.db, "storage_min_free_bytes")
	if err != nil {
		return defaultMinFree
	}
	v, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return defaultMinFree
	}
	return v
}

func (p *Pool) SelectSegmentsOutputDir(ctx context.Context, mediaID uuid.UUID) (string, error) {
	areas, err := sqlite.ListStorageAreasByKind(ctx, p.db, models.StorageAreaKindSegments, true)
	if err != nil {
		return "", fmt.Errorf("loading segment storage areas: %w", err)
	}
	if len(areas) == 0 {
		return "", fmt.Errorf("no eligible segment storage area: no enabled segments areas configured")
	}

	minFree := p.loadStorageMinFreeBytes(ctx)
	bestPath := ""
	bestFree := uint64(0)
	statuses := make([]string, 0, len(areas))

	for _, area := range areas {
		m := storage.CollectPathMetrics(area.Path, minFree, true)
		statuses = append(statuses, fmt.Sprintf("%s=%s", area.Path, m.Status))
		if !m.EligibleSegment {
			continue
		}
		if bestPath == "" || m.FreeBytes > bestFree {
			bestPath = area.Path
			bestFree = m.FreeBytes
		}
	}

	if bestPath == "" {
		return "", fmt.Errorf("no eligible segment storage area (min_free_bytes=%d): %s", minFree, strings.Join(statuses, ", "))
	}

	return filepath.Join(bestPath, mediaID.String()), nil
}

func (p *Pool) fail(ctx context.Context, j *models.TranscodeJob, errMsg string) {
	slog.Warn("transcode job failed", "job_id", j.ID, "error", errMsg)
	_ = sqlite.UpdateJobStatus(ctx, p.db, j.ID, models.TranscodeStatusFailed, &errMsg)
	_ = sqlite.SetMediaTranscodeStatus(ctx, p.db, j.MediaItemID, models.TranscodeStatusFailed)
	p.hub.Publish(ProgressEvent{JobID: j.ID, Progress: 0, Done: true, Error: errMsg})
	if p.eventBus != nil {
		p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
			JobID:       j.ID,
			MediaItemID: j.MediaItemID,
			Done:        true,
			Error:       errMsg,
		})
	}
}

func writeArtifactSidecar(ctx context.Context, db *sql.DB, item *models.MediaItem, outputDir string, profiles []ffmpeg.RenditionProfile, duration float64) error {
	slog.Info("writeArtifactSidecar: evaluating write", "outputDir", outputDir, "itemID", item.ID.String(), "filePath", item.FilePath)
	if outputDir == "" || item.ID == uuid.Nil || item.FilePath == "" {
		slog.Warn("writeArtifactSidecar: skipped writing sidecar due to empty parameter(s)")
		return nil
	}

	var fp string
	if item.SourceFingerprint != nil && *item.SourceFingerprint != "" {
		fp = *item.SourceFingerprint
		slog.Info("writeArtifactSidecar: using existing fingerprint from DB", "fp", fp)
	} else {
		slog.Info("writeArtifactSidecar: generating fingerprint", "filePath", item.FilePath)
		var err error
		fp, err = fingerprint.GenerateDeterministic(item.FilePath)
		if err != nil {
			slog.Error("writeArtifactSidecar: generating fingerprint failed", "error", err)
			return fmt.Errorf("generating source fingerprint: %w", err)
		}
		slog.Info("writeArtifactSidecar: fingerprint generated", "fp", fp)
	}

	// Derive a relative source path from the file path.
	// This is a best-effort normalization for the sidecar.
	sourcePath := item.FilePath

	var profilesInfo []artifact.RenditionInfo
	for _, p := range profiles {
		profilesInfo = append(profilesInfo, artifact.RenditionInfo{
			Name:          p.Name,
			Height:        p.Height,
			Width:         p.Width,
			VideoBitrateK: p.VideoBitrateK,
			AudioBitrateK: p.AudioBitrateK,
		})
	}

	meta := &artifact.SidecarMetadata{
		MediaItemID:       item.ID.String(),
		SourcePath:        sourcePath,
		SourceFingerprint: fp,
		OutputDir:         outputDir,
		MPDPath:           "manifest.mpd",
		Profiles:          profilesInfo,
		Duration:          duration,
	}

	slog.Info("writeArtifactSidecar: writing sidecar metadata file", "outputDir", outputDir)
	if err := artifact.WriteSidecar(outputDir, meta); err != nil {
		slog.Error("writeArtifactSidecar: writing sidecar failed", "error", err)
		return fmt.Errorf("writing artifact sidecar: %w", err)
	}
	slog.Info("writeArtifactSidecar: sidecar metadata file written successfully")
	return nil
}

// WriteSidecarForMediaItem fetches the media item and writes its sidecar metadata
// file directly without running the FFmpeg transcode pipeline.
func WriteSidecarForMediaItem(ctx context.Context, db *sql.DB, itemID uuid.UUID) error {
	item, err := sqlite.GetMediaItemByID(ctx, db, itemID)
	if err != nil {
		return fmt.Errorf("getting media item: %w", err)
	}

	if item.MPDPath == nil || *item.MPDPath == "" {
		return fmt.Errorf("media item does not have a transcode bundle (mpd_path is empty)")
	}

	outputDir := filepath.Dir(*item.MPDPath)

	// Try reading the existing sidecar to preserve transcode rendition profiles.
	var profiles []ffmpeg.RenditionProfile
	if existing, err := artifact.ReadSidecar(outputDir); err == nil && existing != nil {
		for _, p := range existing.Profiles {
			profiles = append(profiles, ffmpeg.RenditionProfile{
				Name:          p.Name,
				Height:        p.Height,
				Width:         p.Width,
				VideoBitrateK: p.VideoBitrateK,
				AudioBitrateK: p.AudioBitrateK,
			})
		}
	}

	// If no existing sidecar was found or profiles list is empty, derive from default profiles
	if len(profiles) == 0 {
		profiles = ffmpeg.DefaultProfiles()
		if item.Height > 0 && item.Width > 0 {
			var filtered []ffmpeg.RenditionProfile
			for _, prof := range profiles {
				if prof.Height <= item.Height || (prof.Width > 0 && item.Width >= prof.Width) {
					filtered = append(filtered, prof)
				}
			}
			if len(filtered) > 0 {
				profiles = filtered
			}
		}
	}

	return writeArtifactSidecar(ctx, db, item, outputDir, profiles, item.Duration)
}

