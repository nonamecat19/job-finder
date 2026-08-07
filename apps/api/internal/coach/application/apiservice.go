package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	coachdomain "github.com/job-finder/api/internal/coach/domain"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/keyword/domain"
)

var ErrNoDiff = errors.New("coach: no keyword diff for job")

var ErrNotAssessed = errors.New("coach: no assessment computed yet")

type DiffReader interface {
	GetKeywordDiffByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.KeywordDiff, error)
}

type ProfileEntriesFunc func(ctx context.Context) ([]coachdomain.ProfileEntry, error)

type AssessmentService struct {
	svc     *Service
	diffs   DiffReader
	entries ProfileEntriesFunc

	mu    sync.RWMutex
	cache map[string]*coachdomain.FitGapAssessment
}

func NewAssessmentService(svc *Service, diffs DiffReader, entries ProfileEntriesFunc) *AssessmentService {
	return &AssessmentService{
		svc:     svc,
		diffs:   diffs,
		entries: entries,
		cache:   make(map[string]*coachdomain.FitGapAssessment),
	}
}

func (a *AssessmentService) Assess(ctx context.Context, jobID string) (dto.FitGapAssessmentDto, error) {
	diffResult, err := a.loadDiff(ctx, jobID)
	if err != nil {
		return dto.FitGapAssessmentDto{}, err
	}

	var entries []coachdomain.ProfileEntry
	if a.entries != nil {
		entries, err = a.entries(ctx)
		if err != nil {
			return dto.FitGapAssessmentDto{}, fmt.Errorf("coach: load profile entries: %w", err)
		}
	}

	out := a.svc.Assess(ctx, jobID, diffResult, entries, "")

	a.mu.Lock()
	a.cache[jobID] = out
	a.mu.Unlock()

	return out.ToDto(), nil
}

func (a *AssessmentService) CachedAssessment(_ context.Context, jobID string) (dto.FitGapAssessmentDto, error) {
	a.mu.RLock()
	out, ok := a.cache[jobID]
	a.mu.RUnlock()
	if !ok {
		return dto.FitGapAssessmentDto{}, ErrNotAssessed
	}
	return out.ToDto(), nil
}

func (a *AssessmentService) loadDiff(ctx context.Context, jobID string) (*domain.DiffResult, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return nil, ErrNoDiff
	}
	row, err := a.diffs.GetKeywordDiffByJobID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoDiff
		}
		return nil, fmt.Errorf("coach: read keyword diff: %w", err)
	}

	matched, err := unmarshalDiffTerms(row.Matched)
	if err != nil {
		return nil, fmt.Errorf("coach: decode matched: %w", err)
	}
	missingRequired, err := unmarshalDiffTerms(row.MissingRequired)
	if err != nil {
		return nil, fmt.Errorf("coach: decode missingRequired: %w", err)
	}
	missingPreferred, err := unmarshalDiffTerms(row.MissingPreferred)
	if err != nil {
		return nil, fmt.Errorf("coach: decode missingPreferred: %w", err)
	}

	var matchedRequired, matchedPreferred int
	for _, t := range matched {
		if t.Polarity == domain.PolarityRequired {
			matchedRequired++
		} else {
			matchedPreferred++
		}
	}
	totalRequired := matchedRequired + len(missingRequired)
	totalPreferred := matchedPreferred + len(missingPreferred)

	coveragePct := 0.0
	if row.CoveragePct != nil {
		coveragePct = *row.CoveragePct
	} else if total := totalRequired + totalPreferred; total > 0 {
		coveragePct = float64(matchedRequired+matchedPreferred) / float64(total) * 100
	}

	return &domain.DiffResult{
		Matched:          matched,
		MissingRequired:  missingRequired,
		MissingPreferred: missingPreferred,
		Metadata: domain.DiffMetadata{
			TotalRequired:    totalRequired,
			TotalPreferred:   totalPreferred,
			MatchedRequired:  matchedRequired,
			MatchedPreferred: matchedPreferred,
			CoveragePct:      coveragePct,
		},
	}, nil
}

func unmarshalDiffTerms(raw []byte) ([]domain.DiffTerm, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var terms []domain.DiffTerm
	if err := json.Unmarshal(raw, &terms); err != nil {
		return nil, err
	}
	return terms, nil
}
