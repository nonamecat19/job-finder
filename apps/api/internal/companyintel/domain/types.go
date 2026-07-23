// Package domain scrapes public, best-effort company-level signals
// (funding, layoffs, Glassdoor rating, headcount, tech stack) and holds the
// core model: the Scraper extensibility point, the Registry, the
// Repository persistence port, and the ErrNoCompany sentinel, mirroring
// specs/004-company-intel-card. Every scraper fails independently and
// closed (FR-006): a blocked or errored source degrades to "no data" for
// that one signal, never a card-level failure.
package domain

import "context"

// Signal kinds — mirror the CompanySignal.kind values in migration
// 00007_company_intel.sql and the CompanyIntelDto field names.
const (
	KindFunding         = "funding"
	KindLayoffs         = "layoffs"
	KindGlassdoorRating = "glassdoor_rating"
	KindHeadcount       = "headcount"
	KindTechStack       = "tech_stack"
)

// Input carries everything a scraper needs to probe a company. Fields are
// best-effort — a scraper must degrade gracefully (return nil, nil to skip,
// or nil, err to fail) when a field it needs is missing.
type Input struct {
	// CompanyName is the job's raw (non-normalized) company name.
	CompanyName string
	// Website is the company's own site, if already known from a prior
	// scrape. Empty when never resolved — headcount and tech-stack "own
	// site" probes are skipped in that case (edge case in spec.md).
	Website string
	// JobDescription is the posting text backing the tech-stack fallback
	// (FR-015) when BuiltWith is unreachable.
	JobDescription string
	// PreviousHeadcount is the last recorded headcount snapshot, if any —
	// lets the headcount scraper compute a trend on the second-and-later
	// probe instead of only ever reporting a baseline.
	PreviousHeadcount *int
}

// SignalResult is what a scraper returns on a successful probe (including a
// deliberate "zero results" probe, e.g. layoffs.fyi finding no rows for the
// company — that is success with an empty/placeholder Value, not an error).
type SignalResult struct {
	// Kind is one of the Kind* constants above.
	Kind string
	// Value is the flattened, human-readable value stored in
	// CompanySignal.value and surfaced directly on CompanyIntelDto — a
	// string for every kind except glassdoor_rating, which is a float64.
	Value any
	// Source is the URL the value was scraped from, stored in
	// CompanySignal.source.
	Source string
	// Raw is the full scraped payload (e.g. the matched HTML fragment or
	// JSON), stored in CompanySignal.raw for post-hoc debugging (FR-010).
	Raw any
}

// Scraper is the extensibility point every signal source implements — one
// file per source, registered once in the Registry. Scrape must never
// panic; a source that cannot be reached or parsed returns (nil, err) so
// the caller can log a warning and leave the previously-cached signal (if
// any) untouched. Returning (nil, nil) means "deliberately skipped" (e.g.
// no website known yet) and is not logged as a failure.
type Scraper interface {
	// Kind identifies which CompanySignal.kind this scraper produces.
	Kind() string
	// Domain is the external host this scraper hits, used both for the
	// per-source pacing (FR-012) and for diagnostic log lines (FR-016).
	Domain() string
	Scrape(ctx context.Context, in Input) (*SignalResult, error)
}
