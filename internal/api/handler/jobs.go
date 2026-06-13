package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ringmaster217/prism/internal/store/sqlite"
	"github.com/ringmaster217/prism/internal/transcoder"
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

	job, err := h.pool.Enqueue(r.Context(), mediaID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not enqueue transcode job", err)
		return
	}

	respondJSON(w, http.StatusAccepted, job)
}

// ListJobs handles GET /api/v1/jobs.
func (h *JobsHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := sqlite.ListTranscodeJobs(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list jobs", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(jobs))
}

// GetJob handles GET /api/v1/jobs/{id}.
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
	default:
		respondError(w, http.StatusBadRequest, "unknown filter")
	}
}

// PrioritizeJob handles POST /api/v1/jobs/{id}/prioritize.
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
