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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/artifact"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/storage"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/dash"
	"github.com/prismatic-media/prism-server/pkg/events"
	"github.com/prismatic-media/prism-server/pkg/ffmpeg"
	"github.com/prismatic-media/prism-server/pkg/fingerprint"
)

// ProgressEvent is emitted during a transcode job.
type ProgressEvent struct {
	JobID    uuid.UUID                 `json:"job_id"`
	Progress float64                   `json:"progress"` // 0–100
	Done     bool                      `json:"done"`
	Error    string                    `json:"error,omitempty"`
	SubJobs  []*models.TranscodeSubJob `json:"sub_jobs,omitempty"`
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
func (p *Pool) Enqueue(ctx context.Context, mediaItemID uuid.UUID, force bool) (*models.TranscodeJob, error) {
	item, err := sqlite.GetMediaItemByID(ctx, p.db, mediaItemID)
	if err != nil {
		return nil, err
	}
	if !force && item.BundleStatus == models.BundleStatusAvailable && item.TranscodeStatus == models.TranscodeStatusDone {
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

		j, err := sqlite.ClaimNextSubJob(ctx, p.db, nil)
		if err != nil {
			slog.Warn("claiming next transcode sub-job failed", "error", err)
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

				subJobs, _ := sqlite.ListTranscodeSubJobsByJob(ctx, p.db, job.JobID)

				// Publish to WebSocket hub
				p.hub.Publish(ProgressEvent{
					JobID:    job.JobID,
					Progress: 0,
					Done:     false,
					SubJobs:  subJobs,
				})

				// Publish events to the system bus
				if p.eventBus != nil {
					p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
						JobID:       job.JobID,
						MediaItemID: job.MediaItemID,
						Progress:    0,
						Done:        false,
						SubJobs:     subJobs,
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

			if _, err := p.Enqueue(ctx, payload.MediaItemID, false); err != nil {
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

// process runs a single transcode sub-job.
func (p *Pool) process(ctx context.Context, j *models.TranscodeSubJob) {
	parentJob, err := sqlite.GetTranscodeJobByID(ctx, p.db, j.JobID)
	if err != nil {
		p.fail(ctx, j, fmt.Sprintf("fetching parent job: %v", err))
		return
	}

	log := slog.With("sub_job_id", j.ID, "job_id", j.JobID, "media_item_id", parentJob.MediaItemID)
	log.Info("starting transcode sub-job", "type", j.Type)

	item, err := sqlite.GetMediaItemByID(ctx, p.db, parentJob.MediaItemID)
	if err != nil {
		p.fail(ctx, j, fmt.Sprintf("fetching media item: %v", err))
		return
	}

	// Clean up old transcode files if a bundle is currently available,
	// but ONLY if this is the first sub-job of the parent job to start processing.
	isFirstSubJob := false
	if subJobs, err := sqlite.ListTranscodeSubJobsByJob(ctx, p.db, j.JobID); err == nil {
		nonPendingCount := 0
		for _, sj := range subJobs {
			if sj.Status != models.TranscodeStatusPending {
				nonPendingCount++
			}
		}
		if nonPendingCount <= 1 {
			isFirstSubJob = true
		}
	}

	if isFirstSubJob && item.BundleStatus == models.BundleStatusAvailable {
		outputDir, err := p.SelectSegmentsOutputDir(ctx, parentJob.MediaItemID)
		if err == nil && outputDir != "" {
			log.Info("cleaning up old transcode bundle directory before local processing", "path", outputDir)
			if err := os.RemoveAll(outputDir); err != nil {
				log.Warn("failed to delete old transcode directory", "error", err)
			}
		}
		if err := sqlite.SetMediaBundleStatus(ctx, p.db, parentJob.MediaItemID, models.BundleStatusNone); err != nil {
			log.Warn("failed to update bundle status to none", "error", err)
		}
	}

	if p.eventBus != nil {
		p.eventBus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
			MediaItemID:     parentJob.MediaItemID,
			LibraryID:       item.LibraryID,
			TranscodeStatus: string(models.TranscodeStatusProcessing),
		})
	}

	ffmpegPath := "ffmpeg"
	ffprobePath := "ffprobe"

	var duration float64
	var subtitleStreams []ffmpeg.SubtitleStream
	var isHDR bool
	var pixFmt, colorSpace, colorTransfer, colorPrimaries string

	if ffprobePath != "" {
		if probe, err := ffmpeg.Probe(ctx, ffprobePath, item.FilePath); err == nil {
			duration = probe.Duration
			subtitleStreams = probe.SubtitleStreams
			isHDR = probe.IsHDR()
			pixFmt = probe.PixFmt
			colorSpace = probe.ColorSpace
			colorTransfer = probe.ColorTransfer
			colorPrimaries = probe.ColorPrimaries
		}
	}

	outputDir, err := p.SelectSegmentsOutputDir(ctx, parentJob.MediaItemID)
	if err != nil {
		p.fail(ctx, j, err.Error())
		return
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		p.fail(ctx, j, fmt.Sprintf("creating output dir: %v", err))
		return
	}

	if j.Type == models.SubJobTypeVideo {
		if j.ProfileName == nil {
			p.fail(ctx, j, "video sub-job has nil profile name")
			return
		}
		prof := ffmpeg.RenditionProfile{
			Name:          *j.ProfileName,
			Height:        *j.Height,
			Width:         *j.Width,
			VideoBitrateK: *j.VideoBitrateK,
			AudioBitrateK: *j.AudioBitrateK,
			Codec:         *j.Codec,
		}

		progressFn := func(pct float64) {
			if pct > 99 {
				pct = 99
			}
			_ = sqlite.UpdateSubJobProgress(ctx, p.db, j.ID, pct)
			
			parentJob, err := sqlite.GetTranscodeJobByID(ctx, p.db, j.JobID)
			if err == nil && parentJob != nil {
				p.hub.Publish(ProgressEvent{
					JobID:    j.JobID,
					Progress: parentJob.Progress,
					SubJobs:  parentJob.SubJobs,
				})
				if p.eventBus != nil {
					p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
						JobID:       j.JobID,
						MediaItemID: parentJob.MediaItemID,
						Progress:    parentJob.Progress,
						SubJobs:     parentJob.SubJobs,
					})
				}
			}
		}

		hwaccel, err := sqlite.GetSetting(ctx, p.db, "ffmpeg_hwaccel")
		if err != nil {
			hwaccel = "none"
		}

		opts := ffmpeg.TranscodeOptions{
			InputPath:            item.FilePath,
			OutputDir:            outputDir,
			Profiles:             []ffmpeg.RenditionProfile{prof},
			Duration:             duration,
			SourceWidth:          item.Width,
			SourceHeight:         item.Height,
			ProgressFn:           progressFn,
			HWAccelType:          hwaccel,
			SourceIsHDR:          isHDR,
			SourcePixFmt:         pixFmt,
			SourceColorSpace:     colorSpace,
			SourceColorTransfer:  colorTransfer,
			SourceColorPrimaries: colorPrimaries,
		}

		if err := ffmpeg.TranscodeDASH(ctx, ffmpegPath, opts); err != nil {
			p.fail(ctx, j, err.Error())
			return
		}

	} else if j.Type == models.SubJobTypeSubtitles {
		if err := ffmpeg.ExtractSubtitles(ctx, ffmpegPath, item.FilePath, outputDir, subtitleStreams); err != nil {
			log.Warn("failed to extract embedded subtitles", "error", err)
		}

		var existingLangs = make(map[string]bool)
		for _, s := range subtitleStreams {
			lang := s.Language
			if lang == "" {
				lang = fmt.Sprintf("%d", s.Index)
			}
			existingLangs[lang] = true
		}

		sidecars := ffmpeg.FindSidecarSubtitles(item.FilePath)
		for _, sc := range sidecars {
			lang := sc.Language
			if lang == "" {
				lang = "default"
			}
			if existingLangs[lang] {
				continue
			}
			_, err := ffmpeg.CopySidecarSubtitle(ctx, ffmpegPath, sc, outputDir)
			if err != nil {
				log.Warn("sidecar subtitle processing failed", "path", sc.Path, "error", err)
				continue
			}
		}
	} else {
		p.fail(ctx, j, fmt.Sprintf("unknown sub-job type: %s", j.Type))
		return
	}

	if err := sqlite.UpdateSubJobStatus(ctx, p.db, j.ID, models.TranscodeStatusDone, nil); err != nil {
		log.Warn("could not mark sub-job done", "error", err)
	}
	if err := sqlite.UpdateSubJobProgress(ctx, p.db, j.ID, 100); err != nil {
		log.Warn("could not set sub-job final progress", "error", err)
	}

	if err := RegenerateManifestForJob(ctx, p.db, j.JobID, outputDir); err != nil {
		log.Warn("failed to regenerate manifest", "error", err)
	}

	mpdPath := filepath.Join(outputDir, "manifest.mpd")
	p.mpdCache.Set(parentJob.MediaItemID, mpdPath)

	if err := WriteSidecarForMediaItem(ctx, p.db, parentJob.MediaItemID); err != nil {
		log.Warn("failed to write recovery sidecar", "error", err)
	}

	parentJob, err = sqlite.GetTranscodeJobByID(ctx, p.db, j.JobID)
	if err == nil && parentJob != nil {
		isDone := parentJob.Status == models.TranscodeStatusDone || parentJob.Status == models.TranscodeStatusFailed
		var errStr string
		if parentJob.ErrorMsg != nil {
			errStr = *parentJob.ErrorMsg
		}

		p.hub.Publish(ProgressEvent{
			JobID:    parentJob.ID,
			Progress: parentJob.Progress,
			Done:     isDone,
			Error:    errStr,
			SubJobs:  parentJob.SubJobs,
		})

		if p.eventBus != nil {
			p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
				JobID:       parentJob.ID,
				MediaItemID: parentJob.MediaItemID,
				Progress:    parentJob.Progress,
				Done:        isDone,
				Error:       errStr,
				SubJobs:     parentJob.SubJobs,
			})
			if isDone {
				p.eventBus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
					MediaItemID:     parentJob.MediaItemID,
					LibraryID:       item.LibraryID,
					TranscodeStatus: string(parentJob.Status),
				})
			} else {
				p.eventBus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
					MediaItemID:     parentJob.MediaItemID,
					LibraryID:       item.LibraryID,
					TranscodeStatus: string(models.TranscodeStatusProcessing),
				})
			}
		}
	}

	log.Info("transcode sub-job complete", "type", j.Type)
}

