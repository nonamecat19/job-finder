package domain

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	UpdateJobSalary(ctx context.Context, arg sqlcgen.UpdateJobSalaryParams) error
	UpsertSalaryCache(ctx context.Context, arg sqlcgen.UpsertSalaryCacheParams) error
	GetSalaryCacheByBucket(ctx context.Context, bucket string) ([]sqlcgen.SalaryCache, error)
}

type SalarySource string

const (
	SourceLLM           SalarySource = "llm"
	SourceLevelsFyi     SalarySource = "levels-fyi"
	SourceIngestedCache SalarySource = "ingested-cache"
	SourceBlended       SalarySource = "blended"
)

type SalaryBand struct {
	Min        int          `json:"min" jsonschema:"description=Minimum annual salary in the currency units"`
	Max        int          `json:"max" jsonschema:"description=Maximum annual salary in the currency units"`
	Currency   string       `json:"currency" jsonschema:"description=ISO 4217 currency code,enum=USD,enum=EUR,enum=GBP,enum=UAH,enum=PLN,enum=CAD,enum=AUD"`
	Confidence float64      `json:"confidence" jsonschema:"description=Confidence 0-1,minimum=0,maximum=1"`
	Source     SalarySource `json:"source" jsonschema:"description=Source of the salary data,enum=llm,enum=levels-fyi,enum=ingested-cache,enum=blended"`
}

func (b SalaryBand) Validate() error {
	if b.Min < 0 {
		return fmt.Errorf("min must be >= 0, got %d", b.Min)
	}
	if b.Max < 0 {
		return fmt.Errorf("max must be >= 0, got %d", b.Max)
	}
	if b.Min > b.Max {
		return fmt.Errorf("min (%d) must be <= max (%d)", b.Min, b.Max)
	}
	if b.Confidence < 0 || b.Confidence > 1 {
		return fmt.Errorf("confidence must be 0-1, got %f", b.Confidence)
	}
	if b.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	return nil
}

type BlendedBand struct {
	Bands       []SalaryBand `json:"bands" jsonschema:"description=Source bands that contributed to the blend"`
	Final       SalaryBand   `json:"final" jsonschema:"description=Weighted result band"`
	Confidence  float64      `json:"confidence" jsonschema:"description=Normalized confidence 0-1,minimum=0,maximum=1"`
	Explanation string       `json:"explanation" jsonschema:"description=How the blend was computed"`
}

var (
	currencySymbols = map[string]string{
		"$":  "USD",
		"€":  "EUR",
		"£":  "GBP",
		"₴":  "UAH",
		"zł": "PLN",
	}

	currencyCodes = map[string]string{
		"USD": "USD",
		"EUR": "EUR",
		"GBP": "GBP",
		"UAH": "UAH",
		"PLN": "PLN",
		"CAD": "CAD",
		"AUD": "AUD",
	}

	reRange = regexp.MustCompile(
		`(?i)(?:USD|EUR|GBP|UAH|PLN|CAD|AUD|[$€£₴])\s*` +
			`(\d[\d\s,]*)\s*(?:k|K|000)?\s*` +
			`(?:-|–|—|to|до)\s*` +
			`(?:USD|EUR|GBP|UAH|PLN|CAD|AUD|[$€£₴])?\s*` +
			`(\d[\d\s,]*)\s*(k|K|000)?`,
	)

	reSingle = regexp.MustCompile(
		`(?i)(?:USD|EUR|GBP|UAH|PLN|CAD|AUD|[$€£₴])\s*` +
			`(\d[\d\s,]*)\s*(k|K|000)?`,
	)

	reUpTo = regexp.MustCompile(
		`(?i)(?:up\s*to|до)\s*` +
			`(?:USD|EUR|GBP|UAH|PLN|CAD|AUD|[$€£₴])?\s*` +
			`(\d[\d\s,]*)\s*(k|K|000)?`,
	)

	reCurrencyPrefix = regexp.MustCompile(`(?i)^(USD|EUR|GBP|UAH|PLN|CAD|AUD)\s+`)
	reSymbolPrefix   = regexp.MustCompile(`^([$€£₴])\s*`)
	reSymbolAnywhere = regexp.MustCompile(`([$€£₴])`)
	reZloty          = regexp.MustCompile(`(\d[\d\s,]*)\s*zł`)
)

