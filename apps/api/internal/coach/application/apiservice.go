package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/coach/domain"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/keyword"
)

// ErrNoDiff is returned when no keyword diff (008) has been computed for the
// job yet — the coach has nothing to assess a fit-gap against. The HTTP
// layer maps this to 404.
var ErrNoDiff = errors.New("coach: no keyword diff for job")

// ErrNotAssessed is returned by CachedAssessment when Assess has never been
// run for jobID in this process. See AssessmentService's doc comment for why
// this is a process-local cache rather than a persisted one.
var ErrNotAssessed = errors.New("coach: no assessment computed yet")

// DiffReader is the outbound port to read the persisted keyword diff (008-2)
// the fit-gap assessment is computed from. *sqlcgen.Queries satisfies it
// structurally.
type DiffReader interface {
	GetKeywordDiffByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.KeywordDiff, error)
}

// ProfileEntriesFunc supplies the grounding source for adjacency evidence:
// the user's whole profile, one entry per resume bullet paired with its
// source label. It is a func rather than an interface bound to a concrete
// profile type so this package never needs to import internal/profile —
// main.go closes over profile.Service when wiring the AssessmentService,
// keeping the dependency graph acyclic (mirrors keyword.BulletsProvider).
type ProfileEntriesFunc func(ctx context.Context) ([]domain.ProfileEntry, error)

// AssessmentService orchestrates the fit-gap coach for the HTTP layer
// (009-wiring): it reads the persisted keyword diff and the profile's
// grounding entries, runs Service.Assess, and caches the last result per job
// in memory.
//
// Caching decision: the 009 wiring plan calls for no DB migration, and each
// assessment is itself a live LLM call per missing must-have (mirroring
// 008-6's keyword-diff rephrase suggestions), so a process-local cache is
// the right scope here — it lets GET /coach/assessment return the last
// result without re-paying that LLM cost, without adding persistence the
// plan never asked for. The cache does not survive a process restart;
// callers that need a fresh result call POST /coach/assess again.
type AssessmentService struct {
	svc     *Service
	diffs   DiffReader
	entries ProfileEntriesFunc

	mu    sync.RWMutex
	cache map[string]*domain.FitGapAssessment
}

// NewAssessmentService wires an AssessmentService. entries may be nil, in
// which case gaps are assessed with no profile evidence (every gap comes
// back with NoAdjacentEvidence = true) rather than failing the request.
func NewAssessmentService(svc *Service, diffs DiffReader, entries ProfileEntriesFunc) *AssessmentService {
	return &AssessmentService{
		svc:     svc,
		diffs:   diffs,
		entries: entries,
		cache:   make(map[string]*domain.FitGapAssessment),
	}
}

// Assess runs a fresh fit-gap assessment for jobID — the job's keyword diff
// (008) must already exist — and caches the result for subsequent
// CachedAssessment reads.
func (a *AssessmentService) Assess(ctx context.Context, jobID string) (dto.FitGapAssessmentDto, error) {
	diffResult, err := a.loadDiff(ctx, jobID)
	if err != nil {
		return dto.FitGapAssessmentDto{}, err
	}

	var entries []domain.ProfileEntry
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

// CachedAssessment returns the last Assess result for jobID computed in this
// process, or ErrNotAssessed if none exists yet.
func (a *AssessmentService) CachedAssessment(_ context.Context, jobID string) (dto.FitGapAssessmentDto, error) {
	a.mu.RLock()
	out, ok := a.cache[jobID]
	a.mu.RUnlock()
	if !ok {
		return dto.FitGapAssessmentDto{}, ErrNotAssessed
	}
	return out.ToDto(), nil
}

// loadDiff reads the persisted 008 keyword diff for jobID and reconstitutes
// it as a *keyword.DiffResult. The stored jsonb term shape and keyword.DiffTerm
// share the same JSON tags (spec 008-1 §3), so this is a direct unmarshal —
// no dependency on keyword's unexported apiservice helpers.
func (a *AssessmentService) loadDiff(ctx context.Context, jobID string) (*keyword.DiffResult, error) {
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
		if t.Polarity == keyword.PolarityRequired {
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

	return &keyword.DiffResult{
		Matched:          matched,
		MissingRequired:  missingRequired,
		MissingPreferred: missingPreferred,
		Metadata: keyword.DiffMetadata{
			TotalRequired:    totalRequired,
			TotalPreferred:   totalPreferred,
			MatchedRequired:  matchedRequired,
			MatchedPreferred: matchedPreferred,
			CoveragePct:      coveragePct,
		},
	}, nil
}

func unmarshalDiffTerms(raw []byte) ([]keyword.DiffTerm, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var terms []keyword.DiffTerm
	if err := json.Unmarshal(raw, &terms); err != nil {
		return nil, err
	}
	return terms, nil
}
