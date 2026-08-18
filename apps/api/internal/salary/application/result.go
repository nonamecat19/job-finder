package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/events"
)

// TxRunner is the subset of *db.DB a ResultHandler needs: run fn inside one
// transaction so the idempotency ledger write and the persisted result
// commit atomically, mirroring ghostjob.TxRunner.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}

// SalaryCompletedResult is the capability-specific shape salary.completed
// carries in Result.Result: the same fields domain.SalaryBand validates in
// the Go path (min/max/currency/confidence/source), so a python-inferred
// band lands in the same shape a Go-inferred one does.
type SalaryCompletedResult struct {
	Min        int     `json:"min"`
	Max        int     `json:"max"`
	Currency   string  `json:"currency"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

// NewResultHandler builds the events.ResultHandler for salary.completed,
// persisting a python-produced band through the same UpdateJobSalary write
// the LLM branch of Infer uses today — no cache write, matching Infer's LLM
// path (only the salaryRaw-parse branch upserts salary_cache). It admits the
// result into the idempotency ledger and persists in the same transaction: a
// duplicate or superseded result is a no-op, never a second write or an
// overwrite of a later attempt's result.
func NewResultHandler(tx TxRunner) events.ResultHandler {
	return func(ctx context.Context, envelope events.Envelope, result events.Result) error {
		return tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
			disposition, err := events.Admit(ctx, q, Kind, envelope.IdempotencyKey, envelope.RunID)
			if err != nil {
				return fmt.Errorf("salary: admit result: %w", err)
			}
			if disposition != events.Accepted {
				return nil
			}

			if result.Status != events.ResultSucceeded {
				msg := ""
				if result.Failure != nil {
					msg = result.Failure.Message
				}
				slog.Error("salary: python inference failed", "job", envelope.WorkID, "trace_id", envelope.TraceID, "error", msg)
				return nil
			}

			var band SalaryCompletedResult
			if err := json.Unmarshal(result.Result, &band); err != nil {
				return fmt.Errorf("salary: unmarshal salary.completed result: %w", err)
			}

			uid, err := dbutil.ParseUUID(envelope.WorkID)
			if err != nil {
				return fmt.Errorf("salary: persist result: %w", err)
			}

			confidence := band.Confidence
			if confidence == 0 {
				confidence = 0.3
			}
			currency := band.Currency
			source := "llm"
			min32 := int32(band.Min)
			max32 := int32(band.Max)
			if err := q.UpdateJobSalary(ctx, sqlcgen.UpdateJobSalaryParams{
				ID:               uid,
				SalaryMin:        &min32,
				SalaryMax:        &max32,
				SalaryCurrency:   &currency,
				SalaryConfidence: &confidence,
				SalarySource:     &source,
			}); err != nil {
				return fmt.Errorf("salary: upsert: %w", err)
			}
			return nil
		})
	}
}
