package adapters

import (
	"encoding/json"
	"testing"

	"github.com/job-finder/api/internal/dto"
)

func TestRemotiveResponseParsing(t *testing.T) {
	tests := []struct {
		name      string
		response  remotiveResponse
		wantJobs  int
		wantTitle string
	}{
		{
			name: "valid response with jobs",
			response: remotiveResponse{
				Jobs: []remotiveJob{
					{
						ID:                        12345,
						Title:                     "Go Developer",
						CompanyName:               "Tech Corp",
						CandidateRequiredLocation: "Remote",
						URL:                       "https://remotive.com/job/12345",
						Description:               "<p>We are looking for a Go developer</p>",
						PublicationDate:           "2024-01-15T10:00:00Z",
						Salary:                    "$120k - $150k",
					},
				},
			},
			wantJobs:  1,
			wantTitle: "Go Developer",
		},
		{
			name: "empty jobs list",
			response: remotiveResponse{
				Jobs: []remotiveJob{},
			},
			wantJobs: 0,
		},
		{
			name: "multiple jobs",
			response: remotiveResponse{
				Jobs: []remotiveJob{
					{ID: 1, Title: "Job 1", CompanyName: "Company 1"},
					{ID: 2, Title: "Job 2", CompanyName: "Company 2"},
					{ID: 3, Title: "Job 3", CompanyName: "Company 3"},
				},
			},
			wantJobs:  3,
			wantTitle: "Job 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.response.Jobs) != tt.wantJobs {
				t.Errorf("expected %d jobs, got %d", tt.wantJobs, len(tt.response.Jobs))
			}
			if tt.wantJobs > 0 && tt.response.Jobs[0].Title != tt.wantTitle {
				t.Errorf("expected title %q, got %q", tt.wantTitle, tt.response.Jobs[0].Title)
			}
		})
	}
}

func TestRemotiveJobMapping(t *testing.T) {
	job := remotiveJob{
		ID:                        12345,
		Title:                     "Go Developer",
		CompanyName:               "Tech Corp",
		CandidateRequiredLocation: "Remote",
		URL:                       "https://remotive.com/job/12345",
		Description:               "<p>We are looking for a Go developer</p>",
		Salary:                    "$120k",
	}

	if job.ID != 12345 {
		t.Errorf("expected ID 12345, got %d", job.ID)
	}
	if job.Title != "Go Developer" {
		t.Errorf("expected title 'Go Developer', got %q", job.Title)
	}
	if job.CompanyName != "Tech Corp" {
		t.Errorf("expected company 'Tech Corp', got %q", job.CompanyName)
	}
	if job.CandidateRequiredLocation != "Remote" {
		t.Errorf("expected location 'Remote', got %q", job.CandidateRequiredLocation)
	}
}

func TestRemotiveJobToNormalized(t *testing.T) {
	job := remotiveJob{
		ID:                        12345,
		Title:                     "Go Developer",
		CompanyName:               "Tech Corp",
		CandidateRequiredLocation: "Remote",
		URL:                       "https://remotive.com/job/12345",
		Description:               "<p>We are looking for a Go developer</p>",
		Salary:                    "$120k",
	}

	normalized := dto.NormalizedJob{
		SourceKey:  "remotive",
		ExternalID: strPtr("12345"),
		Title:      job.Title,
		Company:    job.CompanyName,
		Location:   strPtr(job.CandidateRequiredLocation),
		Remote:     true,
		SalaryRaw:  strPtr(job.Salary),
		URL:        job.URL,
	}

	if normalized.SourceKey != "remotive" {
		t.Errorf("expected source 'remotive', got %q", normalized.SourceKey)
	}
	if normalized.ExternalID == nil || *normalized.ExternalID != "12345" {
		t.Error("expected external ID '12345'")
	}
	if !normalized.Remote {
		t.Error("expected remote to be true")
	}
}

func TestRemotiveResponseJSON(t *testing.T) {
	raw := `{"jobs":[{"id":1,"title":"Go Dev","company_name":"Acme","url":"http://x","description":"desc"}]}`
	var resp remotiveResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp.Jobs))
	}
	if resp.Jobs[0].CompanyName != "Acme" {
		t.Errorf("expected company 'Acme', got %q", resp.Jobs[0].CompanyName)
	}
}

func TestRemotiveKey(t *testing.T) {
	adapter := RemotiveAdapter{}
	if adapter.Key() != "remotive" {
		t.Errorf("expected key 'remotive', got %q", adapter.Key())
	}
}

func TestRemotiveKind(t *testing.T) {
	adapter := RemotiveAdapter{}
	if adapter.Kind() != dto.SourceKindAPI {
		t.Errorf("expected kind %q, got %q", dto.SourceKindAPI, adapter.Kind())
	}
}

func TestRemotiveHealthCheck(t *testing.T) {
	adapter := RemotiveAdapter{}
	_, err := adapter.HealthCheck(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func strPtr(s string) *string {
	return &s
}
