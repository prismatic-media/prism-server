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

var defaultVideoProfiles = []struct {
	Name          string
	Width         int
	Height        int
	VideoBitrateK int
	AudioBitrateK int
	Codec         string
}{
	{Name: "360p",  Height: 360,  Width: 640,  VideoBitrateK: 400,  AudioBitrateK: 64,  Codec: "h264"},
	{Name: "480p",  Height: 480,  Width: 854,  VideoBitrateK: 800,  AudioBitrateK: 96,  Codec: "h264"},
	{Name: "720p",  Height: 720,  Width: 1280, VideoBitrateK: 2500, AudioBitrateK: 128, Codec: "h264"},
	{Name: "1080p", Height: 1080, Width: 1920, VideoBitrateK: 8000, AudioBitrateK: 192, Codec: "h264"},
}

// CreateTranscodeJob inserts a new transcode_jobs row with status=pending.
// It also creates corresponding sub-jobs for each eligible video profile and subtitles.
// j.ID is assigned by this function.
func CreateTranscodeJob(ctx context.Context, db *sql.DB, j *models.TranscodeJob) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	j.ID = uuid.New()
	j.Status = models.TranscodeStatusPending
	j.Priority = 0
	j.CreatedAt = time.Now().UTC()

	_, err = tx.ExecContext(ctx, `
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

	// Create sub-jobs
	if err := createSubJobsForJob(ctx, tx, j.ID, j.MediaItemID); err != nil {
		return err
	}

	return tx.Commit()
}

// GetTranscodeJobByID fetches a single transcode job by primary key.
func GetTranscodeJobByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.TranscodeJob, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, media_item_id, worker_id, status, progress, priority, error_msg, started_at, finished_at, created_at
		FROM transcode_jobs WHERE id = ?`, id.String())
	job, err := scanJob(row)
	if err != nil {
		return nil, err
	}
	subJobs, err := ListTranscodeSubJobsByJob(ctx, db, job.ID)
	if err == nil {
		job.SubJobs = subJobs
	}
	return job, nil
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()

	for _, j := range jobs {
		subJobs, err := ListTranscodeSubJobsByJob(ctx, db, j.ID)
		if err == nil {
			j.SubJobs = subJobs
		}
	}
	return jobs, nil
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

