package roster

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

const discoveryScanLimit = 5000

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
			continue
		}
		if _, err := s.q.GetBoardCandidate(ctx, sqlcgen.GetBoardCandidateParams{Vendor: vendor, EmployerIdentifier: employerIdentifier}); err == nil {
			continue
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

func (s *Service) Reject(ctx context.Context, candidateID string) error {
	cid, err := dbutil.ParseUUID(candidateID)
	if err != nil {
		return err
	}
	return s.q.DecideBoardCandidate(ctx, sqlcgen.DecideBoardCandidateParams{ID: cid, State: "rejected"})
}
