package roster

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

const StaleAfterConsecutiveEmptyRuns = 5

const (
	MaxEmployersPerRun     = 200
	MaxPostingsPerEmployer = 500
)

type EmployerHealthChecker func(ctx context.Context, employerIdentifier string) (postingCount int, err error)

type UnsupportedVendorError struct{ URL string }

func (e *UnsupportedVendorError) Error() string {
	return fmt.Sprintf("unsupported board vendor for url %q; supported vendors: %v", e.URL, SupportedVendors)
}

type UnreadableError struct {
	Vendor, EmployerIdentifier string
	Cause                      error
}

func (e *UnreadableError) Error() string {
	return fmt.Sprintf("board %s/%s did not respond with a valid posting list: %v", e.Vendor, e.EmployerIdentifier, e.Cause)
}
func (e *UnreadableError) Unwrap() error { return e.Cause }

type Service struct {
	q        Repository
	checkers map[string]EmployerHealthChecker
}

func NewService(q Repository, checkers map[string]EmployerHealthChecker) *Service {
	return &Service{q: q, checkers: checkers}
}

func (s *Service) List(ctx context.Context) ([]sqlcgen.EmployerBoard, error) {
	return s.q.ListEmployerBoards(ctx)
}

func (s *Service) ListForRun(ctx context.Context, vendor string) ([]sqlcgen.EmployerBoard, error) {
	all, err := s.q.ListEmployerBoardsByVendor(ctx, vendor)
	if err != nil {
		return nil, err
	}
	if len(all) > MaxEmployersPerRun {
		all = all[:MaxEmployersPerRun]
	}
	return all, nil
}

func (s *Service) RegisterFromURL(ctx context.Context, rawURL, displayName string) (sqlcgen.EmployerBoard, error) {
	vendor, employerIdentifier, ok := MatchVendor(rawURL)
	if !ok {
		return sqlcgen.EmployerBoard{}, &UnsupportedVendorError{URL: rawURL}
	}
	return s.register(ctx, vendor, employerIdentifier, displayName, "pasted")
}

func (s *Service) register(ctx context.Context, vendor, employerIdentifier, displayName, addedVia string) (sqlcgen.EmployerBoard, error) {
	if displayName == "" {
		displayName = employerIdentifier
	}
	checker, ok := s.checkers[vendor]
	if ok {
		if _, err := checker(ctx, employerIdentifier); err != nil {
			return sqlcgen.EmployerBoard{}, &UnreadableError{Vendor: vendor, EmployerIdentifier: employerIdentifier, Cause: err}
		}
	}
	return s.q.InsertEmployerBoard(ctx, sqlcgen.InsertEmployerBoardParams{
		Vendor:             vendor,
		EmployerIdentifier: employerIdentifier,
		DisplayName:        displayName,
		AddedVia:           addedVia,
	})
}

func (s *Service) Remove(ctx context.Context, id string) error {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.q.DeleteEmployerBoard(ctx, uid)
}

func (s *Service) RecordRunOutcome(ctx context.Context, employerID string, postingCount int) error {
	uid, err := dbutil.ParseUUID(employerID)
	if err != nil {
		return err
	}
	return s.q.RecordEmployerBoardRunOutcome(ctx, sqlcgen.RecordEmployerBoardRunOutcomeParams{
		ID:               uid,
		LastPostingCount: int32(postingCount),
	})
}

func Stale(e sqlcgen.EmployerBoard) bool {
	return e.ConsecutiveEmptyRuns >= StaleAfterConsecutiveEmptyRuns
}

func (s *Service) getByVendorAndEmployer(ctx context.Context, vendor, employerIdentifier string) (sqlcgen.EmployerBoard, error) {
	board, err := s.q.GetEmployerBoard(ctx, sqlcgen.GetEmployerBoardParams{Vendor: vendor, EmployerIdentifier: employerIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlcgen.EmployerBoard{}, nil
	}
	return board, err
}