// ResetTranscodeJob resets an existing job back to pending status and recreates sub-jobs.
func ResetTranscodeJob(ctx context.Context, db *sql.DB, j *models.TranscodeJob) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete old sub-jobs
	_, err = tx.ExecContext(ctx, `DELETE FROM transcode_sub_jobs WHERE job_id = ?`, j.ID.String())
	if err != nil {
		return fmt.Errorf("deleting old sub-jobs: %w", err)
	}

	// Reset parent job
	_, err = tx.ExecContext(ctx, `
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

	// Recreate sub-jobs
	if err := createSubJobsForJob(ctx, tx, j.ID, j.MediaItemID); err != nil {
		return err
	}

	return tx.Commit()
}

// RecoverStaleJobs re-queues jobs stuck in processing status after a crash.
func RecoverStaleJobs(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		UPDATE transcode_sub_jobs
		SET status = 'pending', started_at = NULL, finished_at = NULL, error_msg = NULL, progress = 0
		WHERE status = 'processing'`)
	if err != nil {
		return fmt.Errorf("recovering stale sub-jobs: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE transcode_jobs
		SET status = 'pending', started_at = NULL, finished_at = NULL, error_msg = NULL, progress = 0
		WHERE status = 'processing'`)
	if err != nil {
		return fmt.Errorf("recovering stale jobs: %w", err)
	}

	return tx.Commit()
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

func createSubJobsForJob(ctx context.Context, tx *sql.Tx, jobID, mediaItemID uuid.UUID) error {
	// Fetch media item dimensions
	var width, height int
	err := tx.QueryRowContext(ctx, "SELECT width, height FROM media_items WHERE id = ?", mediaItemID.String()).Scan(&width, &height)
	if err != nil {
		return fmt.Errorf("fetching media item: %w", err)
	}

	// Fetch active profiles
	rows, err := tx.QueryContext(ctx, "SELECT id, name, width, height, video_bitrate_k, audio_bitrate_k, codec, is_active FROM transcode_profiles WHERE is_active = 1")
	if err != nil {
		return fmt.Errorf("listing active profiles: %w", err)
	}
	defer rows.Close()

	var profiles []models.TranscodeProfile
	for rows.Next() {
		var p models.TranscodeProfile
		var id string
		var isActive int
		err := rows.Scan(&id, &p.Name, &p.Width, &p.Height, &p.VideoBitrateK, &p.AudioBitrateK, &p.Codec, &isActive)
		if err != nil {
			return fmt.Errorf("scanning transcode profile: %w", err)
		}
		p.ID, _ = uuid.Parse(id)
		p.IsActive = isActive == 1
		profiles = append(profiles, p)
	}

	if len(profiles) == 0 {
		// Fallback to defaults
		for _, dp := range defaultVideoProfiles {
			profiles = append(profiles, models.TranscodeProfile{
				ID:            uuid.Nil,
				Name:          dp.Name,
				Width:         dp.Width,
				Height:        dp.Height,
				VideoBitrateK: dp.VideoBitrateK,
				AudioBitrateK: dp.AudioBitrateK,
				Codec:         dp.Codec,
				IsActive:      true,
			})
		}
	}

	// Filter profiles to avoid upscaling
	var filtered []models.TranscodeProfile
	if width > 0 && height > 0 {
		for _, prof := range profiles {
			if prof.Height <= height || (prof.Width > 0 && width >= prof.Width) {
				filtered = append(filtered, prof)
			}
		}
	}
	if len(filtered) > 0 {
		profiles = filtered
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Create video sub-jobs
	for _, prof := range profiles {
		var profileIDVal any
		if prof.ID != uuid.Nil {
			profileIDVal = prof.ID.String()
		} else {
			profileIDVal = nil
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO transcode_sub_jobs (
				id, job_id, worker_id, type, profile_id,
				profile_name, width, height, video_bitrate_k, audio_bitrate_k, codec,
				status, progress, error_msg, started_at, finished_at, created_at
			)
			VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NULL, NULL, NULL, ?)`,
			uuid.New().String(), jobID.String(), string(models.SubJobTypeVideo), profileIDVal,
			prof.Name, prof.Width, prof.Height, prof.VideoBitrateK, prof.AudioBitrateK, prof.Codec,
			string(models.TranscodeStatusPending), now,
		)
		if err != nil {
			return fmt.Errorf("creating video sub-job: %w", err)
		}
	}

	// Create subtitle sub-job
	_, err = tx.ExecContext(ctx, `
		INSERT INTO transcode_sub_jobs (id, job_id, worker_id, type, profile_id, status, progress, error_msg, started_at, finished_at, created_at)
		VALUES (?, ?, NULL, ?, NULL, ?, 0, NULL, NULL, NULL, ?)`,
		uuid.New().String(), jobID.String(), string(models.SubJobTypeSubtitles), string(models.TranscodeStatusPending), now,
	)
	if err != nil {
		return fmt.Errorf("creating subtitle sub-job: %w", err)
	}

	// Create whisper transcription sub-job
	_, err = tx.ExecContext(ctx, `
		INSERT INTO transcode_sub_jobs (id, job_id, worker_id, type, profile_id, status, progress, error_msg, started_at, finished_at, created_at)
		VALUES (?, ?, NULL, ?, NULL, ?, 0, NULL, NULL, NULL, ?)`,
		uuid.New().String(), jobID.String(), string(models.SubJobTypeWhisper), string(models.TranscodeStatusPending), now,
	)
	if err != nil {
		return fmt.Errorf("creating whisper sub-job: %w", err)
	}

	return nil
}

