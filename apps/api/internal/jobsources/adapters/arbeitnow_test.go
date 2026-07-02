package adapters

import (
	"encoding/json"
	"testing"

	"github.com/job-finder/api/internal/dto"
)

func TestArbeitnowResponseParsing(t *testing.T) {
	tests := []struct {
		name      string
		response  arbeitnowResponse
		wantJobs  int
		wantTitle string
	}{
		{
			name: "valid response with jobs",
			response: arbeitnowResponse{
				Data: []arbeitnowJob{
					{
						Slug:        "go-developer",
						CompanyName: "Tech Corp",
						Title:       "Go Developer",
						Description: "We are looking for a Go developer",
						Remote:      true,
						URL:         "https://arbeitnow.com/job/go-developer",
						Tags:        []string{"go", "backend"},
						Location:    "Remote",
					},
				},
			},
			wantJobs:  1,
			wantTitle: "Go Developer",
		},
		{
			name: "empty data",
			response: arbeitnowResponse{
				Data: []arbeitnowJob{},
			},
			wantJobs: 0,
		},
		{
			name: "multiple jobs",
			response: arbeitnowResponse{
				Data: []arbeitnowJob{
					{Slug: "job-1", Title: "Job 1", CompanyName: "Company 1"},
					{Slug: "job-2", Title: "Job 2", CompanyName: "Company 2"},
				},
			},
			wantJobs:  2,
			wantTitle: "Job 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.response.Data) != tt.wantJobs {
				t.Errorf("expected %d jobs, got %d", tt.wantJobs, len(tt.response.Data))
			}
			if tt.wantJobs > 0 && tt.response.Data[0].Title != tt.wantTitle {
				t.Errorf("expected title %q, got %q", tt.wantTitle, tt.response.Data[0].Title)
			}
		})
	}
}

func TestArbeitnowJobMapping(t *testing.T) {
	job := arbeitnowJob{
		Slug:        "go-developer",
		CompanyName: "Tech Corp",
		Title:       "Go Developer",
		Description: "Job description",
		Remote:      true,
		URL:         "https://arbeitnow.com/job/go-developer",
		Tags:        []string{"go", "backend"},
		Location:    "Remote",
	}

	if job.Slug != "go-developer" {
		t.Errorf("expected slug 'go-developer', got %q", job.Slug)
	}
	if job.Title != "Go Developer" {
		t.Errorf("expected title 'Go Developer', got %q", job.Title)
	}
	if !job.Remote {
		t.Error("expected remote to be true")
	}
}

func TestArbeitnowResponseJSON(t *testing.T) {
	raw := `{"data":[{"slug":"go-dev","company_name":"Acme","title":"Go Dev","url":"http://x"}]}`
	var resp arbeitnowResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp.Data))
	}
	if resp.Data[0].CompanyName != "Acme" {
		t.Errorf("expected company 'Acme', got %q", resp.Data[0].CompanyName)
	}
}

func TestArbeitnowKey(t *testing.T) {
	adapter := ArbeitnowAdapter{}
	if adapter.Key() != "arbeitnow" {
		t.Errorf("expected key 'arbeitnow', got %q", adapter.Key())
	}
}

func TestArbeitnowKind(t *testing.T) {
	adapter := ArbeitnowAdapter{}
	if adapter.Kind() != dto.SourceKindAPI {
		t.Errorf("expected kind %q, got %q", dto.SourceKindAPI, adapter.Kind())
	}
}

func TestArbeitnowHealthCheck(t *testing.T) {
	adapter := ArbeitnowAdapter{}
	_, err := adapter.HealthCheck(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
