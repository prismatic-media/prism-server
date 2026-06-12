package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding response", "error", err)
	}
}

func respondError(w http.ResponseWriter, status int, msg string, errs ...error) {
	if status >= 500 {
		var err error
		if len(errs) > 0 {
			err = errs[0]
		}
		if err != nil {
			slog.Error("5xx response error", "status", status, "message", msg, "error", err)
		} else {
			slog.Error("5xx response error", "status", status, "message", msg)
		}
	}
	respondJSON(w, status, map[string]string{"error": msg})
}