// ClaimNextSubJob atomically claims the highest-priority pending sub-job.
func ClaimNextSubJob(ctx context.Context, db *sql.DB, workerID *uuid.UUID) (*models.TranscodeSubJob, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	threshold := time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339)

	var workerIDVal any
	if workerID != nil {
		workerIDVal = workerID.String()
	}

	query := `
		UPDATE transcode_sub_jobs
		SET status = 'processing', worker_id = ?, started_at = ?, finished_at = NULL, error_msg = NULL
		WHERE id = (
			WITH active_workers AS (
				SELECT id FROM transcode_workers
				WHERE status != 'offline' AND last_heartbeat >= ?
			),
			job_pinnings AS (
				SELECT DISTINCT job_id, worker_id AS pinned_worker_id
				FROM transcode_sub_jobs
				WHERE status IN ('processing', 'done', 'failed')
				  AND (
					  worker_id IS NULL OR
					  worker_id IN (SELECT id FROM active_workers)
				  )
			),
			candidate_sub_jobs AS (
				SELECT s.id AS sub_job_id,
					   s.job_id,
					   j.priority,
					   j.created_at,
					   s.type,
					   jp.pinned_worker_id,
					   CASE
						   WHEN (jp.pinned_worker_id IS NULL AND ? IS NULL) OR (jp.pinned_worker_id = ?) THEN 1
						   WHEN jp.pinned_worker_id IS NULL THEN 2
						   ELSE 3
					   END AS pinning_category
				FROM transcode_sub_jobs s
				JOIN transcode_jobs j ON s.job_id = j.id
				LEFT JOIN job_pinnings jp ON s.job_id = jp.job_id
				WHERE s.status = 'pending' AND j.status != 'failed'
			)
			SELECT sub_job_id FROM candidate_sub_jobs
			WHERE pinning_category IN (1, 2)
			   OR (pinning_category = 3 AND NOT EXISTS (
				   SELECT 1 FROM candidate_sub_jobs WHERE pinning_category IN (1, 2)
			   ))
			ORDER BY 
				pinning_category ASC,
				priority DESC,
				created_at ASC,
				type DESC,
				sub_job_id ASC
			LIMIT 1
		)
		RETURNING id, job_id, worker_id, type, profile_id, profile_name, width, height, video_bitrate_k, audio_bitrate_k, codec, status, progress, error_msg, started_at, finished_at, created_at`

	args := []any{
		workerIDVal,
		now,
		threshold,
		workerIDVal,
		workerIDVal,
	}

	row := tx.QueryRowContext(ctx, query, args...)
	subJob, err := scanSubJob(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claiming next sub-job: %w", err)
	}

	// Fetch parent job
	var parentStatus string
	var mediaItemID string
	err = tx.QueryRowContext(ctx, `SELECT status, media_item_id FROM transcode_jobs WHERE id = ?`, subJob.JobID.String()).Scan(&parentStatus, &mediaItemID)
	if err != nil {
		return nil, fmt.Errorf("fetching parent job status: %w", err)
	}

	// If parent job is pending, update it to processing
	if parentStatus == string(models.TranscodeStatusPending) {
		_, err = tx.ExecContext(ctx, `UPDATE transcode_jobs SET status = 'processing', started_at = ? WHERE id = ?`, now, subJob.JobID.String())
		if err != nil {
			return nil, fmt.Errorf("updating parent job to processing: %w", err)
		}
		// Update media item transcode status to processing
		_, err = tx.ExecContext(ctx, `UPDATE media_items SET transcode_status = 'processing', updated_at = ? WHERE id = ?`, now, mediaItemID)
		if err != nil {
			return nil, fmt.Errorf("updating media item to processing: %w", err)
		}
	}

	subJob.MediaItemID, _ = uuid.Parse(mediaItemID)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing claimed sub-job transaction: %w", err)
	}

	return subJob, nil
}

