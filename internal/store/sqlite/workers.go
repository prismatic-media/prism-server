package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/prismatic-media/prism-server/internal/models"
)

// CreateWorker creates a new worker with the given name and a generated API Key.
func CreateWorker(ctx context.Context, db *sql.DB, name string) (*models.TranscodeWorker, error) {
	id := uuid.New()
	apiKey, err := generateWorkerAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generating api key: %w", err)
	}

	now := time.Now().UTC()
	worker := &models.TranscodeWorker{
		ID:          id,
		Name:        name,
		APIKey:      apiKey,
		Threads:     1,
		HWAccel:     "none",
		Status:      "offline",
		IsEphemeral: false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO transcode_workers (id, name, api_key, threads, hwaccel, status, is_ephemeral, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		worker.ID.String(), worker.Name, worker.APIKey, worker.Threads, worker.HWAccel,
		worker.Status, boolToInt(worker.IsEphemeral), worker.CreatedAt.Format(time.RFC3339), worker.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("creating worker: %w", err)
	}

	return worker, nil
}

// CreateEphemeralWorker creates a new ephemeral worker with the given name and a generated API Key.
func CreateEphemeralWorker(ctx context.Context, db *sql.DB, name string) (*models.TranscodeWorker, error) {
	id := uuid.New()
	apiKey, err := generateWorkerAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generating api key: %w", err)
	}

	now := time.Now().UTC()
	worker := &models.TranscodeWorker{
		ID:          id,
		Name:        name,
		APIKey:      apiKey,
		Threads:     1,
		HWAccel:     "none",
		Status:      "idle",
		IsEphemeral: true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO transcode_workers (id, name, api_key, threads, hwaccel, status, is_ephemeral, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		worker.ID.String(), worker.Name, worker.APIKey, worker.Threads, worker.HWAccel,
		worker.Status, boolToInt(worker.IsEphemeral), worker.CreatedAt.Format(time.RFC3339), worker.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("creating ephemeral worker: %w", err)
	}

	return worker, nil
}

// GetWorkerByID retrieves a worker by ID.
func GetWorkerByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.TranscodeWorker, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, api_key, threads, hwaccel, status, last_heartbeat, is_ephemeral, created_at, updated_at
		FROM transcode_workers WHERE id = ?`, id.String())
	return scanWorker(row)
}

// GetWorkerByAPIKey retrieves a worker by their unique API key.
func GetWorkerByAPIKey(ctx context.Context, db *sql.DB, apiKey string) (*models.TranscodeWorker, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, api_key, threads, hwaccel, status, last_heartbeat, is_ephemeral, created_at, updated_at
		FROM transcode_workers WHERE api_key = ?`, apiKey)
	return scanWorker(row)
}

