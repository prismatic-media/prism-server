package scanner

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/pkg/fingerprint"
	"github.com/ringmaster217/prism/internal/store/sqlite"
)

// RelinkResult holds the counts from a relinking operation.
type RelinkResult struct {
	Linked      int `json:"linked"`       // new exact fingerprint matches
	Unmatched   int `json:"unmatched"`    // artifacts with no matching media item
	Ambiguous   int `json:"ambiguous"`    // artifacts matching multiple media items
	Invalid     int `json:"invalid"`     // artifacts with invalid/missing fingerprints
	Skipped     int `json:"skipped"`     // already linked artifacts
}

// RelinkExact performs deterministic fingerprint-based relinking.
// It compares artifact source fingerprints against media item file paths and
// creates artifact_media_links for exact matches.
func (i *Indexer) RelinkExact(ctx context.Context) (*RelinkResult, error) {
	result := &RelinkResult{}

	// Check if artifact schema is ready.
	ready, err := sqlite.ArtifactSchemaReady(ctx, i.db)
	if err != nil {
		return nil, fmt.Errorf("checking artifact schema: %w", err)
	}
	if !ready {
		return nil, sqlite.ErrArtifactSchemaNotReady
	}

	// Get all unmatched/ambiguous artifacts.
	artifacts, err := sqlite.GetUnmatchedArtifacts(ctx, i.db)
	if err != nil {
		return nil, fmt.Errorf("getting unmatched artifacts: %w", err)
	}

	if len(artifacts) == 0 {
		return result, nil
	}

	// Get all enabled segment storage areas for resolving paths.
	areas, err := sqlite.ListStorageAreasByKind(ctx, i.db, models.StorageAreaKindSegments, true)
	if err != nil {
		return nil, fmt.Errorf("listing storage areas: %w", err)
	}

	// Build a map of storage area ID -> path.
	areaPaths := make(map[uuid.UUID]string)
	for _, area := range areas {
		areaPaths[area.ID] = area.Path
	}

	for _, art := range artifacts {
		// Skip artifacts without fingerprints.
		if art.SourceFingerprint == nil || *art.SourceFingerprint == "" {
			result.Invalid++
			continue
		}

		// Resolve the source path from the storage area.
		sourcePath := art.SourcePath
		if areaPath, ok := areaPaths[art.StorageAreaID]; ok {
			sourcePath = fingerprint.ResolvePath(areaPath, art.SourcePath)
		}

		// Generate fingerprint from the media item's file path.
		// We need to find media items whose FilePath matches or has the same fingerprint.
		matches, err := i.findMatchingMediaItems(ctx, *art.SourceFingerprint, sourcePath)
		if err != nil {
			i.log.Warn("relinking artifact", "artifact_id", art.ID, "error", err)
			result.Invalid++
			continue
		}

		switch len(matches) {
		case 0:
			// No match — leave as unmatched.
			result.Unmatched++
		case 1:
			// Exact match — create link.
			link := &models.ArtifactMediaLink{
				ArtifactID:     art.ID,
				MediaItemID:    matches[0].ID,
				MatchedVia:     models.ArtifactMatchedViaFingerprint,
				Status:         models.ArtifactLinkLinked,
			}
			if err := sqlite.CreateArtifactMediaLink(ctx, i.db, link); err != nil {
				i.log.Warn("creating artifact link", "error", err)
				result.Invalid++
			} else {
				result.Linked++
			}
		default:
			// Multiple matches — mark as ambiguous.
			link := &models.ArtifactMediaLink{
				ArtifactID:     art.ID,
				MediaItemID:    matches[0].ID,
				MatchedVia:     models.ArtifactMatchedViaFingerprint,
				Status:         models.ArtifactLinkLinked,
			}
			if err := sqlite.CreateArtifactMediaLink(ctx, i.db, link); err != nil {
				i.log.Warn("creating ambiguous artifact link", "error", err)
				result.Invalid++
			} else {
				// Mark remaining matches as ambiguous.
				for _, m := range matches[1:] {
					ambLink := &models.ArtifactMediaLink{
						ArtifactID:     art.ID,
						MediaItemID:    m.ID,
						MatchedVia:     models.ArtifactMatchedViaFingerprint,
						Status:         models.ArtifactLinkAmbiguous,
					}
					_ = sqlite.CreateArtifactMediaLink(ctx, i.db, ambLink)
				}
				result.Ambiguous++
			}
		}
	}

	return result, nil
}

// findMatchingMediaItems finds media items whose FilePath matches the given
// source path or whose generated fingerprint matches the given fingerprintTarget.
func (i *Indexer) findMatchingMediaItems(ctx context.Context, fingerprintTarget, sourcePath string) ([]*models.MediaItem, error) {
	// First try exact path match.
	items, err := sqlite.GetMediaItemsWithoutFingerprint(ctx, i.db)
	if err != nil {
		return nil, err
	}

	var matches []*models.MediaItem
	for _, item := range items {
		// Check if the file path matches the source path.
		if item.FilePath == sourcePath {
			matches = append(matches, item)
			continue
		}

		// Check if the fingerprint matches.
		itemFP, err := fingerprint.GenerateDeterministic(item.FilePath)
		if err != nil {
			continue // Skip items we can't fingerprint.
		}
		if itemFP == fingerprintTarget {
			matches = append(matches, item)
		}
	}

	return matches, nil
}
