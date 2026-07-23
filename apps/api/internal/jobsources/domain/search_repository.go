package domain

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

// SearchRepository is the outbound persistence port for the SavedSearch/
// SourceRun/Job ingestion use-cases (application.SearchService, the
// interfaces/worker Handler and Scheduler). *sqlcgen.Queries satisfies it
// structurally. It embeds activity.Store because the worker handler
// starts/resumes an activity record from the same value (activity.New,
// activity.FromID).
type SearchRepository interface {
	activity.Store

	CreateSavedSearch(ctx context.Context, arg sqlcgen.CreateSavedSearchParams) (sqlcgen.SavedSearch, error)
	DeleteActivityRunsBefore(ctx context.Context, createdat pgtype.Timestamp) error
	DeleteSavedSearch(ctx context.Context, id pgtype.UUID) error
	FinishSourceRunError(ctx context.Context, arg sqlcgen.FinishSourceRunErrorParams) error
	FinishSourceRunOk(ctx context.Context, arg sqlcgen.FinishSourceRunOkParams) error
	GetJobByDedupeKey(ctx context.Context, dedupekey string) (pgtype.UUID, error)
	GetSavedSearch(ctx context.Context, id pgtype.UUID) (sqlcgen.SavedSearch, error)
	GetSubscription(ctx context.Context, id pgtype.UUID) (sqlcgen.Subscription, error)
	InsertJob(ctx context.Context, arg sqlcgen.InsertJobParams) (sqlcgen.Job, error)
	InsertSourceRun(ctx context.Context, arg sqlcgen.InsertSourceRunParams) (sqlcgen.SourceRun, error)
	RecordJobRepost(ctx context.Context, dedupekey string) (sqlcgen.Job, error)
	ListEnabledSavedSearches(ctx context.Context) ([]sqlcgen.SavedSearch, error)
	ListSavedSearches(ctx context.Context) ([]sqlcgen.SavedSearch, error)
	ListSubscriptions(ctx context.Context) ([]sqlcgen.Subscription, error)
	RecentRunsJoined(ctx context.Context, limit int32) ([]sqlcgen.RecentRunsJoinedRow, error)
	RecentSourceRunsForSource(ctx context.Context, arg sqlcgen.RecentSourceRunsForSourceParams) ([]*bool, error)
	SetJobSourceHealthy(ctx context.Context, arg sqlcgen.SetJobSourceHealthyParams) error
	TouchSavedSearchLastRun(ctx context.Context, id pgtype.UUID) error
	TouchSubscriptionLastRun(ctx context.Context, id pgtype.UUID) error
	UpdateSavedSearch(ctx context.Context, arg sqlcgen.UpdateSavedSearchParams) (sqlcgen.SavedSearch, error)
}
