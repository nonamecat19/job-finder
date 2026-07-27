package retrieval

import (
	"testing"
	"time"
)

func TestPageOutcome(t *testing.T) {
	o := PageOutcome{
		Status: PageRead,
		Method: "direct",
		URL:    "https://example.com/foo",
	}
	if o.Status != "read" {
		t.Errorf("expected read, got %s", o.Status)
	}
}

func TestRunVerdict(t *testing.T) {
	if VerdictSuccess != "success" {
		t.Error("VerdictSuccess should be 'success'")
	}
	if VerdictPartial != "partial" {
		t.Error("VerdictPartial should be 'partial'")
	}
	if VerdictBlocked != "blocked" {
		t.Error("VerdictBlocked should be 'blocked'")
	}
}

func TestRungForKey(t *testing.T) {
	r, ok := RungForKey("direct")
	if !ok || r.Order != 0 {
		t.Errorf("direct rung: ok=%v order=%d", ok, r.Order)
	}
	r, ok = RungForKey("browser")
	if !ok || r.Order != 1 {
		t.Errorf("browser rung: ok=%v order=%d", ok, r.Order)
	}
	r, ok = RungForKey("flaresolverr")
	if !ok || r.Order != 2 {
		t.Errorf("flaresolverr rung: ok=%v order=%d", ok, r.Order)
	}
	r, ok = RungForKey("nonexistent")
	if ok {
		t.Error("nonexistent rung should not be found")
	}
}

func TestRetrievalMethodNext(t *testing.T) {
	tests := []struct {
		key      string
		wantNext string
		wantOk   bool
	}{
		{"direct", "browser", true},
		{"browser", "flaresolverr", true},
		{"flaresolverr", "", false},
	}
	for _, tt := range tests {
		r, _ := RungForKey(tt.key)
		next, ok := r.Next()
		if ok != tt.wantOk {
			t.Errorf("%s.Next() ok=%v, want %v", tt.key, ok, tt.wantOk)
		}
		if ok && next.Key != tt.wantNext {
			t.Errorf("%s.Next().Key=%s, want %s", tt.key, next.Key, tt.wantNext)
		}
	}
}

func TestRetrievalMethodAvailable(t *testing.T) {
	r, _ := RungForKey("direct")
	if !r.Available() {
		t.Error("direct should be available")
	}
	r, _ = RungForKey("browser")
	if !r.Available() {
		t.Error("browser should be available")
	}
	r, _ = RungForKey("flaresolverr")
	if !r.Available() {
		t.Error("flaresolverr should be available")
	}
}

func TestFetchRequest(t *testing.T) {
	req := FetchRequest{
		URL:             "https://example.com",
		Headers:         map[string]string{"Accept": "text/html"},
		UsesUserAccount: true,
		RefererPage:     "https://google.com",
	}
	if req.URL != "https://example.com" {
		t.Error("URL not set")
	}
	if !req.UsesUserAccount {
		t.Error("UsesUserAccount should be true")
	}
}

func TestHostStatus(t *testing.T) {
	now := time.Now()
	cd := 2
	s := HostStatus{
		Host:              "example.com",
		IdentityVersion:   "v1",
		CurrentRung:       "direct",
		LastBlockAt:       &now,
		LastBlockReason:   "403",
		CrawlDelaySeconds: &cd,
		Pacing:            HostPacing{RequestsPerSecond: 0.2, IntervalSeconds: 5, Source: "site-requested"},
	}
	if s.Host != "example.com" {
		t.Error("Host not set")
	}
	if s.CurrentRung != "direct" {
		t.Error("CurrentRung not set")
	}
	if s.Pacing.Source != "site-requested" {
		t.Error("Pacing.Source not set")
	}
	if s.Pacing.RequestsPerSecond != 0.2 {
		t.Error("Pacing.RequestsPerSecond not set")
	}
	if s.CrawlDelaySeconds == nil || *s.CrawlDelaySeconds != 2 {
		t.Error("CrawlDelaySeconds not set correctly")
	}
}
