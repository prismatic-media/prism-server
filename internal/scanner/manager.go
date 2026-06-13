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

	mu       sync.Mutex
	scanners map[uuid.UUID]*Scanner
	cancels  map[uuid.UUID]context.CancelFunc
}

// NewManager creates a Manager. enricher may be nil to skip metadata
// enrichment. Call StartAll to start watching all libraries in the DB.
func NewManager(db *sql.DB, enricher *metadata.Enricher, bus *events.Bus) *Manager {
	ctx, stop := context.WithCancel(context.Background())
	return &Manager{
		db:       db,
		enricher: enricher,
		eventBus: bus,
		ctx:      ctx,
		stop:     stop,
		scanners: make(map[uuid.UUID]*Scanner),
		cancels:  make(map[uuid.UUID]context.CancelFunc),
	}
}

// Shutdown stops all scanners and releases resources. Call during server shutdown.
func (m *Manager) Shutdown() {
	m.stop()
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

// Scan triggers an immediate full scan of a library (blocking).
func (m *Manager) Scan(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	s, ok := m.scanners[id]
	m.mu.Unlock()
	if !ok {
		return ErrScannerNotFound
	}
	return s.ScanAll(ctx)
}

// add is the internal implementation of Add — must not hold m.mu on entry.
func (m *Manager) add(lib *models.Library) {
	m.mu.Lock()
	if _, exists := m.scanners[lib.ID]; exists {
		m.mu.Unlock()
		return
	}
	s := New(m.db, lib, m.enricher, m.eventBus)
	watchCtx, cancel := context.WithCancel(m.ctx)
	m.scanners[lib.ID] = s
	m.cancels[lib.ID] = cancel
	m.mu.Unlock()

	// Initial scan (non-blocking).
	go func() {
		if err := s.ScanAll(watchCtx); err != nil {
			slog.Warn("initial scan failed", "path", lib.Path, "error", err)
		}
	}()

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