// UpdateSubJobProgress sets the progress field (0-100) of a sub-job and updates the parent's average progress.
func UpdateSubJobProgress(ctx context.Context, db *sql.DB, subJobID uuid.UUID, progress float64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Update sub-job progress
	_, err = tx.ExecContext(ctx, `UPDATE transcode_sub_jobs SET progress = ? WHERE id = ?`, progress, subJobID.String())
	if err != nil {
		return err
	}

	// Fetch job ID for this sub-job
	var jobID string
	err = tx.QueryRowContext(ctx, `SELECT job_id FROM transcode_sub_jobs WHERE id = ?`, subJobID.String()).Scan(&jobID)
	if err != nil {
		return err
	}

	// Calculate average progress of all sub-jobs for this job
	var avgProgress float64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(AVG(progress), 0) FROM transcode_sub_jobs WHERE job_id = ?`, jobID).Scan(&avgProgress)
	if err != nil {
		return err
	}

	// Update parent job progress
	_, err = tx.ExecContext(ctx, `UPDATE transcode_jobs SET progress = ? WHERE id = ?`, avgProgress, jobID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateSubJobStatus updates a sub-job's status and checks parent job completion/failure states.
func UpdateSubJobStatus(ctx context.Context, db *sql.DB, subJobID uuid.UUID, status models.TranscodeStatus, errMsg *string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	var query string
	var args []any

	switch status {
	case models.TranscodeStatusProcessing:
		query = `UPDATE transcode_sub_jobs SET status = ?, started_at = ? WHERE id = ?`
		args = []any{string(status), now, subJobID.String()}
	case models.TranscodeStatusDone, models.TranscodeStatusFailed:
		msg := ""
		if errMsg != nil {
			msg = *errMsg
		}
		query = `UPDATE transcode_sub_jobs SET status = ?, finished_at = ?, error_msg = ? WHERE id = ?`
		args = []any{string(status), now, nullStr(msg), subJobID.String()}
	default:
		query = `UPDATE transcode_sub_jobs SET status = ? WHERE id = ?`
		args = []any{string(status), subJobID.String()}
	}

	_, err = tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating sub-job status: %w", err)
	}

	// Fetch job ID
	var jobID string
	err = tx.QueryRowContext(ctx, `SELECT job_id FROM transcode_sub_jobs WHERE id = ?`, subJobID.String()).Scan(&jobID)
	if err != nil {
		return err
	}

	// Fetch parent job status and media item ID
	var parentStatus, mediaItemID string
	err = tx.QueryRowContext(ctx, `SELECT status, media_item_id FROM transcode_jobs WHERE id = ?`, jobID).Scan(&parentStatus, &mediaItemID)
	if err != nil {
		return err
	}

	if status == models.TranscodeStatusFailed {
		// If a sub-job fails, mark the parent job as failed
		errStr := "sub-job failed"
		if errMsg != nil {
			errStr = *errMsg
		}
		_, err = tx.ExecContext(ctx, `UPDATE transcode_jobs SET status = 'failed', finished_at = ?, error_msg = ? WHERE id = ?`, now, nullStr(errStr), jobID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE media_items SET transcode_status = 'failed', updated_at = ? WHERE id = ?`, now, mediaItemID)
		if err != nil {
			return err
		}
	} else if status == models.TranscodeStatusDone {
		// If sub-job is done, check if all sub-jobs are done
		var pendingOrProcessing int
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM transcode_sub_jobs WHERE job_id = ? AND status IN ('pending', 'processing')`, jobID).Scan(&pendingOrProcessing)
		if err != nil {
			return err
		}

		if pendingOrProcessing == 0 {
			// Check if any sub-job failed
			var failedCount int
			err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM transcode_sub_jobs WHERE job_id = ? AND status = 'failed'`, jobID).Scan(&failedCount)
			if err != nil {
				return err
			}

			var finalStatus string
			if failedCount > 0 {
				finalStatus = "failed"
			} else {
				finalStatus = "done"
			}

			_, err = tx.ExecContext(ctx, `UPDATE transcode_jobs SET status = ?, finished_at = ? WHERE id = ?`, finalStatus, now, jobID)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE media_items SET transcode_status = ?, updated_at = ? WHERE id = ?`, finalStatus, now, mediaItemID)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func GetTranscodeSubJobByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.TranscodeSubJob, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, job_id, worker_id, type, profile_id, profile_name, width, height, video_bitrate_k, audio_bitrate_k, codec, status, progress, error_msg, started_at, finished_at, created_at
		FROM transcode_sub_jobs WHERE id = ?`, id.String())
	sj, err := scanSubJob(row)
	if err != nil {
		return nil, err
	}
	var mediaItemID string
	err = db.QueryRowContext(ctx, `SELECT media_item_id FROM transcode_jobs WHERE id = ?`, sj.JobID.String()).Scan(&mediaItemID)
	if err != nil {
		return nil, fmt.Errorf("fetching media_item_id for sub-job: %w", err)
	}
	sj.MediaItemID, _ = uuid.Parse(mediaItemID)
	return sj, nil
}

