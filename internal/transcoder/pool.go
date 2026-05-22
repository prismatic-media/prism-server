// Package transcoder implements a worker-pool-based DASH transcode engine.
//
// Architecture:
//   - Pool manages N worker goroutines and a job queue channel.
//   - Worker picks up jobs, calls ffmpeg, updates DB progress/status.
//   - Hub broadcasts progress events to WebSocket subscribers.
package transcoder

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ringmaster217/galactic-media-server/internal/models"
	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
	"github.com/ringmaster217/galactic-media-server/pkg/dash"
	"github.com/ringmaster217/galactic-media-server/pkg/events"
	"github.com/ringmaster217/galactic-media-server/pkg/ffmpeg"
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

func (h *Hub) publish(evt ProgressEvent) {
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
	db          *sql.DB
	ffmpegPath  string
	ffprobePath string
	segmentsDir string
	mpdCache    *dash.Cache
	eventBus    *events.Bus
	hub         *Hub
	jobs        chan *models.TranscodeJob
	workers     int
	wg          sync.WaitGroup
}

// NewPool creates a Pool. Call Start to begin processing.
func NewPool(db *sql.DB, ffmpegPath, ffprobePath, segmentsDir string, workers int, mpdCache *dash.Cache, bus *events.Bus) *Pool {
	return &Pool{
		db:          db,
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		segmentsDir: segmentsDir,
		mpdCache:    mpdCache,
		eventBus:    bus,
		hub:         newHub(),
		jobs:        make(chan *models.TranscodeJob, 64),
		workers:     workers,
	}
}

// Hub returns the event hub for WebSocket subscriptions.
func (p *Pool) Hub() *Hub { return p.hub }

// MPDCache returns the in-process MPD path cache.
func (p *Pool) MPDCache() *dash.Cache { return p.mpdCache }

// Start launches worker goroutines and re-enqueues pending jobs from the DB.
// It is non-blocking; workers run until ctx is cancelled or Stop is called.
func (p *Pool) Start(ctx context.Context) error {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.runWorker(ctx)
		}()
	}

	// Re-enqueue any pending jobs that survived a previous crash/restart.
	pending, err := sqlite.ListPendingJobs(ctx, p.db)
	if err != nil {
		return fmt.Errorf("loading pending jobs: %w", err)
	}
	for _, j := range pending {
		p.enqueue(j)
	}
	return nil
}

// Stop signals workers to stop and waits for them to finish.
func (p *Pool) Stop() {
	close(p.jobs)
	p.wg.Wait()
}

// Enqueue creates a new transcode job for mediaItemID and queues it.
func (p *Pool) Enqueue(ctx context.Context, mediaItemID uuid.UUID) (*models.TranscodeJob, error) {
	j := &models.TranscodeJob{MediaItemID: mediaItemID}
	if err := sqlite.CreateTranscodeJob(ctx, p.db, j); err != nil {
		return nil, err
	}
	if err := sqlite.SetMediaTranscodeStatus(ctx, p.db, mediaItemID, models.TranscodeStatusPending); err != nil {
		return nil, err
	}
	p.mpdCache.Invalidate(mediaItemID)
	p.enqueue(j)
	return j, nil
}

func (p *Pool) enqueue(j *models.TranscodeJob) {
	select {
	case p.jobs <- j:
	default:
		slog.Warn("transcode queue full, dropping job", "job_id", j.ID)
	}
}

// runWorker is the per-goroutine loop.
func (p *Pool) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j, ok := <-p.jobs:
			if !ok {
				return
			}
			p.process(ctx, j)
		}
	}
}

