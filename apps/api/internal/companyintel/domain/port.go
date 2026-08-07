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

var ErrNoCompany = errors.New("companyintel: job has no parseable company name")

type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	GetCompanyByNormalizedName(ctx context.Context, normalizedName string) (sqlcgen.Company, error)
	UpsertCompany(ctx context.Context, arg sqlcgen.UpsertCompanyParams) (sqlcgen.Company, error)
	UpdateCompanyLastRefreshed(ctx context.Context, id pgtype.UUID) error
	GetCompanySignals(ctx context.Context, companyId pgtype.UUID) ([]sqlcgen.CompanySignal, error)
	GetCompanySignalByKind(ctx context.Context, arg sqlcgen.GetCompanySignalByKindParams) (sqlcgen.CompanySignal, error)
	UpsertCompanySignal(ctx context.Context, arg sqlcgen.UpsertCompanySignalParams) (sqlcgen.CompanySignal, error)
}

func NormalizeCompanyName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func DecodeString(raw []byte) (string, bool) {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	return v, true
}

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
