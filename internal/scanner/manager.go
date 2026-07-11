package scanner

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/metadata"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/events"
	"github.com/prismatic-media/prism-server/pkg/ffmpeg"
)

// Manager owns one Scanner per library and keeps them running.
type Manager struct {
	db       *sql.DB
	enricher *metadata.Enricher
	eventBus *events.Bus

	// ctx is the Manager's own long-lived context, independent of any HTTP
	// request or startup context. Scanners derive their contexts from here so
	// they are not canceled when a request completes.
	ctx  context.Context
	stop context.CancelFunc

	mu        sync.Mutex
	scanners  map[uuid.UUID]*Scanner
	cancels   map[uuid.UUID]context.CancelFunc
	taskQueue []task
	cond      *sync.Cond
}

// task represents a serialized operation to be executed by the manager's background worker.
type task interface {
	execute(ctx context.Context)
}

type scanTask struct {
	manager   *Manager
	libraryID uuid.UUID
	isManual  bool
}

func (t *scanTask) execute(ctx context.Context) {
	t.manager.mu.Lock()
	s, ok := t.manager.scanners[t.libraryID]
	t.manager.mu.Unlock()
	if !ok {
		return
	}
	if err := s.ScanAll(ctx, t.isManual); err != nil {
		slog.Warn("scan failed", "library_id", t.libraryID, "error", err)
	}
}

type probeItemTask struct {
	db   *sql.DB
	bus  *events.Bus
	item *models.MediaItem
}

func (t *probeItemTask) execute(ctx context.Context) {
	_ = sqlite.UpdateMediaProbeStatus(ctx, t.db, t.item.ID, models.ProbeStatusProcessing)

	ffprobePath := "ffprobe"
	probe, err := ffmpeg.Probe(ctx, ffprobePath, t.item.FilePath)
	if err != nil {
		slog.Warn("ffprobe failed inside task", "path", t.item.FilePath, "error", err)
		_ = sqlite.UpdateMediaProbeStatus(ctx, t.db, t.item.ID, models.ProbeStatusFailed)
		return
	}

	t.item.Duration = probe.Duration
	t.item.Width = probe.Width
	t.item.Height = probe.Height
	t.item.VideoCodec = probe.VideoCodec
	t.item.AudioCodec = probe.AudioCodec
	t.item.ProbeStatus = models.ProbeStatusDone

	if err := sqlite.UpsertMediaItem(ctx, t.db, t.item); err != nil {
		slog.Warn("failed to upsert probed media item", "path", t.item.FilePath, "error", err)
		_ = sqlite.UpdateMediaProbeStatus(ctx, t.db, t.item.ID, models.ProbeStatusFailed)
		return
	}

	slog.Info("successfully probed media file", "path", t.item.FilePath, "duration", probe.Duration)
}

type enrichItemTask struct {
	enricher *metadata.Enricher
	db       *sql.DB
	bus      *events.Bus
	item     *models.MediaItem
}

func (t *enrichItemTask) execute(ctx context.Context) {
	_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusProcessing)

	apiKey, _ := sqlite.GetSetting(ctx, t.db, "tmdb_api_key")
	if apiKey == "" {
		slog.Info("TMDB enrichment failed: no API key configured", "id", t.item.ID)
		_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusFailed)
		return
	}

	t.enricher.EnrichItem(ctx, t.item)

	updated, err := sqlite.GetMediaItemByID(ctx, t.db, t.item.ID)
	if err == nil {
		if updated.TMDBId != nil {
			_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusDone)
			if updated.PosterPath != nil && t.bus != nil {
				t.bus.Publish(events.EventMediaEnriched, events.MediaEnrichedPayload{
					MediaItemID: updated.ID,
					LibraryID:   updated.LibraryID,
					PosterPath:  *updated.PosterPath,
				})
			}
		} else {
			_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusFailed)
		}
	} else {
		_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusFailed)
	}
}

type enrichTVEpisodeTask struct {
	enricher *metadata.Enricher
	db       *sql.DB
	bus      *events.Bus
	item     *models.MediaItem
	showID   uuid.UUID
	seasonID uuid.UUID
}

func (t *enrichTVEpisodeTask) execute(ctx context.Context) {
	_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusProcessing)

	apiKey, _ := sqlite.GetSetting(ctx, t.db, "tmdb_api_key")
	if apiKey == "" {
		slog.Info("TMDB episode enrichment failed: no API key configured", "id", t.item.ID)
		_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusFailed)
		return
	}

	t.enricher.EnrichTVEpisode(ctx, t.item, t.showID, t.seasonID)

	updated, err := sqlite.GetMediaItemByID(ctx, t.db, t.item.ID)
	if err == nil {
		if updated.TMDBId != nil {
			_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusDone)
			if updated.PosterPath != nil && t.bus != nil {
				t.bus.Publish(events.EventMediaEnriched, events.MediaEnrichedPayload{
					MediaItemID: updated.ID,
					LibraryID:   updated.LibraryID,
					PosterPath:  *updated.PosterPath,
				})
			}
		} else {
			_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusFailed)
		}
	} else {
		_ = sqlite.UpdateMediaEnrichmentStatus(ctx, t.db, t.item.ID, models.EnrichmentStatusFailed)
	}
}

