package apperr

import (
	"errors"
	"net/http"
	"testing"
)

func TestError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{"simple", New(KindNotFound, "user not found"), "not_found: user not found"},
		{"wrapped", Wrap(KindInternal, "db error", errors.New("connection refused")), "internal: db error: connection refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	inner := errors.New("inner")
	err := Wrap(KindInternal, "outer", inner)
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestHTTPStatusCode(t *testing.T) {
	tests := []struct {
		kind Kind
		want int
	}{
		{KindNotFound, http.StatusNotFound},
		{KindValidation, http.StatusBadRequest},
		{KindConflict, http.StatusConflict},
		{KindUnauthorized, http.StatusUnauthorized},
		{KindForbidden, http.StatusForbidden},
		{KindPrecondition, http.StatusPreconditionFailed},
		{KindTooManyRequests, http.StatusTooManyRequests},
		{KindInternal, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if got := HTTPStatusCode(tt.kind); got != tt.want {
				t.Errorf("HTTPStatusCode(%q) = %d, want %d", tt.kind, got, tt.want)
			}
		})
	}
}

func TestConstructorHelpers(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		kind Kind
	}{
		{"NotFound", NotFound("user", "123"), KindNotFound},
		{"Validation", Validation("invalid email"), KindValidation},
		{"Conflict", Conflict("duplicate"), KindConflict},
		{"Precondition", Precondition("no config"), KindPrecondition},
		{"Internal", Internal("boom"), KindInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Kind != tt.kind {
				t.Errorf("Kind = %q, want %q", tt.err.Kind, tt.kind)
			}
		})
	}
}
