package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RequestContextWriter wraps a response writer to keep track of the request
type RequestContextWriter struct {
	http.ResponseWriter
	req *http.Request
}

// Unwrap returns the underlying ResponseWriter. This allows chi's middleware
// and other wrapper middlewares to unwrap and access the inner writer.
func (w *RequestContextWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// GetRequest returns the request associated with this writer.
func (w *RequestContextWriter) GetRequest() *http.Request {
	return w.req
}

// Hijack implements the http.Hijacker interface by forwarding the call
// to the underlying ResponseWriter if it supports hijacking.
func (w *RequestContextWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("websocket: response does not implement http.Hijacker")
}

// Flush implements the http.Flusher interface by forwarding the call
// to the underlying ResponseWriter if it supports flushing.
func (w *RequestContextWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// RequestContextMiddleware is a middleware that wraps the ResponseWriter
// to track the request context so it can be recovered in error responses.
func RequestContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&RequestContextWriter{ResponseWriter: w, req: r}, r)
	})
}

func getRequestFromWriter(w http.ResponseWriter) *http.Request {
	curr := w
	for curr != nil {
		type requestCarrier interface {
			GetRequest() *http.Request
		}
		if carrier, ok := curr.(requestCarrier); ok {
			return carrier.GetRequest()
		}
		if unwrapper, ok := curr.(interface{ Unwrap() http.ResponseWriter }); ok {
			curr = unwrapper.Unwrap()
		} else {
			break
		}
	}
	return nil
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding response", "error", err)
	}
}

func respondError(w http.ResponseWriter, status int, msg string, errs ...error) {
	var err error
	if len(errs) > 0 {
		err = errs[0]
	}

	logAttrs := []any{
		"status", status,
		"message", msg,
	}
	if err != nil {
		logAttrs = append(logAttrs, "error", err)
	}

	if r := getRequestFromWriter(w); r != nil {
		logAttrs = append(logAttrs,
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"remote_ip", r.RemoteAddr,
		)
		if reqID := r.Header.Get("X-Request-Id"); reqID != "" {
			logAttrs = append(logAttrs, "request_id", reqID)
		} else if reqID := chimw.GetReqID(r.Context()); reqID != "" {
			logAttrs = append(logAttrs, "request_id", reqID)
		}
	}

	if status >= 500 {
		logAttrs = append(logAttrs, "stack_trace", string(debug.Stack()))
		slog.Error("5xx response error", logAttrs...)
	} else if status >= 400 {
		slog.Warn("4xx response warning", logAttrs...)
	}
	respondJSON(w, status, map[string]string{"error": msg})
}
