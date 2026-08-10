package domain

import "github.com/job-finder/api/internal/dto"

// Outcome is the terminal state of one manual add attempt. Every attempt ends
// in exactly one of these and writes exactly one SourceRun.
type Outcome string

const (
	OutcomeCreated     Outcome = "created"
	OutcomeDuplicate   Outcome = "duplicate"
	OutcomeNeedsFillIn Outcome = "needs_fill_in"
	OutcomeFailed      Outcome = "failed"
)

// Result is what the service hands back to the HTTP edge. Job is set for
// created and duplicate; Draft for needs_fill_in; Kind and Reason for
// needs_fill_in and failed.
type Result struct {
	Outcome Outcome
	Job     *dto.JobDto
	Kind    FailureKind
	Reason  string
	Draft   *Draft
}

// Draft is whatever was extracted before the attempt stalled, so the operator
// completes rather than retypes (FR-019).
type Draft struct {
	URL         string
	SourceKey   *string
	Title       *string
	Company     *string
	Location    *string
	Remote      bool
	SalaryRaw   *string
	Description *string
	PostedAt    *string
}

// DraftFromPosting keeps only the fields the operator would otherwise retype.
// Blank strings become nil so the form renders an empty input rather than the
// literal empty value.
func DraftFromPosting(rawURL string, j dto.NormalizedJob) *Draft {
	d := &Draft{URL: rawURL, Remote: j.Remote, Location: j.Location, SalaryRaw: j.SalaryRaw, PostedAt: j.PostedAt}
	if j.SourceKey != "" {
		d.SourceKey = &j.SourceKey
	}
	if j.Title != "" {
		d.Title = &j.Title
	}
	if j.Company != "" {
		d.Company = &j.Company
	}
	if j.Description != "" {
		d.Description = &j.Description
	}
	return d
}
