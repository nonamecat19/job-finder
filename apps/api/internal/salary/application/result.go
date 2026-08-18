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

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}

type SalaryCompletedResult struct {
	Min        int     `json:"min"`
	Max        int     `json:"max"`
	Currency   string  `json:"currency"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

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
