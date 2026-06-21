package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
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

type hijackFlushMockResponseWriter struct {
	http.ResponseWriter
	hijackCalled bool
	flushCalled  bool
}

func (m *hijackFlushMockResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	m.hijackCalled = true
	return nil, nil, nil
}

func (m *hijackFlushMockResponseWriter) Flush() {
	m.flushCalled = true
}

func TestRequestContextWriterHijackAndFlush(t *testing.T) {
	mock := &hijackFlushMockResponseWriter{}
	w := &RequestContextWriter{
		ResponseWriter: mock,
	}

	// Verify Hijack propagation
	if _, ok := interface{}(w).(http.Hijacker); !ok {
		t.Fatal("RequestContextWriter does not implement http.Hijacker")
	}
	_, _, err := w.Hijack()
	if err != nil {
		t.Errorf("unexpected error from Hijack: %v", err)
	}
	if !mock.hijackCalled {
		t.Error("expected Hijack to be called on underlying ResponseWriter")
	}

	// Verify Flush propagation
	if _, ok := interface{}(w).(http.Flusher); !ok {
		t.Fatal("RequestContextWriter does not implement http.Flusher")
	}
	w.Flush()
	if !mock.flushCalled {
		t.Error("expected Flush to be called on underlying ResponseWriter")
	}
}
