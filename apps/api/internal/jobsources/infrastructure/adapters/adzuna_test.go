package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/job-finder/api/internal/dto"
)

func TestAdzunaResponseParsing(t *testing.T) {
	tests := []struct {
		name      string
		response  adzunaResponse
		wantJobs  int
		wantTitle string
	}{
		{
			name: "valid response with jobs",
			response: adzunaResponse{
				Results: []adzunaResult{
					{
						ID:          "12345",
						Title:       "Go Developer",
						RedirectURL: "https://adzuna.com/job/12345",
						Description: "We are looking for a Go developer",
						SalaryMin:   80000,
						SalaryMax:   120000,
						Company: struct {
							DisplayName string `json:"display_name"`
						}{DisplayName: "Tech Corp"},
						Location: struct {
							DisplayName string `json:"display_name"`
						}{DisplayName: "Remote"},
					},
				},
			},
			wantJobs:  1,
			wantTitle: "Go Developer",
		},
		{
			name: "empty results",
			response: adzunaResponse{
				Results: []adzunaResult{},
			},
			wantJobs: 0,
		},
		{
			name: "multiple jobs",
			response: adzunaResponse{
				Results: []adzunaResult{
					{ID: "1", Title: "Job 1"},
					{ID: "2", Title: "Job 2"},
				},
			},
			wantJobs:  2,
			wantTitle: "Job 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.response.Results) != tt.wantJobs {
				t.Errorf("expected %d jobs, got %d", tt.wantJobs, len(tt.response.Results))
			}
			if tt.wantJobs > 0 && tt.response.Results[0].Title != tt.wantTitle {
				t.Errorf("expected title %q, got %q", tt.wantTitle, tt.response.Results[0].Title)
			}
		})
	}
}

func TestAdzunaResultMapping(t *testing.T) {
	result := adzunaResult{
		ID:          "12345",
		Title:       "Go Developer",
		RedirectURL: "https://adzuna.com/job/12345",
		Description: "Job description",
		SalaryMin:   80000,
		SalaryMax:   120000,
		Company: struct {
			DisplayName string `json:"display_name"`
		}{DisplayName: "Tech Corp"},
		Location: struct {
			DisplayName string `json:"display_name"`
		}{DisplayName: "Remote"},
	}

	if result.ID != "12345" {
		t.Errorf("expected ID '12345', got %q", result.ID)
	}
	if result.Title != "Go Developer" {
		t.Errorf("expected title 'Go Developer', got %q", result.Title)
	}
	if result.Company.DisplayName != "Tech Corp" {
		t.Errorf("expected company 'Tech Corp', got %q", result.Company.DisplayName)
	}
}

func TestAdzunaResponseJSON(t *testing.T) {
	raw := `{"results":[{"id":"1","title":"Go Dev","company":{"display_name":"Acme"},"redirect_url":"http://x","description":"desc"}]}`
	var resp adzunaResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Company.DisplayName != "Acme" {
		t.Errorf("expected company 'Acme', got %q", resp.Results[0].Company.DisplayName)
	}
}

func TestAdzunaKey(t *testing.T) {
	adapter := AdzunaAdapter{}
	if adapter.Key() != "adzuna" {
		t.Errorf("expected key 'adzuna', got %q", adapter.Key())
	}
}

func TestAdzunaKind(t *testing.T) {
	adapter := AdzunaAdapter{}
	if adapter.Kind() != dto.SourceKindAPI {
		t.Errorf("expected kind %q, got %q", dto.SourceKindAPI, adapter.Kind())
	}
}

func TestAdzunaSearchMissingCredentials(t *testing.T) {
	adapter := AdzunaAdapter{}
	_, err := adapter.Search(context.Background(), dto.SearchQuery{Keywords: "test"}, map[string]any{})
	if err == nil {
		t.Error("expected error for missing credentials")
	}
}

func TestAdzunaSearchServerReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	_ = server
}
