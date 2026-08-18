package application

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/job-finder/api/internal/coach/domain"
	"github.com/job-finder/api/internal/keyword"
)

type Service struct {
	model RephraseModel
	log   *slog.Logger
}

// RephraseRequest is the grounding data a rephrase call needs (T107):
// structured, rather than a pre-built prompt string, so a RephraseModel
// implementation backed by the `rephrase` capability (aiclient) can send it
// straight through as that capability's input model — the capability builds
// its own prompt server-side (apps/ai's prompts/rephrase.py) instead of
// receiving Go's.
type RephraseRequest struct {
	Term            string
	Canonical       string
	SourceBullet    string
	SourceLabel     string
	PriorViolations []string
}

type RephraseModel interface {
	Rephrase(ctx context.Context, req RephraseRequest) (string, error)
}

func NewService(model RephraseModel) *Service {
	return &Service{
		model: model,
		log:   slog.Default(),
	}
}

func (s *Service) WithLogger(l *slog.Logger) *Service {
	if l != nil {
		s.log = l
	}
	return s
}

func (s *Service) Assess(ctx context.Context, jobID string, diffResult *keyword.DiffResult, profileEntries []domain.ProfileEntry, roleContext string) *domain.FitGapAssessment {
	if diffResult == nil {
		return &domain.FitGapAssessment{
			JobID:           jobID,
			TotalMustHaves:  0,
			FailedMustHaves: 0,
			CoveragePct:     0,
			Gaps:            []domain.GapItem{},
		}
	}

	totalRequired := diffResult.Metadata.TotalRequired
	failedRequired := len(diffResult.MissingRequired)
	coveragePct := diffResult.Metadata.CoveragePct

	gaps := make([]domain.GapItem, 0, len(diffResult.MissingRequired))
	for _, term := range diffResult.MissingRequired {
		gap := s.assessGap(ctx, term, profileEntries, roleContext)
		gaps = append(gaps, gap)
	}

	return &domain.FitGapAssessment{
		JobID:           jobID,
		TotalMustHaves:  totalRequired,
		FailedMustHaves: failedRequired,
		CoveragePct:     coveragePct,
		Gaps:            gaps,
	}
}

func (s *Service) assessGap(ctx context.Context, term keyword.DiffTerm, profileEntries []domain.ProfileEntry, roleContext string) domain.GapItem {
	gap := domain.GapItem{
		Term:             term.Term,
		Polarity:         string(term.Polarity),
		AdjacentEvidence: []domain.EvidenceItem{},
	}

	adjacencies := keyword.Adjacent(term.Canonical, roleContext)
	if len(adjacencies) == 0 {
		gap.NoAdjacentEvidence = true
		return gap
	}

	adjacentStems := make(map[string]keyword.Proximity)
	for _, adj := range adjacencies {
		stemmed := domain.Stem(domain.LowerASCII(adj.Term))
		if stemmed != "" {
			if prev, ok := adjacentStems[stemmed]; !ok || domain.ProximityRank(adj.Proximity) < domain.ProximityRank(prev) {
				adjacentStems[stemmed] = adj.Proximity
			}
		}
	}

	type match struct {
		entry     domain.ProfileEntry
		proximity keyword.Proximity
	}
	var matches []match
	for _, entry := range profileEntries {
		tokens := domain.TokenRe.FindAllString(entry.Bullet, -1)
		for _, tok := range tokens {
			stemmed := domain.Stem(domain.LowerASCII(tok))
			if prox, ok := adjacentStems[stemmed]; ok {
				matches = append(matches, match{entry: entry, proximity: prox})
				break
			}
		}
	}

	if len(matches) == 0 {
		gap.NoAdjacentEvidence = true
		return gap
	}

	sort.Slice(matches, func(i, j int) bool {
		if domain.ProximityRank(matches[i].proximity) != domain.ProximityRank(matches[j].proximity) {
			return domain.ProximityRank(matches[i].proximity) < domain.ProximityRank(matches[j].proximity)
		}
		return matches[i].entry.SourceLabel < matches[j].entry.SourceLabel
	})

	limit := 3
	if len(matches) < limit {
		limit = len(matches)
	}

	for i := 0; i < limit; i++ {
		m := matches[i]
		rephrase := s.generateGroundedRephrase(ctx, term, m.entry)
		if rephrase == "" {
			continue
		}
		gap.AdjacentEvidence = append(gap.AdjacentEvidence, domain.EvidenceItem{
			SourceEntry:  m.entry.SourceLabel,
			SourceBullet: m.entry.Bullet,
			Proximity:    m.proximity,
			Rephrase:     rephrase,
		})
	}

	if len(gap.AdjacentEvidence) == 0 {
		gap.NoAdjacentEvidence = true
	}

	return gap
}

func (s *Service) generateGroundedRephrase(ctx context.Context, term keyword.DiffTerm, entry domain.ProfileEntry) string {
	const maxAttempts = 2

	allowedProper := domain.PropernounSet([]string{entry.Bullet})
	sourceNums := domain.NumberSet(entry.Bullet)

	sourceSeniority := domain.ExtractSeniority(entry.SourceLabel)
	sourceDateRange := domain.ExtractDateRange(entry.SourceLabel)

	want := term.Term
	if term.Canonical != "" {
		want = term.Canonical
	}

	var lastViol []string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req := RephraseRequest{
			Term:            want,
			Canonical:       term.Canonical,
			SourceBullet:    entry.Bullet,
			SourceLabel:     entry.SourceLabel,
			PriorViolations: lastViol,
		}
		raw, err := s.model.Rephrase(ctx, req)
		if err != nil {
			s.log.WarnContext(ctx, "coach: rephrase model error",
				"term", term.Term, "error", err)
			return ""
		}
		out := strings.TrimSpace(raw)

		lastViol = domain.VerifyRephraseGrounding(entry.Bullet, allowedProper, sourceNums, out, sourceSeniority, sourceDateRange)
		if len(lastViol) == 0 {
			return out
		}

		s.log.WarnContext(ctx, "coach: rejected rephrase generation (grounding violation)",
			"term", term.Term,
			"attempt", attempt+1,
			"violations", strings.Join(lastViol, "; "),
			"output", out)
	}

	return ""
}

