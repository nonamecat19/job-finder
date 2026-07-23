package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/job-finder/api/internal/dto"
)

func TestJobSpyResponseParsing(t *testing.T) {
	tests := []struct {
		name      string
		response  jobspySearchResponse
		wantJobs  int
		wantTitle string
	}{
		{
			name: "valid response with jobs",
			response: jobspySearchResponse{
				Jobs: []jobspySidecarJob{
					{
						ID:          strPtr("12345"),
						Site:        "linkedin",
						Title:       "Go Developer",
						Company:     strPtr("Tech Corp"),
						Location:    strPtr("Remote"),
						IsRemote:    boolPtr(true),
						Salary:      strPtr("$120k"),
						JobURL:      "https://linkedin.com/job/12345",
						Description: strPtr("Job description"),
						DatePosted:  strPtr("2024-01-15"),
					},
				},
			},
			wantJobs:  1,
			wantTitle: "Go Developer",
		},
		{
			name: "empty jobs list",
			response: jobspySearchResponse{
				Jobs: []jobspySidecarJob{},
			},
			wantJobs: 0,
		},
		{
			name: "multiple jobs",
			response: jobspySearchResponse{
				Jobs: []jobspySidecarJob{
					{Title: "Job 1", Site: "linkedin"},
					{Title: "Job 2", Site: "indeed"},
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

func TestJobSpyJobMapping(t *testing.T) {
	job := jobspySidecarJob{
		ID:          strPtr("12345"),
		Site:        "linkedin",
		Title:       "Go Developer",
		Company:     strPtr("Tech Corp"),
		Location:    strPtr("Remote"),
		IsRemote:    boolPtr(true),
		Salary:      strPtr("$120k"),
		JobURL:      "https://linkedin.com/job/12345",
		Description: strPtr("Job description"),
	}

	if job.Site != "linkedin" {
		t.Errorf("expected site 'linkedin', got %q", job.Site)
	}
	if job.Title != "Go Developer" {
		t.Errorf("expected title 'Go Developer', got %q", job.Title)
	}
	if job.Company == nil || *job.Company != "Tech Corp" {
		t.Error("expected company 'Tech Corp'")
	}
}

func TestJobSpySearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := jobspySearchResponse{
			Jobs: []jobspySidecarJob{
				{
					ID:          strPtr("1"),
					Site:        "linkedin",
					Title:       "Go Developer",
					Company:     strPtr("Tech Corp"),
					Location:    strPtr("Remote"),
					IsRemote:    boolPtr(true),
					JobURL:      "https://example.com/job/1",
					Description: strPtr("Job description"),
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	adapter := JobSpyAdapter{}
	query := dto.SearchQuery{
		Keywords: "golang",
		Site:     strPtr("linkedin"),
		Remote:   boolPtr(true),
	}

	config := map[string]any{
		"url": server.URL,
	}

	jobs, err := adapter.Search(context.Background(), query, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	if jobs[0].Title != "Go Developer" {
		t.Errorf("expected title 'Go Developer', got %q", jobs[0].Title)
	}
}

func TestJobSpyResponseJSON(t *testing.T) {
	raw := `{"jobs":[{"site":"linkedin","title":"Go Dev","company":"Acme","job_url":"http://x"}]}`
	var resp jobspySearchResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp.Jobs))
	}
	if resp.Jobs[0].Company == nil || *resp.Jobs[0].Company != "Acme" {
		t.Errorf("expected company 'Acme'")
	}
}

func TestJobSpyKey(t *testing.T) {
	adapter := JobSpyAdapter{}
	if adapter.Key() != "jobspy" {
		t.Errorf("expected key 'jobspy', got %q", adapter.Key())
	}
}

func TestJobSpyKind(t *testing.T) {
	adapter := JobSpyAdapter{}
	if adapter.Kind() != dto.SourceKindSidecar {
		t.Errorf("expected kind %q, got %q", dto.SourceKindSidecar, adapter.Kind())
	}
}

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
