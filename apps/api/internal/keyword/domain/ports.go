package domain

type Polarity string

const (
	PolarityRequired  Polarity = "required"
	PolarityPreferred Polarity = "preferred"
)

type ExtractedTerm struct {
	Term      string   `json:"term"`
	Polarity  Polarity `json:"polarity"`
	Section   string   `json:"source_section"`
	Canonical string   `json:"canonical"`
	Stemmed   string   `json:"normalized"`
	Evidence  string   `json:"evidence,omitempty"`
}

type ExtractResult struct {
	Terms []ExtractedTerm `json:"terms"`
}

type Extractor interface {
	Extract(jobDescription string) (*ExtractResult, error)
}
