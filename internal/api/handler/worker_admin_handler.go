package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ringmaster217/prism/internal/store/sqlite"
)

type WorkerAdminHandler struct {
	db *sql.DB
}

func NewWorkerAdminHandler(db *sql.DB) *WorkerAdminHandler {
	return &WorkerAdminHandler{db: db}
}

type createWorkerRequest struct {
	Name string `json:"name"`
}

func (h *WorkerAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	workers, err := sqlite.ListWorkers(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workers", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(workers))
}

func (h *WorkerAdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createWorkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "worker name is required")
		return
	}

	worker, err := sqlite.CreateWorker(r.Context(), h.db, req.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create worker", err)
		return
	}

	respondJSON(w, http.StatusCreated, worker)
}

type updateWorkerRequest struct {
	Threads int    `json:"threads"`
	HWAccel string `json:"hwaccel"`
}

func (h *WorkerAdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid worker id")
		return
	}

	var req updateWorkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Threads <= 0 {
		respondError(w, http.StatusBadRequest, "threads must be greater than 0")
		return
	}

	err = sqlite.UpdateWorkerSettings(r.Context(), h.db, id, req.Threads, req.HWAccel)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "worker not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update worker settings", err)
		return
	}

	// Fetch updated worker to return
	worker, err := sqlite.GetWorkerByID(r.Context(), h.db, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch updated worker", err)
		return
	}

	respondJSON(w, http.StatusOK, worker)
}

func (h *WorkerAdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid worker id")
		return
	}

	err = sqlite.DeleteWorker(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "worker not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete worker", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
