package coach

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"

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
func (s *Service) Assess(ctx context.Context, jobID string, diffResult *keyword.DiffResult, profileEntries []ProfileEntry, roleContext string) *FitGapAssessment {
	if diffResult == nil {
		return &FitGapAssessment{
			JobID:           jobID,
			TotalMustHaves:  0,
			FailedMustHaves: 0,
			CoveragePct:     0,
			Gaps:            []GapItem{},
		}
	}

	totalRequired := diffResult.Metadata.TotalRequired
	failedRequired := len(diffResult.MissingRequired)
	coveragePct := diffResult.Metadata.CoveragePct

	gaps := make([]GapItem, 0, len(diffResult.MissingRequired))
	for _, term := range diffResult.MissingRequired {
		gap := s.assessGap(ctx, term, profileEntries, roleContext)
		gaps = append(gaps, gap)
	}

	return &FitGapAssessment{
		JobID:           jobID,
		TotalMustHaves:  totalRequired,
		FailedMustHaves: failedRequired,
		CoveragePct:     coveragePct,
		Gaps:            gaps,
	}
}

// assessGap finds up to 3 adjacent profile entries for one missing term.
func (s *Service) assessGap(ctx context.Context, term keyword.DiffTerm, profileEntries []ProfileEntry, roleContext string) GapItem {
	gap := GapItem{
		Term:             term.Term,
		Polarity:         string(term.Polarity),
		AdjacentEvidence: []EvidenceItem{},
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
		stemmed := stem(lowerASCII(adj.Term))
		if stemmed != "" {
			// Keep closest proximity if duplicate stems
			if prev, ok := adjacentStems[stemmed]; !ok || proximityRank(adj.Proximity) < proximityRank(prev) {
				adjacentStems[stemmed] = adj.Proximity
			}
		}
	}

	// Find profile entries that mention adjacent terms
	type match struct {
		entry     ProfileEntry
		proximity keyword.Proximity
	}
	var matches []match
	for _, entry := range profileEntries {
		tokens := tokenRe.FindAllString(entry.Bullet, -1)
		for _, tok := range tokens {
			stemmed := stem(lowerASCII(tok))
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
		if proximityRank(matches[i].proximity) != proximityRank(matches[j].proximity) {
			return proximityRank(matches[i].proximity) < proximityRank(matches[j].proximity)
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
		gap.AdjacentEvidence = append(gap.AdjacentEvidence, EvidenceItem{
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
func (s *Service) generateGroundedRephrase(ctx context.Context, term keyword.DiffTerm, entry ProfileEntry) string {
	const maxAttempts = 2

	// Build allowed proper noun set from the source bullet only (spec 009 §4.3)
	allowedProper := properNounSet([]string{entry.Bullet})
	sourceNums := numberSet(entry.Bullet)

	var lastViol []string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		prompt := buildRephrasePrompt(term, entry.Bullet, lastViol)
		raw, err := s.model.Rephrase(ctx, prompt)
		if err != nil {
			s.log.WarnContext(ctx, "coach: rephrase model error",
				"term", term.Term, "error", err)
			return ""
		}
		out := strings.TrimSpace(raw)

		// Grounding post-check (spec 009 §4.3)
		lastViol = verifyRephraseGrounding(entry.Bullet, allowedProper, sourceNums, out)
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
func buildRephrasePrompt(term keyword.DiffTerm, sourceBullet string, priorViolations []string) string {
	want := term.Term
	if term.Canonical != "" {
		want = term.Canonical
	}
	var b strings.Builder
	b.WriteString("Reframe the candidate's EXISTING resume bullet so that it honestly surfaces the target skill/term the job wants.\n\n")
	b.WriteString("STRICT RULES:\n")
	b.WriteString("- Rephrase ONLY the experience shown in the source bullet below.\n")
	b.WriteString("- Do NOT add any skill, technology, employer, job title, date, or metric that is not already in the source bullet.\n")
	b.WriteString("- If the source bullet does not genuinely support the target term, do not stretch it — return the bullet unchanged.\n")
	b.WriteString("- Output the single reframed bullet as plain text. No preamble, no quotes, no bullet marker.\n\n")
	b.WriteString("TARGET TERM: " + want + "\n\n")
	b.WriteString("SOURCE BULLET:\n" + sourceBullet + "\n")
	if len(priorViolations) > 0 {
		b.WriteString("\nYour previous attempt violated the no-invention rule:\n- ")
		b.WriteString(strings.Join(priorViolations, "\n- "))
		b.WriteString("\nRegenerate using only what the source bullet contains.\n")
	}
	return b.String()
}

// --- Grounding verification (mirrors keyword/rephrase.go) ---

var numberRe = regexp.MustCompile(`\d+(?:[.,]\d+)*%?`)

// verifyRephraseGrounding checks that every proper noun and number in the
// rephrase is traceable to the source. Returns violations; empty slice means
// grounded.
func verifyRephraseGrounding(sourceBullet string, allowedProper, sourceNums map[string]bool, rephrase string) []string {
	if strings.TrimSpace(rephrase) == "" {
		return []string{"empty rephrase"}
	}
	var violations []string

	for _, p := range properNounsInText(rephrase) {
		if !allowedProper[lowerASCII(p)] {
			violations = append(violations, fmt.Sprintf("proper noun / technology %q not in source bullet", p))
		}
	}
	for _, n := range numberRe.FindAllString(rephrase, -1) {
		if !sourceNums[normNumber(n)] {
			violations = append(violations, fmt.Sprintf("metric/number %q not in source bullet", n))
		}
	}
	return violations
}

var sentenceSplitRe = regexp.MustCompile(`[.!?]\s+`)
var tokenRe = regexp.MustCompile(`\b[\w']+\b`)

func splitSentences(s string) []string { return sentenceSplitRe.Split(s, -1) }

func properNounsInText(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, sentence := range splitSentences(s) {
		tokens := tokenRe.FindAllString(sentence, -1)
		for i, tok := range tokens {
			if !looksProper(tok) {
				continue
			}
			if i == 0 && isPlainCapitalized(tok) {
				continue
			}
			key := lowerASCII(tok)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, tok)
		}
	}
	return out
}

func looksProper(tok string) bool {
	for _, r := range tok {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func isPlainCapitalized(tok string) bool {
	if tok == "" {
		return false
	}
	r := []rune(tok)
	if !(r[0] >= 'A' && r[0] <= 'Z') {
		return false
	}
	for _, c := range r[1:] {
		if c >= 'A' && c <= 'Z' {
			return false
		}
	}
	return true
}

func properNounSet(bullets []string) map[string]bool {
	set := map[string]bool{}
	for _, b := range bullets {
		for _, w := range tokenRe.FindAllString(b, -1) {
			set[lowerASCII(w)] = true
		}
	}
	return set
}

func numberSet(text string) map[string]bool {
	set := map[string]bool{}
	for _, n := range numberRe.FindAllString(text, -1) {
		set[normNumber(n)] = true
	}
	return set
}

func normNumber(n string) string {
	n = strings.TrimSuffix(n, "%")
	return strings.ReplaceAll(n, ",", "")
}

// --- Helpers (mirrors keyword/helpers.go) ---

func lowerASCII(s string) string { return strings.ToLower(s) }

func proximityRank(p keyword.Proximity) int {
	switch p {
	case keyword.ProximityClose:
		return 0
	case keyword.ProximityModerate:
		return 1
	case keyword.ProximityDistant:
		return 2
	default:
		return 3
	}
}

// stem is a minimal stemmer (mirrors keyword.stem).
func stem(s string) string {
	s = strings.TrimSuffix(s, "ing")
	s = strings.TrimSuffix(s, "ed")
	s = strings.TrimSuffix(s, "s")
	return s
}
