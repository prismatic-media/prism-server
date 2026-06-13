package handler

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/store/sqlite"
	"github.com/ringmaster217/prism/internal/transcoder"
	"github.com/ringmaster217/prism/pkg/events"
)

type WorkerHandler struct {
	db   *sql.DB
	pool *transcoder.Pool
	bus  *events.Bus
}

func NewWorkerHandler(db *sql.DB, pool *transcoder.Pool, bus *events.Bus) *WorkerHandler {
	return &WorkerHandler{db: db, pool: pool, bus: bus}
}

type contextKey string

const workerContextKey contextKey = "worker"

func WorkerFromContext(ctx context.Context) *models.TranscodeWorker {
	v, _ := ctx.Value(workerContextKey).(*models.TranscodeWorker)
	return v
}

// Authenticate is a middleware that validates the worker API Key from the X-Worker-API-Key header.
func (h *WorkerHandler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Worker-API-Key")
		if key == "" {
			respondError(w, http.StatusUnauthorized, "missing X-Worker-API-Key header")
			return
		}

		worker, err := sqlite.GetWorkerByAPIKey(r.Context(), h.db, key)
		if errors.Is(err, sqlite.ErrNotFound) {
			respondError(w, http.StatusUnauthorized, "invalid worker api key")
			return
		} else if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to validate worker key", err)
			return
		}

		ctx := context.WithValue(r.Context(), workerContextKey, worker)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Heartbeat receives worker heartbeats and returns the next claimed job (if capacity allows).
