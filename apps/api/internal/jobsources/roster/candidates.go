package roster

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

// discoveryScanLimit bounds how many recent Job URLs one Discover call
// inspects — keeps discovery cheap on a large table (research.md §3).
const discoveryScanLimit = 5000

// Discover scans recently ingested Job URLs for supported-vendor board
// links and proposes a BoardCandidate for each employer not already known —
// either already in the roster or already decided/proposed (FR-009,
// FR-013, Edge Case: "found an employer already in the roster: not offered
// again"). Returns how many new candidates were created.
func (s *Service) Discover(ctx context.Context) (created int, err error) {
	urls, err := s.q.ListApplyURLsForDiscovery(ctx, discoveryScanLimit)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]bool)
	for _, u := range urls {
		vendor, employerIdentifier, ok := MatchVendor(u)
		if !ok {
			continue
		}
		key := vendor + "|" + employerIdentifier
		if seen[key] {
			continue
		}
		seen[key] = true

		if existing, err := s.getByVendorAndEmployer(ctx, vendor, employerIdentifier); err != nil {
			return created, err
		} else if existing.ID.Valid {
			continue // already in roster
		}
		if _, err := s.q.GetBoardCandidate(ctx, sqlcgen.GetBoardCandidateParams{Vendor: vendor, EmployerIdentifier: employerIdentifier}); err == nil {
			continue // already proposed, accepted, or rejected
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return created, err
		}

		if _, err := s.q.InsertBoardCandidate(ctx, sqlcgen.InsertBoardCandidateParams{
			Vendor:             vendor,
			EmployerIdentifier: employerIdentifier,
		}); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func (s *Service) ListCandidates(ctx context.Context) ([]sqlcgen.BoardCandidate, error) {
	return s.q.ListBoardCandidates(ctx)
}

// Accept promotes a proposed candidate into the roster (FR-010).
func (s *Service) Accept(ctx context.Context, candidateID string) (sqlcgen.EmployerBoard, error) {
	cid, err := dbutil.ParseUUID(candidateID)
	if err != nil {
		return sqlcgen.EmployerBoard{}, err
	}
	cand, err := s.q.GetBoardCandidateByID(ctx, cid)
	if err != nil {
		return sqlcgen.EmployerBoard{}, err
	}
	board, err := s.register(ctx, cand.Vendor, cand.EmployerIdentifier, "", "proposed")
	if err != nil {
		return sqlcgen.EmployerBoard{}, err
	}
	if err := s.q.DecideBoardCandidate(ctx, sqlcgen.DecideBoardCandidateParams{ID: cid, State: "accepted"}); err != nil {
		return sqlcgen.EmployerBoard{}, err
	}
	return board, nil
}

// Reject marks a candidate rejected, terminal — Discover's GetBoardCandidate
// check keeps it from ever being re-proposed (FR-010).
func (s *Service) Reject(ctx context.Context, candidateID string) error {
	cid, err := dbutil.ParseUUID(candidateID)
	if err != nil {
		return err
	}
	return s.q.DecideBoardCandidate(ctx, sqlcgen.DecideBoardCandidateParams{ID: cid, State: "rejected"})
}
