package domain

import "errors"

var (
	ErrProfileNotFound    = errors.New("profile not found")
	ErrNoProfileConfig    = errors.New("no profile configuration")
	ErrInvalidProfileID   = errors.New("invalid profile ID")
	ErrJobNotFound        = errors.New("job not found")
	ErrInvalidJobID       = errors.New("invalid job ID")
	ErrSearchNameRequired = errors.New("search name and keywords are required")
)