func (h *WorkerHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	worker := WorkerFromContext(r.Context())
	if worker == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Count active processing jobs for this worker
	var activeCount int
	err := h.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM transcode_jobs 
		WHERE worker_id = ? AND status = 'processing'`, 
		worker.ID.String(),
	).Scan(&activeCount)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count active jobs", err)
		return
	}

	status := "idle"
	if activeCount > 0 {
		status = "transcoding"
	}

	err = sqlite.UpdateWorkerHeartbeat(r.Context(), h.db, worker.ID, status)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update heartbeat", err)
		return
	}

	var claimedJob *models.TranscodeJob
	if activeCount < worker.Threads {
		// Claim next job
		claimedJob, err = sqlite.ClaimNextJob(r.Context(), h.db, &worker.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to claim job", err)
			return
		}

		if claimedJob != nil {
			// Update status of worker to transcoding
			_ = sqlite.UpdateWorkerHeartbeat(r.Context(), h.db, worker.ID, "transcoding")
			
			// Set media transcode status & publish events
			item, err := sqlite.GetMediaItemByID(r.Context(), h.db, claimedJob.MediaItemID)
			if err == nil {
				_ = sqlite.SetMediaTranscodeStatus(r.Context(), h.db, claimedJob.MediaItemID, models.TranscodeStatusProcessing)
				if h.bus != nil {
					h.bus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
						MediaItemID:     claimedJob.MediaItemID,
						LibraryID:       item.LibraryID,
						TranscodeStatus: string(models.TranscodeStatusProcessing),
					})
				}
			}
		}
	}

	type heartbeatResponse struct {
		Threads int                  `json:"threads"`
		HWAccel string               `json:"hwaccel"`
		Job     *models.TranscodeJob `json:"job"`
	}

	respondJSON(w, http.StatusOK, heartbeatResponse{
		Threads: worker.Threads,
		HWAccel: worker.HWAccel,
		Job:     claimedJob,
	})
}

// DownloadSource streams the source video file back to the worker.
func (h *WorkerHandler) DownloadSource(w http.ResponseWriter, r *http.Request) {
	if WorkerFromContext(r.Context()) == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	mediaID, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, mediaID)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get media item", err)
		return
	}

	http.ServeFile(w, r, item.FilePath)
}

type progressRequest struct {
	Progress float64 `json:"progress"`
	Status   string  `json:"status"` // "processing" or "failed"
	ErrorMsg string  `json:"error_msg"`
}

// UpdateProgress updates the transcode job progress and status reported by the worker.
func (h *WorkerHandler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	worker := WorkerFromContext(r.Context())
	if worker == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	jobID, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var req progressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	job, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, jobID)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch job", err)
		return
	}

	if job.WorkerID == nil || *job.WorkerID != worker.ID {
		respondError(w, http.StatusForbidden, "job not assigned to this worker")
		return
	}

	if req.Status == "failed" {
		errStr := req.ErrorMsg
		if errStr == "" {
			errStr = "remote worker transcode failed"
		}
		_ = sqlite.UpdateJobStatus(r.Context(), h.db, jobID, models.TranscodeStatusFailed, &errStr)
		_ = sqlite.SetMediaTranscodeStatus(r.Context(), h.db, job.MediaItemID, models.TranscodeStatusFailed)

		h.pool.Hub().Publish(transcoder.ProgressEvent{
			JobID:    jobID,
			Progress: 0,
			Done:     true,
			Error:    errStr,
		})

		if h.bus != nil {
			item, _ := sqlite.GetMediaItemByID(r.Context(), h.db, job.MediaItemID)
			var libraryID uuid.UUID
			if item != nil {
				libraryID = item.LibraryID
			}
			h.bus.Publish(events.EventJobProgress, events.JobProgressPayload{
				JobID:       jobID,
				MediaItemID: job.MediaItemID,
				Done:        true,
				Error:       errStr,
			})
			h.bus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
				MediaItemID:     job.MediaItemID,
				LibraryID:       libraryID,
				TranscodeStatus: string(models.TranscodeStatusFailed),
			})
		}
	} else {
		_ = sqlite.UpdateJobProgress(r.Context(), h.db, jobID, req.Progress)
		h.pool.Hub().Publish(transcoder.ProgressEvent{
			JobID:    jobID,
			Progress: req.Progress,
		})

		if h.bus != nil {
			h.bus.Publish(events.EventJobProgress, events.JobProgressPayload{
				JobID:       jobID,
				MediaItemID: job.MediaItemID,
				Progress:    req.Progress,
			})
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// UploadBundle receives a ZIP file of the transcoding outputs and extracts them.
func (h *WorkerHandler) UploadBundle(w http.ResponseWriter, r *http.Request) {
	worker := WorkerFromContext(r.Context())
	if worker == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	jobID, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	job, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, jobID)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch job", err)
		return
	}

	if job.WorkerID == nil || *job.WorkerID != worker.ID {
		respondError(w, http.StatusForbidden, "job not assigned to this worker")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, job.MediaItemID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get media item", err)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse multipart form", err)
		return
	}

	var file io.Reader
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			respondError(w, http.StatusBadRequest, "failed to read next part", err)
			return
		}
		if part.FormName() == "bundle" {
			file = part
			defer func() { _ = part.Close() }()
			break
		}
		_ = part.Close()
	}

	if file == nil {
		respondError(w, http.StatusBadRequest, "missing bundle file")
		return
	}

	outputDir, err := h.pool.SelectSegmentsOutputDir(r.Context(), job.MediaItemID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to select output path", err)
		return
	}

	tempFile, err := os.CreateTemp(filepath.Dir(outputDir), "transcode-bundle-*.zip")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create temp file", err)
		return
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	defer func() { _ = tempFile.Close() }()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to write zip file", err)
		return
	}

	_ = os.RemoveAll(outputDir)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create output dir", err)
		return
	}

	if err := unzipFile(tempFile.Name(), outputDir); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to extract transcode zip", err)
		return
	}

	mpdPath := filepath.Join(outputDir, "manifest.mpd")

	if err := sqlite.SetMediaMPDPath(r.Context(), h.db, job.MediaItemID, mpdPath); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to set mpd path", err)
		return
	}
	h.pool.MPDCache().Set(job.MediaItemID, mpdPath)

	// Write the artifact.json sidecar file for recovery.
	if err := transcoder.WriteSidecarForMediaItem(r.Context(), h.db, job.MediaItemID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to write artifact sidecar", err)
		return
	}

	now := time.Now().UTC()
	job.FinishedAt = &now
	_ = sqlite.UpdateJobStatus(r.Context(), h.db, job.ID, models.TranscodeStatusDone, nil)
	_ = sqlite.UpdateJobProgress(r.Context(), h.db, job.ID, 100)

	h.pool.Hub().Publish(transcoder.ProgressEvent{
		JobID:    job.ID,
		Progress: 100,
		Done:     true,
	})

	if h.bus != nil {
		h.bus.Publish(events.EventJobProgress, events.JobProgressPayload{
			JobID:       job.ID,
			MediaItemID: job.MediaItemID,
			Progress:    100,
			Done:        true,
		})
		h.bus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
			MediaItemID:     job.MediaItemID,
			LibraryID:       item.LibraryID,
			TranscodeStatus: string(models.TranscodeStatusDone),
		})
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func unzipFile(src string, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path (zip slip security violation)", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				return err
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			_ = outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		_ = outFile.Close()
		_ = rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}
