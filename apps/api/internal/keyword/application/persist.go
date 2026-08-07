package application

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/keyword/domain"
)

const DefaultDiffModel = "keyword-diff-v1"

type DiffWriter interface {
	UpsertKeywordDiff(ctx context.Context, arg sqlcgen.UpsertKeywordDiffParams) (sqlcgen.KeywordDiff, error)
}

func Persist(ctx context.Context, w DiffWriter, jobID pgtype.UUID, res *domain.DiffResult, model string) (sqlcgen.KeywordDiff, error) {
	if res == nil {
		return sqlcgen.KeywordDiff{}, fmt.Errorf("keyword: nil diff result")
	}
	if model == "" {
		model = DefaultDiffModel
	}

	matched, err := json.Marshal(res.Matched)
	if err != nil {
		return sqlcgen.KeywordDiff{}, fmt.Errorf("keyword: marshal matched: %w", err)
	}
	missingRequired, err := json.Marshal(res.MissingRequired)
	if err != nil {
		return sqlcgen.KeywordDiff{}, fmt.Errorf("keyword: marshal missingRequired: %w", err)
	}
	missingPreferred, err := json.Marshal(res.MissingPreferred)
	if err != nil {
		return sqlcgen.KeywordDiff{}, fmt.Errorf("keyword: marshal missingPreferred: %w", err)
	}

	coverage := res.Metadata.CoveragePct
	return w.UpsertKeywordDiff(ctx, sqlcgen.UpsertKeywordDiffParams{
		JobId:            jobID,
		Matched:          matched,
		MissingRequired:  missingRequired,
		MissingPreferred: missingPreferred,
		CoveragePct:      &coverage,
		Model:            model,
	})
}
