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
	"github.com/ringmaster217/prism/internal/models"
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
		ID:        id,
		Name:      name,
		APIKey:    apiKey,
		Threads:   2,
		HWAccel:   "none",
		Status:    "offline",
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO transcode_workers (id, name, api_key, threads, hwaccel, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		worker.ID.String(), worker.Name, worker.APIKey, worker.Threads, worker.HWAccel,
		worker.Status, worker.CreatedAt.Format(time.RFC3339), worker.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("creating worker: %w", err)
	}

	return worker, nil
}

// GetWorkerByID retrieves a worker by ID.
func GetWorkerByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.TranscodeWorker, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, api_key, threads, hwaccel, status, last_heartbeat, created_at, updated_at
		FROM transcode_workers WHERE id = ?`, id.String())
	return scanWorker(row)
}

// GetWorkerByAPIKey retrieves a worker by their unique API key.
func GetWorkerByAPIKey(ctx context.Context, db *sql.DB, apiKey string) (*models.TranscodeWorker, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, api_key, threads, hwaccel, status, last_heartbeat, created_at, updated_at
		FROM transcode_workers WHERE api_key = ?`, apiKey)
	return scanWorker(row)
}

// ListWorkers returns all registered transcode workers.
func ListWorkers(ctx context.Context, db *sql.DB) ([]*models.TranscodeWorker, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, api_key, threads, hwaccel, status, last_heartbeat, created_at, updated_at
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

// DeleteWorker deletes a worker by ID.
func DeleteWorker(ctx context.Context, db *sql.DB, id uuid.UUID) error {
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

	err := row.Scan(&id, &name, &apiKey, &w.Threads, &w.HWAccel, &status, &lastHeartbeat, &createdAt, &updatedAt)
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

	err := rows.Scan(&id, &name, &apiKey, &w.Threads, &w.HWAccel, &status, &lastHeartbeat, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning worker row: %w", err)
	}

	w.ID, _ = uuid.Parse(id)
	w.Name = name
	w.APIKey = apiKey
	w.Status = status
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

// RecoverFailedWorkers finds workers whose last heartbeat is older than the timeout
// and updates their status to 'offline'. For any job currently 'processing' on those workers,
// it moves the job status back to 'pending', resets progress to 0, clears the worker ID,
// and sets the media item transcode status back to 'pending'.
// Returns the list of jobs that were re-queued so caller can broadcast status events.
func RecoverFailedWorkers(ctx context.Context, db *sql.DB, timeout time.Duration) ([]RequeuedJobInfo, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	threshold := time.Now().UTC().Add(-timeout).Format(time.RFC3339)

	// 1. Find all active workers that haven't sent a heartbeat within the timeout threshold
	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM transcode_workers
		WHERE status != 'offline' AND (last_heartbeat IS NULL OR last_heartbeat < ?)`,
		threshold,
	)
	if err != nil {
		return nil, fmt.Errorf("querying failed workers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var failedWorkerIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning failed worker: %w", err)
		}
		failedWorkerIDs = append(failedWorkerIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(failedWorkerIDs) == 0 {
		return nil, nil
	}

	// 2. Mark these workers as offline
	now := time.Now().UTC().Format(time.RFC3339)
	for _, wID := range failedWorkerIDs {
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

	// 3. Find all processing jobs associated with these workers along with their media item and library info
	var requeuedJobs []RequeuedJobInfo
	for _, wID := range failedWorkerIDs {
		jRows, err := tx.QueryContext(ctx, `
			SELECT j.id, j.media_item_id, m.library_id
			FROM transcode_jobs j
			JOIN media_items m ON j.media_item_id = m.id
			WHERE j.status = 'processing' AND j.worker_id = ?`,
			wID,
		)
		if err != nil {
			return nil, fmt.Errorf("querying processing jobs for worker: %w", err)
		}
		defer func() { _ = jRows.Close() }()

		for jRows.Next() {
			var jIDStr, mIDStr, lIDStr string
			if err := jRows.Scan(&jIDStr, &mIDStr, &lIDStr); err != nil {
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
		if err := jRows.Err(); err != nil {
			return nil, err
		}
	}

	// 4. Re-queue those jobs and update corresponding media items
	for _, job := range requeuedJobs {
		_, err := tx.ExecContext(ctx, `
			UPDATE transcode_jobs
			SET status = 'pending', worker_id = NULL, started_at = NULL, finished_at = NULL, error_msg = NULL, progress = 0
			WHERE id = ?`,
			job.JobID.String(),
		)
		if err != nil {
			return nil, fmt.Errorf("resetting job: %w", err)
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
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing failed worker recovery: %w", err)
	}

	return requeuedJobs, nil
}
