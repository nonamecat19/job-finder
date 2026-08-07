package adapters

import (
	"context"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/job-finder/api/internal/dto"
)

const douDetailFixtureHTML = `
<html><body>
<div class="b-vacancy">
	<div class="b-compinfo">
		<div class="info">
			<div class="l-n">
				<a href="https://jobs.dou.ua/companies/glovo/">Glovo</a>
				<a class="all-v" href="https://jobs.dou.ua/companies/glovo/vacancies/">Всі вакансії компанії</a>
			</div>
			<div class="l-t">Glovo — іспанська технологічна мультикатегорійна платформа.</div>
		</div>
	</div>
	<ul class="b-hr-admin-panel">
		<li class="breadcrumbs"><a href="#">Всі вакансії</a> / <a href="#">Analyst</a> / <a href="#">Київ</a></li>
	</ul>
	<div class="l-vacancy" itemprop="description">
		<div class="date">13 липня 2026</div>
		<h1 class="g-h2">Sr. Data Analyst (Supply Operations)</h1>
		<div class="sh-info">
			<span class="place bi bi-geo-alt-fill"> Київ</span>
			<span class="salary">від&nbsp;$3500</span>
		</div>
		<div class="b-typo vacancy-section">
			<h3>Твоя місія</h3><p>Аналіз та оптимізація операційних процесів.</p>
		</div>
	</div>
</div>
</body></html>
`

func TestParseDouDetailDoesNotConflateCompanyInfoWithVacancyMeta(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(douDetailFixtureHTML))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	job := parseDouDetail(doc, "https://jobs.dou.ua/companies/glovo/vacancies/361394/")

	if job.Title != "Sr. Data Analyst (Supply Operations)" {
		t.Errorf("title: got %q", job.Title)
	}
	if job.Company != "Glovo" {
		t.Errorf("company: got %q", job.Company)
	}
	if job.Location == nil || *job.Location != "Київ" {
		t.Errorf("location: got %v, want 'Київ' (not the company blurb)", job.Location)
	}
	if strings.Contains(job.Description, "іспанська технологічна") {
		t.Errorf("description leaked company blurb: %q", job.Description)
	}
	if !strings.Contains(job.Description, "Аналіз та оптимізація") {
		t.Errorf("description missing expected vacancy text: %q", job.Description)
	}
}

func TestDouKey(t *testing.T) {
	adapter := DouAdapter{}
	if adapter.Key() != "dou" {
		t.Errorf("expected key 'dou', got %q", adapter.Key())
	}
}

func TestDouKind(t *testing.T) {
	adapter := DouAdapter{}
	if adapter.Kind() != dto.SourceKindScrape {
		t.Errorf("expected kind %q, got %q", dto.SourceKindScrape, adapter.Kind())
	}
}

func TestDouHealthCheck(t *testing.T) {
	adapter := DouAdapter{}
	healthy, err := adapter.HealthCheck(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !healthy {
		t.Error("expected healthy to be true")
	}
}

func TestDouSearchWithoutSubscriptionURL(t *testing.T) {
	adapter := DouAdapter{}
	query := dto.SearchQuery{
		Keywords: "golang",
	}
	config := map[string]any{}
	_, err := adapter.Search(context.Background(), query, config)
	if err == nil {
		t.Error("expected error for keyword search")
	}
}
