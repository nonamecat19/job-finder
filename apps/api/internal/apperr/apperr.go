package apperr

import (
	"fmt"
	"net/http"
)

type Kind string

const (
	KindNotFound        Kind = "not_found"
	KindValidation      Kind = "validation"
	KindConflict        Kind = "conflict"
	KindUnauthorized    Kind = "unauthorized"
	KindForbidden       Kind = "forbidden"
	KindPrecondition    Kind = "precondition_failed"
	KindTooManyRequests Kind = "too_many_requests"
	KindUnavailable     Kind = "unavailable"
	KindInternal        Kind = "internal"
)

type Error struct {
	Kind    Kind
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

func Wrap(kind Kind, message string, err error) *Error {
	return &Error{Kind: kind, Message: message, Err: err}
}

func NotFound(entity, id string) *Error {
	return New(KindNotFound, fmt.Sprintf("%s %s not found", entity, id))
}

func Validation(message string) *Error {
	return New(KindValidation, message)
}

func Conflict(message string) *Error {
	return New(KindConflict, message)
}

func Precondition(message string) *Error {
	return New(KindPrecondition, message)
}

func Internal(message string) *Error {
	return New(KindInternal, message)
}

func HTTPStatusCode(k Kind) int {
	switch k {
	case KindNotFound:
		return http.StatusNotFound
	case KindValidation:
		return http.StatusBadRequest
	case KindConflict:
		return http.StatusConflict
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindPrecondition:
		return http.StatusPreconditionFailed
	case KindTooManyRequests:
		return http.StatusTooManyRequests
	case KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
