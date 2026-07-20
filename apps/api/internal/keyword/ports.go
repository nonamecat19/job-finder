// Package keyword ports modules/keyword/*: turn a raw job description into a
// set of typed terms (required vs preferred) and normalize those terms for the
// downstream ATS diff (008). This is the hexagonal seam: the Extractor port
// lives in the domain layer (this file) and the concrete implementation behind
// it is swappable — today a deterministic regex/lexicon extractor, tomorrow an
// LLM-backed one — without touching callers. See spec 008-1 (el-1a4s).
package keyword

// Polarity is the required/preferred classification of an extracted term.
type Polarity string

const (
	// PolarityRequired marks a term the JD frames as mandatory.
	PolarityRequired Polarity = "required"
	// PolarityPreferred marks a term the JD frames as nice-to-have.
	PolarityPreferred Polarity = "preferred"
)

// ExtractedTerm is a single skill-like token pulled from a job description,
// tagged with the polarity inferred from its surrounding section/markers and
// the section it came from.
type ExtractedTerm struct {
	Term      string   `json:"term"`
	Polarity  Polarity `json:"polarity"`
	Section   string   `json:"source_section"`
	Canonical string   `json:"canonical"`
	Stemmed   string   `json:"normalized"`
}

// ExtractResult is the full output of a single extraction pass over one JD.
type ExtractResult struct {
	Terms []ExtractedTerm `json:"terms"`
}

// Extractor is the inbound port the keyword use-case exposes. The concrete
// implementation (RegexExtractor) satisfies it structurally, so tests can swap
// in a fake and the wiring layer can inject the production adapter.
type Extractor interface {
	// Extract parses raw job-description text into typed, normalized terms.
	Extract(jobDescription string) (*ExtractResult, error)
}
