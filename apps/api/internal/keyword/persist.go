package keyword

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

// DefaultDiffModel labels rows produced by the deterministic (non-LLM) engine
// so the cache can be invalidated when the algorithm changes.
const DefaultDiffModel = "keyword-diff-v1"

// DiffWriter is the outbound persistence port for a computed diff. The sqlc
// *Queries type satisfies it structurally, so the wiring layer injects the
// production DB and tests can inject a fake without a database.
type DiffWriter interface {
	UpsertKeywordDiff(ctx context.Context, arg sqlcgen.UpsertKeywordDiffParams) (sqlcgen.KeywordDiff, error)
}

// Persist writes a DiffResult into the KeywordDiff cache row (008-2) for jobID,
// upserting on the unique jobId so re-running the diff refreshes in place. The
// model label defaults to DefaultDiffModel when empty.
func Persist(ctx context.Context, w DiffWriter, jobID pgtype.UUID, res *DiffResult, model string) (sqlcgen.KeywordDiff, error) {
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
