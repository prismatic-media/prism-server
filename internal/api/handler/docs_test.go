package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prismatic-media/prism-server/internal/api/handler"
)

func TestDocsHandler(t *testing.T) {
	h := handler.NewDocsHandler()

	t.Run("ServeDocsHTML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/docs", nil)
		rec := httptest.NewRecorder()
		h.ServeDocsHTML(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		contentType := rec.Header().Get("Content-Type")
		if !strings.Contains(contentType, "text/html") {
			t.Errorf("content-type = %q, want text/html", contentType)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "<title>Prism API Documentation</title>") {
			t.Error("body does not contain expected HTML title")
		}
	})

	t.Run("ServeSwaggerYAML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/swagger.yaml", nil)
		rec := httptest.NewRecorder()
		h.ServeSwaggerYAML(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		contentType := rec.Header().Get("Content-Type")
		if !strings.Contains(contentType, "application/x-yaml") {
			t.Errorf("content-type = %q, want application/x-yaml", contentType)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "swagger:") {
			t.Error("body does not contain expected swagger version indicator")
		}
	})
}
