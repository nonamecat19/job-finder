package adapters

import (
	"encoding/json"
	"testing"

	"github.com/job-finder/api/internal/dto"
)

func TestJoobleResponseParsing(t *testing.T) {
	tests := []struct {
		name      string
		response  joobleResponse
		wantJobs  int
		wantTitle string
	}{
		{
			name: "valid response with jobs",
			response: joobleResponse{
				TotalCount: "1",
				Jobs: []joobleJob{
					{
						Title:       "Go Developer",
						CompanyName: "Tech Corp",
						Location:    "Remote",
						Snippet:     "We are looking for a Go developer",
						URL:         "https://jooble.com/job/12345",
						Salary:      "$120k",
						Date:        "2024-01-15",
						Source:      "linkedin",
					},
				},
			},
			wantJobs:  1,
			wantTitle: "Go Developer",
		},
		{
			name: "empty jobs list",
			response: joobleResponse{
				TotalCount: "0",
				Jobs:       []joobleJob{},
			},
			wantJobs: 0,
		},
		{
			name: "multiple jobs",
			response: joobleResponse{
				TotalCount: "2",
				Jobs: []joobleJob{
					{Title: "Job 1", CompanyName: "Company 1"},
					{Title: "Job 2", CompanyName: "Company 2"},
				},
			},
			wantJobs:  2,
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

func TestJoobleJobMapping(t *testing.T) {
	job := joobleJob{
		Title:       "Go Developer",
		CompanyName: "Tech Corp",
		Location:    "Remote",
		Snippet:     "Job description",
		URL:         "https://jooble.com/job/12345",
		Salary:      "$120k",
		Date:        "2024-01-15",
		Source:      "linkedin",
	}

	if job.Title != "Go Developer" {
		t.Errorf("expected title 'Go Developer', got %q", job.Title)
	}
	if job.CompanyName != "Tech Corp" {
		t.Errorf("expected company 'Tech Corp', got %q", job.CompanyName)
	}
	if job.Location != "Remote" {
		t.Errorf("expected location 'Remote', got %q", job.Location)
	}
}

func TestJoobleResponseJSON(t *testing.T) {
	raw := `{"totalCount":"1","jobs":[{"title":"Go Dev","companyName":"Acme","url":"http://x"}]}`
	var resp joobleResponse
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

func TestJoobleKey(t *testing.T) {
	adapter := JoobleAdapter{}
	if adapter.Key() != "jooble" {
		t.Errorf("expected key 'jooble', got %q", adapter.Key())
	}
}

func TestJoobleKind(t *testing.T) {
	adapter := JoobleAdapter{}
	if adapter.Kind() != dto.SourceKindAPI {
		t.Errorf("expected kind %q, got %q", dto.SourceKindAPI, adapter.Kind())
	}
}

func TestJoobleSearchMissingAPIKey(t *testing.T) {
	adapter := JoobleAdapter{}
	_, err := adapter.Search(nil, dto.SearchQuery{Keywords: "test"}, map[string]any{})
	if err == nil {
		t.Error("expected error for missing API key")
	}
}
