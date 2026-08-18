//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type manualFeedFixture struct {
	manualDjinni sqlcgen.Subscription
	manualDou    sqlcgen.Subscription
	crawlDjinni  sqlcgen.Subscription
	jobs         map[string]sqlcgen.Job
}

func seedManualFeed(t *testing.T, ctx context.Context) manualFeedFixture {
	t.Helper()
	truncateAll(t)

	mustInsertJobSource(t, "djinni", "scrape")
	mustInsertJobSource(t, "dou", "scrape")

	f := manualFeedFixture{jobs: map[string]sqlcgen.Job{}}
	var err error
	f.manualDjinni, err = testDB.Queries.EnsureManualSubscription(ctx, sqlcgen.EnsureManualSubscriptionParams{
		SourceKey: "djinni", Cron: "0 */6 * * *",
	})
	if err != nil {
		t.Fatalf("ensure manual djinni: %v", err)
	}
	f.manualDou, err = testDB.Queries.EnsureManualSubscription(ctx, sqlcgen.EnsureManualSubscriptionParams{
		SourceKey: "dou", Cron: "0 */6 * * *",
	})
	if err != nil {
		t.Fatalf("ensure manual dou: %v", err)
	}
	f.crawlDjinni, err = testDB.Queries.CreateSubscription(ctx, sqlcgen.CreateSubscriptionParams{
		SourceKey: "djinni", Name: strPtr("Saved search"), Url: "https://djinni.co/jobs/?search_type=basic-search",
		Enabled: true, Cron: "0 */6 * * *", Kind: "crawl",
	})
	if err != nil {
		t.Fatalf("create crawl subscription: %v", err)
	}

	f.jobs["manual-djinni"] = attachJob(t, ctx, "djinni", "manual-djinni", "Manual Djinni role", f.manualDjinni.ID)
	f.jobs["manual-dou"] = attachJob(t, ctx, "dou", "manual-dou", "Manual DOU role", f.manualDou.ID)
	f.jobs["crawled"] = attachJob(t, ctx, "djinni", "crawled", "Crawled role", f.crawlDjinni.ID)
	return f
}

func attachJob(t *testing.T, ctx context.Context, sourceKey, dedupeKey, title string, subID pgtype.UUID) sqlcgen.Job {
	t.Helper()
	job := mustInsertJob(t, sourceKey, dedupeKey, title)
	if _, err := testDB.Pool.Exec(ctx, `UPDATE "Job" SET "subscriptionId" = $2 WHERE "id" = $1`, job.ID, subID); err != nil {
		t.Fatalf("attach job to subscription: %v", err)
	}
	return job
}

func listByScore(t *testing.T, ctx context.Context, onlyManual *bool) []sqlcgen.ListJobsByScoreRow {
	t.Helper()
	rows, err := testDB.Queries.ListJobsByScore(ctx, sqlcgen.ListJobsByScoreParams{
		OnlyManual: onlyManual, Limit: 50,
	})
	if err != nil {
		t.Fatalf("list jobs by score: %v", err)
	}
	return rows
}

func TestIntegration_OnlyManualSpansEverySource(t *testing.T) {
	ctx := context.Background()
	seedManualFeed(t, ctx)

	onlyManual := true
	rows := listByScore(t, ctx, &onlyManual)

	titles := map[string]bool{}
	for _, row := range rows {
		titles[row.Title] = true
	}
	if !titles["Manual Djinni role"] || !titles["Manual DOU role"] {
		t.Errorf("expected manual adds from every source, got %v", titles)
	}
	if titles["Crawled role"] {
		t.Error("expected the crawled vacancy to be excluded")
	}

	count, err := testDB.Queries.CountJobs(ctx, sqlcgen.CountJobsParams{OnlyManual: &onlyManual})
	if err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 — the total must agree with the page", count)
	}
}

