package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/job-finder/api/internal/apperr"
)

type ErrorResponse struct {
	Message string      `json:"message"`
	Code    apperr.Kind `json:"code,omitempty"`
	Details any         `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response failed", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	if status >= 500 {
		slog.Error("handler error", "status", status, "message", message)
	}
	writeJSON(w, status, map[string]string{"message": message})
}

func writeAppError(w http.ResponseWriter, err error) {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		status := apperr.HTTPStatusCode(ae.Kind)
		if status >= 500 {
			slog.Error("handler error", "kind", ae.Kind, "message", ae.Message, "error", ae.Unwrap())
		}
		writeJSON(w, status, ErrorResponse{Message: ae.Message, Code: ae.Kind})
		return
	}
	slog.Error("handler error", "error", err)
	writeJSON(w, http.StatusInternalServerError, ErrorResponse{Message: "internal server error", Code: apperr.KindInternal})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func reencode[T any](v any) T {
	var out T
	b, err := json.Marshal(v)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(b, &out)
	return out
}
