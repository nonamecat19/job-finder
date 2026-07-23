package domain

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// ErrNoCompany is returned when the job's company name is empty/whitespace
// — the card is hidden entirely per FR-014, so there is nothing to probe.
var ErrNoCompany = errors.New("companyintel: job has no parseable company name")

// Repository is the outbound persistence port for the companyintel
// use-case. *sqlcgen.Queries satisfies it structurally.
type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	GetCompanyByNormalizedName(ctx context.Context, normalizedName string) (sqlcgen.Company, error)
	UpsertCompany(ctx context.Context, arg sqlcgen.UpsertCompanyParams) (sqlcgen.Company, error)
	UpdateCompanyLastRefreshed(ctx context.Context, id pgtype.UUID) error
	GetCompanySignals(ctx context.Context, companyId pgtype.UUID) ([]sqlcgen.CompanySignal, error)
	GetCompanySignalByKind(ctx context.Context, arg sqlcgen.GetCompanySignalByKindParams) (sqlcgen.CompanySignal, error)
	UpsertCompanySignal(ctx context.Context, arg sqlcgen.UpsertCompanySignalParams) (sqlcgen.CompanySignal, error)
}

// NormalizeCompanyName lowercases and trims a job's company field —
// the sole company-identity join key (spec.md "Company name is the join
// key"). Returns "" for an empty/whitespace-only name, which callers treat
// as "no company" (FR-014).
func NormalizeCompanyName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// DecodeString unmarshals a jsonb-encoded string column value.
func DecodeString(raw []byte) (string, bool) {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	return v, true
}

// ParseLeadingInt extracts the leading integer from a previously-stored
// headcount value string (e.g. "350 employees (baseline captured)" -> 350)
// so the next refresh can compute a trend.
func ParseLeadingInt(raw []byte) (int, bool) {
	s, ok := DecodeString(raw)
	if !ok {
		return 0, false
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(strings.ReplaceAll(fields[0], ",", ""))
	if err != nil {
		return 0, false
	}
	return n, true
}