func ParseSalaryRaw(raw string) (SalaryBand, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SalaryBand{}, false
	}

	currency := detectCurrency(raw)
	if currency == "" {
		return SalaryBand{}, false
	}

	if band, ok := parseRange(raw, currency); ok {
		return band, true
	}
	if band, ok := parseUpTo(raw, currency); ok {
		return band, true
	}
	if band, ok := parseSingle(raw, currency); ok {
		return band, true
	}

	return SalaryBand{}, false
}

func detectCurrency(raw string) string {
	if m := reCurrencyPrefix.FindStringSubmatch(raw); m != nil {
		if c, ok := currencyCodes[strings.ToUpper(m[1])]; ok {
			return c
		}
	}
	if m := reSymbolPrefix.FindStringSubmatch(raw); m != nil {
		if c, ok := currencySymbols[m[1]]; ok {
			return c
		}
	}
	if m := reSymbolAnywhere.FindStringSubmatch(raw); m != nil {
		if c, ok := currencySymbols[m[1]]; ok {
			return c
		}
	}
	if reZloty.MatchString(raw) {
		return "PLN"
	}
	return ""
}

func parseRange(raw, currency string) (SalaryBand, bool) {
	m := reRange.FindStringSubmatch(raw)
	if m == nil {
		return SalaryBand{}, false
	}
	lo := parseNumber(m[1])
	hi := parseNumber(m[2])
	if lo <= 0 || hi <= 0 {
		return SalaryBand{}, false
	}
	loMult := 1
	hiMult := 1
	if strings.EqualFold(m[2], "k") || strings.EqualFold(m[2], "K") {
		hiMult = 1000
	}
	if strings.EqualFold(m[3], "k") || strings.EqualFold(m[3], "K") || strings.EqualFold(m[3], "000") {
		hiMult = 1000
	}
	if lo < 1000 && hi < 1000 {
		loMult = 1000
		hiMult = 1000
	}
	lo = lo * loMult
	hi = hi * hiMult
	if lo > hi {
		lo, hi = hi, lo
	}
	return SalaryBand{
		Min:        lo,
		Max:        hi,
		Currency:   currency,
		Confidence: 0.5,
		Source:     SourceIngestedCache,
	}, true
}

func parseUpTo(raw, currency string) (SalaryBand, bool) {
	m := reUpTo.FindStringSubmatch(raw)
	if m == nil {
		return SalaryBand{}, false
	}
	val := parseNumber(m[1])
	if val <= 0 {
		return SalaryBand{}, false
	}
	mult := 1
	if strings.EqualFold(m[2], "k") || strings.EqualFold(m[2], "K") || strings.EqualFold(m[2], "000") {
		mult = 1000
	}
	if val < 1000 {
		mult = 1000
	}
	val = val * mult
	return SalaryBand{
		Min:        0,
		Max:        val,
		Currency:   currency,
		Confidence: 0.5,
		Source:     SourceIngestedCache,
	}, true
}

func parseSingle(raw, currency string) (SalaryBand, bool) {
	m := reSingle.FindStringSubmatch(raw)
	if m == nil {
		return SalaryBand{}, false
	}
	val := parseNumber(m[1])
	if val <= 0 {
		return SalaryBand{}, false
	}
	mult := 1
	if strings.EqualFold(m[2], "k") || strings.EqualFold(m[2], "K") || strings.EqualFold(m[2], "000") {
		mult = 1000
	}
	if val < 1000 {
		mult = 1000
	}
	val = val * mult
	return SalaryBand{
		Min:        val,
		Max:        val,
		Currency:   currency,
		Confidence: 0.5,
		Source:     SourceIngestedCache,
	}, true
}

func parseNumber(s string) int {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ",", "")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
