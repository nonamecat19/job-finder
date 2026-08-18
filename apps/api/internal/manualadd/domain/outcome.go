package domain

import "github.com/job-finder/api/internal/dto"

type Outcome string

const (
	OutcomeCreated     Outcome = "created"
	OutcomeDuplicate   Outcome = "duplicate"
	OutcomeNeedsFillIn Outcome = "needs_fill_in"
	OutcomeFailed      Outcome = "failed"
)

type Result struct {
	Outcome Outcome
	Job     *dto.JobDto
	Kind    FailureKind
	Reason  string
	Draft   *Draft
}

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
