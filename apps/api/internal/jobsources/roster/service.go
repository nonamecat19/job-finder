package roster

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

// StaleAfterConsecutiveEmptyRuns is how many consecutive zero-posting runs
// flag an employer stale (FR-014) — a single system constant, since no
// acceptance scenario asks for a per-employer override.
const StaleAfterConsecutiveEmptyRuns = 5

// MaxEmployersPerRun and MaxPostingsPerEmployer bound a single run's work
// (FR-007). Centralized here — not duplicated per adapter — so every board
// vendor adapter enforces the same cap by calling ListForRun/CapPostings
// instead of re-deriving its own limit (keeps FR-005's "add a vendor without
// changing shared behavior" true in practice).
const (
	MaxEmployersPerRun     = 200
	MaxPostingsPerEmployer = 500
)

// EmployerHealthChecker performs one live read against a vendor's board for
// a single employer, used to validate a pasted URL before it joins the
// roster (FR-011). Returns the number of open postings found.
type EmployerHealthChecker func(ctx context.Context, employerIdentifier string) (postingCount int, err error)

// UnsupportedVendorError is returned when a pasted URL doesn't match any
// supported board vendor (Edge Case: "reject with a message naming the
// supported vendors").
type UnsupportedVendorError struct{ URL string }

func (e *UnsupportedVendorError) Error() string {
	return fmt.Sprintf("unsupported board vendor for url %q; supported vendors: %v", e.URL, SupportedVendors)
}

// UnreadableError wraps a health-check failure for a board that doesn't
// respond with a valid posting list (spec Acceptance Scenario 5 under US2).
type UnreadableError struct {
	Vendor, EmployerIdentifier string
	Cause                      error
}

func (e *UnreadableError) Error() string {
	return fmt.Sprintf("board %s/%s did not respond with a valid posting list: %v", e.Vendor, e.EmployerIdentifier, e.Cause)
}
func (e *UnreadableError) Unwrap() error { return e.Cause }

// Service manages the employer roster: registration, removal, and
// per-run staleness bookkeeping. Candidate discovery lives in candidates.go.
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

// ListForRun returns the enabled roster employers for one vendor, capped at
// MaxEmployersPerRun (FR-007) — the single place every board adapter's
// Search should source its fan-out list from.
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

// RegisterFromURL validates and registers a pasted board URL (FR-011):
// resolves the vendor, rejects unsupported ones by name, performs a live
// health-check read before insert, and marks how it was added.
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

// Remove deletes an employer from the roster; ingested Job rows are
// untouched (FR-012) since Job carries no FK to EmployerBoard.
func (s *Service) Remove(ctx context.Context, id string) error {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.q.DeleteEmployerBoard(ctx, uid)
}

// RecordRunOutcome updates an employer's last-success bookkeeping after one
// run; postingCount == 0 increments the stale counter, > 0 resets it
// (FR-014).
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

// Stale reports whether an employer's consecutive empty-run count has
// reached the threshold (FR-014) — computed at read time, not stored
// redundantly.
func Stale(e sqlcgen.EmployerBoard) bool {
	return e.ConsecutiveEmptyRuns >= StaleAfterConsecutiveEmptyRuns
}

// getByVendorAndEmployer is used by Accept to check for an existing row
// before insert.
func (s *Service) getByVendorAndEmployer(ctx context.Context, vendor, employerIdentifier string) (sqlcgen.EmployerBoard, error) {
	board, err := s.q.GetEmployerBoard(ctx, sqlcgen.GetEmployerBoardParams{Vendor: vendor, EmployerIdentifier: employerIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlcgen.EmployerBoard{}, nil
	}
	return board, err
}
