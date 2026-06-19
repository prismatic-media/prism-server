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

// ErrJobNotPending indicates a job exists but is not in pending state.
var ErrJobNotPending = errors.New("job is not pending")

// CreateTranscodeJob inserts a new transcode_jobs row with status=pending.
// j.ID is assigned by this function.
func CreateTranscodeJob(ctx context.Context, db *sql.DB, j *models.TranscodeJob) error {
	j.ID = uuid.New()
	j.Status = models.TranscodeStatusPending
	j.Priority = 0
	j.CreatedAt = time.Now().UTC()

	_, err := db.ExecContext(ctx, `
		INSERT INTO transcode_jobs (id, media_item_id, status, progress, priority, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		j.ID.String(), j.MediaItemID.String(),
		string(j.Status), j.Progress,
		j.Priority,
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
		SELECT id, media_item_id, worker_id, status, progress, priority, error_msg, started_at, finished_at, created_at
		FROM transcode_jobs WHERE id = ?`, id.String())
	return scanJob(row)
}

// ListTranscodeJobs returns all transcode jobs ordered by creation time (newest first).
func ListTranscodeJobs(ctx context.Context, db *sql.DB) ([]*models.TranscodeJob, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, media_item_id, worker_id, status, progress, priority, error_msg, started_at, finished_at, created_at
		FROM transcode_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing transcode jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
		SELECT id, media_item_id, worker_id, status, progress, priority, error_msg, started_at, finished_at, created_at
		FROM transcode_jobs WHERE status = 'pending' ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing pending jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

// HasTranscodeJobForMediaItem reports whether any transcode job exists for the media item.
func HasTranscodeJobForMediaItem(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM transcode_jobs WHERE media_item_id = ? LIMIT 1)`,
		mediaItemID.String(),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking transcode job existence: %w", err)
	}
	return exists == 1, nil
}

// GetTranscodeJobByMediaItem fetches the transcode job associated with a media item.
func GetTranscodeJobByMediaItem(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) (*models.TranscodeJob, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, media_item_id, worker_id, status, progress, priority, error_msg, started_at, finished_at, created_at
		FROM transcode_jobs WHERE media_item_id = ?`, mediaItemID.String())
	return scanJob(row)
}

// ResetTranscodeJob resets an existing job back to pending status.
func ResetTranscodeJob(ctx context.Context, db *sql.DB, j *models.TranscodeJob) error {
	_, err := db.ExecContext(ctx, `
		UPDATE transcode_jobs
		SET status = ?, progress = 0, worker_id = NULL, error_msg = NULL,
		    started_at = NULL, finished_at = NULL, created_at = ?
		WHERE id = ?`,
		string(models.TranscodeStatusPending),
		j.CreatedAt.Format(time.RFC3339),
		j.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("resetting transcode job: %w", err)
	}
	return nil
}

// ClaimNextJob atomically claims the highest-priority pending job.
// Returns (nil, nil) when there are no pending jobs.
func ClaimNextJob(ctx context.Context, db *sql.DB, workerID *uuid.UUID) (*models.TranscodeJob, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var query string
	var args []any

	if workerID != nil {
		query = `
			UPDATE transcode_jobs
			SET status = 'processing', worker_id = ?, started_at = ?, finished_at = NULL, error_msg = NULL
			WHERE id = (
				SELECT id FROM transcode_jobs
				WHERE status = 'pending'
				ORDER BY priority DESC, created_at ASC
				LIMIT 1
			)
			RETURNING id, media_item_id, worker_id, status, progress, priority, error_msg, started_at, finished_at, created_at`
		args = []any{workerID.String(), now}
	} else {
		query = `
			UPDATE transcode_jobs
			SET status = 'processing', worker_id = NULL, started_at = ?, finished_at = NULL, error_msg = NULL
			WHERE id = (
				SELECT id FROM transcode_jobs
				WHERE status = 'pending'
				ORDER BY priority DESC, created_at ASC
				LIMIT 1
			)
			RETURNING id, media_item_id, worker_id, status, progress, priority, error_msg, started_at, finished_at, created_at`
		args = []any{now}
	}

	row := db.QueryRowContext(ctx, query, args...)

	j, err := scanJob(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claiming next job: %w", err)
	}
	return j, nil
}

// RecoverStaleJobs re-queues jobs stuck in processing status after a crash.
func RecoverStaleJobs(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		UPDATE transcode_jobs
		SET status = 'pending', started_at = NULL, finished_at = NULL, error_msg = NULL
		WHERE status = 'processing'`)
	if err != nil {
		return fmt.Errorf("recovering stale jobs: %w", err)
	}
	return nil
}

// PrioritizeJob raises a pending job to the front by setting priority=max+1.
func PrioritizeJob(ctx context.Context, db *sql.DB, id uuid.UUID) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM transcode_jobs WHERE id = ?`, id.String()).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("loading job status: %w", err)
	}
	if status != string(models.TranscodeStatusPending) {
		return ErrJobNotPending
	}

	var maxPriority int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(priority), 0)
		FROM transcode_jobs
		WHERE status = 'pending' AND id != ?`, id.String()).Scan(&maxPriority)
	if err != nil {
		return fmt.Errorf("loading max priority: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE transcode_jobs SET priority = ? WHERE id = ?`,
		maxPriority+1, id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating priority: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// BulkEnqueueUntranscoded creates jobs for media items with no prior jobs.
