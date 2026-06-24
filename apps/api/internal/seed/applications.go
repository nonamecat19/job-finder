package seed

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

type applicationSeed struct {
	jobUUID string
	status  string
	events  []map[string]any
}

var applications = []applicationSeed{
	{
		jobUUID: uuidJob1, status: "applied",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(12)},
		},
	},
	{
		jobUUID: uuidJob2, status: "interviewing",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(15)},
			{"type": "status_changed", "to": "applied", "ts": daysAgo(14)},
			{"type": "status_changed", "to": "interviewing", "ts": daysAgo(7)},
		},
	},
	{
		jobUUID: uuidJob3, status: "offered",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(20)},
			{"type": "status_changed", "to": "applied", "ts": daysAgo(19)},
			{"type": "status_changed", "to": "interviewing", "ts": daysAgo(12)},
			{"type": "status_changed", "to": "offered", "ts": daysAgo(3)},
		},
	},
	{
		jobUUID: uuidJob4, status: "applied",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(5)},
		},
	},
	{
		jobUUID: uuidJob6, status: "rejected",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(25)},
			{"type": "status_changed", "to": "applied", "ts": daysAgo(24)},
			{"type": "status_changed", "to": "interviewing", "ts": daysAgo(18)},
			{"type": "status_changed", "to": "rejected", "ts": daysAgo(10)},
		},
	},
	{
		jobUUID: uuidJob7, status: "applied",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(8)},
		},
	},
	{
		jobUUID: uuidJob10, status: "applied",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(3)},
		},
	},
	{
		jobUUID: uuidJob12, status: "interviewing",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(10)},
			{"type": "status_changed", "to": "applied", "ts": daysAgo(9)},
			{"type": "status_changed", "to": "interviewing", "ts": daysAgo(5)},
		},
	},
	{
		jobUUID: uuidJob15, status: "applied",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(2)},
		},
	},
	{
		jobUUID: uuidJob18, status: "applied",
		events: []map[string]any{
			{"type": "created", "ts": daysAgo(1)},
		},
	},
}

func seedApplications(ctx context.Context, pool *pgxpool.Pool, q *sqlcgen.Queries) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM "Application"`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		slog.Info("seed: applications already exist, skipping")
		return nil
	}

	for _, a := range applications {
		eventsJSON, err := json.Marshal(a.events)
		if err != nil {
			return err
		}
		jobID, err := dbutil.ParseUUID(a.jobUUID)
		if err != nil {
			return err
		}
		if err := q.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
			JobId:  jobID,
			Status: a.status,
			Events: eventsJSON,
		}); err != nil {
			return err
		}
	}

	slog.Info("seed: created applications", "count", len(applications))
	return nil
}
