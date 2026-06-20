package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestContextMiddlewareAndRespondError(t *testing.T) {
	testErr := errors.New("database connection failed")

	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that getRequestFromWriter can extract the request
		extractedReq := getRequestFromWriter(w)
		if extractedReq == nil {
			t.Error("expected to retrieve request from response writer, got nil")
		} else if extractedReq.URL.Path != "/test-path" {
			t.Errorf("expected path /test-path, got %s", extractedReq.URL.Path)
		}

		// Call respondError to trigger 5xx logging logic
		respondError(w, http.StatusInternalServerError, "internal server failure", testErr)
	})

	// Set up middleware chain
	mw := RequestContextMiddleware(handlerFunc)

	req := httptest.NewRequest("GET", "/test-path?param=value", nil)
	req.Header.Set("X-Request-Id", "test-req-123")
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	var respBody map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}

	if respBody["error"] != "internal server failure" {
		t.Errorf("expected error message 'internal server failure', got '%s'", respBody["error"])
	}
}
