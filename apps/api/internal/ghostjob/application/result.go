package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/ghostjob/domain"
)

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}

type GhostCompletedResult struct {
	Score       float64  `json:"score"`
	Confidence  float64  `json:"confidence"`
	Explanation string   `json:"explanation"`
	TopSignals  []string `json:"topSignals"`
}

func NewResultHandler(tx TxRunner, model string) events.ResultHandler {
	return func(ctx context.Context, envelope events.Envelope, result events.Result) error {
		return tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
			disposition, err := events.Admit(ctx, q, Kind, envelope.IdempotencyKey, envelope.RunID)
			if err != nil {
				return fmt.Errorf("ghostjob: admit result: %w", err)
			}
			if disposition != events.Accepted {
				return nil
			}

			if result.Status != events.ResultSucceeded {
				msg := ""
				if result.Failure != nil {
					msg = result.Failure.Message
				}
				slog.Error("ghostjob: python scoring failed", "job", envelope.WorkID, "trace_id", envelope.TraceID, "error", msg)
				return nil
			}

			var scored GhostCompletedResult
			if err := json.Unmarshal(result.Result, &scored); err != nil {
				return fmt.Errorf("ghostjob: unmarshal ghost.completed result: %w", err)
			}

			uid, err := dbutil.ParseUUID(envelope.WorkID)
			if err != nil {
				return fmt.Errorf("ghostjob: persist result: %w", err)
			}

			var signals domain.GhostSignals
			if job, err := q.GetJobByID(ctx, uid); err == nil {
				signals = MeasureSignals(ctx, q, job)
			}

			breakdown := dto.GhostSignalBreakdownDto{
				RepostCount:       signals.RepostCount,
				DaysOpen:          signals.DaysOpen,
				CrossBoardCount:   signals.CrossBoardCount,
				AlwaysHiringCount: signals.AlwaysHiringCount,
				Confidence:        scored.Confidence,
				Explanation:       scored.Explanation,
				TopSignals:        scored.TopSignals,
				Notes:             signals.Notes,
			}
			raw, err := dbutil.MarshalJSONB(breakdown)
			if err != nil {
				return fmt.Errorf("ghostjob: marshal signals: %w", err)
			}

			score := int32(math.Round(scored.Score))
			m := model
			if _, err := q.UpsertJobSignal(ctx, sqlcgen.UpsertJobSignalParams{
				JobId:   uid,
				Kind:    Kind,
				Score:   score,
				Signals: raw,
				Model:   &m,
			}); err != nil {
				return fmt.Errorf("ghostjob: upsert: %w", err)
			}
			return nil
		})
	}
}
