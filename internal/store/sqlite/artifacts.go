package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/models"
)

// UpsertArtifactRecord inserts or updates an artifact record in a given
// storage area. If a row with the same (storage_area_id, source_path) already
// exists, it is updated in place (idempotent indexing).
func UpsertArtifactRecord(ctx context.Context, db *sql.DB, a *models.ArtifactRecord) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	now := time.Now().UTC()
	a.LastSeenAt = now
	a.UpdatedAt = now

	_, err := db.ExecContext(ctx, `
		INSERT INTO artifact_records
			(id, storage_area_id, source_path, source_fingerprint, output_dir, mpd_path, health,
			 last_seen_at, registered_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(storage_area_id, source_path) DO UPDATE SET
			source_fingerprint = excluded.source_fingerprint,
			output_dir         = excluded.output_dir,
			mpd_path           = excluded.mpd_path,
			health             = excluded.health,
			last_seen_at       = excluded.last_seen_at,
			updated_at         = excluded.updated_at
	`, a.ID.String(), a.StorageAreaID.String(), a.SourcePath,
		nullStringPtr(a.SourceFingerprint),
		nullStringPtrStr(a.OutputDir),
		nullStringPtrStr(a.MPDPath),
		string(a.Health),
		a.LastSeenAt.Format(time.RFC3339),
		a.RegisteredAt.Format(time.RFC3339),
		a.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upserting artifact record: %w", err)
	}
	return nil
}

// GetArtifactRecordByID fetches a single artifact record by primary key.
func GetArtifactRecordByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.ArtifactRecord, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, storage_area_id, source_path, source_fingerprint,
		       output_dir, mpd_path, health, last_seen_at, registered_at, updated_at
		FROM artifact_records WHERE id = ?`, id.String())
	return scanArtifactRecord(row)
}

// GetArtifactRecordByStorageAreaAndPath fetches an artifact record by storage
// area ID and source path. Returns ErrNotFound if no row matches.
func GetArtifactRecordByStorageAreaAndPath(ctx context.Context, db *sql.DB, storageAreaID uuid.UUID, sourcePath string) (*models.ArtifactRecord, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, storage_area_id, source_path, source_fingerprint,
		       output_dir, mpd_path, health, last_seen_at, registered_at, updated_at
		FROM artifact_records
		WHERE storage_area_id = ? AND source_path = ?`,
		storageAreaID.String(), sourcePath)
	return scanArtifactRecord(row)
}

