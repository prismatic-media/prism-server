package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
	"github.com/ringmaster217/galactic-media-server/internal/transcoder"
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
		respondError(w, http.StatusInternalServerError, "could not fetch media item")
		return
	}

	job, err := h.pool.Enqueue(r.Context(), mediaID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not enqueue transcode job")
		return
	}

	respondJSON(w, http.StatusAccepted, job)
}

// ListJobs handles GET /api/v1/jobs.
func (h *JobsHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := sqlite.ListTranscodeJobs(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list jobs")
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
		respondError(w, http.StatusInternalServerError, "could not fetch job")
		return
	}

	respondJSON(w, http.StatusOK, job)
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
		respondError(w, http.StatusInternalServerError, "could not fetch job")
		return
	}

	// If job is already terminal, return current state over WS immediately.
	conn, wsErr := wsUpgrader.Upgrade(w, r, nil)
	if wsErr != nil {
		return
	}
	defer conn.Close()

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