// process runs a single transcode job.
func (p *Pool) process(ctx context.Context, j *models.TranscodeJob) {
	log := slog.With("job_id", j.ID, "media_item_id", j.MediaItemID)
	log.Info("starting transcode job")

	// Mark processing.
	if err := sqlite.UpdateJobStatus(ctx, p.db, j.ID, models.TranscodeStatusProcessing, nil); err != nil {
		log.Warn("could not mark job as processing", "error", err)
	}
	if err := sqlite.SetMediaTranscodeStatus(ctx, p.db, j.MediaItemID, models.TranscodeStatusProcessing); err != nil {
		log.Warn("could not set media processing status", "error", err)
	}

	item, err := sqlite.GetMediaItemByID(ctx, p.db, j.MediaItemID)
	if err != nil {
		p.fail(ctx, j, fmt.Sprintf("fetching media item: %v", err))
		return
	}

	p.eventBus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
		MediaItemID:     j.MediaItemID,
		LibraryID:       item.LibraryID,
		TranscodeStatus: string(models.TranscodeStatusProcessing),
	})

	// Probe to get duration for progress estimation.
	var duration float64
	if p.ffprobePath != "" {
		if probe, err := ffmpeg.Probe(ctx, p.ffprobePath, item.FilePath); err == nil {
			duration = probe.Duration
		}
	}

	outputDir := filepath.Join(p.segmentsDir, j.MediaItemID.String())
	mpdPath := filepath.Join(outputDir, "manifest.mpd")

	profiles := ffmpeg.DefaultProfiles()
	// Only generate renditions smaller-or-equal to the source height.
	if item.Height > 0 {
		var filtered []ffmpeg.RenditionProfile
		for _, prof := range profiles {
			if prof.Height <= item.Height {
				filtered = append(filtered, prof)
			}
		}
		if len(filtered) > 0 {
			profiles = filtered
		}
	}

	// progressFn receives an overall 0–100 percent computed by TranscodeDASH.
	progressFn := func(pct float64) {
		if pct > 99 {
			pct = 99
		}
		_ = sqlite.UpdateJobProgress(ctx, p.db, j.ID, pct)
		p.hub.publish(ProgressEvent{JobID: j.ID, Progress: pct})
		p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
			JobID:       j.ID,
			MediaItemID: j.MediaItemID,
			Progress:    pct,
		})
	}

	// Probe for subtitle streams.
	var subtitleStreams []ffmpeg.SubtitleStream
	if p.ffprobePath != "" {
		if probe, err := ffmpeg.Probe(ctx, p.ffprobePath, item.FilePath); err == nil {
			subtitleStreams = probe.SubtitleStreams
		}
	}

	opts := ffmpeg.TranscodeOptions{
		InputPath:       item.FilePath,
		OutputDir:       outputDir,
		Profiles:        profiles,
		Duration:        duration,
		SubtitleStreams: subtitleStreams,
		ProgressFn:      progressFn,
	}

	if err := ffmpeg.TranscodeDASH(ctx, p.ffmpegPath, opts); err != nil {
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
		vttPath, err := ffmpeg.CopySidecarSubtitle(ctx, p.ffmpegPath, sc, outputDir)
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

	now := time.Now()
	j.FinishedAt = &now
	if err := sqlite.UpdateJobStatus(ctx, p.db, j.ID, models.TranscodeStatusDone, nil); err != nil {
		log.Warn("could not mark job done", "error", err)
	}
	if err := sqlite.UpdateJobProgress(ctx, p.db, j.ID, 100); err != nil {
		log.Warn("could not set final progress", "error", err)
	}

	p.hub.publish(ProgressEvent{JobID: j.ID, Progress: 100, Done: true})
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
	log.Info("transcode job complete", "mpd", mpdPath)
}

func (p *Pool) fail(ctx context.Context, j *models.TranscodeJob, errMsg string) {
	slog.Warn("transcode job failed", "job_id", j.ID, "error", errMsg)
	_ = sqlite.UpdateJobStatus(ctx, p.db, j.ID, models.TranscodeStatusFailed, &errMsg)
	_ = sqlite.SetMediaTranscodeStatus(ctx, p.db, j.MediaItemID, models.TranscodeStatusFailed)
	p.hub.publish(ProgressEvent{JobID: j.ID, Progress: 0, Done: true, Error: errMsg})
	p.eventBus.Publish(events.EventJobProgress, events.JobProgressPayload{
		JobID:       j.ID,
		MediaItemID: j.MediaItemID,
		Done:        true,
		Error:       errMsg,
	})
}
