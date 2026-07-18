package adapters

import (
	"context"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/scraping"
)

func TestDjinniKey(t *testing.T) {
	adapter := DjinniAdapter{}
	if adapter.Key() != "djinni" {
		t.Errorf("expected key 'djinni', got %q", adapter.Key())
	}
}

func TestDjinniKind(t *testing.T) {
	adapter := DjinniAdapter{}
	if adapter.Kind() != dto.SourceKindScrape {
		t.Errorf("expected kind %q, got %q", dto.SourceKindScrape, adapter.Kind())
	}
}

func TestDjinniSubscriptionMissingCookie(t *testing.T) {
	t.Setenv("DJINNI_SESSION_COOKIE", "")
	adapter := DjinniAdapter{Scraping: scraping.New()}
	query := dto.SearchQuery{SubscriptionURL: "https://djinni.co/my/dashboard/subs/996917/"}

	_, err := adapter.Search(context.Background(), query, map[string]any{})
	if err == nil {
		t.Fatal("expected error for subscription with no session cookie")
	}
	if !strings.Contains(err.Error(), "login session") {
		t.Errorf("expected a clear session-cookie error, got %q", err.Error())
	}
}

func TestDjinniIsLoginPage(t *testing.T) {
	login, _ := goquery.NewDocumentFromReader(strings.NewReader(
		`<html><body><form action="/login"><input name="email"><input name="password" type="password"></form></body></html>`))
	if !djinniIsLoginPage(login) {
		t.Error("expected login page to be detected")
	}

	listing, _ := goquery.NewDocumentFromReader(strings.NewReader(
		`<html><body><li id="job-item-1"><a href="/jobs/123">Go Dev</a></li></body></html>`))
	if djinniIsLoginPage(listing) {
		t.Error("job listing must not be detected as a login page")
	}
}

func TestDjinniSearchEmptyConfig(t *testing.T) {
	svc := scraping.New()
	adapter := DjinniAdapter{Scraping: svc}
	query := dto.SearchQuery{
		Keywords: "golang",
	}
	_, err := adapter.Search(nil, query, map[string]any{})
	if err == nil {
		t.Error("expected error for missing session cookie / unreachable host")
	}
}