// NewManager creates a Manager. enricher may be nil to skip metadata
// enrichment. Call StartAll to start watching all libraries in the DB.
func NewManager(db *sql.DB, enricher *metadata.Enricher, bus *events.Bus) *Manager {
	ctx, stop := context.WithCancel(context.Background())
	m := &Manager{
		db:       db,
		enricher: enricher,
		eventBus: bus,
		ctx:      ctx,
		stop:     stop,
		scanners: make(map[uuid.UUID]*Scanner),
		cancels:  make(map[uuid.UUID]context.CancelFunc),
	}
	m.cond = sync.NewCond(&m.mu)
	go m.workerLoop()
	return m
}

// submitTask registers a task to be processed sequentially by the background worker.
func (m *Manager) submitTask(t task) {
	m.mu.Lock()
	m.taskQueue = append(m.taskQueue, t)
	m.cond.Signal()
	m.mu.Unlock()
}

func (m *Manager) workerLoop() {
	for {
		m.mu.Lock()
		for len(m.taskQueue) == 0 && m.ctx.Err() == nil {
			m.cond.Wait()
		}

		if m.ctx.Err() != nil {
			m.mu.Unlock()
			return
		}

		t := m.taskQueue[0]
		m.taskQueue = m.taskQueue[1:]
		m.mu.Unlock()

		t.execute(m.ctx)
	}
}

// Shutdown stops all scanners and releases resources. Call during server shutdown.
func (m *Manager) Shutdown() {
	m.stop()
	m.mu.Lock()
	m.cond.Broadcast()
	m.mu.Unlock()
}

// StartAll loads all libraries from the DB, runs an initial scan, and starts
// watchers. It is intended to be called once at server startup (non-blocking:
// watchers run in background goroutines).
func (m *Manager) StartAll(ctx context.Context) error {
	libs, err := sqlite.ListLibraries(ctx, m.db)
	if err != nil {
		return err
	}
	for _, lib := range libs {
		m.add(lib)
	}

	// Recovery: query DB for pending tasks and queue them
	probes, err := sqlite.ListPendingProbes(ctx, m.db)
	if err == nil {
		for _, item := range probes {
			m.submitTask(&probeItemTask{
				db:   m.db,
				bus:  m.eventBus,
				item: item,
			})
		}
	}
	enrichments, err := sqlite.ListPendingEnrichments(ctx, m.db)
	if err == nil {
		for _, item := range enrichments {
			if item.MediaType == models.MediaTypeEpisode {
				if item.TVShowID != nil && item.TVSeasonID != nil {
					m.submitTask(&enrichTVEpisodeTask{
						enricher: m.enricher,
						db:       m.db,
						bus:      m.eventBus,
						item:     item,
						showID:   *item.TVShowID,
						seasonID: *item.TVSeasonID,
					})
				}
			} else {
				m.submitTask(&enrichItemTask{
					enricher: m.enricher,
					db:       m.db,
					bus:      m.eventBus,
					item:     item,
				})
			}
		}
	}

	return nil
}

// Add starts a scanner for a newly registered library.
func (m *Manager) Add(_ context.Context, lib *models.Library) {
	m.add(lib)
}

// Remove stops and removes the scanner for a deleted library.
func (m *Manager) Remove(id uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, ok := m.cancels[id]; ok {
		cancel()
		delete(m.cancels, id)
	}
	if s, ok := m.scanners[id]; ok {
		s.Stop()
		delete(m.scanners, id)
	}
}

// Scan triggers an immediate full scan of a library (blocking-queue).
func (m *Manager) Scan(id uuid.UUID) error {
	m.mu.Lock()
	_, ok := m.scanners[id]
	m.mu.Unlock()
	if !ok {
		return ErrScannerNotFound
	}
	m.submitTask(&scanTask{manager: m, libraryID: id, isManual: true})
	return nil
}

// add is the internal implementation of Add — must not hold m.mu on entry.
func (m *Manager) add(lib *models.Library) {
	m.mu.Lock()
	if _, exists := m.scanners[lib.ID]; exists {
		m.mu.Unlock()
		return
	}
	s := New(m.db, lib, m.enricher, m.eventBus)
	s.submitTask = m.submitTask
	watchCtx, cancel := context.WithCancel(m.ctx)
	m.scanners[lib.ID] = s
	m.cancels[lib.ID] = cancel
	m.mu.Unlock()

	// Initial scan (queued sequentially).
	m.submitTask(&scanTask{manager: m, libraryID: lib.ID, isManual: false})

	// File-system watcher (non-blocking).
	go func() {
		if err := s.Start(watchCtx); err != nil && watchCtx.Err() == nil {
			slog.Warn("watcher exited", "path", lib.Path, "error", err)
		}
	}()
}

// ErrScannerNotFound is returned when a scan is requested for an unknown library ID.
var ErrScannerNotFound = &scannerNotFoundError{}

type scannerNotFoundError struct{}

func (e *scannerNotFoundError) Error() string { return "scanner: library not found" }
