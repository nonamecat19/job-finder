package adapters

import (
	"errors"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
)

func TestClassifyOutcome(t *testing.T) {
	cases := []struct {
		name   string
		status int
		jobs   []dto.NormalizedJob
		err    error
		want   jobsources.EmployerOutcome
	}{
		{"success with postings", 200, []dto.NormalizedJob{{}}, nil, jobsources.EmployerOutcomeRead},
		{"success no postings", 200, nil, nil, jobsources.EmployerOutcomeNoPostings},
		{"not found", 404, nil, errors.New("boom"), jobsources.EmployerOutcomeNotFound},
		{"unauthorized", 401, nil, errors.New("boom"), jobsources.EmployerOutcomeRefused},
		{"forbidden", 403, nil, errors.New("boom"), jobsources.EmployerOutcomeRefused},
		{"rate limited", 429, nil, errors.New("boom"), jobsources.EmployerOutcomeRefused},
		{"server error", 500, nil, errors.New("boom"), jobsources.EmployerOutcomeUnreadable},
		{"transport failure", 0, nil, errors.New("dial failed"), jobsources.EmployerOutcomeUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyOutcome(tc.status, tc.jobs, tc.err)
			if got != tc.want {
				t.Errorf("classifyOutcome(%d, %v jobs, %v) = %q, want %q", tc.status, len(tc.jobs), tc.err, got, tc.want)
			}
		})
	}
}
