package keyword

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

// ErrDiffNotFound is returned when no KeywordDiff cache row exists for a job
// (the diff has not been computed yet). The HTTP layer maps this to 404.
var ErrDiffNotFound = errors.New("keyword: no diff for job")

// DiffReader is the outbound port the API service needs to read the persisted
// diff. The sqlc *Queries type satisfies it structurally.
type DiffReader interface {
	GetKeywordDiffByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.KeywordDiff, error)
}

// Rephraser produces advisory rephrase suggestions for missing-required terms.
// The keyword *Suggester (008-5) satisfies it; it is optional so the endpoint
// degrades gracefully (empty suggestions) when no model is wired.
type Rephraser interface {
	SuggestAll(ctx context.Context, missingRequired []DiffTerm, profileBullets []string) []RephraseSuggestion
}

// BulletsProvider loads the user's existing resume bullet lines, verbatim, used
// as the grounding source for rephrase suggestions.
type BulletsProvider interface {
	ProfileBullets(ctx context.Context) ([]string, error)
}

// DiffService reads the cached KeywordDiff for a job and shapes it into the API
// response, attaching advisory rephrase suggestions when a Rephraser and
// BulletsProvider are configured.
type DiffService struct {
	reader    DiffReader
	rephraser Rephraser       // optional
	bullets   BulletsProvider // optional
}

// NewDiffService returns a read-only diff service. Pass WithRephraser to enable
// advisory suggestions.
func NewDiffService(reader DiffReader) *DiffService {
	return &DiffService{reader: reader}
}

// WithRephraser wires the (optional) rephrase suggester and its grounding
// source. Both must be non-nil for suggestions to be produced.
func (s *DiffService) WithRephraser(r Rephraser, b BulletsProvider) *DiffService {
	s.rephraser = r
	s.bullets = b
	return s
}

// KeywordDiff returns the diff buckets, coverage metadata, and advisory
// rephrase suggestions for jobID. Returns ErrDiffNotFound when no cache row
// exists (or the id is malformed).
func (s *DiffService) KeywordDiff(ctx context.Context, jobID string) (dto.KeywordDiffDto, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return dto.KeywordDiffDto{}, ErrDiffNotFound
	}

	row, err := s.reader.GetKeywordDiffByJobID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dto.KeywordDiffDto{}, ErrDiffNotFound
		}
		return dto.KeywordDiffDto{}, fmt.Errorf("keyword: read diff: %w", err)
	}

	matched, err := unmarshalTerms(row.Matched)
	if err != nil {
		return dto.KeywordDiffDto{}, fmt.Errorf("keyword: decode matched: %w", err)
	}
	missingRequired, err := unmarshalTerms(row.MissingRequired)
	if err != nil {
		return dto.KeywordDiffDto{}, fmt.Errorf("keyword: decode missingRequired: %w", err)
	}
	missingPreferred, err := unmarshalTerms(row.MissingPreferred)
	if err != nil {
		return dto.KeywordDiffDto{}, fmt.Errorf("keyword: decode missingPreferred: %w", err)
	}

	out := dto.KeywordDiffDto{
		JobID:            jobID,
		Matched:          toTermDtos(matched),
		MissingRequired:  toTermDtos(missingRequired),
		MissingPreferred: toTermDtos(missingPreferred),
		Metadata:         buildMetadata(matched, missingRequired, missingPreferred, row.CoveragePct),
		Suggestions:      s.suggestions(ctx, missingRequired),
	}
	return out, nil
}

// suggestions produces advisory rephrases for the missing-required terms when a
// rephraser + grounding source are wired; otherwise an empty slice. Failures to
// load bullets are non-fatal — the diff still renders without suggestions.
func (s *DiffService) suggestions(ctx context.Context, missingRequired []DiffTerm) []dto.KeywordRephraseSuggestionDto {
	if s.rephraser == nil || s.bullets == nil || len(missingRequired) == 0 {
		return []dto.KeywordRephraseSuggestionDto{}
	}
	bullets, err := s.bullets.ProfileBullets(ctx)
	if err != nil {
		return []dto.KeywordRephraseSuggestionDto{}
	}
	suggestions := s.rephraser.SuggestAll(ctx, missingRequired, bullets)
	out := make([]dto.KeywordRephraseSuggestionDto, 0, len(suggestions))
	for _, sg := range suggestions {
		out = append(out, dto.KeywordRephraseSuggestionDto{
			Term:         sg.Term,
			Canonical:    sg.Canonical,
			Rephrase:     sg.Rephrase,
			SourceBullet: sg.SourceBullet,
			Reason:       sg.Reason,
		})
	}
	return out
}

func unmarshalTerms(raw []byte) ([]DiffTerm, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var terms []DiffTerm
	if err := json.Unmarshal(raw, &terms); err != nil {
		return nil, err
	}
	return terms, nil
}

func toTermDtos(terms []DiffTerm) []dto.KeywordDiffTermDto {
	out := make([]dto.KeywordDiffTermDto, 0, len(terms))
	for _, t := range terms {
		out = append(out, dto.KeywordDiffTermDto{
			Term:       t.Term,
			Canonical:  t.Canonical,
			Polarity:   string(t.Polarity),
			Normalized: t.Stemmed,
			MatchType:  string(t.MatchType),
		})
	}
	return out
}

// buildMetadata recomputes the coverage counters from the buckets (the cache
// row persists only coveragePct) so the panel always has full metadata. The
// persisted coveragePct is authoritative when present; otherwise it is derived.
func buildMetadata(matched, missingRequired, missingPreferred []DiffTerm, coveragePct *float64) dto.KeywordDiffMetadataDto {
	var matchedRequired, matchedPreferred int
	for _, t := range matched {
		if t.Polarity == PolarityRequired {
			matchedRequired++
		} else {
			matchedPreferred++
		}
	}
	totalRequired := matchedRequired + len(missingRequired)
	totalPreferred := matchedPreferred + len(missingPreferred)

	pct := 0.0
	if coveragePct != nil {
		pct = *coveragePct
	} else if total := totalRequired + totalPreferred; total > 0 {
		pct = float64(matchedRequired+matchedPreferred) / float64(total) * 100
	}

	return dto.KeywordDiffMetadataDto{
		TotalRequired:    totalRequired,
		TotalPreferred:   totalPreferred,
		MatchedRequired:  matchedRequired,
		MatchedPreferred: matchedPreferred,
		CoveragePct:      pct,
	}
}
