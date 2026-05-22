// Package scanner discovers media files in library directories and keeps the
// database in sync with the filesystem using both initial walks and fsnotify
// file-system watches.
package scanner

import (
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/ringmaster217/galactic-media-server/internal/metadata"
	"github.com/ringmaster217/galactic-media-server/internal/models"
	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
	"github.com/ringmaster217/galactic-media-server/pkg/events"
	"github.com/ringmaster217/galactic-media-server/pkg/ffmpeg"
)

// videoExtensions is the set of file extensions treated as video files.
var videoExtensions = map[string]struct{}{
	".mkv": {}, ".mp4": {}, ".avi": {}, ".mov": {},
	".wmv": {}, ".flv": {}, ".webm": {}, ".m4v": {},
	".ts": {}, ".m2ts": {}, ".mpeg": {}, ".mpg": {},
}

// Scanner manages library scanning and file-system watching for a single
// library. Create one per library via New, then call Start to begin watching.
type Scanner struct {
	db          *sql.DB
	library     *models.Library
	ffprobePath string
	enricher    *metadata.Enricher
	eventBus    *events.Bus
	watcher     *fsnotify.Watcher
	mu          sync.Mutex
}

// New creates a Scanner for lib. enricher may be nil to skip metadata
// enrichment. Call Start to begin watching.
func New(db *sql.DB, lib *models.Library, ffprobePath string, enricher *metadata.Enricher, bus *events.Bus) *Scanner {
	return &Scanner{
		db:          db,
		library:     lib,
		ffprobePath: ffprobePath,
		enricher:    enricher,
		eventBus:    bus,
	}
}

// ScanAll walks the library path, upserts every video file found, and removes
// stale rows for files that no longer exist on disk.
func (s *Scanner) ScanAll(ctx context.Context) error {
	slog.Info("scanning library", "library", s.library.Name, "path", s.library.Path)

	var paths []string

	err := filepath.WalkDir(s.library.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk error", "path", path, "error", err)
			return nil // skip unreadable entries
		}
		if d.IsDir() || !isVideoFile(path) {
			return nil
		}

		paths = append(paths, path)
		if err := s.upsertFile(ctx, path); err != nil {
			slog.Warn("failed to upsert file", "path", path, "error", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Remove DB rows for files that are no longer on disk.
	if err := sqlite.DeleteMediaItemsNotIn(ctx, s.db, s.library.ID, paths); err != nil {
		slog.Warn("pruning stale media items failed", "library", s.library.Name, "error", err)
	}

	slog.Info("scan complete", "library", s.library.Name, "files", len(paths))
	return nil
}

// Start begins watching the library directory for changes. It blocks until ctx
// is cancelled. Run it in a goroutine.
func (s *Scanner) Start(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.watcher = w
	s.mu.Unlock()
	defer w.Close()

	// Watch the root and all existing subdirectories.
	if err := filepath.WalkDir(s.library.Path, func(path string, d fs.DirEntry, _ error) error {
		if d != nil && d.IsDir() {
			return w.Add(path)
		}
		return nil
	}); err != nil {
		return err
	}

	slog.Info("watching library", "library", s.library.Name, "path", s.library.Path)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-w.Events:
			if !ok {
				return nil
			}
			s.handleEvent(ctx, event, w)
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			slog.Warn("watcher error", "library", s.library.Name, "error", err)
		}
	}
}

// Stop tears down the fsnotify watcher (idempotent).
func (s *Scanner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watcher != nil {
		s.watcher.Close()
		s.watcher = nil
	}
}

// handleEvent processes a single fsnotify event.
func (s *Scanner) handleEvent(ctx context.Context, event fsnotify.Event, w *fsnotify.Watcher) {
	path := event.Name

	switch {
	case event.Has(fsnotify.Create):
		fi, err := os.Stat(path)
		if err != nil {
			return
		}
		if fi.IsDir() {
			_ = w.Add(path)
			return
		}
		if isVideoFile(path) {
			if err := s.upsertFile(ctx, path); err != nil {
				slog.Warn("upsert on create failed", "path", path, "error", err)
			}
		}

	case event.Has(fsnotify.Write):
		if isVideoFile(path) {
			if err := s.upsertFile(ctx, path); err != nil {
				slog.Warn("upsert on write failed", "path", path, "error", err)
			}
		}

	case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
		item, err := sqlite.GetMediaItemByPath(ctx, s.db, path)
		if err != nil {
			return
		}
		if err := sqlite.DeleteMediaItem(ctx, s.db, item.ID); err != nil {
			slog.Warn("delete on remove failed", "path", path, "error", err)
		}
	}
}

// upsertFile probes a video file with FFprobe and upserts its DB row.
func (s *Scanner) upsertFile(ctx context.Context, path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	title := titleFromPath(path)

	m := &models.MediaItem{
		LibraryID:       s.library.ID,
		Title:           title,
		MediaType:       s.library.MediaType,
		FilePath:        path,
		FileSize:        fi.Size(),
		TranscodeStatus: models.TranscodeStatusPending,
	}

	if s.ffprobePath != "" {
		probe, err := ffmpeg.Probe(ctx, s.ffprobePath, path)
		if err != nil {
			slog.Warn("ffprobe failed", "path", path, "error", err)
		} else {
			m.Duration = probe.Duration
			m.Width = probe.Width
			m.Height = probe.Height
			m.VideoCodec = probe.VideoCodec
			m.AudioCodec = probe.AudioCodec
		}
	}

	if err := sqlite.UpsertMediaItem(ctx, s.db, m); err != nil {
		return err
	}

	// Fetch the canonical item so we have its DB-assigned ID for both the
	// real-time event and (optionally) metadata enrichment.
	if item, err := sqlite.GetMediaItemByPath(ctx, s.db, path); err == nil {
		s.eventBus.Publish(events.EventMediaCreated, events.MediaCreatedPayload{
			MediaItemID: item.ID,
			LibraryID:   s.library.ID,
			Title:       item.Title,
		})
		if s.enricher != nil && item.TMDBId == nil {
			enricher := s.enricher
			go enricher.EnrichItem(context.Background(), item)
		}
	}
	return nil
}

// isVideoFile reports whether path has a recognised video extension.
func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := videoExtensions[ext]
	return ok
}

// titleFromPath extracts a human-readable title from a file path by stripping
// the directory and extension.
func titleFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}