func BulkEnqueueUntranscoded(ctx context.Context, db *sql.DB) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT m.id
		FROM media_items m
		WHERE NOT EXISTS (
			SELECT 1 FROM transcode_jobs j WHERE j.media_item_id = m.id
		)
		ORDER BY m.created_at ASC`)
	if err != nil {
		return 0, fmt.Errorf("querying untranscoded media: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mediaIDs []string
	for rows.Next() {
		var mediaID string
		if err := rows.Scan(&mediaID); err != nil {
			return 0, fmt.Errorf("scanning untranscoded media id: %w", err)
		}
		mediaIDs = append(mediaIDs, mediaID)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, mediaID := range mediaIDs {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO transcode_jobs (id, media_item_id, status, progress, priority, created_at)
			VALUES (?, ?, 'pending', 0, 0, ?)`,
			uuid.NewString(), mediaID, now,
		)
		if err != nil {
			return 0, fmt.Errorf("inserting untranscoded job: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE media_items
			SET transcode_status = 'pending', updated_at = ?
			WHERE id = ?`, now, mediaID)
		if err != nil {
			return 0, fmt.Errorf("updating media status: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	return len(mediaIDs), nil
}


// BulkEnqueueFailed resets existing failed jobs to pending, preserving their priority and updating created_at.
func BulkEnqueueFailed(ctx context.Context, db *sql.DB) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Get all media_item_ids for failed jobs so we can update media_items.transcode_status
	rows, err := tx.QueryContext(ctx, `SELECT media_item_id FROM transcode_jobs WHERE status = 'failed'`)
	if err != nil {
		return 0, fmt.Errorf("querying failed jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mediaIDs []string
	for rows.Next() {
		var mediaID string
		if err := rows.Scan(&mediaID); err != nil {
			return 0, fmt.Errorf("scanning media id: %w", err)
		}
		mediaIDs = append(mediaIDs, mediaID)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(mediaIDs) == 0 {
		return 0, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx, `
		UPDATE transcode_jobs
		SET status = 'pending', progress = 0, worker_id = NULL, error_msg = NULL,
		    started_at = NULL, finished_at = NULL, created_at = ?
		WHERE status = 'failed'`,
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("updating failed jobs: %w", err)
	}

	for _, mediaID := range mediaIDs {
		_, err = tx.ExecContext(ctx, `
			UPDATE media_items
			SET transcode_status = 'pending', updated_at = ?
			WHERE id = ?`, now, mediaID)
		if err != nil {
			return 0, fmt.Errorf("updating media status: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}


// BulkEnqueueCompleted resets existing completed (done) jobs to pending.
func BulkEnqueueCompleted(ctx context.Context, db *sql.DB) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Get all media_item_ids for completed jobs so we can update media_items.transcode_status
	rows, err := tx.QueryContext(ctx, `SELECT media_item_id FROM transcode_jobs WHERE status = 'done'`)
	if err != nil {
		return 0, fmt.Errorf("querying completed jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mediaIDs []string
	for rows.Next() {
		var mediaID string
		if err := rows.Scan(&mediaID); err != nil {
			return 0, fmt.Errorf("scanning media id: %w", err)
		}
		mediaIDs = append(mediaIDs, mediaID)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(mediaIDs) == 0 {
		return 0, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx, `
		UPDATE transcode_jobs
		SET status = 'pending', progress = 0, worker_id = NULL, error_msg = NULL,
		    started_at = NULL, finished_at = NULL, created_at = ?
		WHERE status = 'done'`,
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("updating completed jobs: %w", err)
	}

	for _, mediaID := range mediaIDs {
		_, err = tx.ExecContext(ctx, `
			UPDATE media_items
			SET transcode_status = 'pending', updated_at = ?
			WHERE id = ?`, now, mediaID)
		if err != nil {
			return 0, fmt.Errorf("updating media status: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}




// SetMediaMPDPath writes the mpd_path and sets transcode_status=done on the
// media_items row for the given item ID.
func SetMediaMPDPath(ctx context.Context, db *sql.DB, itemID uuid.UUID, mpdPath string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET mpd_path = ?, transcode_status = 'done', bundle_status = 'available', updated_at = ?
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
	var workerID, errMsg, startedAt, finishedAt sql.NullString
	var priority int

	err := row.Scan(&id, &mediaItemID, &workerID, &status, &j.Progress, &priority,
		&errMsg, &startedAt, &finishedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning transcode job: %w", err)
	}
	return populateJob(&j, id, mediaItemID, workerID, status, priority, createdAt, errMsg, startedAt, finishedAt), nil
}

func scanJobRow(rows *sql.Rows) (*models.TranscodeJob, error) {
	var j models.TranscodeJob
	var id, mediaItemID, status, createdAt string
	var workerID, errMsg, startedAt, finishedAt sql.NullString
	var priority int

	err := rows.Scan(&id, &mediaItemID, &workerID, &status, &j.Progress, &priority,
		&errMsg, &startedAt, &finishedAt, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scanning transcode job row: %w", err)
	}
	return populateJob(&j, id, mediaItemID, workerID, status, priority, createdAt, errMsg, startedAt, finishedAt), nil
}

func populateJob(j *models.TranscodeJob, id, mediaItemID string, workerID sql.NullString, status string, priority int, createdAt string, errMsg, startedAt, finishedAt sql.NullString) *models.TranscodeJob {
	j.ID, _ = uuid.Parse(id)
	j.MediaItemID, _ = uuid.Parse(mediaItemID)
	if workerID.Valid && workerID.String != "" {
		wID, _ := uuid.Parse(workerID.String)
		j.WorkerID = &wID
	}
	j.Status = models.TranscodeStatus(status)
	j.Priority = priority
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
