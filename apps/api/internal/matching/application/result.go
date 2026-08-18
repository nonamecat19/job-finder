package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/events"
)

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}

type AutoGenerateGate interface {
	ShouldGenerate(score int) bool
}

type Generator interface {
	EnqueueGeneration(ctx context.Context, id, docType string, profileID *string) (map[string]any, error)
}

type Notifier interface {
	MaybeNotify(ctx context.Context, jobID, matchResultID string, score int)
}

type MatchCompletedResult struct {
	Score         float64  `json:"score"`
	MatchedSkills []string `json:"matchedSkills"`
	MissingSkills []string `json:"missingSkills"`
	Summary       string   `json:"summary"`
	RedFlags      []string `json:"redFlags"`
}

func NewResultHandler(tx TxRunner, notif Notifier, autogen AutoGenerateGate, generator Generator, model string) events.ResultHandler {
	return func(ctx context.Context, envelope events.Envelope, result events.Result) error {
		var matchResultID string
		var score *int32

		err := tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
			disposition, err := events.Admit(ctx, q, Kind, envelope.IdempotencyKey, envelope.RunID)
			if err != nil {
				return fmt.Errorf("matching: admit result: %w", err)
			}
			if disposition != events.Accepted {
				return nil
			}

			if result.Status != events.ResultSucceeded {
				msg := ""
				if result.Failure != nil {
					msg = result.Failure.Message
				}
				slog.Error("matching: python fit analysis failed", "job", envelope.WorkID, "trace_id", envelope.TraceID, "error", msg)
				return nil
			}

			var fit MatchCompletedResult
			if err := json.Unmarshal(result.Result, &fit); err != nil {
				return fmt.Errorf("matching: unmarshal match.completed result: %w", err)
			}

			uid, err := dbutil.ParseUUID(envelope.WorkID)
			if err != nil {
				return fmt.Errorf("matching: persist result: %w", err)
			}

			var similarity float64
			if existing, err := q.GetMatchResultByJobID(ctx, uid); err == nil {
				similarity = existing.Similarity
			}

			matchedJSON, err := jsonOrNull(fit.MatchedSkills)
			if err != nil {
				return err
			}
			missingJSON, err := jsonOrNull(fit.MissingSkills)
			if err != nil {
				return err
			}
			redFlagsJSON, err := jsonOrNull(fit.RedFlags)
			if err != nil {
				return err
			}

			s := int32(math.Round(fit.Score))
			summary := fit.Summary
			row, err := q.UpsertMatchResult(ctx, sqlcgen.UpsertMatchResultParams{
				JobId:         uid,
				Similarity:    similarity,
				Score:         &s,
				MatchedSkills: matchedJSON,
				MissingSkills: missingJSON,
				Summary:       &summary,
				RedFlags:      redFlagsJSON,
				Model:         model,
			})
			if err != nil {
				return fmt.Errorf("matching: upsert: %w", err)
			}

			matchResultID = dbutil.UUIDString(row.ID)
			score = row.Score
			return nil
		})
		if err != nil || score == nil {
			return err
		}

		if notif != nil {
			notif.MaybeNotify(ctx, envelope.WorkID, matchResultID, int(*score))
		}
		if autogen != nil && generator != nil && autogen.ShouldGenerate(int(*score)) {
			if _, err := generator.EnqueueGeneration(ctx, envelope.WorkID, "resume", nil); err != nil {
				slog.Warn("matching: auto-generate skipped", "jobId", envelope.WorkID, "score", *score, "error", err)
			}
		}
		return nil
	}
}
