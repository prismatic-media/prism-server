// Package scanner discovers media files in library directories and keeps the
// database in sync with the filesystem using both initial walks and fsnotify
// file-system watches.
package scanner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/artifact"
	"github.com/prismatic-media/prism-server/internal/metadata"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/events"
	"github.com/prismatic-media/prism-server/pkg/ffmpeg"
	"github.com/prismatic-media/prism-server/pkg/fingerprint"
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
	db       *sql.DB
	library  *models.Library
	enricher *metadata.Enricher
	eventBus *events.Bus
	watcher  *fsnotify.Watcher
	mu       sync.Mutex
}

// New creates a Scanner for lib. enricher may be nil to skip metadata
// enrichment. Call Start to begin watching.
func New(db *sql.DB, lib *models.Library, enricher *metadata.Enricher, bus *events.Bus) *Scanner {
	return &Scanner{
		db:       db,
		library:  lib,
		enricher: enricher,
		eventBus: bus,
	}
}

// ScanAll walks the library path, upserts every video file found, and removes
// stale rows for files that no longer exist on disk.
func (s *Scanner) ScanAll(ctx context.Context) error {
	slog.Info("scanning library", "path", s.library.Path, "media_type", s.library.MediaType)

	var (
		paths       []string
		upserted    int
		failed      int
		skipped     int
	)

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
			failed++
		} else {
			upserted++
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Remove DB rows for files that are no longer on disk.
	if err := sqlite.PruneStaleMediaItems(ctx, s.db, s.library.ID, paths); err != nil {
		slog.Warn("pruning stale media items failed", "path", s.library.Path, "error", err)
	}

	skipped = len(paths) - upserted - failed
	slog.Info("scan complete",
		"path", s.library.Path,
		"media_type", s.library.MediaType,
		"total_files", len(paths),
		"upserted", upserted,
		"skipped", skipped,
		"failed", failed,
	)
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
	defer func() { _ = w.Close() }()

	// Watch the root and all existing subdirectories.
	if err := filepath.WalkDir(s.library.Path, func(path string, d fs.DirEntry, _ error) error {
		if d != nil && d.IsDir() {
			return w.Add(path)
		}
		return nil
	}); err != nil {
		return err
	}

	slog.Info("watching library", "path", s.library.Path)

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
			slog.Warn("watcher error", "path", s.library.Path, "error", err)
		}
	}
}

// Stop tears down the fsnotify watcher (idempotent).
func (s *Scanner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watcher != nil {
		_ = s.watcher.Close()
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
		if item.BundleStatus == models.BundleStatusNone {
			if err := sqlite.DeleteMediaItem(ctx, s.db, item.ID); err != nil {
				slog.Warn("delete on remove failed", "path", path, "error", err)
			}
		} else {
			if err := sqlite.SetMediaSourceStatus(ctx, s.db, item.ID, models.SourceStatusMissing); err != nil {
				slog.Warn("set source status missing on remove failed", "path", path, "error", err)
			}
		}
	}
}

// upsertFile probes a video file with FFprobe and upserts its DB row.
func (s *Scanner) upsertFile(ctx context.Context, path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	// Optimize: if file already exists in DB with same size, skip redundant processing.
	if existing, err := sqlite.GetMediaItemByPath(ctx, s.db, path); err == nil && existing != nil && existing.FileSize == fi.Size() {
		if existing.SourceStatus != models.SourceStatusAvailable {
			if err := sqlite.SetMediaSourceStatus(ctx, s.db, existing.ID, models.SourceStatusAvailable); err != nil {
				slog.Warn("failed to update source status to available", "path", path, "error", err)
			}
		}
		if existing.TranscodeStatus != models.TranscodeStatusDone {
			_, _ = s.tryLinkExistingItem(ctx, existing, path)
		}
		return nil
	}

	// For TV show libraries, parse the filename as an episode and resolve the
	// parent show + season records first.
	if s.library.MediaType == models.MediaTypeTVShow {
		return s.upsertTVEpisodeFile(ctx, path, fi.Size())
	}

	title := titleFromPath(path)

	m := &models.MediaItem{
		LibraryID:       s.library.ID,
		Title:           title,
		MediaType:       s.library.MediaType,
		FilePath:        path,
		FileSize:        fi.Size(),
		TranscodeStatus: models.TranscodeStatusNone,
	}

	ffprobePath := "ffprobe"
	probe, err := ffmpeg.Probe(ctx, ffprobePath, path)
	if err != nil {
		slog.Warn("ffprobe failed", "path", path, "error", err)
	} else {
		m.Duration = probe.Duration
		m.Width = probe.Width
		m.Height = probe.Height
		m.VideoCodec = probe.VideoCodec
		m.AudioCodec = probe.AudioCodec
	}

	// Generate fingerprint & handle deduplication
	handled, err := s.processDeduplicationAndLinking(ctx, path, m)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	if err := sqlite.UpsertMediaItem(ctx, s.db, m); err != nil {
		return err
	}

	// Fetch the canonical item so we have its DB-assigned ID for both the
	// real-time event and (optionally) metadata enrichment.
	if item, err := sqlite.GetMediaItemByPath(ctx, s.db, path); err == nil {
		if s.eventBus != nil {
			s.eventBus.Publish(events.EventMediaCreated, events.MediaCreatedPayload{
				MediaItemID: item.ID,
				LibraryID:   s.library.ID,
				Title:       item.Title,
			})
		}
		if s.enricher != nil && item.TMDBId == nil {
			enricher := s.enricher
			itemID := item.ID
			go func() {
				enricher.EnrichItem(context.Background(), item)
				// After enrichment, notify clients if a poster is now available.
				if updated, err := sqlite.GetMediaItemByID(context.Background(), s.db, itemID); err == nil && updated.PosterPath != nil {
					if s.eventBus != nil {
						s.eventBus.Publish(events.EventMediaEnriched, events.MediaEnrichedPayload{
							MediaItemID: updated.ID,
							LibraryID:   updated.LibraryID,
							PosterPath:  *updated.PosterPath,
						})
					}
				}
			}()
		}
	}
	return nil
}

