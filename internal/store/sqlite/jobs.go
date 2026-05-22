package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ringmaster217/galactic-media-server/internal/models"
)

// CreateTranscodeJob inserts a new transcode_jobs row with status=pending.
// j.ID is assigned by this function.
func CreateTranscodeJob(ctx context.Context, db *sql.DB, j *models.TranscodeJob) error {
	j.ID = uuid.New()
	j.Status = models.TranscodeStatusPending
	j.CreatedAt = time.Now().UTC()

	_, err := db.ExecContext(ctx, `
		INSERT INTO transcode_jobs (id, media_item_id, status, progress, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		j.ID.String(), j.MediaItemID.String(),
		string(j.Status), j.Progress,
		j.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("creating transcode job: %w", err)
	}
	return nil
}

// GetTranscodeJobByID fetches a single transcode job by primary key.
func GetTranscodeJobByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.TranscodeJob, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, media_item_id, status, progress, error_msg, started_at, finished_at, created_at
		FROM transcode_jobs WHERE id = ?`, id.String())
	return scanJob(row)
}

// ListTranscodeJobs returns all transcode jobs ordered by creation time (newest first).
func ListTranscodeJobs(ctx context.Context, db *sql.DB) ([]*models.TranscodeJob, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, media_item_id, status, progress, error_msg, started_at, finished_at, created_at
		FROM transcode_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing transcode jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*models.TranscodeJob
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// ListPendingJobs returns all jobs with status=pending, oldest first.
func ListPendingJobs(ctx context.Context, db *sql.DB) ([]*models.TranscodeJob, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, media_item_id, status, progress, error_msg, started_at, finished_at, created_at
		FROM transcode_jobs WHERE status = 'pending' ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing pending jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*models.TranscodeJob
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateJobStatus updates the status and optionally sets timestamps and error message.
func UpdateJobStatus(ctx context.Context, db *sql.DB, id uuid.UUID, status models.TranscodeStatus, errMsg *string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	var query string
	var args []any

	switch status {
	case models.TranscodeStatusProcessing:
		query = `UPDATE transcode_jobs SET status = ?, started_at = ? WHERE id = ?`
		args = []any{string(status), now, id.String()}
	case models.TranscodeStatusDone, models.TranscodeStatusFailed:
		msg := ""
		if errMsg != nil {
			msg = *errMsg
		}
		query = `UPDATE transcode_jobs SET status = ?, finished_at = ?, error_msg = ? WHERE id = ?`
		args = []any{string(status), now, nullStr(msg), id.String()}
	default:
		query = `UPDATE transcode_jobs SET status = ? WHERE id = ?`
		args = []any{string(status), id.String()}
	}

	_, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating job status: %w", err)
	}
	return nil
}

// UpdateJobProgress sets the progress field (0–100) of a job.
func UpdateJobProgress(ctx context.Context, db *sql.DB, id uuid.UUID, progress float64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE transcode_jobs SET progress = ? WHERE id = ?`,
		progress, id.String(),
	)
	return err
}

// SetMediaMPDPath writes the mpd_path and sets transcode_status=done on the
// media_items row for the given item ID.
func SetMediaMPDPath(ctx context.Context, db *sql.DB, itemID uuid.UUID, mpdPath string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET mpd_path = ?, transcode_status = 'done', updated_at = ?
		WHERE id = ?`,
		mpdPath, now, itemID.String(),
	)
	if err != nil {
		return fmt.Errorf("setting mpd path: %w", err)
	}
	return nil
}

// SetMediaTranscodeStatus updates transcode_status on the media_items row.
func SetMediaTranscodeStatus(ctx context.Context, db *sql.DB, itemID uuid.UUID, status models.TranscodeStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		UPDATE media_items SET transcode_status = ?, updated_at = ? WHERE id = ?`,
		string(status), now, itemID.String(),
	)
	if err != nil {
		return fmt.Errorf("setting transcode status: %w", err)
	}
	return nil
}

// --- internal scan helpers ---

func scanJob(row *sql.Row) (*models.TranscodeJob, error) {
	var j models.TranscodeJob
	var id, mediaItemID, status, createdAt string
	var errMsg, startedAt, finishedAt sql.NullString

	err := row.Scan(&id, &mediaItemID, &status, &j.Progress,
		&errMsg, &startedAt, &finishedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning transcode job: %w", err)
	}
	return populateJob(&j, id, mediaItemID, status, createdAt, errMsg, startedAt, finishedAt), nil
}

func scanJobRow(rows *sql.Rows) (*models.TranscodeJob, error) {
	var j models.TranscodeJob
	var id, mediaItemID, status, createdAt string
	var errMsg, startedAt, finishedAt sql.NullString

	err := rows.Scan(&id, &mediaItemID, &status, &j.Progress,
		&errMsg, &startedAt, &finishedAt, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scanning transcode job row: %w", err)
	}
	return populateJob(&j, id, mediaItemID, status, createdAt, errMsg, startedAt, finishedAt), nil
}

func populateJob(j *models.TranscodeJob, id, mediaItemID, status, createdAt string, errMsg, startedAt, finishedAt sql.NullString) *models.TranscodeJob {
	j.ID, _ = uuid.Parse(id)
	j.MediaItemID, _ = uuid.Parse(mediaItemID)
	j.Status = models.TranscodeStatus(status)
	j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if errMsg.Valid {
		j.ErrorMsg = &errMsg.String
	}
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		j.StartedAt = &t
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		j.FinishedAt = &t
	}
	return j
}
