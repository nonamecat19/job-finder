package adapters

import (
	"encoding/json"
	"testing"
)

func TestLeverResponseJSON(t *testing.T) {
	raw := `[{"id":"abc123","text":"Go Engineer","hostedUrl":"https://jobs.lever.co/acme/abc123","createdAt":1700000000000,"categories":{"location":"Remote","commitment":"Full-time"},"descriptionPlain":"full text"}]`
	var res []leverPosting
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 posting, got %d", len(res))
	}
	if res[0].Text != "Go Engineer" || res[0].DescriptionPlain == "" {
		t.Errorf("unexpected fields: %+v", res[0])
	}
}

func TestLeverAdapterDoesNotNeedDetail(t *testing.T) {
	a := &LeverAdapter{}
	if _, ok := any(a).(interface{ NeedsDetail() bool }); ok {
		t.Fatal("LeverAdapter must not implement DetailNeeder — Search returns full descriptions (FR-004)")
	}
}

func TestLeverKey(t *testing.T) {
	a := &LeverAdapter{}
	if a.Key() != "lever" {
		t.Errorf("expected key 'lever', got %q", a.Key())
	}
}