// upsertTVEpisodeFile handles upsert for a file inside a tvshow library.
// It parses the filename, upserts the parent TVShow and TVSeason, then upserts
// the episode as a MediaItem with type "episode".
func (s *Scanner) upsertTVEpisodeFile(ctx context.Context, path string, fileSize int64) error {
	// Optimize: if file already exists in DB with same size, skip redundant processing.
	if existing, err := sqlite.GetMediaItemByPath(ctx, s.db, path); err == nil && existing != nil && existing.FileSize == fileSize && existing.TVShowID != nil && existing.TVSeasonID != nil {
		if existing.SourceStatus != models.SourceStatusAvailable {
			if err := sqlite.SetMediaSourceStatus(ctx, s.db, existing.ID, models.SourceStatusAvailable); err != nil {
				slog.Warn("failed to update source status to available", "path", path, "error", err)
			}
		}
		if existing.TranscodeStatus != models.TranscodeStatusDone {
			_, _ = s.tryLinkExistingItem(ctx, existing, path)
		}
		return nil
	}

	info, ok := metadata.ParseTVEpisode(path)
	if !ok {
		slog.Info("skipping file: could not parse as TV episode",
			"path", path,
			"library_path", s.library.Path,
		)
		return nil
	}

	// Upsert the parent TV show.
	show := &models.TVShow{
		LibraryID: s.library.ID,
		Name:      info.ShowName,
	}
	if err := sqlite.UpsertTVShow(ctx, s.db, show); err != nil {
		return fmt.Errorf("upserting tv show: %w", err)
	}

	// Upsert the season.
	season := &models.TVSeason{
		TVShowID:     show.ID,
		SeasonNumber: info.SeasonNumber,
	}
	if err := sqlite.UpsertTVSeason(ctx, s.db, season); err != nil {
		return fmt.Errorf("upserting tv season: %w", err)
	}

	m := &models.MediaItem{
		LibraryID:       s.library.ID,
		Title:           info.EpisodeName,
		MediaType:       models.MediaTypeEpisode,
		FilePath:        path,
		FileSize:        fileSize,
		TVShowID:        &show.ID,
		TVSeasonID:      &season.ID,
		SeasonNumber:    &info.SeasonNumber,
		EpisodeNumber:   &info.EpisodeNumber,
		TranscodeStatus: models.TranscodeStatusNone,
	}

	ffprobePath := "ffprobe"
	probe, err := ffmpeg.Probe(ctx, ffprobePath, path)
	if err != nil {
		slog.Warn("ffprobe failed", "path", path, "error", err)
	} else {
		m.Duration = probe.Duration
		m.Width = probe.Width
		m.Height = probe.Height
		m.VideoCodec = probe.VideoCodec
		m.AudioCodec = probe.AudioCodec
	}

	// Generate fingerprint & handle deduplication
	handled, err := s.processDeduplicationAndLinking(ctx, path, m)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	if err := sqlite.UpsertMediaItem(ctx, s.db, m); err != nil {
		return err
	}

	if item, err := sqlite.GetMediaItemByPath(ctx, s.db, path); err == nil {
		if s.eventBus != nil {
			s.eventBus.Publish(events.EventMediaCreated, events.MediaCreatedPayload{
				MediaItemID: item.ID,
				LibraryID:   s.library.ID,
				Title:       item.Title,
			})
		}
		if s.enricher != nil && item.TMDBId == nil {
			enricher := s.enricher
			showID := show.ID
			seasonID := season.ID
			itemCopy := item
			go func() {
				enricher.EnrichTVEpisode(context.Background(), itemCopy, showID, seasonID)
				if updated, err := sqlite.GetMediaItemByID(context.Background(), s.db, itemCopy.ID); err == nil && updated.PosterPath != nil {
					if s.eventBus != nil {
						s.eventBus.Publish(events.EventMediaEnriched, events.MediaEnrichedPayload{
							MediaItemID: updated.ID,
							LibraryID:   updated.LibraryID,
							PosterPath:  *updated.PosterPath,
						})
					}
				}
			}()
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

// processDeduplicationAndLinking checks for duplicate/moved files and links bundles.
// Returns true if a move or bundle link was handled (meaning we should skip normal insert/events),
// or false if we should proceed with normal upsert.
func (s *Scanner) processDeduplicationAndLinking(ctx context.Context, path string, m *models.MediaItem) (bool, error) {
	if existingByPath, err := sqlite.GetMediaItemByPath(ctx, s.db, path); err == nil && existingByPath != nil {
		m.ID = existingByPath.ID
	}

	fp, err := fingerprint.GenerateDeterministic(path)
	if err != nil {
		slog.Warn("failed to generate fingerprint", "path", path, "error", err)
		return false, nil
	}
	m.SourceFingerprint = &fp

	// 1. Check for file-move detection: same library, same fingerprint, different path.
	existing, err := sqlite.GetMediaItemByFingerprint(ctx, s.db, s.library.ID, fp)
	if err == nil && existing != nil {
		if existing.FilePath != path {
			// Found moved file! Update the file path and set source status to available.
			if err := sqlite.UpdateMediaItemFilePath(ctx, s.db, existing.ID, path); err != nil {
				return false, fmt.Errorf("updating file path for moved item: %w", err)
			}
			slog.Info("detected file move", "old_path", existing.FilePath, "new_path", path)
			return true, nil
		}
		// Rescan of same file path: update the fingerprint in DB since we computed it
		if existing.SourceFingerprint == nil || *existing.SourceFingerprint != fp {
			_ = sqlite.UpsertMediaItem(ctx, s.db, m)
		}
	}

	// 2. Check for bundle linking: if fingerprint matches an artifact_records entry.
	arts, err := sqlite.ListArtifactRecordsByFingerprint(ctx, s.db, fp)
	var matchedArt *models.ArtifactRecord
	if err == nil && len(arts) > 0 {
		for _, art := range arts {
			area, err := sqlite.GetStorageAreaByID(ctx, s.db, art.StorageAreaID)
			if err == nil && area != nil && area.Enabled {
				matchedArt = art
				break
			}
		}
	}

	// If not found in DB, check disk segment storage
	if matchedArt == nil {
		if art, err := s.findAndIndexBundleByFingerprint(ctx, fp); err == nil && art != nil {
			matchedArt = art
		}
	}

	if matchedArt != nil {
		// Link the media item to the existing bundle:
		// set bundle_status=available, transcode_status=done, assign mpd_path.
		m.BundleStatus = models.BundleStatusAvailable
		m.TranscodeStatus = models.TranscodeStatusDone
		absMPD := filepath.Join(matchedArt.OutputDir, matchedArt.MPDPath)
		m.MPDPath = &absMPD

		if m.ID == uuid.Nil {
			m.ID = uuid.New()
		}

		if err := sqlite.UpsertMediaItem(ctx, s.db, m); err != nil {
			return false, fmt.Errorf("upserting bundle-linked media item: %w", err)
		}

		link := &models.ArtifactMediaLink{
			ArtifactID:  matchedArt.ID,
			MediaItemID: m.ID,
			MatchedVia:  models.ArtifactMatchedViaFingerprint,
			Status:      models.ArtifactLinkLinked,
		}
		if err := sqlite.CreateArtifactMediaLink(ctx, s.db, link); err != nil {
			slog.Warn("failed to create artifact media link", "error", err)
		}

		slog.Info("linked discovered file to existing transcode bundle", "path", path, "mpd", matchedArt.MPDPath)
		return false, nil
	}

	return false, nil
}

// tryLinkExistingItem attempts to link an existing media item to a transcode bundle.
func (s *Scanner) tryLinkExistingItem(ctx context.Context, m *models.MediaItem, path string) (bool, error) {
	// Generate fingerprint only if it is not already in the DB.
	var fp string
	if m.SourceFingerprint != nil && *m.SourceFingerprint != "" {
		fp = *m.SourceFingerprint
	} else {
		var err error
		fp, err = fingerprint.GenerateDeterministic(path)
		if err != nil {
			slog.Warn("failed to generate fingerprint for existing item", "path", path, "error", err)
			return false, nil
		}
		m.SourceFingerprint = &fp
		// Save the fingerprint in the DB.
		_ = sqlite.UpsertMediaItem(ctx, s.db, m)
	}

	// Check if we can link it to a bundle.
	arts, err := sqlite.ListArtifactRecordsByFingerprint(ctx, s.db, fp)
	var matchedArt *models.ArtifactRecord
	if err == nil && len(arts) > 0 {
		for _, art := range arts {
			area, err := sqlite.GetStorageAreaByID(ctx, s.db, art.StorageAreaID)
			if err == nil && area != nil && area.Enabled {
				matchedArt = art
				break
			}
		}
	}
	if matchedArt == nil {
		if art, err := s.findAndIndexBundleByFingerprint(ctx, fp); err == nil && art != nil {
			matchedArt = art
		}
	}

	if matchedArt != nil {
		m.BundleStatus = models.BundleStatusAvailable
		m.TranscodeStatus = models.TranscodeStatusDone
		absMPD := filepath.Join(matchedArt.OutputDir, matchedArt.MPDPath)
		m.MPDPath = &absMPD

		if err := sqlite.UpsertMediaItem(ctx, s.db, m); err != nil {
			return false, fmt.Errorf("upserting bundle-linked media item: %w", err)
		}

		link := &models.ArtifactMediaLink{
			ArtifactID:  matchedArt.ID,
			MediaItemID: m.ID,
			MatchedVia:  models.ArtifactMatchedViaFingerprint,
			Status:      models.ArtifactLinkLinked,
		}
		if err := sqlite.CreateArtifactMediaLink(ctx, s.db, link); err != nil {
			slog.Warn("failed to create artifact media link", "error", err)
		}
		slog.Info("linked discovered file to existing transcode bundle", "path", path, "mpd", matchedArt.MPDPath)
		return true, nil
	}

	return false, nil
}

// findAndIndexBundleByFingerprint scans enabled segment storage areas for an artifact.json
// sidecar file that matches the source fingerprint. If found, it registers the artifact
// in the database and returns it.
func (s *Scanner) findAndIndexBundleByFingerprint(ctx context.Context, fp string) (*models.ArtifactRecord, error) {
	ready, err := sqlite.ArtifactSchemaReady(ctx, s.db)
	if err != nil || !ready {
		return nil, nil // schema not ready or error checking
	}

	areas, err := sqlite.ListStorageAreasByKind(ctx, s.db, models.StorageAreaKindSegments, true)
	if err != nil {
		return nil, fmt.Errorf("listing storage areas: %w", err)
	}

	var foundRecord *models.ArtifactRecord
	errDone := errors.New("done")

	for _, area := range areas {
		err := filepath.WalkDir(area.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable files/folders
			}
			if path == area.Path {
				return nil
			}
			if d.Name() == artifact.SidecarFilename {
				sidecar, err := artifact.ReadSidecar(filepath.Dir(path))
				if err != nil {
					return nil // skip invalid sidecars
				}
				if sidecar.SourceFingerprint == fp {
					outputDir := filepath.Dir(path)
					relMPDPath := sidecar.MPDPath
					record := &models.ArtifactRecord{
						StorageAreaID:     area.ID,
						SourcePath:        sidecar.SourcePath,
						SourceFingerprint: &sidecar.SourceFingerprint,
						OutputDir:         outputDir,
						MPDPath:           relMPDPath,
						Health:            models.ArtifactHealthHealthy,
						LastSeenAt:        time.Now().UTC(),
						RegisteredAt:      time.Now().UTC(),
					}

					// Upsert record into the DB
					existing, err := sqlite.GetArtifactRecordByStorageAreaAndPath(ctx, s.db, area.ID, sidecar.SourcePath)
					if err == nil && existing != nil {
						existing.Health = models.ArtifactHealthHealthy
						existing.LastSeenAt = time.Now().UTC()
						existing.UpdatedAt = time.Now().UTC()
						if sidecar.SourceFingerprint != "" {
							existing.SourceFingerprint = &sidecar.SourceFingerprint
						}
						if err := sqlite.UpsertArtifactRecord(ctx, s.db, existing); err != nil {
							return fmt.Errorf("updating artifact record: %w", err)
						}
						foundRecord = existing
					} else {
						record.ID = uuid.New()
						if err := sqlite.UpsertArtifactRecord(ctx, s.db, record); err != nil {
							return fmt.Errorf("creating artifact record: %w", err)
						}
						foundRecord = record
					}
					return errDone // Stop walking completely
				}
			}
			return nil
		})
		if errors.Is(err, errDone) {
			break
		}
		if err != nil {
			slog.Warn("walking storage area failed", "path", area.Path, "error", err)
		}
	}

	return foundRecord, nil
}
