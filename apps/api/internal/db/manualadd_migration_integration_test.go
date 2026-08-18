//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

func TestIntegration_Migration00041_ExistingRowsDefaultToCrawlAndScheduled(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	source := mustInsertJobSource(t, "js-041-defaults", "api")

	var subID string
	if err := testDB.Pool.QueryRow(ctx,
		`INSERT INTO "Subscription" ("sourceKey", "name", "url", "enabled") VALUES ($1, 'Legacy', 'https://example.com/jobs', true) RETURNING "id"`,
		source.Key).Scan(&subID); err != nil {
		t.Fatalf("insert legacy subscription: %v", err)
	}

	var kind string
	if err := testDB.Pool.QueryRow(ctx, `SELECT "kind" FROM "Subscription" WHERE "id" = $1`, subID).Scan(&kind); err != nil {
		t.Fatalf("read kind: %v", err)
	}
	if kind != "crawl" {
		t.Errorf("kind = %q, want crawl — an existing subscription is a crawling one", kind)
	}

	var runID string
	if err := testDB.Pool.QueryRow(ctx,
		`INSERT INTO "SourceRun" ("sourceId") VALUES ($1) RETURNING "id"`, source.ID).Scan(&runID); err != nil {
		t.Fatalf("insert legacy source run: %v", err)
	}

	var trigger string
	var subscriptionID *string
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT "trigger", "subscriptionId"::text FROM "SourceRun" WHERE "id" = $1`, runID).Scan(&trigger, &subscriptionID); err != nil {
		t.Fatalf("read trigger: %v", err)
	}
	if trigger != "scheduled" {
		t.Errorf("trigger = %q, want scheduled", trigger)
	}
	if subscriptionID != nil {
		t.Errorf("expected an unattributed run to have a null subscriptionId, got %v", *subscriptionID)
	}
}

func TestIntegration_Migration00041_ManualJobSourceIsIdempotent(t *testing.T) {
	ctx := context.Background()

	for range 2 {
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO "JobSource" ("key", "kind", "enabled", "config", "healthy")
			 VALUES ('manual', 'manual', true, '{}'::jsonb, true) ON CONFLICT ("key") DO NOTHING`); err != nil {
			t.Fatalf("re-apply manual job source insert: %v", err)
		}
	}

	var count int
	if err := testDB.Pool.QueryRow(ctx, `SELECT count(*) FROM "JobSource" WHERE "key" = 'manual'`).Scan(&count); err != nil {
		t.Fatalf("count manual job source: %v", err)
	}
	if count != 1 {
		t.Errorf("manual JobSource rows = %d, want exactly 1", count)
	}
}

func TestIntegration_Migration00041_OneManualSubscriptionPerSource(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	source := mustInsertJobSource(t, "js-041-unique", "api")

	first, err := testDB.Queries.EnsureManualSubscription(ctx, sqlcgen.EnsureManualSubscriptionParams{
		SourceKey: source.Key, Cron: "0 */6 * * *",
	})
	if err != nil {
		t.Fatalf("ensure manual subscription: %v", err)
	}
	second, err := testDB.Queries.EnsureManualSubscription(ctx, sqlcgen.EnsureManualSubscriptionParams{
		SourceKey: source.Key, Cron: "0 */6 * * *",
	})
	if err != nil {
		t.Fatalf("ensure manual subscription (second): %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected one manual row per source, got %v and %v", first.ID, second.ID)
	}

	if _, err := testDB.Queries.CreateSubscription(ctx, sqlcgen.CreateSubscriptionParams{
		SourceKey: source.Key, Name: strPtr("Saved search"), Url: "https://example.com/jobs",
		Enabled: true, Cron: "0 */6 * * *", Kind: "crawl",
	}); err != nil {
		t.Fatalf("expected the partial index to leave crawl rows alone: %v", err)
	}
}

func TestIntegration_Migration00041_ManualRowsAreInvisibleToSchedulerAndHealth(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	source := mustInsertJobSource(t, "js-041-hidden", "api")

	manual, err := testDB.Queries.EnsureManualSubscription(ctx, sqlcgen.EnsureManualSubscriptionParams{
		SourceKey: source.Key, Cron: "0 */6 * * *",
	})
	if err != nil {
		t.Fatalf("ensure manual subscription: %v", err)
	}

	enabled, err := testDB.Queries.ListEnabledSubscriptions(ctx)
	if err != nil {
		t.Fatalf("list enabled subscriptions: %v", err)
	}
	for _, sub := range enabled {
		if sub.ID == manual.ID {
			t.Fatal("the scheduler's due-query returned a manual subscription")
		}
	}

	for range 3 {
		run, err := testDB.Queries.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
			SourceId: source.ID, SubscriptionId: manual.ID, Trigger: "manual",
		})
		if err != nil {
			t.Fatalf("insert manual run: %v", err)
		}
		msg := "unreachable"
		if err := testDB.Queries.FinishSourceRunError(ctx, sqlcgen.FinishSourceRunErrorParams{ID: run.ID, Error: &msg}); err != nil {
			t.Fatalf("finish manual run: %v", err)
		}
	}

	recent, err := testDB.Queries.RecentSourceRunsForSource(ctx, sqlcgen.RecentSourceRunsForSourceParams{
		SourceId: source.ID, Limit: 3,
	})
	if err != nil {
		t.Fatalf("recent source runs: %v", err)
	}
	if len(recent) != 0 {
		t.Errorf("expected manual runs to be excluded from health accounting, got %d rows", len(recent))
	}
}

func TestIntegration_Migration00041_DownKeepsManualSourceAndItsVacancies(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	if _, err := testDB.Pool.Exec(ctx,
		`INSERT INTO "JobSource" ("key", "kind", "enabled", "config", "healthy")
		 VALUES ('manual', 'manual', true, '{}'::jsonb, true) ON CONFLICT ("key") DO NOTHING`); err != nil {
		t.Fatalf("ensure manual job source: %v", err)
	}
	job := mustInsertJob(t, "manual", "manual-dedupe-041", "Hand-entered role")

	downStatements := []string{
		`DROP INDEX IF EXISTS "SourceRun_subscriptionId_idx"`,
		`ALTER TABLE "SourceRun" DROP CONSTRAINT IF EXISTS "SourceRun_trigger_check"`,
		`ALTER TABLE "SourceRun" DROP CONSTRAINT IF EXISTS "SourceRun_subscriptionId_Subscription_id_fk"`,
		`ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "trigger"`,
		`ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "subscriptionId"`,
		`DROP INDEX IF EXISTS "Subscription_manual_per_source_idx"`,
		`ALTER TABLE "Subscription" DROP CONSTRAINT IF EXISTS "Subscription_kind_check"`,
		`ALTER TABLE "Subscription" DROP COLUMN IF EXISTS "kind"`,
	}
	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, stmt := range downStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("down statement %q: %v", stmt, err)
		}
	}

	var sourceCount, jobCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM "JobSource" WHERE "key" = 'manual'`).Scan(&sourceCount); err != nil {
		t.Fatalf("count manual source after down: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM "Job" WHERE "id" = $1`, job.ID).Scan(&jobCount); err != nil {
		t.Fatalf("count manual job after down: %v", err)
	}
	if sourceCount != 1 {
		t.Error("Down removed the manual JobSource row; it must survive so hand-entered vacancies do")
	}
	if jobCount != 1 {
		t.Error("Down destroyed a hand-entered vacancy")
	}
}