// ListWorkers returns all registered transcode workers.
func ListWorkers(ctx context.Context, db *sql.DB) ([]*models.TranscodeWorker, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, api_key, threads, hwaccel, status, last_heartbeat, is_ephemeral, created_at, updated_at
		FROM transcode_workers ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing workers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var workers []*models.TranscodeWorker
	for rows.Next() {
		w, err := scanWorkerRow(rows)
		if err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

// UpdateWorkerSettings updates a worker's thread limit and hardware acceleration in the database.
func UpdateWorkerSettings(ctx context.Context, db *sql.DB, id uuid.UUID, threads int, hwaccel string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.ExecContext(ctx, `
		UPDATE transcode_workers
		SET threads = ?, hwaccel = ?, updated_at = ?
		WHERE id = ?`,
		threads, hwaccel, now, id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating worker settings: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateWorkerHeartbeat updates a worker's last_heartbeat timestamp and status.
func UpdateWorkerHeartbeat(ctx context.Context, db *sql.DB, id uuid.UUID, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.ExecContext(ctx, `
		UPDATE transcode_workers
		SET last_heartbeat = ?, status = ?, updated_at = ?
		WHERE id = ?`,
		now, status, now, id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating worker heartbeat: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWorker deletes a worker by ID. It also requeues any of its active jobs.
func DeleteWorker(ctx context.Context, db *sql.DB, id uuid.UUID) error {
	// Requeue jobs first
	_, err := RequeueWorkerJobs(ctx, db, id)
	if err != nil {
		return fmt.Errorf("requeueing jobs: %w", err)
	}

	result, err := db.ExecContext(ctx, `DELETE FROM transcode_workers WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting worker: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// RequeueWorkerJobs requeues all processing jobs assigned to workerID.
func RequeueWorkerJobs(ctx context.Context, db *sql.DB, workerID uuid.UUID) ([]RequeuedJobInfo, error) {
	// 1. Find all processing jobs associated with this worker
	rows, err := db.QueryContext(ctx, `
		SELECT j.id, j.media_item_id, m.library_id
		FROM transcode_jobs j
		JOIN media_items m ON j.media_item_id = m.id
		WHERE j.status = 'processing' AND j.worker_id = ?`,
		workerID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying processing jobs for worker: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var requeuedJobs []RequeuedJobInfo
	for rows.Next() {
		var jIDStr, mIDStr, lIDStr string
		if err := rows.Scan(&jIDStr, &mIDStr, &lIDStr); err != nil {
			return nil, fmt.Errorf("scanning job info: %w", err)
		}
		jID, _ := uuid.Parse(jIDStr)
		mID, _ := uuid.Parse(mIDStr)
		lID, _ := uuid.Parse(lIDStr)
		requeuedJobs = append(requeuedJobs, RequeuedJobInfo{
			JobID:       jID,
			MediaItemID: mID,
			LibraryID:   lID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Re-queue those jobs and update corresponding media items
	now := time.Now().UTC().Format(time.RFC3339)
	for _, job := range requeuedJobs {
		_, err := db.ExecContext(ctx, `
			UPDATE transcode_jobs
			SET status = 'pending', worker_id = NULL, started_at = NULL, finished_at = NULL, error_msg = NULL, progress = 0
			WHERE id = ?`,
			job.JobID.String(),
		)
		if err != nil {
			return nil, fmt.Errorf("resetting job: %w", err)
		}

		_, err = db.ExecContext(ctx, `
			UPDATE media_items
			SET transcode_status = 'pending', updated_at = ?
			WHERE id = ?`,
			now, job.MediaItemID.String(),
		)
		if err != nil {
			return nil, fmt.Errorf("resetting media transcode status: %w", err)
		}
	}

	return requeuedJobs, nil
}

// GetWorkerByName retrieves a worker by name.
func GetWorkerByName(ctx context.Context, db *sql.DB, name string) (*models.TranscodeWorker, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, api_key, threads, hwaccel, status, last_heartbeat, is_ephemeral, created_at, updated_at
		FROM transcode_workers WHERE name = ?`, name)
	return scanWorker(row)
}

// Helper methods

func generateWorkerAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pw_" + hex.EncodeToString(b), nil
}

func scanWorker(row *sql.Row) (*models.TranscodeWorker, error) {
	var w models.TranscodeWorker
	var id, name, apiKey, status, createdAt, updatedAt string
	var lastHeartbeat sql.NullString
	var isEphemeral int

	err := row.Scan(&id, &name, &apiKey, &w.Threads, &w.HWAccel, &status, &lastHeartbeat, &isEphemeral, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning worker: %w", err)
	}

	w.ID, _ = uuid.Parse(id)
	w.Name = name
	w.APIKey = apiKey
	w.Status = status
	w.IsEphemeral = isEphemeral != 0
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if lastHeartbeat.Valid {
		t, _ := time.Parse(time.RFC3339, lastHeartbeat.String)
		w.LastHeartbeat = &t
	}
	return &w, nil
}

func scanWorkerRow(rows *sql.Rows) (*models.TranscodeWorker, error) {
	var w models.TranscodeWorker
	var id, name, apiKey, status, createdAt, updatedAt string
	var lastHeartbeat sql.NullString
	var isEphemeral int

	err := rows.Scan(&id, &name, &apiKey, &w.Threads, &w.HWAccel, &status, &lastHeartbeat, &isEphemeral, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning worker row: %w", err)
	}

	w.ID, _ = uuid.Parse(id)
	w.Name = name
	w.APIKey = apiKey
	w.Status = status
	w.IsEphemeral = isEphemeral != 0
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if lastHeartbeat.Valid {
		t, _ := time.Parse(time.RFC3339, lastHeartbeat.String)
		w.LastHeartbeat = &t
	}
	return &w, nil
}

// RequeuedJobInfo stores IDs of a transcode job, its media item, and its library.
type RequeuedJobInfo struct {
	JobID       uuid.UUID
	MediaItemID uuid.UUID
	LibraryID   uuid.UUID
}

// RecoverFailedWorkers finds workers whose last heartbeat is older than the timeout.
// For non-ephemeral workers, it updates their status to 'offline'.
// For ephemeral workers, it deletes them entirely from the database.
// In both cases, any sub-job currently 'processing' on those workers is moved back to 'pending',
// progress is reset to 0, and the worker ID is cleared.
// Returns the list of parent jobs that were re-queued so caller can broadcast status events.
func RecoverFailedWorkers(ctx context.Context, db *sql.DB, timeout time.Duration) ([]RequeuedJobInfo, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	threshold := time.Now().UTC().Add(-timeout).Format(time.RFC3339)

	// 1. Find all active workers that haven't sent a heartbeat within the timeout threshold
	rows, err := tx.QueryContext(ctx, `
		SELECT id, is_ephemeral FROM transcode_workers
		WHERE status != 'offline' AND (last_heartbeat IS NULL OR last_heartbeat < ?)`,
		threshold,
	)
	if err != nil {
		return nil, fmt.Errorf("querying failed workers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ephemeralWorkerIDs []string
	var standardWorkerIDs []string
	var allFailedWorkerIDs []string

	for rows.Next() {
		var id string
		var isEphemeral int
		if err := rows.Scan(&id, &isEphemeral); err != nil {
			return nil, fmt.Errorf("scanning failed worker: %w", err)
		}
		allFailedWorkerIDs = append(allFailedWorkerIDs, id)
		if isEphemeral != 0 {
			ephemeralWorkerIDs = append(ephemeralWorkerIDs, id)
		} else {
			standardWorkerIDs = append(standardWorkerIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(allFailedWorkerIDs) == 0 {
		return nil, nil
	}

	// 2. Find all processing sub-jobs associated with these workers along with their parent job, media item, and library info (BEFORE we delete/update anything)
	var requeuedJobs []RequeuedJobInfo
	seenJobs := make(map[uuid.UUID]bool)
	for _, wID := range allFailedWorkerIDs {
		sjRows, err := tx.QueryContext(ctx, `
			SELECT sj.id, sj.job_id, j.media_item_id, m.library_id
			FROM transcode_sub_jobs sj
			JOIN transcode_jobs j ON sj.job_id = j.id
			JOIN media_items m ON j.media_item_id = m.id
			WHERE sj.status = 'processing' AND sj.worker_id = ?`,
			wID,
		)
		if err != nil {
			return nil, fmt.Errorf("querying processing sub-jobs for worker: %w", err)
		}
		defer func() { _ = sjRows.Close() }()

		for sjRows.Next() {
			var sjIDStr, jIDStr, mIDStr, lIDStr string
			if err := sjRows.Scan(&sjIDStr, &jIDStr, &mIDStr, &lIDStr); err != nil {
				return nil, fmt.Errorf("scanning sub-job info: %w", err)
			}
			jID, _ := uuid.Parse(jIDStr)
			mID, _ := uuid.Parse(mIDStr)
			lID, _ := uuid.Parse(lIDStr)
			if !seenJobs[jID] {
				seenJobs[jID] = true
				requeuedJobs = append(requeuedJobs, RequeuedJobInfo{
					JobID:       jID,
					MediaItemID: mID,
					LibraryID:   lID,
				})
			}
		}
		if err := sjRows.Err(); err != nil {
			return nil, err
		}
	}

	// 3. Update standard workers to offline
	now := time.Now().UTC().Format(time.RFC3339)
	for _, wID := range standardWorkerIDs {
		_, err := tx.ExecContext(ctx, `
			UPDATE transcode_workers
			SET status = 'offline', updated_at = ?
			WHERE id = ?`,
			now, wID,
		)
		if err != nil {
			return nil, fmt.Errorf("updating worker status to offline: %w", err)
		}
	}

	// 4. Delete ephemeral workers
	for _, wID := range ephemeralWorkerIDs {
		_, err := tx.ExecContext(ctx, `DELETE FROM transcode_workers WHERE id = ?`, wID)
		if err != nil {
			return nil, fmt.Errorf("deleting ephemeral worker: %w", err)
		}
	}

	// 5. Re-queue those sub-jobs
	for _, wID := range allFailedWorkerIDs {
		_, err := tx.ExecContext(ctx, `
			UPDATE transcode_sub_jobs
			SET status = 'pending', worker_id = NULL, started_at = NULL, finished_at = NULL, error_msg = NULL, progress = 0
			WHERE status = 'processing' AND worker_id = ?`,
			wID,
		)
		if err != nil {
			return nil, fmt.Errorf("resetting sub-jobs: %w", err)
		}
	}

	// 6. Reset parent jobs and media item statuses if they were failed, and recalculate parent progress
	for _, job := range requeuedJobs {
		// Clear worker_id on parent job if it matches any of the failed workers
		for _, wID := range allFailedWorkerIDs {
			_, _ = tx.ExecContext(ctx, `UPDATE transcode_jobs SET worker_id = NULL WHERE id = ? AND worker_id = ?`, job.JobID.String(), wID)
		}

		// Check if any sub-job is not pending
		var activeCount int
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM transcode_sub_jobs WHERE job_id = ? AND status IN ('processing', 'done')`, job.JobID.String()).Scan(&activeCount)
		if err != nil {
			return nil, fmt.Errorf("counting active sub-jobs: %w", err)
		}

		if activeCount == 0 {
			// All sub-jobs are pending (or failed). Reset parent job and media item to pending
			_, err = tx.ExecContext(ctx, `
				UPDATE transcode_jobs
				SET status = 'pending', worker_id = NULL, error_msg = NULL, started_at = NULL
				WHERE id = ?`,
				job.JobID.String(),
			)
			if err != nil {
				return nil, fmt.Errorf("resetting parent job to pending: %w", err)
			}

			_, err = tx.ExecContext(ctx, `
				UPDATE media_items
				SET transcode_status = 'pending', updated_at = ?
				WHERE id = ?`,
				now, job.MediaItemID.String(),
			)
			if err != nil {
				return nil, fmt.Errorf("resetting media transcode status: %w", err)
			}
		} else {
			// Some sub-jobs are done or processing. Keep parent job as processing, but clear error_msg if failed
			_, err = tx.ExecContext(ctx, `
				UPDATE transcode_jobs
				SET status = 'processing', error_msg = NULL
				WHERE id = ? AND status = 'failed'`,
				job.JobID.String(),
			)
			if err != nil {
				return nil, fmt.Errorf("updating parent job: %w", err)
			}

			_, err = tx.ExecContext(ctx, `
				UPDATE media_items
				SET transcode_status = 'processing', updated_at = ?
				WHERE id = ? AND transcode_status = 'failed'`,
				now, job.MediaItemID.String(),
			)
			if err != nil {
				return nil, fmt.Errorf("updating media transcode status: %w", err)
			}
		}

		// Recalculate average progress for parent job
		var avgProgress float64
		err = tx.QueryRowContext(ctx, `SELECT COALESCE(AVG(progress), 0) FROM transcode_sub_jobs WHERE job_id = ?`, job.JobID.String()).Scan(&avgProgress)
		if err != nil {
			return nil, fmt.Errorf("recalculating parent progress: %w", err)
		}

		_, err = tx.ExecContext(ctx, `UPDATE transcode_jobs SET progress = ? WHERE id = ?`, avgProgress, job.JobID.String())
		if err != nil {
			return nil, fmt.Errorf("updating parent progress: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing failed worker recovery: %w", err)
	}

	return requeuedJobs, nil
}

// GenerateEphemeralWorkerToken generates a new random token string.
func GenerateEphemeralWorkerToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ewt_" + hex.EncodeToString(b), nil
}

// CreateEphemeralWorkerToken creates an ephemeral token in the database.
func CreateEphemeralWorkerToken(ctx context.Context, db *sql.DB, name string) (*models.EphemeralWorkerToken, error) {
	id := uuid.New()
	token, err := GenerateEphemeralWorkerToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	t := &models.EphemeralWorkerToken{
		ID:        id,
		Token:     token,
		Name:      name,
		CreatedAt: now,
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO ephemeral_worker_tokens (id, token, name, created_at)
		VALUES (?, ?, ?, ?)`,
		t.ID.String(), t.Token, t.Name, t.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("creating ephemeral token: %w", err)
	}

	return t, nil
}

// GetEphemeralWorkerTokenByValue fetches a token by its token string.
func GetEphemeralWorkerTokenByValue(ctx context.Context, db *sql.DB, token string) (*models.EphemeralWorkerToken, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, token, name, created_at FROM ephemeral_worker_tokens WHERE token = ?`, token)
	
	var t models.EphemeralWorkerToken
	var id, tok, name, createdAt string
	err := row.Scan(&id, &tok, &name, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning ephemeral token: %w", err)
	}

	t.ID, _ = uuid.Parse(id)
	t.Token = tok
	t.Name = name
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &t, nil
}

// ListEphemeralWorkerTokens lists all ephemeral worker tokens.
func ListEphemeralWorkerTokens(ctx context.Context, db *sql.DB) ([]*models.EphemeralWorkerToken, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, token, name, created_at FROM ephemeral_worker_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing ephemeral tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tokens []*models.EphemeralWorkerToken
	for rows.Next() {
		var t models.EphemeralWorkerToken
		var id, tok, name, createdAt string
		if err := rows.Scan(&id, &tok, &name, &createdAt); err != nil {
			return nil, err
		}
		t.ID, _ = uuid.Parse(id)
		t.Token = tok
		t.Name = name
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		tokens = append(tokens, &t)
	}
	return tokens, rows.Err()
}

// DeleteEphemeralWorkerToken deletes an ephemeral token by ID.
func DeleteEphemeralWorkerToken(ctx context.Context, db *sql.DB, id uuid.UUID) error {
	result, err := db.ExecContext(ctx, `DELETE FROM ephemeral_worker_tokens WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting ephemeral token: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