func TestIntegration_ManualAddsSurfaceFirstUnderScoreSortForTwentyFourHours(t *testing.T) {
	ctx := context.Background()
	f := seedManualFeed(t, ctx)

	mustScore(t, ctx, f.jobs["crawled"].ID, 95)
	mustScore(t, ctx, f.jobs["manual-djinni"].ID, 10)

	rows := listByScore(t, ctx, nil)
	if len(rows) == 0 || rows[0].Title != "Manual Djinni role" {
		t.Fatalf("expected the fresh manual add to surface first under sort=score, got %v", titlesOf(rows))
	}

	byDate, err := testDB.Queries.ListJobsByDate(ctx, sqlcgen.ListJobsByDateParams{Limit: 50})
	if err != nil {
		t.Fatalf("list jobs by date: %v", err)
	}
	if len(byDate) == 0 {
		t.Fatal("expected rows")
	}

	if byDate[0].Title != "Crawled role" {
		t.Errorf("sort=date must be pure recency, got %v", byDateTitles(byDate))
	}

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE "Job" SET "ingestedAt" = now() - interval '25 hours' WHERE "id" = $1`,
		f.jobs["manual-djinni"].ID); err != nil {
		t.Fatalf("age the manual vacancy: %v", err)
	}
	rows = listByScore(t, ctx, nil)
	if rows[0].Title == "Manual Djinni role" {
		t.Error("expected the surfacing boost to expire at 24 hours")
	}
}

func TestIntegration_ManualAddPostedAtIsNeverTheAddTime(t *testing.T) {
	ctx := context.Background()
	f := seedManualFeed(t, ctx)

	posted := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	for _, key := range []string{"manual-djinni", "crawled"} {
		if _, err := testDB.Pool.Exec(ctx,
			`UPDATE "Job" SET "postedAt" = $2 WHERE "id" = $1`, f.jobs[key].ID, posted); err != nil {
			t.Fatalf("set postedAt: %v", err)
		}
	}

	manual, err := testDB.Queries.GetJobByID(ctx, f.jobs["manual-djinni"].ID)
	if err != nil {
		t.Fatalf("get manual job: %v", err)
	}
	crawled, err := testDB.Queries.GetJobByID(ctx, f.jobs["crawled"].ID)
	if err != nil {
		t.Fatalf("get crawled job: %v", err)
	}
	if !manual.PostedAt.Valid || !crawled.PostedAt.Valid {
		t.Fatal("expected both to carry the posting's own date")
	}
	if !manual.PostedAt.Time.Equal(crawled.PostedAt.Time) {
		t.Errorf("manual %v != crawled %v — age-sensitive readers must see the same date",
			manual.PostedAt.Time, crawled.PostedAt.Time)
	}
	if manual.PostedAt.Time.Equal(manual.IngestedAt.Time) {
		t.Error("postedAt must never be defaulted to the add time")
	}
}

func TestIntegration_ManualSubscriptionStats(t *testing.T) {
	ctx := context.Background()
	f := seedManualFeed(t, ctx)

	source, err := testDB.Queries.GetJobSourceByKey(ctx, "djinni")
	if err != nil {
		t.Fatalf("get job source: %v", err)
	}

	for _, added := range []int32{1, 1} {
		run, err := testDB.Queries.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
			SourceId: source.ID, SubscriptionId: f.manualDjinni.ID, Trigger: "manual",
		})
		if err != nil {
			t.Fatalf("insert run: %v", err)
		}
		if err := testDB.Queries.FinishSourceRunOk(ctx, sqlcgen.FinishSourceRunOkParams{
			ID: run.ID, Found: 1, New: added,
		}); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}
	failed, err := testDB.Queries.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
		SourceId: source.ID, SubscriptionId: f.manualDjinni.ID, Trigger: "manual",
	})
	if err != nil {
		t.Fatalf("insert failed run: %v", err)
	}
	msg := "unreachable"
	if err := testDB.Queries.FinishSourceRunError(ctx, sqlcgen.FinishSourceRunErrorParams{
		ID: failed.ID, Error: &msg,
	}); err != nil {
		t.Fatalf("finish failed run: %v", err)
	}

	stats, err := testDB.Queries.ManualSubscriptionStats(ctx, f.manualDjinni.ID)
	if err != nil {
		t.Fatalf("manual subscription stats: %v", err)
	}
	if stats.Added != 2 {
		t.Errorf("added = %d, want 2 — a failed attempt adds nothing", stats.Added)
	}
	if !stats.LastAddedAt.Valid {
		t.Error("expected a most-recent-addition timestamp")
	}

	empty, err := testDB.Queries.ManualSubscriptionStats(ctx, f.manualDou.ID)
	if err != nil {
		t.Fatalf("manual subscription stats (empty): %v", err)
	}
	if empty.Added != 0 || empty.LastAddedAt.Valid {
		t.Errorf("expected zero and no timestamp, got %+v", empty)
	}
}

func mustScore(t *testing.T, ctx context.Context, jobID pgtype.UUID, score int32) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`INSERT INTO "MatchResult" ("jobId", "similarity", "score", "model") VALUES ($1, 0.5, $2, 'test')`,
		jobID, score); err != nil {
		t.Fatalf("score job: %v", err)
	}
}

func titlesOf(rows []sqlcgen.ListJobsByScoreRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}

func byDateTitles(rows []sqlcgen.ListJobsByDateRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}
