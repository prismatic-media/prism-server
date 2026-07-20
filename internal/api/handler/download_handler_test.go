package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/api/handler"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func newTestDownloadRouter(t *testing.T, h *handler.DownloadHandler) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.Get("/api/v1/stream/{media_id}/renditions/{quality}", h.ListRenditionSegments)
	})
	return r
}

func TestListRenditionSegments(t *testing.T) {
	db := openTestDB(t)
	downloadH := handler.NewDownloadHandler(db)
	router := newTestDownloadRouter(t, downloadH)

	admin := createUser(t, db, "admin", "admin@prism.io", "adminpass", true)
	token := bearerToken(t, admin.ID, true)
	headers := adminHeader(token)

	// Create a dummy library first
	lib := &models.Library{
		Path:      t.TempDir(),
		MediaType: models.MediaTypeMovie,
	}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatalf("failed to create library: %v", err)
	}

	t.Run("MediaNotExists", func(t *testing.T) {
		fakeID := uuid.New().String()
		rec := do(t, router, http.MethodGet, "/api/v1/stream/"+fakeID+"/renditions/720p", nil, headers)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("TranscodePending", func(t *testing.T) {
		mediaItem := &models.MediaItem{
			LibraryID:       lib.ID,
			Title:           "Pending Video",
			MediaType:       models.MediaTypeMovie,
			FilePath:        "pending.mp4",
			SourceStatus:    "available",
			TranscodeStatus: models.TranscodeStatusPending,
		}
		if err := sqlite.UpsertMediaItem(context.Background(), db, mediaItem); err != nil {
			t.Fatalf("failed to create media item: %v", err)
		}

		rec := do(t, router, http.MethodGet, "/api/v1/stream/"+mediaItem.ID.String()+"/renditions/720p", nil, headers)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("RenditionQualityNotFound", func(t *testing.T) {
		tempDir := t.TempDir()
		mpdPath := filepath.Join(tempDir, "manifest.mpd")
		_ = os.WriteFile(mpdPath, []byte("<MPD></MPD>"), 0644)

		mediaItem := &models.MediaItem{
			LibraryID:       lib.ID,
			Title:           "Transcoded Movie",
			MediaType:       models.MediaTypeMovie,
			FilePath:        "movie.mp4",
			SourceStatus:    "available",
			TranscodeStatus: models.TranscodeStatusDone,
			MPDPath:         &mpdPath,
		}
		if err := sqlite.UpsertMediaItem(context.Background(), db, mediaItem); err != nil {
			t.Fatalf("failed to create media item: %v", err)
		}

		rec := do(t, router, http.MethodGet, "/api/v1/stream/"+mediaItem.ID.String()+"/renditions/1080p", nil, headers)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Success", func(t *testing.T) {
		tempDir := t.TempDir()
		mpdPath := filepath.Join(tempDir, "manifest.mpd")
		_ = os.WriteFile(mpdPath, []byte("<MPD></MPD>"), 0644)

		// Create rendition folder
		qualityDir := filepath.Join(tempDir, "720p")
		if err := os.MkdirAll(qualityDir, 0755); err != nil {
			t.Fatalf("failed to create quality dir: %v", err)
		}

		// Write dummy segments
		_ = os.WriteFile(filepath.Join(qualityDir, "init.mp4"), []byte("init content"), 0644)
		_ = os.WriteFile(filepath.Join(qualityDir, "seg_00001.m4s"), []byte("seg 1 content"), 0644)

		// Write dummy subtitle
		_ = os.WriteFile(filepath.Join(tempDir, "sub_eng.vtt"), []byte("vtt content"), 0644)

		mediaItem := &models.MediaItem{
			LibraryID:       lib.ID,
			Title:           "Success Movie",
			MediaType:       models.MediaTypeMovie,
			FilePath:        "success_movie.mp4",
			SourceStatus:    "available",
			TranscodeStatus: models.TranscodeStatusDone,
			MPDPath:         &mpdPath,
		}
		if err := sqlite.UpsertMediaItem(context.Background(), db, mediaItem); err != nil {
			t.Fatalf("failed to create media item: %v", err)
		}

		// Insert subtitle in DB to verify mapping
		sub := &models.MediaSubtitle{
			MediaItemID: mediaItem.ID,
			Language:    "eng",
			Label:       "English (SDH)",
			VTTContent:  "vtt content",
		}
		if err := sqlite.AddMediaSubtitle(context.Background(), db, sub); err != nil {
			t.Fatalf("failed to add media subtitle: %v", err)
		}

		rec := do(t, router, http.MethodGet, "/api/v1/stream/"+mediaItem.ID.String()+"/renditions/720p", nil, headers)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp handler.RenditionListingResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp.Segments) != 2 {
			t.Fatalf("expected 2 segments, got %d", len(resp.Segments))
		}
		if len(resp.Subtitles) != 1 {
			t.Fatalf("expected 1 subtitle, got %d", len(resp.Subtitles))
		}
		if resp.Subtitles[0].Label != "English" {
			t.Fatalf("expected label English, got %s", resp.Subtitles[0].Label)
		}
	})

	t.Run("NoSubtitles_SerializedAsEmptyArray", func(t *testing.T) {
		tempDir := t.TempDir()
		mpdPath := filepath.Join(tempDir, "manifest.mpd")
		_ = os.WriteFile(mpdPath, []byte("<MPD></MPD>"), 0644)

		qualityDir := filepath.Join(tempDir, "720p")
		_ = os.MkdirAll(qualityDir, 0755)
		_ = os.WriteFile(filepath.Join(qualityDir, "init.mp4"), []byte("init content"), 0644)

		mediaItem := &models.MediaItem{
			LibraryID:       lib.ID,
			Title:           "No Subs Movie",
			MediaType:       models.MediaTypeMovie,
			FilePath:        "no_subs.mp4",
			SourceStatus:    "available",
			TranscodeStatus: models.TranscodeStatusDone,
			MPDPath:         &mpdPath,
		}
		if err := sqlite.UpsertMediaItem(context.Background(), db, mediaItem); err != nil {
			t.Fatalf("failed to create media item: %v", err)
		}

		rec := do(t, router, http.MethodGet, "/api/v1/stream/"+mediaItem.ID.String()+"/renditions/720p", nil, headers)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		bodyStr := rec.Body.String()
		if !strings.Contains(bodyStr, `"subtitles":[]`) {
			t.Fatalf("expected JSON body to contain '\"subtitles\":[]', got: %s", bodyStr)
		}
	})
}
