package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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

// EnqueueTranscode handles POST /api/v1/media/{id}/transcode.
// @Summary Enqueue Transcode Job (Admin Only)
// @Description Adds a media item to the FFmpeg transcoding queue to produce adaptive bitrate DASH stream formats.
// @Tags Transcoding Jobs
// @Security BearerAuth
// @Produce json
// @Param id path string true "Media Item ID" format(uuid)
// @Success 202 {object} models.TranscodeJob "Job enqueued"
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 404 {object} map[string]string "Media item not found"
// @Router /media/{id}/transcode [post]
func (h *JobsHandler) EnqueueTranscode(w http.ResponseWriter, r *http.Request) {
	mediaID, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	// Verify the media item exists.
	if _, err := sqlite.GetMediaItemByID(r.Context(), h.db, mediaID); errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	force := r.URL.Query().Get("force") == "true"
	job, err := h.pool.Enqueue(r.Context(), mediaID, force)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not enqueue transcode job", err)
		return
	}

	respondJSON(w, http.StatusAccepted, job)
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
		respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	job, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch job", err)
		return
	}

	respondJSON(w, http.StatusOK, job)
}

type bulkEnqueueRequest struct {
	Filter string `json:"filter"`
}

// BulkEnqueueJobs handles POST /api/v1/jobs/bulk-enqueue.
// @Summary Bulk Enqueue Jobs (Admin Only)
// @Description Add multiple media items to the queue based on a filter ("untranscoded" or "failed").
// @Tags Transcoding Jobs
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body bulkEnqueueRequest true "Filter specification"
// @Success 200 {object} map[string]int "Returns number of jobs enqueued: {'enqueued': N}"
// @Failure 400 {object} map[string]string "Invalid request body or unknown filter"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Router /jobs/bulk-enqueue [post]
func (h *JobsHandler) BulkEnqueueJobs(w http.ResponseWriter, r *http.Request) {
	var req bulkEnqueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	switch strings.TrimSpace(req.Filter) {
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
		respondError(w, http.StatusBadRequest, "unknown filter")
	}
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
// @Router /jobs/{id}/prioritize [post]
func (h *JobsHandler) PrioritizeJob(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	err = sqlite.PrioritizeJob(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found")
		return
	}
	if errors.Is(err, sqlite.ErrJobNotPending) {
		respondError(w, http.StatusConflict, "job is not pending")
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
		respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	// Verify job exists.
	job, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch job", err)
		return
	}

	// If job is already terminal, return current state over WS immediately.
	conn, wsErr := wsUpgrader.Upgrade(w, r, nil)
	if wsErr != nil {
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
				return
			}
			if err := conn.WriteJSON(evt); err != nil {
				return
			}
			if evt.Done {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}
