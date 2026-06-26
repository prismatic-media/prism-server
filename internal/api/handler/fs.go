package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FsHandler provides filesystem utilities for admin use (e.g. path autocomplete).
type FsHandler struct{}

func NewFsHandler() *FsHandler { return &FsHandler{} }

// BrowseDir handles GET /api/v1/fs/browse?path=...
//
// Returns the immediate subdirectories that match the partial path so the
// frontend can populate a path-autocomplete dropdown. Hidden directories
// (names starting with ".") are omitted. Only accessible to admins.
// @Summary Browse Directories
// @Description Browse immediate subdirectories matching a partial path for autocomplete in the setup/admin panel.
// @Tags Admin Configuration
// @Security BearerAuth
// @Produce json
// @Param path query string false "Partial absolute path"
// @Success 200 {object} map[string][]string "Returns list of matched absolute directory paths: {'dirs': [...]}"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Router /fs:browse [get]
func (h *FsHandler) BrowseDir(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")

	var dir, prefix string
	if raw == "" {
		dir = "/"
	} else {
		// Normalise to an absolute path so the caller cannot accidentally
		// request a relative path.
		clean := filepath.Clean("/" + strings.TrimPrefix(filepath.ToSlash(raw), "/"))
		if strings.HasSuffix(raw, "/") || raw == "/" {
			// Trailing slash means "list the contents of this directory".
			dir = clean
		} else {
			// No trailing slash: list the parent dir, filtering by the
			// last component as a prefix.
			dir = filepath.Dir(clean)
			prefix = strings.ToLower(filepath.Base(clean))
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		// Return an empty list — the caller handles this gracefully.
		respondJSON(w, http.StatusOK, map[string][]string{"dirs": {}})
		return
	}

	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue // skip hidden directories
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(e.Name()), prefix) {
			continue
		}
		dirs = append(dirs, filepath.Join(dir, e.Name()))
	}
	sort.Strings(dirs)
	respondJSON(w, http.StatusOK, map[string][]string{"dirs": dirs})
}