// ListArtifactRecordsByStorageArea returns all artifact records for a storage
// area, ordered by health then last_seen_at.
func ListArtifactRecordsByStorageArea(ctx context.Context, db *sql.DB, storageAreaID uuid.UUID) ([]*models.ArtifactRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage_area_id, source_path, source_fingerprint,
		       output_dir, mpd_path, health, last_seen_at, registered_at, updated_at
		FROM artifact_records
		WHERE storage_area_id = ?
		ORDER BY health ASC, last_seen_at DESC`, storageAreaID.String())
	if err != nil {
		return nil, fmt.Errorf("listing artifact records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.ArtifactRecord
	for rows.Next() {
		a, err := scanArtifactRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artifact records: %w", err)
	}
	return out, nil
}

// ListArtifactRecordsByHealth returns all artifact records matching a health
// state across all storage areas.
func ListArtifactRecordsByHealth(ctx context.Context, db *sql.DB, health models.ArtifactHealth) ([]*models.ArtifactRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage_area_id, source_path, source_fingerprint,
		       output_dir, mpd_path, health, last_seen_at, registered_at, updated_at
		FROM artifact_records
		WHERE health = ?
		ORDER BY last_seen_at DESC`, string(health))
	if err != nil {
		return nil, fmt.Errorf("listing artifact records by health: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.ArtifactRecord
	for rows.Next() {
		a, err := scanArtifactRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artifact records: %w", err)
	}
	return out, nil
}

// ListArtifactRecordsByFingerprint returns all artifact records with a given
// source fingerprint.
func ListArtifactRecordsByFingerprint(ctx context.Context, db *sql.DB, fingerprint string) ([]*models.ArtifactRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage_area_id, source_path, source_fingerprint,
		       output_dir, mpd_path, health, last_seen_at, registered_at, updated_at
		FROM artifact_records
		WHERE source_fingerprint = ?
		ORDER BY last_seen_at DESC`, fingerprint)
	if err != nil {
		return nil, fmt.Errorf("listing artifact records by fingerprint: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.ArtifactRecord
	for rows.Next() {
		a, err := scanArtifactRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artifact records: %w", err)
	}
	return out, nil
}

// MarkArtifactsMissing marks all artifact records in a storage area as missing
// that were not included in the provided sourcePaths set (used for idempotent
// indexing with stale/missing transitions).
func MarkArtifactsMissing(ctx context.Context, db *sql.DB, storageAreaID uuid.UUID, sourcePaths map[string]struct{}) error {
	placeholders := make([]string, 0, len(sourcePaths))
	args := make([]any, 0, len(sourcePaths))
	for p := range sourcePaths {
		placeholders = append(placeholders, "?")
		args = append(args, p)
	}
	if len(placeholders) == 0 {
		// No paths at all — mark everything as missing.
		_, err := db.ExecContext(ctx, `
			UPDATE artifact_records
			SET health = 'missing', updated_at = ?
			WHERE storage_area_id = ? AND health != 'missing'`,
			time.Now().UTC().Format(time.RFC3339), storageAreaID.String())
		return err
	}

	query := fmt.Sprintf(`
		UPDATE artifact_records
		SET health = 'missing', updated_at = ?
		WHERE storage_area_id = ?
		  AND source_path NOT IN (%s)
		  AND health != 'missing'`,
		joinplaceholders(placeholders))
	args = append(args, time.Now().UTC().Format(time.RFC3339), storageAreaID.String())

	_, err := db.ExecContext(ctx, query, args...)
	return err
}

// CreateArtifactMediaLink inserts a new link between an artifact and a media
// item. Returns ErrNotFound if the artifact or media item does not exist.
func CreateArtifactMediaLink(ctx context.Context, db *sql.DB, link *models.ArtifactMediaLink) error {
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	link.CreatedAt = time.Now().UTC()

	_, err := db.ExecContext(ctx, `
		INSERT INTO artifact_media_links
			(id, artifact_id, media_item_id, matched_via, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(artifact_id, media_item_id) DO NOTHING
	`, link.ID.String(), link.ArtifactID.String(), link.MediaItemID.String(),
		string(link.MatchedVia), string(link.Status),
		link.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("creating artifact media link: %w", err)
	}
	return nil
}

// GetArtifactMediaLinkByArtifact fetches the link for a given artifact, if any.
func GetArtifactMediaLinkByArtifact(ctx context.Context, db *sql.DB, artifactID uuid.UUID) (*models.ArtifactMediaLink, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, artifact_id, media_item_id, matched_via, status, created_at
		FROM artifact_media_links WHERE artifact_id = ?`, artifactID.String())
	return scanArtifactMediaLink(row)
}

// GetArtifactMediaLinkByMedia fetches links for a given media item.
func GetArtifactMediaLinkByMedia(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) ([]*models.ArtifactMediaLink, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, artifact_id, media_item_id, matched_via, status, created_at
		FROM artifact_media_links WHERE media_item_id = ?
		ORDER BY created_at DESC`, mediaItemID.String())
	if err != nil {
		return nil, fmt.Errorf("listing artifact media links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.ArtifactMediaLink
	for rows.Next() {
		l, err := scanArtifactMediaLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artifact media links: %w", err)
	}
	return out, nil
}

// DeleteArtifactMediaLink removes a specific artifact-media link.
func DeleteArtifactMediaLink(ctx context.Context, db *sql.DB, linkID uuid.UUID) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM artifact_media_links WHERE id = ?`, linkID.String())
	if err != nil {
		return fmt.Errorf("deleting artifact media link: %w", err)
	}
	return nil
}

// CountArtifactRecordsByHealth counts artifact records grouped by health
// state for a given storage area.
type ArtifactHealthCount struct {
	Health models.ArtifactHealth `json:"health"`
	Count  int                   `json:"count"`
}

