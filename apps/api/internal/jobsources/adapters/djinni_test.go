package adapters

import (
	"testing"

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
