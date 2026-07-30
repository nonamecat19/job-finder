package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/platform/scraping"
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

func makeBasicSearchFixture(cardsHTML string) string {
	return `<html><body><ul class="list-jobs">` + cardsHTML + `</ul></body></html>`
}

func basicSearchCard(href, title string) string {
	return `<li class="list-jobs__item"><a href="` + href + `">` + title + `</a></li>`
}

func TestDjinniSearchBasicSearchSinglePage(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page := r.URL.Query().Get("page")
		if page == "" || page == "1" {
			_, _ = w.Write([]byte(makeBasicSearchFixture(
				basicSearchCard("/jobs/1", "Go Dev") + basicSearchCard("/jobs/2", "Rust Dev"),
			)))
			return
		}
		_, _ = w.Write([]byte(makeBasicSearchFixture("")))
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Golang"}
	jobs, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 cards, got %d", len(jobs))
	}
	if requests != 2 {
		t.Errorf("expected 2 fetches (page 1 + page 2), got %d", requests)
	}
}

func TestDjinniSearchBasicSearchMultiPage(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			_, _ = w.Write([]byte(makeBasicSearchFixture(
				basicSearchCard("/jobs/1", "Go Dev") + basicSearchCard("/jobs/2", "Rust Dev"),
			)))
		case "2":
			_, _ = w.Write([]byte(makeBasicSearchFixture(
				basicSearchCard("/jobs/3", "Python Dev") + basicSearchCard("/jobs/4", "JS Dev"),
			)))
		default:
			_, _ = w.Write([]byte(makeBasicSearchFixture("")))
		}
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Golang"}
	jobs, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(jobs) != 4 {
		t.Errorf("expected 4 cards (2 pages), got %d", len(jobs))
	}
	if requests != 3 {
		t.Errorf("expected 3 fetches (pages 1-3), got %d", requests)
	}
}

func TestDjinniSearchBasicSearchRedirectLoop(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(makeBasicSearchFixture(
			basicSearchCard("/jobs/1", "Go Dev") + basicSearchCard("/jobs/2", "Rust Dev"),
		)))
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Golang"}
	jobs, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 cards (only page 1, loop guard), got %d", len(jobs))
	}
	if requests != 2 {
		t.Errorf("expected 2 fetches (stopped after page 2 redirect-loop), got %d", requests)
	}
}

func TestDjinniSearchBasicSearchPreservesQueryParams(t *testing.T) {
	var reqURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqURL == "" {
			reqURL = r.URL.String()
		}
		_, _ = w.Write([]byte(makeBasicSearchFixture(
			basicSearchCard("/jobs/1", "Go Dev"),
		)))
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	subURL := srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote"
	query := dto.SearchQuery{SubscriptionURL: subURL}
	_, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	parsed, err := url.Parse("http://" + reqURL)
	if err != nil {
		t.Fatalf("could not parse request URL %q: %v", reqURL, err)
	}
	q := parsed.Query()
	if q.Get("search_type") != "basic-search" {
		t.Error("search_type missing or wrong")
	}
	if q.Get("primary_keyword") != "Node.js" {
		t.Error("primary_keyword mismatch")
	}
	if q.Get("salary") != "3000" {
		t.Error("salary mismatch")
	}
	if q.Get("employment") != "remote" {
		t.Error("employment mismatch")
	}
	levels := q["exp_level"]
	if len(levels) != 4 {
		t.Fatalf("expected 4 exp_level values, got %d: %v", len(levels), levels)
	}
	if levels[0] != "2y" || levels[1] != "3y" || levels[2] != "4y" || levels[3] != "5y" {
		t.Errorf("exp_level values mismatch: %v", levels)
	}
}

func TestDjinniSearchBasicSearchStripsPageParamFromSavedURL(t *testing.T) {
	var firstReqPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if firstReqPage == "" {
			firstReqPage = r.URL.Query().Get("page")
		}
		_, _ = w.Write([]byte(makeBasicSearchFixture(
			basicSearchCard("/jobs/1", "Go Dev"),
		)))
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Golang&page=4"}
	_, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if firstReqPage != "1" {
		t.Errorf("expected first fetch to use page=1, got page=%s", firstReqPage)
	}
}
