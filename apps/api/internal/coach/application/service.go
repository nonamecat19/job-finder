// Package application computes fit-gap assessments: for each missing
// must-have, find up to 3 adjacent profile entries, each with a grounded
// rephrase suggestion (spec 009 §1).
package application

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/job-finder/api/internal/coach/domain"
	"github.com/job-finder/api/internal/keyword"
)

// Service computes fit-gap assessments: for each missing must-have, find up to
// 3 adjacent profile entries, each with a grounded rephrase suggestion.
type Service struct {
	model RephraseModel
	log   *slog.Logger
}

// RephraseModel is the minimal LLM port for generating grounded rephrases.
type RephraseModel interface {
	Rephrase(ctx context.Context, prompt string) (string, error)
}

// NewService returns a coach service backed by the given rephrase model.
func NewService(model RephraseModel) *Service {
	return &Service{
		model: model,
		log:   slog.Default(),
	}
}

// WithLogger sets the logger used for rejected-generation observability.
func (s *Service) WithLogger(l *slog.Logger) *Service {
	if l != nil {
		s.log = l
	}
	return s
}

// Assess computes the fit-gap assessment for a job+profile pair. jobID is for
// traceability only; the actual diff comes from diffResult (the output of
// keyword.Differ.Diff). profileEntries are all the user's profile bullets with
// their source metadata. roleContext is the job role hint for adjacency lookup
// (e.g. "backend", "systems", "frontend"); pass "" for context-agnostic.
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

// assessGap finds up to 3 adjacent profile entries for one missing term.
func (s *Service) assessGap(ctx context.Context, term keyword.DiffTerm, profileEntries []domain.ProfileEntry, roleContext string) domain.GapItem {
	gap := domain.GapItem{
		Term:             term.Term,
		Polarity:         string(term.Polarity),
		AdjacentEvidence: []domain.EvidenceItem{},
	}

	// Lookup adjacency for the missing term
	adjacencies := keyword.Adjacent(term.Canonical, roleContext)
	if len(adjacencies) == 0 {
		gap.NoAdjacentEvidence = true
		return gap
	}

	// Build a map of adjacent term stems for matching
	adjacentStems := make(map[string]keyword.Proximity)
	for _, adj := range adjacencies {
		stemmed := domain.Stem(domain.LowerASCII(adj.Term))
		if stemmed != "" {
			// Keep closest proximity if duplicate stems
			if prev, ok := adjacentStems[stemmed]; !ok || domain.ProximityRank(adj.Proximity) < domain.ProximityRank(prev) {
				adjacentStems[stemmed] = adj.Proximity
			}
		}
	}

	// Find profile entries that mention adjacent terms
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
				break // One match per entry
			}
		}
	}

	if len(matches) == 0 {
		gap.NoAdjacentEvidence = true
		return gap
	}

	// Sort by proximity (closest first), take up to 3
	sort.Slice(matches, func(i, j int) bool {
		if domain.ProximityRank(matches[i].proximity) != domain.ProximityRank(matches[j].proximity) {
			return domain.ProximityRank(matches[i].proximity) < domain.ProximityRank(matches[j].proximity)
		}
		// Stable tie-breaker
		return matches[i].entry.SourceLabel < matches[j].entry.SourceLabel
	})

	limit := 3
	if len(matches) < limit {
		limit = len(matches)
	}

	// Generate grounded rephrases for the top matches
	for i := 0; i < limit; i++ {
		m := matches[i]
		rephrase := s.generateGroundedRephrase(ctx, term, m.entry)
		if rephrase == "" {
			// Grounding failed after retries; skip this evidence
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

// generateGroundedRephrase attempts to generate a truthful rephrase for the
// source entry, with grounding post-check retry. Returns empty string on
// failure (all attempts violated grounding).
func (s *Service) generateGroundedRephrase(ctx context.Context, term keyword.DiffTerm, entry domain.ProfileEntry) string {
	const maxAttempts = 2

	// Build allowed proper noun set from the source bullet only (spec 009 §4.3)
	allowedProper := domain.PropernounSet([]string{entry.Bullet})
	sourceNums := domain.NumberSet(entry.Bullet)

	// Extract seniority and date range from the source label for grounding
	sourceSeniority := domain.ExtractSeniority(entry.SourceLabel)
	sourceDateRange := domain.ExtractDateRange(entry.SourceLabel)

	var lastViol []string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		prompt := buildRephrasePrompt(term, entry.Bullet, entry.SourceLabel, lastViol)
		raw, err := s.model.Rephrase(ctx, prompt)
		if err != nil {
			s.log.WarnContext(ctx, "coach: rephrase model error",
				"term", term.Term, "error", err)
			return ""
		}
		out := strings.TrimSpace(raw)

		// Grounding post-check (spec 009 §4.3)
		lastViol = domain.VerifyRephraseGrounding(entry.Bullet, allowedProper, sourceNums, out, sourceSeniority, sourceDateRange)
		if len(lastViol) == 0 {
			return out
		}

		// Log rejection
		s.log.WarnContext(ctx, "coach: rejected rephrase generation (grounding violation)",
			"term", term.Term,
			"attempt", attempt+1,
			"violations", strings.Join(lastViol, "; "),
			"output", out)
	}

	return ""
}

// buildRephrasePrompt mirrors keyword.buildRephrasePrompt (spec 009 §4.3).
// sourceLabel provides the entry header (e.g. "DevOps Engineer, Acme Corp (2022–2024)")
// which grounds seniority, employer, and duration claims.
func buildRephrasePrompt(term keyword.DiffTerm, sourceBullet string, sourceLabel string, priorViolations []string) string {
	want := term.Term
	if term.Canonical != "" {
		want = term.Canonical
	}
	var b strings.Builder
	b.WriteString("Reframe the candidate's EXISTING resume bullet so that it honestly surfaces the target skill/term the job wants.\n\n")
	b.WriteString("STRICT RULES:\n")
	b.WriteString("- Rephrase ONLY the experience shown in the source bullet below.\n")
	b.WriteString("- Do NOT add any skill, technology, employer, job title, date, or metric that is not already in the source bullet.\n")
	b.WriteString("- Do NOT inflate seniority. If the source label says 'Junior', do not call it 'Senior'.\n")
	b.WriteString("- Do NOT inflate duration. If the source label says '2022–2024', do not claim '10+ years'.\n")
	b.WriteString("- Do NOT borrow technologies from other entries. Only use what is in the source bullet.\n")
	b.WriteString("- If the source bullet does not genuinely support the target term, do not stretch it — return the bullet unchanged.\n")
	b.WriteString("- Output the single reframed bullet as plain text. No preamble, no quotes, no bullet marker.\n\n")
	b.WriteString("TARGET TERM: " + want + "\n\n")
	b.WriteString("SOURCE LABEL (seniority, employer, dates):\n" + sourceLabel + "\n\n")
	b.WriteString("SOURCE BULLET:\n" + sourceBullet + "\n")
	if len(priorViolations) > 0 {
		b.WriteString("\nYour previous attempt violated the no-invention rule:\n- ")
		b.WriteString(strings.Join(priorViolations, "\n- "))
		b.WriteString("\nRegenerate using only what the source bullet and label contain.\n")
	}
	return b.String()
}
