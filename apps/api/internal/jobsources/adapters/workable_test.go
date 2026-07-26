package adapters

import (
	"encoding/json"
	"testing"
)

func TestWorkableResponseJSON(t *testing.T) {
	raw := `{"jobs":[{"shortcode":"ABC123","title":"Go Engineer","description":"full text","url":"https://apply.workable.com/acme/j/ABC123","remote":true,"created_at":"2026-01-01","location":{"city":"Remote","country_name":""}}]}`
	var res workableResponse
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(res.Jobs))
	}
	if res.Jobs[0].Title != "Go Engineer" || res.Jobs[0].Description == "" {
		t.Errorf("unexpected fields: %+v", res.Jobs[0])
	}
}

func TestWorkableAdapterDoesNotNeedDetail(t *testing.T) {
	a := &WorkableAdapter{}
	if _, ok := any(a).(interface{ NeedsDetail() bool }); ok {
		t.Fatal("WorkableAdapter must not implement DetailNeeder — Search returns full descriptions (FR-004)")
	}
}

func TestWorkableKey(t *testing.T) {
	a := &WorkableAdapter{}
	if a.Key() != "workable" {
		t.Errorf("expected key 'workable', got %q", a.Key())
	}
}
