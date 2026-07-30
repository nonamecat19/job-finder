package adapters

import (
	"encoding/json"
	"testing"
)

func TestSmartRecruitersResponseJSON(t *testing.T) {
	raw := `{"content":[{"id":"p1","name":"Go Engineer","releasedDate":"2026-01-01","ref":"https://jobs.smartrecruiters.com/Acme/p1","location":{"city":"Remote","country":"","remote":true},"jobAd":{"sections":{"jobDescription":{"text":"full text"}}}}]}`
	var res smartRecruitersResponse
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 posting, got %d", len(res.Content))
	}
	if res.Content[0].Name != "Go Engineer" || res.Content[0].JobAd.Sections.JobDescription.Text == "" {
		t.Errorf("unexpected fields: %+v", res.Content[0])
	}
}

func TestSmartRecruitersAdapterDoesNotNeedDetail(t *testing.T) {
	a := &SmartRecruitersAdapter{}
	if _, ok := any(a).(interface{ NeedsDetail() bool }); ok {
		t.Fatal("SmartRecruitersAdapter must not implement DetailNeeder — Search returns full descriptions (FR-004)")
	}
}

func TestSmartRecruitersKey(t *testing.T) {
	a := &SmartRecruitersAdapter{}
	if a.Key() != "smartrecruiters" {
		t.Errorf("expected key 'smartrecruiters', got %q", a.Key())
	}
}