func (p *Pool) fail(ctx context.Context, j *models.TranscodeSubJob, errMsg string) {
	slog.Warn("transcode sub-job failed", "sub_job_id", j.ID, "job_id", j.JobID, "error", errMsg)
	_ = sqlite.UpdateSubJobStatus(ctx, p.db, j.ID, models.TranscodeStatusFailed, &errMsg)

	var subJobs []*models.TranscodeSubJob
	parentJob, err := sqlite.GetTranscodeJobByID(ctx, p.db, j.JobID)
	if err == nil && parentJob != nil {
		subJobs = parentJob.SubJobs
	}

	p.hub.Publish(ProgressEvent{
		JobID:    j.JobID,
		Progress: 0,
		Done:     true,
		Error:    errMsg,
		SubJobs:  subJobs,
	})
	if p.eventBus != nil {
		p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
			JobID:       j.JobID,
			MediaItemID: j.MediaItemID,
			Done:        true,
			Error:       errMsg,
			SubJobs:     subJobs,
		})

		if parentJob != nil {
			item, err := sqlite.GetMediaItemByID(ctx, p.db, parentJob.MediaItemID)
			var libraryID uuid.UUID
			if err == nil && item != nil {
				libraryID = item.LibraryID
			}
			p.eventBus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
				MediaItemID:     parentJob.MediaItemID,
				LibraryID:       libraryID,
				TranscodeStatus: string(models.TranscodeStatusFailed),
			})
		}
	}
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

	mpdPath := filepath.Join(outputDir, "manifest.mpd")
	sizes := artifact.GetTranscodeSizesInfo(mpdPath)

	meta := &artifact.SidecarMetadata{
		MediaItemID:       item.ID.String(),
		SourcePath:        sourcePath,
		SourceFingerprint: fp,
		OutputDir:         outputDir,
		MPDPath:           "manifest.mpd",
		Profiles:          profilesInfo,
		Duration:          duration,
		Sizes:             &sizes,
	}

	slog.Info("writeArtifactSidecar: writing sidecar metadata file", "outputDir", outputDir)
	if err := artifact.WriteSidecar(outputDir, meta); err != nil {
		slog.Error("writeArtifactSidecar: writing sidecar failed", "error", err)
		return fmt.Errorf("writing artifact sidecar: %w", err)
	}
	slog.Info("writeArtifactSidecar: sidecar metadata file written successfully")

	// Update transcode sizes in the database
	if err := sqlite.SetMediaTranscodeSizes(ctx, db, item.ID, &sizes); err != nil {
		slog.Error("writeArtifactSidecar: updating transcode sizes in database failed", "error", err)
	} else {
		slog.Info("writeArtifactSidecar: transcode sizes updated in database successfully")
		item.TranscodeSizes = &sizes
	}
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

	// If no existing sidecar was found or profiles list is empty, derive from active profiles in DB
	if len(profiles) == 0 {
		if dbProfiles, err := sqlite.ListTranscodeProfiles(ctx, db, true); err == nil && len(dbProfiles) > 0 {
			for _, dp := range dbProfiles {
				profiles = append(profiles, ffmpeg.RenditionProfile{
					Name:          dp.Name,
					Height:        dp.Height,
					Width:         dp.Width,
					VideoBitrateK: dp.VideoBitrateK,
					AudioBitrateK: dp.AudioBitrateK,
					Codec:         dp.Codec,
				})
			}
		} else {
			profiles = ffmpeg.DefaultProfiles()
		}

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

// RegenerateManifestForJob regenerates the master manifest file using all currently completed sub-jobs.
func RegenerateManifestForJob(ctx context.Context, db *sql.DB, jobID uuid.UUID, outputDir string) error {
	job, err := sqlite.GetTranscodeJobByID(ctx, db, jobID)
	if err != nil {
		return err
	}
	item, err := sqlite.GetMediaItemByID(ctx, db, job.MediaItemID)
	if err != nil {
		return err
	}

	subJobs, err := sqlite.ListTranscodeSubJobsByJob(ctx, db, jobID)
	if err != nil {
		return err
	}

	var renditions []dash.RenditionInfo
	for _, sj := range subJobs {
		if sj.Type == models.SubJobTypeVideo && sj.Status == models.TranscodeStatusDone {
			name := ""
			if sj.ProfileName != nil {
				name = *sj.ProfileName
			}
			width := 0
			if sj.Width != nil {
				width = *sj.Width
			}
			height := 0
			if sj.Height != nil {
				height = *sj.Height
			}
			videoBitrateK := 0
			if sj.VideoBitrateK != nil {
				videoBitrateK = *sj.VideoBitrateK
			}
			audioBitrateK := 0
			if sj.AudioBitrateK != nil {
				audioBitrateK = *sj.AudioBitrateK
			}
			codec := "h264"
			if sj.Codec != nil {
				codec = *sj.Codec
			}
			renditions = append(renditions, dash.RenditionInfo{
				Name:          name,
				Width:         width,
				Height:        height,
				VideoBitrateK: videoBitrateK,
				AudioBitrateK: audioBitrateK,
				Codec:         codec,
			})
		}
	}

	// Write database-persisted subtitles to outputDir before scanning
	uploadedSubs, err := sqlite.ListMediaSubtitles(ctx, db, item.ID)
	if err == nil {
		for _, sub := range uploadedSubs {
			filename := fmt.Sprintf("sub_uploaded_%s_%s.vtt", sub.Language, sub.ID.String())
			_ = os.WriteFile(filepath.Join(outputDir, filename), []byte(sub.VTTContent), 0o644)
		}
	}

	var subs []dash.SubtitleInfo
	entries, err := os.ReadDir(outputDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "sub_") && strings.HasSuffix(entry.Name(), ".vtt") {
				lang := strings.TrimPrefix(entry.Name(), "sub_")
				lang = strings.TrimSuffix(lang, ".vtt")
				subs = append(subs, dash.SubtitleInfo{
					Language: lang,
					VTTPath:  filepath.Join(outputDir, entry.Name()),
				})
			}
		}
	}

	mpdPath := filepath.Join(outputDir, "manifest.mpd")
	if len(renditions) > 0 {
		if err := dash.GenerateMPD(outputDir, mpdPath, renditions, subs, item.Duration); err != nil {
			return fmt.Errorf("generating MPD: %w", err)
		}

		now := time.Now().UTC().Format(time.RFC3339)
		_, err = db.ExecContext(ctx, `
			UPDATE media_items
			SET mpd_path = ?, bundle_status = 'available', updated_at = ?
			WHERE id = ?`,
			mpdPath, now, item.ID.String())
		if err != nil {
			return fmt.Errorf("updating media item mpd_path: %w", err)
		}
	}
	return nil
}

