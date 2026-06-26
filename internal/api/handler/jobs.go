package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/internal/transcoder"
)

var wsUpgrader = websocket.Upgrader{
	HandshakeTimeout: 5 * time.Second,
	CheckOrigin:      func(r *http.Request) bool { return true }, // tightened via CORS at router level
	ReadBufferSize:   512,
	WriteBufferSize:  4096,
}

// JobsHandler serves transcode job REST endpoints and the progress WebSocket.
type JobsHandler struct {
	db   *sql.DB
	pool *transcoder.Pool
}

func NewJobsHandler(db *sql.DB, pool *transcoder.Pool) *JobsHandler {
	return &JobsHandler{db: db, pool: pool}
}

// CreateJobRequest is the payload for creating single or bulk transcode jobs.
type CreateJobRequest struct {
	MediaItemID *uuid.UUID `json:"media_item_id,omitempty"`
	Force       *bool      `json:"force,omitempty"`
	Filter      *string    `json:"filter,omitempty"`
}

// CreateJob handles POST /api/v1/jobs.
// @Summary Create Transcode Job(s) (Admin Only)
// @Description Creates a new transcode job for a specific media item, or bulk enqueues based on a filter.
// @Tags Transcoding Jobs
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CreateJobRequest true "Job creation parameters"
// @Success 202 {object} models.TranscodeJob "Returns the created transcode job (for single item)"
// @Success 200 {object} map[string]int "Returns number of jobs enqueued (for bulk filter): {'enqueued': N}"
// @Failure 400 {object} map[string]string "Invalid input or parameters"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 404 {object} map[string]string "Media item not found"
// @Router /jobs [post]
func (h *JobsHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// 1. Single Media Item Enqueue
	if req.MediaItemID != nil {
		mediaID := *req.MediaItemID
		// Verify the media item exists.
		if _, err := sqlite.GetMediaItemByID(r.Context(), h.db, mediaID); errors.Is(err, sqlite.ErrNotFound) {
			respondError(w, http.StatusNotFound, "media item not found", err)
			return
		} else if err != nil {
			respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
			return
		}

		force := false
		if req.Force != nil {
			force = *req.Force
		}
		job, err := h.pool.Enqueue(r.Context(), mediaID, force)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not enqueue transcode job", err)
			return
		}
		respondJSON(w, http.StatusAccepted, job)
		return
	}

	// 2. Bulk Filter Enqueue
	if req.Filter != nil {
		filter := strings.TrimSpace(*req.Filter)
		switch filter {
		case "untranscoded":
			n, err := sqlite.BulkEnqueueUntranscoded(r.Context(), h.db)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "could not bulk enqueue jobs", err)
				return
			}
			respondJSON(w, http.StatusOK, map[string]int{"enqueued": n})
		case "failed":
			n, err := sqlite.BulkEnqueueFailed(r.Context(), h.db)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "could not bulk enqueue jobs", err)
				return
			}
			respondJSON(w, http.StatusOK, map[string]int{"enqueued": n})
		case "completed":
			n, err := sqlite.BulkEnqueueCompleted(r.Context(), h.db)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "could not bulk enqueue jobs", err)
				return
			}
			respondJSON(w, http.StatusOK, map[string]int{"enqueued": n})
		default:
			respondError(w, http.StatusBadRequest, "unknown filter: "+filter)
		}
		return
	}

	respondError(w, http.StatusBadRequest, "either media_item_id or filter is required")
}

// ListJobs handles GET /api/v1/jobs.
// @Summary List Transcode Jobs (Admin Only)
// @Description Retrieve a list of all active/historical transcoding jobs.
// @Tags Transcoding Jobs
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.TranscodeJob
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Router /jobs [get]
func (h *JobsHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := sqlite.ListTranscodeJobs(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list jobs", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(jobs))
}

// GetJob handles GET /api/v1/jobs/{id}.
// @Summary Get Transcode Job Details (Admin Only)
// @Description Retrieve a single transcode job by ID.
// @Tags Transcoding Jobs
// @Security BearerAuth
// @Produce json
// @Param id path string true "Job ID" format(uuid)
// @Success 200 {object} models.TranscodeJob
// @Failure 400 {object} map[string]string "Invalid job ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 404 {object} map[string]string "Job not found"
// @Router /jobs/{id} [get]
func (h *JobsHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id", err)
		return
	}

	job, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch job", err)
		return
	}

	respondJSON(w, http.StatusOK, job)
}

// PrioritizeJob handles POST /api/v1/jobs/{id}/prioritize.
// @Summary Prioritize Job (Admin Only)
// @Description Move a pending job to the front of the transcoding queue.
// @Tags Transcoding Jobs
// @Security BearerAuth
// @Produce json
// @Param id path string true "Job ID" format(uuid)
// @Success 200 {object} map[string]string "Returns {'status': 'ok'}"
// @Failure 400 {object} map[string]string "Invalid job ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 404 {object} map[string]string "Job not found"
// @Failure 409 {object} map[string]string "Job is not pending (already processing or finished)"
// @Router /jobs/{id}:prioritize [post]
func (h *JobsHandler) PrioritizeJob(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id", err)
		return
	}

	err = sqlite.PrioritizeJob(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found", err)
		return
	}
	if errors.Is(err, sqlite.ErrJobNotPending) {
		respondError(w, http.StatusConflict, "job is not pending", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not prioritize job", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// JobProgress handles GET /api/v1/ws/jobs/{id} — upgrades to WebSocket and
// streams ProgressEvents until the job completes or the client disconnects.
func (h *JobsHandler) JobProgress(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id", err)
		return
	}

	// Verify job exists.
	job, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch job", err)
		return
	}

	// If job is already terminal, return current state over WS immediately.
	conn, wsErr := wsUpgrader.Upgrade(w, r, nil)
	if wsErr != nil {
		slog.Error("Failed to upgrade websocket for job progress", "error", wsErr)
		return
	}
	defer func() { _ = conn.Close() }()

	if job.Status == "done" || job.Status == "failed" {
		errStr := ""
		if job.ErrorMsg != nil {
			errStr = *job.ErrorMsg
		}
		evt := transcoder.ProgressEvent{
			JobID:    id,
			Progress: job.Progress,
			Done:     true,
			Error:    errStr,
			SubJobs:  job.SubJobs,
		}
		_ = conn.WriteJSON(evt)
		return
	}

	ch := h.pool.Hub().Subscribe(id)
	defer h.pool.Hub().Unsubscribe(id, ch)

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				slog.Info("job progress channel closed", "job_id", id)
				return
			}
			if err := conn.WriteJSON(evt); err != nil {
				slog.Error("failed to write job progress event", "job_id", id, "error", err)
				return
			}
			if evt.Done {
				slog.Info("job progress stream complete", "job_id", id)
				return
			}
		case <-r.Context().Done():
			slog.Info("job progress request context done", "job_id", id)
			return
		}
	}
}
