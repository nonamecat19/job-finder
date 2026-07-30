package application

import (
	"testing"

	"github.com/job-finder/api/internal/salary/domain"
)

func TestBlend(t *testing.T) {
	a := domain.SalaryBand{Min: 100000, Max: 150000, Currency: "USD", Confidence: 0.7, Source: domain.SourceIngestedCache}
	b := domain.SalaryBand{Min: 120000, Max: 160000, Currency: "USD", Confidence: 0.6, Source: domain.SourceLevelsFyi}

	result := blend(a, b)

	if result.Source != domain.SourceBlended {
		t.Errorf("blend source = %s, want %s", result.Source, domain.SourceBlended)
	}
	if result.Currency != "USD" {
		t.Errorf("blend currency = %s, want USD", result.Currency)
	}
	if result.Confidence > 1.0 {
		t.Errorf("blend confidence = %f, want <= 1.0", result.Confidence)
	}
	if result.Min <= 0 || result.Max <= 0 {
		t.Errorf("blend min/max should be positive, got %d/%d", result.Min, result.Max)
	}
}

func TestMakeBucket(t *testing.T) {
	b := makeBucket("Senior Backend Engineer", "London, UK", "51-200")
	if b != "senior-backend-engineer|london-uk|51-200" {
		t.Errorf("makeBucket = %s", b)
	}

	b2 := makeBucket("DevOps", "", "")
	if b2 != "devops||unknown" {
		t.Errorf("makeBucket empty = %s", b2)
	}
}
