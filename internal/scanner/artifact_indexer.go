package scanner

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/artifact"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/events"
)

// Indexer scans storage areas for artifact sidecar files and registers/updates
// artifact records in the database. It is designed to run as a one-shot
// operation or periodically via an admin endpoint.
type Indexer struct {
	db          *sql.DB
	eventBus    *events.Bus
	log         *slog.Logger
	staleAfter  time.Duration // duration after last_seen before marking stale (default: 7 days)
}

// NewIndexer creates a new Indexer.
func NewIndexer(db *sql.DB, eventBus *events.Bus) *Indexer {
	return &Indexer{
		db:         db,
		eventBus:   eventBus,
		log:        slog.Default(),
		staleAfter: 7 * 24 * time.Hour, // 7 days default
	}
}

// IndexSummary holds counts from a single indexing run.
type IndexSummary struct {
	StorageAreaID     uuid.UUID
	StorageAreaPath   string
	Registered        int // new artifacts registered
	Updated           int // existing artifacts updated
	Removed           int // artifacts marked missing (sidecar gone)
	Ambiguous         int // directories with multiple sidecars (should not happen)
	Errors            int // directories that failed to process
	MediaItemsCreated int // media items created from sidecars
}

// IndexStorageArea scans a single storage area for artifact sidecars and
// updates the artifact_records table. Returns a summary of changes.
func (i *Indexer) IndexStorageArea(ctx context.Context, area *models.StorageArea) (*IndexSummary, error) {
	sum := &IndexSummary{
		StorageAreaID:   area.ID,
		StorageAreaPath: area.Path,
	}

	// Check if the directory exists.
	info, err := os.Stat(area.Path)
	if err != nil {
		if os.IsNotExist(err) {
			i.log.Warn("storage area path does not exist", "path", area.Path)
			return sum, nil
		}
		return nil, fmt.Errorf("stat storage area %s: %w", area.Path, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("storage area %s is not a directory", area.Path)
	}

	// Walker walk storage area looking for artifact sidecars.
	// Artifacts are stored in subdirectories like <media_id>/
	// containing artifact.json sidecars.
	existingArtifacts := make(map[string]*models.ArtifactRecord) // sourcePath -> record

	err = filepath.WalkDir(area.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory.
		if path == area.Path {
			return nil
		}

		// Look for artifact.json sidecars.
		if d.Name() == artifact.SidecarFilename {
			// Read the sidecar.
			sidecar, err := artifact.ReadSidecar(filepath.Dir(path))
			if err != nil {
				i.log.Warn("reading sidecar", "path", path, "error", err)
				sum.Errors++
				return nil // Continue walking.
			}

			// Create or update the artifact record.
			outputDir := filepath.Dir(path)
			relMPDPath := sidecar.MPDPath
			record := &models.ArtifactRecord{
				StorageAreaID:   area.ID,
				SourcePath:      sidecar.SourcePath,
				SourceFingerprint: &sidecar.SourceFingerprint,
				OutputDir:       outputDir,
				MPDPath:         relMPDPath,
				Health:          models.ArtifactHealthHealthy,
				LastSeenAt:      time.Now().UTC(),
				RegisteredAt:    time.Now().UTC(),
			}

			// Check if this artifact already exists (by source_path).
			existing, err := sqlite.GetArtifactRecordByStorageAreaAndPath(ctx, i.db, area.ID, sidecar.SourcePath)
			if err == nil && existing != nil {
				// Update existing record.
				existing.Health = models.ArtifactHealthHealthy
				existing.LastSeenAt = time.Now().UTC()
				existing.UpdatedAt = time.Now().UTC()
				if sidecar.SourceFingerprint != "" {
					existing.SourceFingerprint = &sidecar.SourceFingerprint
				}
				if err := sqlite.UpsertArtifactRecord(ctx, i.db, existing); err != nil {
					i.log.Warn("updating artifact record", "error", err)
					sum.Errors++
					return nil
				}
				sum.Updated++
			} else {
				// Create new record.
				record.ID = uuid.New()
				if err := sqlite.UpsertArtifactRecord(ctx, i.db, record); err != nil {
					i.log.Warn("creating artifact record", "error", err)
					sum.Errors++
					return nil
				}
				sum.Registered++
			}

			existingArtifacts[sidecar.SourcePath] = record

			// Link media item if fingerprint exists in the library
			if sidecar.SourceFingerprint != "" {
				// Find library for the source path prefix
				lib, err := i.findLibraryForPath(ctx, sidecar.SourcePath)
				if err != nil {
					i.log.Warn("failed to query libraries for bundle indexer", "error", err)
				} else if lib != nil {
					// Check if a media item with this fingerprint already exists in this library
					existingMedia, err := sqlite.GetMediaItemByFingerprint(ctx, i.db, lib.ID, sidecar.SourceFingerprint)
					if err != nil && err != sqlite.ErrNotFound {
						i.log.Warn("failed to check media item by fingerprint during indexing", "error", err)
					}
					if existingMedia != nil {
						// Link existing media item to this bundle
						existingMedia.BundleStatus = models.BundleStatusAvailable
						existingMedia.TranscodeStatus = models.TranscodeStatusDone

						absMPD := filepath.Join(outputDir, sidecar.MPDPath)
						existingMedia.MPDPath = &absMPD
						if sidecar.Sizes != nil {
							existingMedia.TranscodeSizes = sidecar.Sizes
						}

						if err := sqlite.UpsertMediaItem(ctx, i.db, existingMedia); err != nil {
							i.log.Warn("failed to link existing media item to bundle during indexing", "path", sidecar.SourcePath, "error", err)
						} else {
							// Create artifact media link too
							link := &models.ArtifactMediaLink{
								ArtifactID:  record.ID,
								MediaItemID: existingMedia.ID,
								MatchedVia:  models.ArtifactMatchedViaFingerprint,
								Status:      models.ArtifactLinkLinked,
							}
							if err := sqlite.CreateArtifactMediaLink(ctx, i.db, link); err != nil {
								i.log.Warn("failed to create artifact media link during indexing", "error", err)
							}
						}
					}
				}
			}

			return nil
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking storage area %s: %w", area.Path, err)
	}

	// Mark artifacts as missing if their sidecar no longer exists.
	// First, get all artifacts for this storage area.
	allArtifacts, err := sqlite.ListArtifactRecordsByStorageArea(ctx, i.db, area.ID)
	if err != nil {
		i.log.Warn("listing existing artifacts", "error", err)
		return sum, nil // Don't fail the indexing if we can't list existing.
	}

	for _, art := range allArtifacts {
		if _, exists := existingArtifacts[art.SourcePath]; !exists {
			// Sidecar no longer exists — mark as missing.
			now := time.Now().UTC()
			_, err := i.db.ExecContext(ctx, `
				UPDATE artifact_records SET health = ?, updated_at = ?
				WHERE storage_area_id = ? AND source_path = ?`,
				string(models.ArtifactHealthMissing),
				now.Format(time.RFC3339),
				area.ID.String(),
				art.SourcePath,
			)
			if err != nil {
				i.log.Warn("marking artifact missing", "error", err)
				sum.Errors++
			} else {
				sum.Removed++
			}
		} else if art.Health == models.ArtifactHealthStale || art.Health == models.ArtifactHealthMissing {
			// Re-mark as healthy if the sidecar was previously missing/stale
			// but now exists again (e.g., storage was remounted).
			_, err := i.db.ExecContext(ctx, `
				UPDATE artifact_records SET health = ?, updated_at = ?
				WHERE storage_area_id = ? AND source_path = ?`,
				string(models.ArtifactHealthHealthy),
				time.Now().UTC().Format(time.RFC3339),
				area.ID.String(),
				art.SourcePath,
			)
			if err != nil {
				i.log.Warn("marking artifact healthy again", "error", err)
			}
		}
	}

	// Mark artifacts as stale if their last_seen_at is older than staleAfter.
	staleThreshold := time.Now().UTC().Add(-i.staleAfter)
	_, err = i.db.ExecContext(ctx, `
		UPDATE artifact_records SET health = ?, updated_at = ?
		WHERE storage_area_id = ?
		  AND health NOT IN ('missing', 'metadata_invalid', 'unavailable')
		  AND last_seen_at < ?`,
		string(models.ArtifactHealthStale),
		time.Now().UTC().Format(time.RFC3339),
		area.ID.String(),
		staleThreshold.Format(time.RFC3339),
	)
	if err != nil {
		i.log.Warn("marking artifacts stale", "error", err)
	}

	return sum, nil
}

// IndexAll scans all enabled storage areas and returns a summary for each.
func (i *Indexer) IndexAll(ctx context.Context) ([]IndexSummary, error) {
	// Check if artifact schema is ready.
	ready, err := sqlite.ArtifactSchemaReady(ctx, i.db)
	if err != nil {
		return nil, fmt.Errorf("checking artifact schema: %w", err)
	}
	if !ready {
		return nil, sqlite.ErrArtifactSchemaNotReady
	}

	areas, err := sqlite.ListStorageAreasByKind(ctx, i.db, models.StorageAreaKindSegments, true)
	if err != nil {
		return nil, fmt.Errorf("listing storage areas: %w", err)
	}

	var summaries []IndexSummary
	for _, area := range areas {
		sum, err := i.IndexStorageArea(ctx, area)
		if err != nil {
			i.log.Warn("indexing storage area", "id", area.ID, "error", err)
			continue
		}
		summaries = append(summaries, *sum)
	}

	return summaries, nil
}

func (i *Indexer) findLibraryForPath(ctx context.Context, sourcePath string) (*models.Library, error) {
	libs, err := sqlite.ListLibraries(ctx, i.db)
	if err != nil {
		return nil, err
	}

	var bestLib *models.Library
	bestLen := 0
	for _, lib := range libs {
		cleanLibPath := filepath.Clean(lib.Path)
		cleanSrcPath := filepath.Clean(sourcePath)
		if strings.HasPrefix(cleanSrcPath, cleanLibPath) {
			if len(cleanLibPath) > bestLen {
				bestLib = lib
				bestLen = len(cleanLibPath)
			}
		}
	}
	return bestLib, nil
}
