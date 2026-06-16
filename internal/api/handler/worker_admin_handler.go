package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/prismatic-media/prism-server/internal/store/sqlite"
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

// @Summary List Transcode Workers
// @Description Retrieve a list of all registered remote transcode workers.
// @Tags Worker Administration
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.TranscodeWorker
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/workers [get]
func (h *WorkerAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	workers, err := sqlite.ListWorkers(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workers", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(workers))
}

// @Summary Register Transcode Worker
// @Description Register a new remote transcode worker. Returns the created worker metadata including its authentication secret key.
// @Tags Worker Administration
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createWorkerRequest true "Worker parameters"
// @Success 201 {object} models.TranscodeWorker
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/workers [post]
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

// @Summary Update Transcode Worker Settings
// @Description Update concurrency limits (threads) or hardware acceleration configuration for a registered worker.
// @Tags Worker Administration
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Worker ID" format(uuid)
// @Param body body updateWorkerRequest true "Worker settings"
// @Success 200 {object} models.TranscodeWorker
// @Failure 400 {object} map[string]string "Invalid ID or request body"
// @Failure 404 {object} map[string]string "Worker not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/workers/{id} [put]
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

// @Summary Delete Transcode Worker
// @Description De-register a remote transcode worker and invalidate its API credentials.
// @Tags Worker Administration
// @Security BearerAuth
// @Param id path string true "Worker ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string "Invalid ID"
// @Failure 404 {object} map[string]string "Worker not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/workers/{id} [delete]
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

type createEphemeralTokenRequest struct {
	Name string `json:"name"`
}

// @Summary List Ephemeral Worker Tokens
// @Description Retrieve a list of all active ephemeral worker registration tokens.
// @Tags Worker Administration
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.EphemeralWorkerToken
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/workers/ephemeral-tokens [get]
func (h *WorkerAdminHandler) ListEphemeralTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := sqlite.ListEphemeralWorkerTokens(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list ephemeral tokens", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(tokens))
}

// @Summary Create Ephemeral Worker Token
// @Description Create a new re-usable registration token for ephemeral workers.
// @Tags Worker Administration
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createEphemeralTokenRequest true "Token parameters"
// @Success 201 {object} models.EphemeralWorkerToken
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/workers/ephemeral-tokens [post]
func (h *WorkerAdminHandler) CreateEphemeralToken(w http.ResponseWriter, r *http.Request) {
	var req createEphemeralTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "token name is required")
		return
	}

	token, err := sqlite.CreateEphemeralWorkerToken(r.Context(), h.db, req.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create ephemeral token", err)
		return
	}

	respondJSON(w, http.StatusCreated, token)
}

// @Summary Delete Ephemeral Worker Token
// @Description Revoke/delete an ephemeral worker registration token.
// @Tags Worker Administration
// @Security BearerAuth
// @Param id path string true "Token ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string "Invalid ID"
// @Failure 404 {object} map[string]string "Token not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/workers/ephemeral-tokens/{id} [delete]
func (h *WorkerAdminHandler) DeleteEphemeralToken(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid token id")
		return
	}

	err = sqlite.DeleteEphemeralWorkerToken(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "token not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete token", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

