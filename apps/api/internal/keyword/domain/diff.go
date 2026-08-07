package domain

import "sort"

type MatchType string

const (
	MatchExact      MatchType = "exact"
	MatchNormalized MatchType = "normalized"
)

type DiffTerm struct {
	Term      string    `json:"term"`
	Canonical string    `json:"canonical"`
	Polarity  Polarity  `json:"polarity"`
	Stemmed   string    `json:"normalized"`
	MatchType MatchType `json:"matchType,omitempty"`
}

type DiffMetadata struct {
	TotalRequired    int     `json:"totalRequired"`
	TotalPreferred   int     `json:"totalPreferred"`
	MatchedRequired  int     `json:"matchedRequired"`
	MatchedPreferred int     `json:"matchedPreferred"`
	CoveragePct      float64 `json:"coveragePct"`
}

type DiffResult struct {
	Matched          []DiffTerm   `json:"matched"`
	MissingRequired  []DiffTerm   `json:"missingRequired"`
	MissingPreferred []DiffTerm   `json:"missingPreferred"`
	Metadata         DiffMetadata `json:"metadata"`
}

type Differ struct{}

func NewDiffer() *Differ { return &Differ{} }

type resumeIndex struct {
	canonical map[string]bool
	stemmed   map[string]bool
}

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

func (idx resumeIndex) match(t ExtractedTerm) (MatchType, bool) {
	if idx.canonical[lowerASCII(t.Canonical)] {
		return MatchExact, true
	}
	if t.Stemmed != "" && idx.stemmed[t.Stemmed] {
		return MatchNormalized, true
	}
	return "", false
}

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

func sortMatched(terms []DiffTerm) {
	sort.SliceStable(terms, func(i, j int) bool {
		a, b := terms[i], terms[j]
		if a.Polarity != b.Polarity {
			return a.Polarity == PolarityRequired
		}
		if a.Canonical != b.Canonical {
			return a.Canonical < b.Canonical
		}
		return a.Term < b.Term
	})
}

func sortMissing(terms []DiffTerm) {
	sort.SliceStable(terms, func(i, j int) bool {
		a, b := terms[i], terms[j]
		if a.Canonical != b.Canonical {
			return a.Canonical < b.Canonical
		}
		return a.Term < b.Term
	})
}

func ExtractResumeTerms(profileText string) []string {
	phrases := extractPhrases(profileText)
	out := make([]string, 0, len(phrases))
	seen := make(map[string]bool, len(phrases))
	for _, p := range phrases {
		t := normalizeTerm(p.phrase)
		if t.Canonical == "" || seen[lowerASCII(t.Canonical)] {
			continue
		}
		seen[lowerASCII(t.Canonical)] = true
		out = append(out, t.Canonical)
	}
	return out
}
