// Package httpx holds the JSON request/response helpers shared by every
// feature's HTTP adapter. It is deliberately a leaf: apart from apperr — the
// error vocabulary the whole codebase already speaks — it depends on the
// standard library only, so every feature can import it without any risk of
// an import cycle (027-http-handler-decomposition, FR-003).
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/job-finder/api/internal/apperr"
)

// ErrorResponse is the wire shape of every error this API returns.
type ErrorResponse struct {
	Message string      `json:"message"`
	Code    apperr.Kind `json:"code,omitempty"`
	Details any         `json:"details,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response failed", "error", err)
	}
}

func WriteError(w http.ResponseWriter, status int, message string) {
	if status >= 500 {
		slog.Error("handler error", "status", status, "message", message)
	}
	WriteJSON(w, status, map[string]string{"message": message})
}

func WriteAppError(w http.ResponseWriter, err error) {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		status := apperr.HTTPStatusCode(ae.Kind)
		if status >= 500 {
			slog.Error("handler error", "kind", ae.Kind, "message", ae.Message, "error", ae.Unwrap())
		}
		WriteJSON(w, status, ErrorResponse{Message: ae.Message, Code: ae.Kind})
		return
	}
	slog.Error("handler error", "error", err)
	WriteJSON(w, http.StatusInternalServerError, ErrorResponse{Message: "internal server error", Code: apperr.KindInternal})
}

func DecodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}
