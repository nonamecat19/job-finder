package domain

import "context"

const (
	KindFunding         = "funding"
	KindLayoffs         = "layoffs"
	KindGlassdoorRating = "glassdoor_rating"
	KindHeadcount       = "headcount"
	KindTechStack       = "tech_stack"
)

type Input struct {
	CompanyName       string
	Website           string
	JobDescription    string
	PreviousHeadcount *int
}

type SignalResult struct {
	Kind   string
	Value  any
	Source string
	Raw    any
}

type Scraper interface {
	Kind() string
	Domain() string
	Scrape(ctx context.Context, in Input) (*SignalResult, error)
}
