package salary

import (
	"testing"
)

func TestParseSalaryRaw(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantOk   bool
		wantMin  int
		wantMax  int
		wantCurr string
	}{
		{name: "$120k-$150k", raw: "$120k-$150k", wantOk: true, wantMin: 120000, wantMax: 150000, wantCurr: "USD"},
		{name: "$120,000 - 150,000", raw: "$120,000 - 150,000", wantOk: true, wantMin: 120000, wantMax: 150000, wantCurr: "USD"},
		{name: "€80-90k", raw: "€80-90k", wantOk: true, wantMin: 80000, wantMax: 90000, wantCurr: "EUR"},
		{name: "£60k", raw: "£60k", wantOk: true, wantMin: 60000, wantMax: 60000, wantCurr: "GBP"},
		{name: "₴30-40k", raw: "₴30-40k", wantOk: true, wantMin: 30000, wantMax: 40000, wantCurr: "UAH"},
		{name: "UAH 40000-60000", raw: "UAH 40000-60000", wantOk: true, wantMin: 40000, wantMax: 60000, wantCurr: "UAH"},
		{name: "USD 100k-130k", raw: "USD 100k-130k", wantOk: true, wantMin: 100000, wantMax: 130000, wantCurr: "USD"},
		{name: "up to $90k", raw: "up to $90k", wantOk: true, wantMin: 0, wantMax: 90000, wantCurr: "USD"},
		{name: "от 100 000 грн", raw: "от 100 000 грн", wantOk: false},
		{name: "PLN 10 000 - 15 000", raw: "PLN 10 000 - 15 000", wantOk: true, wantMin: 10000, wantMax: 15000, wantCurr: "PLN"},
		{name: "CAD 80k-100k", raw: "CAD 80k-100k", wantOk: true, wantMin: 80000, wantMax: 100000, wantCurr: "CAD"},
		{name: "AUD 120000-150000", raw: "AUD 120000-150000", wantOk: true, wantMin: 120000, wantMax: 150000, wantCurr: "AUD"},
		{name: "EUR 50k", raw: "EUR 50k", wantOk: true, wantMin: 50000, wantMax: 50000, wantCurr: "EUR"},
		{name: "empty", raw: "", wantOk: false},
		{name: "no currency", raw: "100k-150k", wantOk: false},
		{name: "garbage", raw: "competitive salary", wantOk: false},
		{name: "$120k to $150k", raw: "$120k to $150k", wantOk: true, wantMin: 120000, wantMax: 150000, wantCurr: "USD"},
		{name: "€80k–€90k (en dash)", raw: "€80k–€90k", wantOk: true, wantMin: 80000, wantMax: 90000, wantCurr: "EUR"},
		{name: "USD 100k — 130k (em dash)", raw: "USD 100k — 130k", wantOk: true, wantMin: 100000, wantMax: 130000, wantCurr: "USD"},
		{name: "до 100 000 грн", raw: "до 100 000 грн", wantOk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			band, ok := ParseSalaryRaw(tt.raw)
			if ok != tt.wantOk {
				t.Errorf("ParseSalaryRaw(%q) ok = %v, want %v", tt.raw, ok, tt.wantOk)
				return
			}
			if !ok {
				return
			}
			if band.Min != tt.wantMin {
				t.Errorf("ParseSalaryRaw(%q) min = %d, want %d", tt.raw, band.Min, tt.wantMin)
			}
			if band.Max != tt.wantMax {
				t.Errorf("ParseSalaryRaw(%q) max = %d, want %d", tt.raw, band.Max, tt.wantMax)
			}
			if band.Currency != tt.wantCurr {
				t.Errorf("ParseSalaryRaw(%q) currency = %s, want %s", tt.raw, band.Currency, tt.wantCurr)
			}
			if band.Confidence != 0.5 {
				t.Errorf("ParseSalaryRaw(%q) confidence = %f, want 0.5", tt.raw, band.Confidence)
			}
			if band.Source != SourceIngestedCache {
				t.Errorf("ParseSalaryRaw(%q) source = %s, want %s", tt.raw, band.Source, SourceIngestedCache)
			}
		})
	}
}

func TestSalaryBandValidate(t *testing.T) {
	tests := []struct {
		name    string
		band    SalaryBand
		wantErr bool
	}{
		{name: "valid", band: SalaryBand{Min: 50000, Max: 100000, Currency: "USD", Confidence: 0.5}, wantErr: false},
		{name: "negative min", band: SalaryBand{Min: -1, Max: 100000, Currency: "USD", Confidence: 0.5}, wantErr: true},
		{name: "negative max", band: SalaryBand{Min: 50000, Max: -1, Currency: "USD", Confidence: 0.5}, wantErr: true},
		{name: "min > max", band: SalaryBand{Min: 100000, Max: 50000, Currency: "USD", Confidence: 0.5}, wantErr: true},
		{name: "confidence > 1", band: SalaryBand{Min: 50000, Max: 100000, Currency: "USD", Confidence: 1.5}, wantErr: true},
		{name: "confidence < 0", band: SalaryBand{Min: 50000, Max: 100000, Currency: "USD", Confidence: -0.1}, wantErr: true},
		{name: "empty currency", band: SalaryBand{Min: 50000, Max: 100000, Currency: "", Confidence: 0.5}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.band.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBlend(t *testing.T) {
	a := SalaryBand{Min: 100000, Max: 150000, Currency: "USD", Confidence: 0.7, Source: SourceIngestedCache}
	b := SalaryBand{Min: 120000, Max: 160000, Currency: "USD", Confidence: 0.6, Source: SourceLevelsFyi}

	result := blend(a, b)

	if result.Source != SourceBlended {
		t.Errorf("blend source = %s, want %s", result.Source, SourceBlended)
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
