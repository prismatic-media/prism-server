package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
)

func TestTimeout_AppliesTimeout(t *testing.T) {
	var gotErr error
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			gotErr = r.Context().Err()
			w.WriteHeader(http.StatusGatewayTimeout)
		case <-time.After(50 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	})

	// Wrap handler with a 10ms timeout
	mw := apimw.Timeout(10 * time.Millisecond)(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media", nil)

	mw.ServeHTTP(rec, req)

	if gotErr != context.DeadlineExceeded {
		t.Errorf("expected context deadline exceeded, got: %v", gotErr)
	}
	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("expected status 504, got: %d", rec.Code)
	}
}

func TestTimeout_SkipsWebSocket(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			w.WriteHeader(http.StatusGatewayTimeout)
		case <-time.After(20 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	})

	// Wrap handler with a 5ms timeout, but send a WebSocket upgrade request
	mw := apimw.Timeout(5 * time.Millisecond)(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws/events", nil)
	req.Header.Set("Upgrade", "websocket")

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 (timeout skipped for WebSocket), got: %d", rec.Code)
	}
}

func TestTimeout_SkipsSourceDownload(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			w.WriteHeader(http.StatusGatewayTimeout)
		case <-time.After(20 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	})

	// Wrap handler with a 5ms timeout, but call the /source endpoint
	mw := apimw.Timeout(5 * time.Millisecond)(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/123/source", nil)

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 (timeout skipped for source download), got: %d", rec.Code)
	}
}