func CountArtifactRecordsByHealth(ctx context.Context, db *sql.DB, storageAreaID uuid.UUID) ([]ArtifactHealthCount, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT health, COUNT(*) as count
		FROM artifact_records
		WHERE storage_area_id = ?
		GROUP BY health
		ORDER BY health ASC`, storageAreaID.String())
	if err != nil {
		return nil, fmt.Errorf("counting artifact records by health: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var counts []ArtifactHealthCount
	for rows.Next() {
		var hc ArtifactHealthCount
		var healthStr string
		if err := rows.Scan(&healthStr, &hc.Count); err != nil {
			return nil, err
		}
		hc.Health = models.ArtifactHealth(healthStr)
		counts = append(counts, hc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artifact health counts: %w", err)
	}
	return counts, nil
}

// CountArtifactsByStatus returns artifact counts grouped by link status.
func CountArtifactsByStatus(ctx context.Context, db *sql.DB) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT status, COUNT(*) as count
		FROM artifact_media_links
		GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("counting artifacts by status: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artifact status counts: %w", err)
	}
	return counts, nil
}

// GetMediaItemByArtifactID returns the linked media item for an artifact.
func GetMediaItemByArtifactID(ctx context.Context, db *sql.DB, artifactID uuid.UUID) (*models.MediaItem, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, transcode_sizes, created_at, updated_at
		FROM media_items
		WHERE id IN (
			SELECT media_item_id FROM artifact_media_links
			WHERE artifact_id = ? AND status = 'linked'
		)
		ORDER BY created_at DESC LIMIT 1`, artifactID.String())
	return scanMediaItem(row)
}

// GetArtifactByMediaID returns the linked artifact for a media item.
func GetArtifactByMediaID(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) (*models.ArtifactRecord, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, storage_area_id, source_path, source_fingerprint,
		       output_dir, mpd_path, health, last_seen_at, registered_at, updated_at
		FROM artifact_records
		WHERE id IN (
			SELECT artifact_id FROM artifact_media_links
			WHERE media_item_id = ? AND status = 'linked'
		)
		ORDER BY created_at DESC LIMIT 1`, mediaItemID.String())
	return scanArtifactRecord(row)
}

// GetArtifactsByMediaID returns all linked artifacts for a media item.
func GetArtifactsByMediaID(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) ([]*models.ArtifactRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage_area_id, source_path, source_fingerprint,
		       output_dir, mpd_path, health, last_seen_at, registered_at, updated_at
		FROM artifact_records
		WHERE id IN (
			SELECT artifact_id FROM artifact_media_links
			WHERE media_item_id = ? AND status = 'linked'
		)
		ORDER BY created_at DESC`, mediaItemID.String())
	if err != nil {
		return nil, fmt.Errorf("listing artifacts by media item: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.ArtifactRecord
	for rows.Next() {
		a, err := scanArtifactRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artifacts by media: %w", err)
	}
	return out, nil
}

// DeleteArtifactsForStorageArea removes all artifact records and links for a
// storage area. Used during storage area deletion.
func DeleteArtifactsForStorageArea(ctx context.Context, db *sql.DB, storageAreaID uuid.UUID) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM artifact_media_links WHERE artifact_id IN
			(SELECT id FROM artifact_records WHERE storage_area_id = ?)`,
		storageAreaID.String())
	if err != nil {
		return fmt.Errorf("deleting artifact links: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		DELETE FROM artifact_records WHERE storage_area_id = ?`,
		storageAreaID.String())
	if err != nil {
		return fmt.Errorf("deleting artifact records: %w", err)
	}
	return nil
}

// CountUnmatchedArtifacts returns the count of artifacts not linked to any
// media item (status = 'unmatched').
func CountUnmatchedArtifacts(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM artifact_media_links WHERE status = 'unmatched'`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting unmatched artifacts: %w", err)
	}
	return count, nil
}

// CountAmbiguousArtifacts returns the count of artifacts with ambiguous links.
func CountAmbiguousArtifacts(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM artifact_media_links WHERE status = 'ambiguous'`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting ambiguous artifacts: %w", err)
	}
	return count, nil
}

// GetUnmatchedArtifacts returns artifacts that have no link or are explicitly
// marked as unmatched.
func GetUnmatchedArtifacts(ctx context.Context, db *sql.DB) ([]*models.ArtifactRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage_area_id, source_path, source_fingerprint,
		       output_dir, mpd_path, health, last_seen_at, registered_at, updated_at
		FROM artifact_records
		WHERE id NOT IN (
			SELECT artifact_id FROM artifact_media_links WHERE status = 'linked'
		)
		ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing unmatched artifacts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.ArtifactRecord
	for rows.Next() {
		a, err := scanArtifactRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unmatched artifacts: %w", err)
	}
	return out, nil
}

// GetArtifactsWithoutFingerprint returns artifacts that have no source
// fingerprint, for heuristic matching.
func GetArtifactsWithoutFingerprint(ctx context.Context, db *sql.DB) ([]*models.ArtifactRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage_area_id, source_path, source_fingerprint,
		       output_dir, mpd_path, health, last_seen_at, registered_at, updated_at
		FROM artifact_records
		WHERE source_fingerprint IS NULL
		ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing artifacts without fingerprint: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.ArtifactRecord
	for rows.Next() {
		a, err := scanArtifactRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artifacts without fingerprint: %w", err)
	}
	return out, nil
}

