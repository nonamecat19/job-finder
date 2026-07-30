package adapters

import (
	"encoding/json"
	"testing"
)

func TestAshbyResponseJSON(t *testing.T) {
	raw := `{"jobs":[{"id":"j1","title":"Go Engineer","location":"Remote","isRemote":true,"jobUrl":"https://jobs.ashbyhq.com/acme/j1","descriptionHtml":"<p>full text</p>","publishedAt":"2026-01-01T00:00:00Z"}]}`
	var res ashbyResponse
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(res.Jobs))
	}
	if res.Jobs[0].Title != "Go Engineer" || res.Jobs[0].DescriptionHTML == "" {
		t.Errorf("unexpected fields: %+v", res.Jobs[0])
	}
}

func TestAshbyAdapterDoesNotNeedDetail(t *testing.T) {
	a := &AshbyAdapter{}
	if _, ok := any(a).(interface{ NeedsDetail() bool }); ok {
		t.Fatal("AshbyAdapter must not implement DetailNeeder — Search returns full descriptions (FR-004)")
	}
}

func TestAshbyKey(t *testing.T) {
	a := &AshbyAdapter{}
	if a.Key() != "ashby" {
		t.Errorf("expected key 'ashby', got %q", a.Key())
	}
}
