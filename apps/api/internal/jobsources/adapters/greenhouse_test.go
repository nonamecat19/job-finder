package adapters

import (
	"encoding/json"
	"testing"
)

func TestGreenhouseResponseJSON(t *testing.T) {
	raw := `{"jobs":[{"id":1,"title":"Go Engineer","absolute_url":"https://boards.greenhouse.io/acme/jobs/1","content":"<p>full text</p>","updated_at":"2026-01-01T00:00:00Z","location":{"name":"Remote"}}]}`
	var res greenhouseResponse
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(res.Jobs))
	}
	j := res.Jobs[0]
	if j.Title != "Go Engineer" || j.AbsURL == "" || j.Content == "" {
		t.Errorf("unexpected fields: %+v", j)
	}
	if j.Content == "" {
		t.Error("expected non-empty description — board postings arrive full (FR-004), no separate enrichment pass")
	}
}

func TestGreenhouseAdapterDoesNotNeedDetail(t *testing.T) {
	a := &GreenhouseAdapter{}
	if _, ok := any(a).(interface{ NeedsDetail() bool }); ok {
		t.Fatal("GreenhouseAdapter must not implement DetailNeeder — Search returns full descriptions (FR-004)")
	}
}

func TestGreenhouseKey(t *testing.T) {
	a := &GreenhouseAdapter{}
	if a.Key() != "greenhouse" {
		t.Errorf("expected key 'greenhouse', got %q", a.Key())
	}
}
