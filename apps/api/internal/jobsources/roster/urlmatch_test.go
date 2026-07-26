package roster

import "testing"

func TestMatchVendor(t *testing.T) {
	cases := []struct {
		name         string
		url          string
		wantVendor   string
		wantEmployer string
		wantOK       bool
	}{
		{"greenhouse", "https://boards.greenhouse.io/acme", "greenhouse", "acme", true},
		{"greenhouse job-boards host", "https://job-boards.greenhouse.io/acme/jobs/123", "greenhouse", "acme", true},
		{"lever", "https://jobs.lever.co/acme", "lever", "acme", true},
		{"lever with posting", "https://jobs.lever.co/acme/1234-5678", "lever", "acme", true},
		{"ashby", "https://jobs.ashbyhq.com/acme", "ashby", "acme", true},
		{"workable", "https://apply.workable.com/acme", "workable", "acme", true},
		{"workable with locale prefix", "https://apply.workable.com/j/acme", "workable", "acme", true},
		{"smartrecruiters careers host", "https://careers.smartrecruiters.com/Acme", "smartrecruiters", "Acme", true},
		{"smartrecruiters jobs host", "https://jobs.smartrecruiters.com/Acme", "smartrecruiters", "Acme", true},
		{"unsupported vendor", "https://example.com/careers/acme", "", "", false},
		{"not a url", "not a url at all", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vendor, employer, ok := MatchVendor(tc.url)
			if ok != tc.wantOK {
				t.Fatalf("MatchVendor(%q) ok = %v, want %v", tc.url, ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if vendor != tc.wantVendor {
				t.Errorf("MatchVendor(%q) vendor = %q, want %q", tc.url, vendor, tc.wantVendor)
			}
			if employer != tc.wantEmployer {
				t.Errorf("MatchVendor(%q) employer = %q, want %q", tc.url, employer, tc.wantEmployer)
			}
		})
	}
}