// GetMediaItemsWithoutFingerprint returns media items that have no source
// fingerprint, for heuristic matching.
func GetMediaItemsWithoutFingerprint(ctx context.Context, db *sql.DB) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, transcode_sizes, created_at, updated_at
		FROM media_items
		WHERE transcode_status = 'done'
		ORDER BY CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("listing media items without fingerprint: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.MediaItem
	for rows.Next() {
		m, err := scanMediaItemRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating media items without fingerprint: %w", err)
	}
	return out, nil
}

// CountArtifactsByStorageArea returns total count per storage area.
func CountArtifactsByStorageArea(ctx context.Context, db *sql.DB) (map[uuid.UUID]int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT storage_area_id, COUNT(*) as count
		FROM artifact_records
		GROUP BY storage_area_id`)
	if err != nil {
		return nil, fmt.Errorf("counting artifacts by storage area: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[uuid.UUID]int)
	for rows.Next() {
		var saID string
		var count int
		if err := rows.Scan(&saID, &count); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(saID)
		if err != nil {
			continue
		}
		counts[id] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artifact counts: %w", err)
	}
	return counts, nil
}

// ---------- Scanners ----------

func scanArtifactRecord(row scannerArtifactRecord) (*models.ArtifactRecord, error) {
	var a models.ArtifactRecord
	var fp sql.NullString
	var outputDir sql.NullString
	var mpdPath sql.NullString
	var healthStr string
	var lastSeenStr, registeredStr, updatedStr sql.NullString
	err := row.Scan(&a.ID, &a.StorageAreaID, &a.SourcePath, &fp, &outputDir, &mpdPath,
		&healthStr, &lastSeenStr, &registeredStr, &updatedStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scanning artifact record: %w", err)
	}
	a.Health = models.ArtifactHealth(healthStr)
	if fp.Valid {
		fpStr := fp.String
		a.SourceFingerprint = &fpStr
	}
	if outputDir.Valid {
		a.OutputDir = outputDir.String
	}
	if mpdPath.Valid {
		a.MPDPath = mpdPath.String
	}
	if lastSeenStr.Valid {
		a.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeenStr.String)
	}
	if registeredStr.Valid {
		a.RegisteredAt, _ = time.Parse(time.RFC3339, registeredStr.String)
	}
	if updatedStr.Valid {
		a.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr.String)
	}
	return &a, nil
}

func scanArtifactMediaLink(row scannerArtifactMediaLink) (*models.ArtifactMediaLink, error) {
	var l models.ArtifactMediaLink
	var statusStr, viaStr, createdAtStr sql.NullString
	err := row.Scan(&l.ID, &l.ArtifactID, &l.MediaItemID, &viaStr, &statusStr, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scanning artifact media link: %w", err)
	}
	l.MatchedVia = models.ArtifactMatchedVia(viaStr.String)
	l.Status = models.ArtifactLinkStatus(statusStr.String)
	if createdAtStr.Valid {
		l.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr.String)
	}
	return &l, nil
}

// ---------- Interfaces for scanning ----------

type scannerArtifactRecord interface {
	Scan(...any) error
}

type scannerArtifactMediaLink interface {
	Scan(...any) error
}

// nullStringPtr returns a sql.NullString for a potentially nil string pointer.
func nullStringPtr(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// nullStringPtrStr returns a sql.NullString for a string value that may be empty.
func nullStringPtrStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// joinplaceholders returns a comma-separated list of ? placeholders.
func joinplaceholders(ph []string) string {
	if len(ph) == 0 {
		return ""
	}
	result := make([]string, len(ph))
	for i := range ph {
		result[i] = "?"
	}
	return joinStr(result, ",")
}

func joinStr(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	result := s[0]
	for i := 1; i < len(s); i++ {
		result += sep + s[i]
	}
	return result
}

// ArtifactSchemaReady checks whether the artifact_records table exists in the
// database. This is used during the rollout period to guard artifact operations
// that require the new schema. Returns true if the migration has been applied.
func ArtifactSchemaReady(ctx context.Context, db *sql.DB) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='artifact_records'",
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking artifact schema: %w", err)
	}
	return count > 0, nil
}

// ErrArtifactSchemaNotReady is returned when an artifact store operation is
// attempted before the artifact_records migration has been applied.
var ErrArtifactSchemaNotReady = errors.New("artifact schema not yet applied — run goose up to enable artifact persistence")
