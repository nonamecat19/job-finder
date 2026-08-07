package application

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
	"github.com/job-finder/api/internal/keyword/domain"
)

var ErrDiffNotFound = errors.New("keyword: no diff for job")

type DiffReader interface {
	GetKeywordDiffByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.KeywordDiff, error)
}

type Rephraser interface {
	SuggestAll(ctx context.Context, missingRequired []domain.DiffTerm, profileBullets []string) []RephraseSuggestion
}

type BulletsProvider interface {
	ProfileBullets(ctx context.Context) ([]string, error)
}

type DiffService struct {
	reader    DiffReader
	rephraser Rephraser
	bullets   BulletsProvider
}

func NewDiffService(reader DiffReader) *DiffService {
	return &DiffService{reader: reader}
}

func (s *DiffService) WithRephraser(r Rephraser, b BulletsProvider) *DiffService {
	s.rephraser = r
	s.bullets = b
	return s
}

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

func (s *DiffService) suggestions(ctx context.Context, missingRequired []domain.DiffTerm) []dto.KeywordRephraseSuggestionDto {
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

func unmarshalTerms(raw []byte) ([]domain.DiffTerm, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var terms []domain.DiffTerm
	if err := json.Unmarshal(raw, &terms); err != nil {
		return nil, err
	}
	return terms, nil
}

func toTermDtos(terms []domain.DiffTerm) []dto.KeywordDiffTermDto {
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

func buildMetadata(matched, missingRequired, missingPreferred []domain.DiffTerm, coveragePct *float64) dto.KeywordDiffMetadataDto {
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
