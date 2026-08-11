package roster

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nonamecat19/job-scraper/ports"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

const StaleAfterConsecutiveEmptyRuns = 5

// MaxEmployersPerRun caps how many boards one vendor run walks. The companion
// cap on postings per employer lives library-side as adapters.MaxPostingsPerEmployer,
// where the board adapters that enforce it now are.
const MaxEmployersPerRun = 200

// EmployerHealthChecker is the library's checker signature; the app aliases it
// so wiring can pass the map adapters.NewBoardAdapters returns straight through.
type EmployerHealthChecker = ports.EmployerHealthChecker

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

// Service is the app-side implementation of the library's RosterPort: it owns
// the sqlcgen row shapes and the UUID conversions, so the board adapters only
// ever see plain-string IDs and library structs.
var _ ports.Roster = (*Service)(nil)

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

func (s *Service) ListForRun(ctx context.Context, vendor string) ([]ports.EmployerBoard, error) {
	all, err := s.q.ListEmployerBoardsByVendor(ctx, vendor)
	if err != nil {
		return nil, err
	}
	if len(all) > MaxEmployersPerRun {
		all = all[:MaxEmployersPerRun]
	}
	out := make([]ports.EmployerBoard, 0, len(all))
	for _, e := range all {
		out = append(out, toPortEmployerBoard(e))
	}
	return out, nil
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
	return s.DeleteEmployerBoard(ctx, id)
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

// --- ports.Roster ---
//
// The port speaks plain strings and library structs; these wrappers are the
// only place UUIDs and sqlcgen rows cross the boundary.

func toPortEmployerBoard(e sqlcgen.EmployerBoard) ports.EmployerBoard {
	out := ports.EmployerBoard{
		ID:                   dbutil.UUIDString(e.ID),
		Vendor:               e.Vendor,
		EmployerIdentifier:   e.EmployerIdentifier,
		DisplayName:          e.DisplayName,
		AddedVia:             e.AddedVia,
		Enabled:              e.Enabled,
		LastPostingCount:     int(e.LastPostingCount),
		ConsecutiveEmptyRuns: int(e.ConsecutiveEmptyRuns),
	}
	if e.LastSuccessAt.Valid {
		t := e.LastSuccessAt.Time
		out.LastSuccessAt = &t
	}
	return out
}

func toPortBoardCandidate(c sqlcgen.BoardCandidate) ports.BoardCandidate {
	return ports.BoardCandidate{
		ID:                 dbutil.UUIDString(c.ID),
		Vendor:             c.Vendor,
		EmployerIdentifier: c.EmployerIdentifier,
		InferredFromJobID:  dbutil.UUIDStringPtr(c.InferredFromJobId),
		State:              c.State,
	}
}

func (s *Service) GetByVendorAndEmployer(ctx context.Context, vendor, employerIdentifier string) (ports.EmployerBoard, error) {
	board, err := s.getByVendorAndEmployer(ctx, vendor, employerIdentifier)
	if err != nil {
		return ports.EmployerBoard{}, err
	}
	return toPortEmployerBoard(board), nil
}

func (s *Service) InsertEmployerBoard(ctx context.Context, vendor, employerIdentifier, displayName, addedVia string) (ports.EmployerBoard, error) {
	board, err := s.q.InsertEmployerBoard(ctx, sqlcgen.InsertEmployerBoardParams{
		Vendor:             vendor,
		EmployerIdentifier: employerIdentifier,
		DisplayName:        displayName,
		AddedVia:           addedVia,
	})
	if err != nil {
		return ports.EmployerBoard{}, err
	}
	return toPortEmployerBoard(board), nil
}

func (s *Service) DeleteEmployerBoard(ctx context.Context, id string) error {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.q.DeleteEmployerBoard(ctx, uid)
}

func (s *Service) ListBoardCandidates(ctx context.Context) ([]ports.BoardCandidate, error) {
	rows, err := s.q.ListBoardCandidates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ports.BoardCandidate, 0, len(rows))
	for _, c := range rows {
		out = append(out, toPortBoardCandidate(c))
	}
	return out, nil
}

func (s *Service) GetBoardCandidate(ctx context.Context, vendor, employerIdentifier string) (ports.BoardCandidate, error) {
	cand, err := s.q.GetBoardCandidate(ctx, sqlcgen.GetBoardCandidateParams{Vendor: vendor, EmployerIdentifier: employerIdentifier})
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.BoardCandidate{}, nil
	}
	if err != nil {
		return ports.BoardCandidate{}, err
	}
	return toPortBoardCandidate(cand), nil
}

func (s *Service) GetBoardCandidateByID(ctx context.Context, id string) (ports.BoardCandidate, error) {
	cid, err := dbutil.ParseUUID(id)
	if err != nil {
		return ports.BoardCandidate{}, err
	}
	cand, err := s.q.GetBoardCandidateByID(ctx, cid)
	if err != nil {
		return ports.BoardCandidate{}, err
	}
	return toPortBoardCandidate(cand), nil
}

func (s *Service) InsertBoardCandidate(ctx context.Context, vendor, employerIdentifier string) (ports.BoardCandidate, error) {
	cand, err := s.q.InsertBoardCandidate(ctx, sqlcgen.InsertBoardCandidateParams{
		Vendor:             vendor,
		EmployerIdentifier: employerIdentifier,
	})
	if err != nil {
		return ports.BoardCandidate{}, err
	}
	return toPortBoardCandidate(cand), nil
}

func (s *Service) DecideBoardCandidate(ctx context.Context, id, state string) error {
	cid, err := dbutil.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.q.DecideBoardCandidate(ctx, sqlcgen.DecideBoardCandidateParams{ID: cid, State: state})
}

func (s *Service) ListApplyURLsForDiscovery(ctx context.Context, limit int32) ([]string, error) {
	return s.q.ListApplyURLsForDiscovery(ctx, limit)
}
