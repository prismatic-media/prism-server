package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/scanner"
	"github.com/prismatic-media/prism-server/internal/storage"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

type StorageHandler struct {
	db      *sql.DB
	indexer *scanner.Indexer
}

func NewStorageHandler(db *sql.DB, indexer *scanner.Indexer) *StorageHandler {
	return &StorageHandler{db: db, indexer: indexer}
}

type storageAreaRequest struct {
	Kind    models.StorageAreaKind `json:"kind"`
	Path    string                 `json:"path"`
	Enabled *bool                  `json:"enabled"`
}

type storageConfigRequest struct {
	StorageMinFreeBytes string `json:"storage_min_free_bytes"`
}

type storageAreaResponse struct {
	ID               uuid.UUID               `json:"id"`
	Kind             models.StorageAreaKind  `json:"kind"`
	Path             string                  `json:"path"`
	Enabled          bool                    `json:"enabled"`
	TotalBytes       uint64                  `json:"total_bytes"`
	UsedBytes        uint64                  `json:"used_bytes"`
	FreeBytes        uint64                  `json:"free_bytes"`
	UtilizationPct   float64                 `json:"utilization_pct"`
	Status           string                  `json:"status"`
	Error            string                  `json:"error,omitempty"`
	EligibleSegments bool                    `json:"eligible_segments"`
}

type storageListResponse struct {
	StorageMinFreeBytes uint64                `json:"storage_min_free_bytes"`
	Areas               []storageAreaResponse `json:"areas"`
}

func (h *StorageHandler) ListStorage(w http.ResponseWriter, r *http.Request) {
	areas, err := sqlite.ListStorageAreas(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list storage areas", err)
		return
	}

	minFree := h.loadMinFreeBytes(r)
	resp := storageListResponse{StorageMinFreeBytes: minFree, Areas: make([]storageAreaResponse, 0, len(areas))}
	for _, area := range areas {
		considerReserve := area.Kind == models.StorageAreaKindSegments && area.Enabled
		m := storage.CollectPathMetrics(area.Path, minFree, considerReserve)
		if !area.Enabled {
			m.Status = storage.PathStatusDisabled
			m.EligibleSegment = false
		}
		resp.Areas = append(resp.Areas, storageAreaResponse{
			ID:               area.ID,
			Kind:             area.Kind,
			Path:             area.Path,
			Enabled:          area.Enabled,
			TotalBytes:       m.TotalBytes,
			UsedBytes:        m.UsedBytes,
			FreeBytes:        m.FreeBytes,
			UtilizationPct:   m.UtilizationPct,
			Status:           m.Status,
			Error:            m.Error,
			EligibleSegments: m.EligibleSegment,
		})
	}

	respondJSON(w, http.StatusOK, resp)
}

func (h *StorageHandler) CreateStorageArea(w http.ResponseWriter, r *http.Request) {
	var req storageAreaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validStorageKind(req.Kind) {
		respondError(w, http.StatusBadRequest, "invalid storage kind")
		return
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		respondError(w, http.StatusBadRequest, "path is required")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	area := &models.StorageArea{Kind: req.Kind, Path: path, Enabled: enabled}
	if err := sqlite.CreateStorageArea(r.Context(), h.db, area); err != nil {
		respondError(w, http.StatusBadRequest, "could not create storage area")
		return
	}

	if area.Kind == models.StorageAreaKindSegments && area.Enabled && h.indexer != nil {
		go func() {
			ctx := context.Background()
			_, _ = h.indexer.IndexStorageArea(ctx, area)
		}()
	}

	respondJSON(w, http.StatusCreated, area)
}

func (h *StorageHandler) UpdateStorageArea(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid storage area id")
		return
	}
	current, err := sqlite.GetStorageAreaByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "storage area not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load storage area", err)
		return
	}

	var req storageAreaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	path := strings.TrimSpace(req.Path)
	if path == "" {
		path = current.Path
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := sqlite.UpdateStorageArea(r.Context(), h.db, id, path, enabled); err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			respondError(w, http.StatusNotFound, "storage area not found")
			return
		}
		respondError(w, http.StatusBadRequest, "could not update storage area")
		return
	}

	updated, err := sqlite.GetStorageAreaByID(r.Context(), h.db, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load updated storage area", err)
		return
	}

	if updated.Kind == models.StorageAreaKindSegments && updated.Enabled && h.indexer != nil && (!current.Enabled || current.Path != updated.Path) {
		go func() {
			ctx := context.Background()
			_, _ = h.indexer.IndexStorageArea(ctx, updated)
		}()
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *StorageHandler) DeleteStorageArea(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid storage area id")
		return
	}

	_, err = h.db.ExecContext(r.Context(), "DELETE FROM storage_areas WHERE id = ?", id.String())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete storage area", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *StorageHandler) UpdateStorageConfig(w http.ResponseWriter, r *http.Request) {
	var req storageConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := strconv.ParseUint(strings.TrimSpace(req.StorageMinFreeBytes), 10, 64); err != nil {
		respondError(w, http.StatusBadRequest, "storage_min_free_bytes must be an unsigned integer")
		return
	}
	if err := sqlite.SetSetting(r.Context(), h.db, "storage_min_free_bytes", strings.TrimSpace(req.StorageMinFreeBytes)); err != nil {
		respondError(w, http.StatusInternalServerError, "could not save storage config", err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *StorageHandler) loadMinFreeBytes(r *http.Request) uint64 {
	const defaultMinFree uint64 = 20 * 1024 * 1024 * 1024
	raw, err := sqlite.GetSetting(r.Context(), h.db, "storage_min_free_bytes")
	if err != nil {
		return defaultMinFree
	}
	n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return defaultMinFree
	}
	return n
}

func validStorageKind(kind models.StorageAreaKind) bool {
	return kind == models.StorageAreaKindSegments
}