func ListTranscodeSubJobsByJob(ctx context.Context, db *sql.DB, jobID uuid.UUID) ([]*models.TranscodeSubJob, error) {
	var mediaItemIDStr string
	err := db.QueryRowContext(ctx, `SELECT media_item_id FROM transcode_jobs WHERE id = ?`, jobID.String()).Scan(&mediaItemIDStr)
	if err != nil {
		return nil, fmt.Errorf("fetching media_item_id for parent job: %w", err)
	}
	mediaItemID, _ := uuid.Parse(mediaItemIDStr)

	rows, err := db.QueryContext(ctx, `
		SELECT id, job_id, worker_id, type, profile_id, profile_name, width, height, video_bitrate_k, audio_bitrate_k, codec, status, progress, error_msg, started_at, finished_at, created_at
		FROM transcode_sub_jobs WHERE job_id = ? ORDER BY type DESC, id ASC`, jobID.String())
	if err != nil {
		return nil, fmt.Errorf("listing transcode sub-jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var subJobs []*models.TranscodeSubJob
	for rows.Next() {
		sj, err := scanSubJobRow(rows)
		if err != nil {
			return nil, err
		}
		sj.MediaItemID = mediaItemID
		subJobs = append(subJobs, sj)
	}
	return subJobs, rows.Err()
}

func scanSubJob(row *sql.Row) (*models.TranscodeSubJob, error) {
	var sj models.TranscodeSubJob
	var id, jobID, status, createdAt string
	var workerID, profileID, errMsg, startedAt, finishedAt sql.NullString
	var profileName, codec sql.NullString
	var width, height, videoBitrateK, audioBitrateK sql.NullInt64

	err := row.Scan(&id, &jobID, &workerID, &sj.Type, &profileID,
		&profileName, &width, &height, &videoBitrateK, &audioBitrateK, &codec,
		&status, &sj.Progress, &errMsg, &startedAt, &finishedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning transcode sub-job: %w", err)
	}
	return populateSubJob(&sj, id, jobID, workerID, profileID, profileName, width, height, videoBitrateK, audioBitrateK, codec, status, createdAt, errMsg, startedAt, finishedAt), nil
}

func scanSubJobRow(rows *sql.Rows) (*models.TranscodeSubJob, error) {
	var sj models.TranscodeSubJob
	var id, jobID, status, createdAt string
	var workerID, profileID, errMsg, startedAt, finishedAt sql.NullString
	var profileName, codec sql.NullString
	var width, height, videoBitrateK, audioBitrateK sql.NullInt64

	err := rows.Scan(&id, &jobID, &workerID, &sj.Type, &profileID,
		&profileName, &width, &height, &videoBitrateK, &audioBitrateK, &codec,
		&status, &sj.Progress, &errMsg, &startedAt, &finishedAt, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scanning transcode sub-job row: %w", err)
	}
	return populateSubJob(&sj, id, jobID, workerID, profileID, profileName, width, height, videoBitrateK, audioBitrateK, codec, status, createdAt, errMsg, startedAt, finishedAt), nil
}

func populateSubJob(
	sj *models.TranscodeSubJob,
	id, jobID string,
	workerID, profileID sql.NullString,
	profileName sql.NullString,
	width, height, videoBitrateK, audioBitrateK sql.NullInt64,
	codec sql.NullString,
	status string,
	createdAt string,
	errMsg, startedAt, finishedAt sql.NullString,
) *models.TranscodeSubJob {
	sj.ID, _ = uuid.Parse(id)
	sj.JobID, _ = uuid.Parse(jobID)
	if workerID.Valid && workerID.String != "" {
		wID, _ := uuid.Parse(workerID.String)
		sj.WorkerID = &wID
	}
	if profileID.Valid && profileID.String != "" {
		pID, _ := uuid.Parse(profileID.String)
		sj.ProfileID = &pID
	}
	if profileName.Valid {
		sj.ProfileName = &profileName.String
	}
	if width.Valid {
		v := int(width.Int64)
		sj.Width = &v
	}
	if height.Valid {
		v := int(height.Int64)
		sj.Height = &v
	}
	if videoBitrateK.Valid {
		v := int(videoBitrateK.Int64)
		sj.VideoBitrateK = &v
	}
	if audioBitrateK.Valid {
		v := int(audioBitrateK.Int64)
		sj.AudioBitrateK = &v
	}
	if codec.Valid {
		sj.Codec = &codec.String
	}
	sj.Status = models.TranscodeStatus(status)
	sj.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if errMsg.Valid {
		sj.ErrorMsg = &errMsg.String
	}
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		sj.StartedAt = &t
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		sj.FinishedAt = &t
	}
	return sj
}

// GetMediaItemLatestJobSubJobs retrieves the sub-jobs of the latest transcode job for a given media item.
func GetMediaItemLatestJobSubJobs(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) ([]*models.TranscodeSubJob, error) {
	var jobIDStr string
	err := db.QueryRowContext(ctx, `
		SELECT id FROM transcode_jobs
		WHERE media_item_id = ?
		ORDER BY created_at DESC LIMIT 1`, mediaItemID.String()).Scan(&jobIDStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting latest job ID: %w", err)
	}
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		return nil, fmt.Errorf("parsing job ID: %w", err)
	}
	return ListTranscodeSubJobsByJob(ctx, db, jobID)
}
