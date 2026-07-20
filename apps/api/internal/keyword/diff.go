package keyword

import "sort"

// Package keyword — diff engine (spec 008-1 §3, task 008-4).
//
// Given the typed terms extracted from a job description (008-3) and the terms
// drawn from the user's *whole* master profile (not only the rendered resume,
// so downstream 009 fit-gap coaching can see adjacent evidence), produce three
// deterministic buckets: matched, missing-required, and missing-preferred.
//
// The engine is pure and dependency-free so it is unit-testable in isolation;
// persistence to the KeywordDiff cache table (008-2) lives in persist.go.

// MatchType records how a JD term matched a resume term, for traceability in
// the UI and downstream features.
type MatchType string

const (
	// MatchExact means the JD term's canonical form equals a resume term's
	// canonical form (after synonym/acronym resolution) — spec §3.1.
	MatchExact MatchType = "exact"
	// MatchNormalized means the terms matched only after stemming (inflectional
	// variation, e.g. "containerization" vs "containerize") — spec §3.1.
	MatchNormalized MatchType = "normalized"
)

// DiffTerm is one term in a diff bucket. The JSON tags mirror the spec §3
// result shape (term / canonical / polarity / normalized) so the persisted
// jsonb and the API surface stay identical.
type DiffTerm struct {
	Term      string    `json:"term"`
	Canonical string    `json:"canonical"`
	Polarity  Polarity  `json:"polarity"`
	Stemmed   string    `json:"normalized"`
	MatchType MatchType `json:"matchType,omitempty"`
}

// DiffMetadata carries the coverage counters the UI renders and 009 consumes.
type DiffMetadata struct {
	TotalRequired    int     `json:"totalRequired"`
	TotalPreferred   int     `json:"totalPreferred"`
	MatchedRequired  int     `json:"matchedRequired"`
	MatchedPreferred int     `json:"matchedPreferred"`
	CoveragePct      float64 `json:"coveragePct"`
}

// DiffResult is the full comparison output (spec §3).
type DiffResult struct {
	Matched          []DiffTerm   `json:"matched"`
	MissingRequired  []DiffTerm   `json:"missingRequired"`
	MissingPreferred []DiffTerm   `json:"missingPreferred"`
	Metadata         DiffMetadata `json:"metadata"`
}

// Differ compares extracted JD terms against a normalized resume-term set.
type Differ struct{}

// NewDiffer returns the diff engine. Constructor kept for symmetry with the
// Extractor and for future dependency injection.
func NewDiffer() *Differ { return &Differ{} }

// resumeIndex holds the lookup sets built once from the profile terms.
type resumeIndex struct {
	canonical map[string]bool // lowercased canonical forms
	stemmed   map[string]bool // stemmed forms
}

// buildResumeIndex normalizes every raw profile/resume term with the same
// sequence used for JD terms (spec §2.4) so matching is symmetric.
func buildResumeIndex(resumeTerms []string) resumeIndex {
	idx := resumeIndex{
		canonical: make(map[string]bool, len(resumeTerms)),
		stemmed:   make(map[string]bool, len(resumeTerms)),
	}
	for _, raw := range resumeTerms {
		t := normalizeTerm(raw)
		if t.Term == "" {
			continue
		}
		idx.canonical[lowerASCII(t.Canonical)] = true
		idx.stemmed[t.Stemmed] = true
	}
	return idx
}

// match reports whether a JD term is present in the resume index and, if so,
// how it matched. Canonical (synonym/acronym-resolved) equality is checked
// first and reported as exact; stemmed equality is the normalized fallback.
func (idx resumeIndex) match(t ExtractedTerm) (MatchType, bool) {
	if idx.canonical[lowerASCII(t.Canonical)] {
		return MatchExact, true
	}
	if t.Stemmed != "" && idx.stemmed[t.Stemmed] {
		return MatchNormalized, true
	}
	return "", false
}

// Diff compares the extracted JD terms against the user's whole-profile resume
// terms. resumeTerms are raw strings pulled from the master profile (skills,
// titles, technologies, certifications, free-text); they are normalized here.
// Output buckets are sorted deterministically so the UI stays stable across
// runs (acceptance criterion).
func (d *Differ) Diff(jd *ExtractResult, resumeTerms []string) *DiffResult {
	res := &DiffResult{
		Matched:          []DiffTerm{},
		MissingRequired:  []DiffTerm{},
		MissingPreferred: []DiffTerm{},
	}
	if jd == nil {
		return res
	}
	idx := buildResumeIndex(resumeTerms)

	for _, t := range jd.Terms {
		dt := DiffTerm{
			Term:      t.Term,
			Canonical: t.Canonical,
			Polarity:  t.Polarity,
			Stemmed:   t.Stemmed,
		}
		switch t.Polarity {
		case PolarityRequired:
			res.Metadata.TotalRequired++
		case PolarityPreferred:
			res.Metadata.TotalPreferred++
		}

		if mt, ok := idx.match(t); ok {
			dt.MatchType = mt
			res.Matched = append(res.Matched, dt)
			switch t.Polarity {
			case PolarityRequired:
				res.Metadata.MatchedRequired++
			case PolarityPreferred:
				res.Metadata.MatchedPreferred++
			}
			continue
		}

		if t.Polarity == PolarityPreferred {
			res.MissingPreferred = append(res.MissingPreferred, dt)
		} else {
			res.MissingRequired = append(res.MissingRequired, dt)
		}
	}

	total := res.Metadata.TotalRequired + res.Metadata.TotalPreferred
	if total > 0 {
		matched := res.Metadata.MatchedRequired + res.Metadata.MatchedPreferred
		res.Metadata.CoveragePct = float64(matched) / float64(total) * 100
	}

	sortMatched(res.Matched)
	sortMissing(res.MissingRequired)
	sortMissing(res.MissingPreferred)
	return res
}

// sortMatched orders matched terms required-first, then by canonical, then by
// term — a total order so output is byte-identical across runs.
func sortMatched(terms []DiffTerm) {
	sort.SliceStable(terms, func(i, j int) bool {
		a, b := terms[i], terms[j]
		if a.Polarity != b.Polarity {
			// required sorts before preferred
			return a.Polarity == PolarityRequired
		}
		if a.Canonical != b.Canonical {
			return a.Canonical < b.Canonical
		}
		return a.Term < b.Term
	})
}

// sortMissing orders a single-polarity bucket by canonical, then term.
func sortMissing(terms []DiffTerm) {
	sort.SliceStable(terms, func(i, j int) bool {
		a, b := terms[i], terms[j]
		if a.Canonical != b.Canonical {
			return a.Canonical < b.Canonical
		}
		return a.Term < b.Term
	})
}

// ExtractResumeTerms pulls candidate skill-like terms out of arbitrary
// whole-profile text (concatenated skills, titles, certifications and
// free-text notes). It reuses the JD phrase tokenizer so resume and JD terms
// are extracted identically, satisfying the "match against the whole profile"
// requirement without a second parser.
func ExtractResumeTerms(profileText string) []string {
	phrases := extractPhrases(profileText)
	out := make([]string, 0, len(phrases))
	seen := make(map[string]bool, len(phrases))
	for _, p := range phrases {
		t := normalizeTerm(p)
		if t.Canonical == "" || seen[lowerASCII(t.Canonical)] {
			continue
		}
		seen[lowerASCII(t.Canonical)] = true
		out = append(out, t.Canonical)
	}
	return out
}
