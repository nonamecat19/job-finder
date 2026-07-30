package adapters

import (
	"errors"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/domain"
)

func TestClassifyOutcome(t *testing.T) {
	cases := []struct {
		name   string
		status int
		jobs   []dto.NormalizedJob
		err    error
		want   domain.EmployerOutcome
	}{
		{"success with postings", 200, []dto.NormalizedJob{{}}, nil, domain.EmployerOutcomeRead},
		{"success no postings", 200, nil, nil, domain.EmployerOutcomeNoPostings},
		{"not found", 404, nil, errors.New("boom"), domain.EmployerOutcomeNotFound},
		{"unauthorized", 401, nil, errors.New("boom"), domain.EmployerOutcomeRefused},
		{"forbidden", 403, nil, errors.New("boom"), domain.EmployerOutcomeRefused},
		{"rate limited", 429, nil, errors.New("boom"), domain.EmployerOutcomeRefused},
		{"server error", 500, nil, errors.New("boom"), domain.EmployerOutcomeUnreadable},
		{"transport failure", 0, nil, errors.New("dial failed"), domain.EmployerOutcomeUnreadable},
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
