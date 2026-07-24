package salary

import "fmt"

type SalarySource string

const (
	SourceLLM           SalarySource = "llm"
	SourceLevelsFyi     SalarySource = "levels-fyi"
	SourceIngestedCache SalarySource = "ingested-cache"
	SourceBlended       SalarySource = "blended"
)

type SalaryBand struct {
	Min        int          `json:"min" jsonschema:"description=Minimum annual salary in the currency units"`
	Max        int          `json:"max" jsonschema:"description=Maximum annual salary in the currency units"`
	Currency   string       `json:"currency" jsonschema:"description=ISO 4217 currency code,enum=USD,enum=EUR,enum=GBP,enum=UAH,enum=PLN,enum=CAD,enum=AUD"`
	Confidence float64      `json:"confidence" jsonschema:"description=Confidence 0-1,minimum=0,maximum=1"`
	Source     SalarySource `json:"source" jsonschema:"description=Source of the salary data,enum=llm,enum=levels-fyi,enum=ingested-cache,enum=blended"`
}

func (b SalaryBand) Validate() error {
	if b.Min < 0 {
		return fmt.Errorf("min must be >= 0, got %d", b.Min)
	}
	if b.Max < 0 {
		return fmt.Errorf("max must be >= 0, got %d", b.Max)
	}
	if b.Min > b.Max {
		return fmt.Errorf("min (%d) must be <= max (%d)", b.Min, b.Max)
	}
	if b.Confidence < 0 || b.Confidence > 1 {
		return fmt.Errorf("confidence must be 0-1, got %f", b.Confidence)
	}
	if b.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	return nil
}

type BlendedBand struct {
	Bands       []SalaryBand `json:"bands" jsonschema:"description=Source bands that contributed to the blend"`
	Final       SalaryBand   `json:"final" jsonschema:"description=Weighted result band"`
	Confidence  float64      `json:"confidence" jsonschema:"description=Normalized confidence 0-1,minimum=0,maximum=1"`
	Explanation string       `json:"explanation" jsonschema:"description=How the blend was computed"`
}
