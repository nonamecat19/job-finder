//go:build integration

package db_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
)

var testDB *db.DB

func TestMain(m *testing.M) {
	database, release, err := dbtest.NewForMain("db")
	if err != nil {
		panic("dbtest: " + err.Error())
	}
	testDB = database

	code := m.Run()
	release()
	os.Exit(code)
}

func truncateAll(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	tables := []string{
		`"SourceRun"`,
		`"KeywordDiff"`,
		`"NormalizedTerm"`,
		`"SynonymOverride"`,
		`"MatchResult"`,
		`"ApplicationOutcome"`,
		`"Application"`,
		`"GeneratedDocument"`,
		`"Subscription"`,
		`"SavedSearch"`,
		`"SalaryCache"`,
		`"JobContact"`,
		`"JobSignal"`,
		`"Job"`,
		`"JobSource"`,
		`"Profile"`,
		`"CompanySignal"`,
		`"Company"`,
	}
	for _, tbl := range tables {
		_, err := testDB.Pool.Exec(ctx, "TRUNCATE TABLE "+tbl+" CASCADE")
		if err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
}

func mustInsertJobSource(t *testing.T, key, kind string) sqlcgen.JobSource {
	t.Helper()
	err := testDB.Queries.UpsertJobSource(context.Background(), sqlcgen.UpsertJobSourceParams{
		Key:    key,
		Kind:   kind,
		Config: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("upsert job source: %v", err)
	}
	src, err := testDB.Queries.GetJobSourceByKey(context.Background(), key)
	if err != nil {
		t.Fatalf("get job source: %v", err)
	}
	return src
}

func mustInsertJob(t *testing.T, sourceKey, dedupeKey, title string) sqlcgen.Job {
	t.Helper()
	job, err := testDB.Queries.InsertJob(context.Background(), sqlcgen.InsertJobParams{
		DedupeKey:   dedupeKey,
		SourceKey:   sourceKey,
		Title:       title,
		Company:     "TestCo",
		Url:         "https://example.com/job/" + dedupeKey,
		Description: "A test job listing for " + title,
		Raw:         []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return job
}

func TestJobSourceCRUD(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	src := mustInsertJobSource(t, "test-src-1", "api")
	if src.Key != "test-src-1" {
		t.Fatalf("expected key test-src-1, got %s", src.Key)
	}
	if !src.Enabled {
		t.Fatal("expected source to be enabled by default")
	}
	if !src.Healthy {
		t.Fatal("expected source to be healthy by default")
	}

	all, err := testDB.Queries.ListJobSources(ctx)
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 source, got %d", len(all))
	}

	got, err := testDB.Queries.GetJobSourceByKey(ctx, "test-src-1")
	if err != nil {
		t.Fatalf("get source by key: %v", err)
	}
	if got.ID != src.ID {
		t.Fatal("ID mismatch")
	}

	err = testDB.Queries.SetJobSourceEnabled(ctx, sqlcgen.SetJobSourceEnabledParams{
		Key:     "test-src-1",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	got, err = testDB.Queries.GetJobSourceByKey(ctx, "test-src-1")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if got.Enabled {
		t.Fatal("expected source to be disabled")
	}

	enabled, err := testDB.Queries.ListEnabledJobSources(ctx)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 0 {
		t.Fatalf("expected 0 enabled sources, got %d", len(enabled))
	}

	err = testDB.Queries.SetJobSourceHealthy(ctx, sqlcgen.SetJobSourceHealthyParams{
		Key:     "test-src-1",
		Healthy: false,
	})
	if err != nil {
		t.Fatalf("set healthy: %v", err)
	}
	got, err = testDB.Queries.GetJobSourceByKey(ctx, "test-src-1")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if got.Healthy {
		t.Fatal("expected source to be unhealthy")
	}

	err = testDB.Queries.SetJobSourceConfig(ctx, sqlcgen.SetJobSourceConfigParams{
		Key:    "test-src-1",
		Config: []byte(`{"token":"abc"}`),
	})
	if err != nil {
		t.Fatalf("set config: %v", err)
	}
	got, err = testDB.Queries.GetJobSourceByKey(ctx, "test-src-1")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	var cfg map[string]string
	if err := json.Unmarshal(got.Config, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg["token"] != "abc" {
		t.Fatalf("expected token abc, got %s", cfg["token"])
	}

	err = testDB.Queries.UpsertJobSource(ctx, sqlcgen.UpsertJobSourceParams{
		Key:    "test-src-1",
		Kind:   "scraped",
		Config: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err = testDB.Queries.GetJobSourceByKey(ctx, "test-src-1")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if got.Kind != "scraped" {
		t.Fatalf("expected kind scraped, got %s", got.Kind)
	}
}

func TestJobCRUD(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-job", "api")

	job := mustInsertJob(t, "js-job", "dedupe-1", "Go Developer")
	if job.ID.Valid != true {
		t.Fatal("expected valid ID")
	}
	if job.Status != "found" {
		t.Fatalf("expected status found, got %s", job.Status)
	}
	if job.IngestedAt.Valid != true {
		t.Fatal("expected ingestedAt to be set")
	}

	got, err := testDB.Queries.GetJobByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job by ID: %v", err)
	}
	if got.Title != "Go Developer" {
		t.Fatalf("expected title Go Developer, got %s", got.Title)
	}
	if got.Company != "TestCo" {
		t.Fatalf("expected company TestCo, got %s", got.Company)
	}
	if got.Remote {
		t.Fatal("expected remote=false")
	}

	gotID, err := testDB.Queries.GetJobByDedupeKey(ctx, "dedupe-1")
	if err != nil {
		t.Fatalf("get by dedupe key: %v", err)
	}
	if gotID != job.ID {
		t.Fatal("dedupe key lookup returned wrong ID")
	}

	updated, err := testDB.Queries.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{
		ID:     job.ID,
		Status: "reviewed",
	})
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if updated.Status != "reviewed" {
		t.Fatalf("expected status reviewed, got %s", updated.Status)
	}

	salary := "$100k-$150k"
	loc := "Remote"
	updatedDetail, err := testDB.Queries.UpdateJobDetail(ctx, sqlcgen.UpdateJobDetailParams{
		ID:          job.ID,
		Description: "Full-stack Go developer needed",
		SalaryRaw:   &salary,
		Location:    &loc,
		Remote:      true,
		Raw:         []byte(`{"enriched":true}`),
	})
	if err != nil {
		t.Fatalf("update detail: %v", err)
	}
	if updatedDetail.SalaryRaw == nil || *updatedDetail.SalaryRaw != salary {
		t.Fatalf("expected salary %s", salary)
	}
	if updatedDetail.Location == nil || *updatedDetail.Location != loc {
		t.Fatalf("expected location %s", loc)
	}
	if !updatedDetail.Remote {
		t.Fatal("expected remote=true after detail update")
	}
	if updatedDetail.DetailScrapedAt.Valid != true {
		t.Fatal("expected detailScrapedAt to be set")
	}

	needDetail, err := testDB.Queries.ListJobsNeedingDetail(ctx, sqlcgen.ListJobsNeedingDetailParams{
		SourceKey: "js-job",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("list needing detail: %v", err)
	}
	if len(needDetail) != 0 {
		t.Fatalf("expected 0 jobs needing detail, got %d", len(needDetail))
	}
}

func TestJobNeedingDetail(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-need", "api")
	mustInsertJob(t, "js-need", "shallow-1", "Shallow Job")

	needDetail, err := testDB.Queries.ListJobsNeedingDetail(ctx, sqlcgen.ListJobsNeedingDetailParams{
		SourceKey: "js-need",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("list needing detail: %v", err)
	}
	if len(needDetail) != 1 {
		t.Fatalf("expected 1 job needing detail, got %d", len(needDetail))
	}
	if needDetail[0].DetailScrapedAt.Valid {
		t.Fatal("expected detailScrapedAt to be null")
	}
}

func TestSavedSearchCRUD(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	query, _ := json.Marshal(map[string]string{"q": "golang"})

	ss, err := testDB.Queries.CreateSavedSearch(ctx, sqlcgen.CreateSavedSearchParams{
		Name:    "Golang jobs",
		Query:   query,
		Cron:    "0 */6 * * *",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create saved search: %v", err)
	}
	if ss.Name != "Golang jobs" {
		t.Fatalf("expected name Golang jobs, got %s", ss.Name)
	}
	if !ss.Enabled {
		t.Fatal("expected enabled=true")
	}
	if ss.Cron != "0 */6 * * *" {
		t.Fatalf("expected cron, got %s", ss.Cron)
	}

	got, err := testDB.Queries.GetSavedSearch(ctx, ss.ID)
	if err != nil {
		t.Fatalf("get saved search: %v", err)
	}
	if got.ID != ss.ID {
		t.Fatal("ID mismatch")
	}

	all, err := testDB.Queries.ListSavedSearches(ctx)
	if err != nil {
		t.Fatalf("list saved searches: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1, got %d", len(all))
	}

	enabled, err := testDB.Queries.ListEnabledSavedSearches(ctx)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled, got %d", len(enabled))
	}

	newName := "Updated Search"
	disabled := false
	updated, err := testDB.Queries.UpdateSavedSearch(ctx, sqlcgen.UpdateSavedSearchParams{
		ID:      ss.ID,
		Name:    &newName,
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("update saved search: %v", err)
	}
	if updated.Name != "Updated Search" {
		t.Fatalf("expected updated name, got %s", updated.Name)
	}
	if updated.Enabled {
		t.Fatal("expected enabled=false after update")
	}

	err = testDB.Queries.TouchSavedSearchLastRun(ctx, ss.ID)
	if err != nil {
		t.Fatalf("touch last run: %v", err)
	}
	touched, err := testDB.Queries.GetSavedSearch(ctx, ss.ID)
	if err != nil {
		t.Fatalf("get touched: %v", err)
	}
	if !touched.LastRunAt.Valid {
		t.Fatal("expected lastRunAt to be set")
	}

	err = testDB.Queries.DeleteSavedSearch(ctx, ss.ID)
	if err != nil {
		t.Fatalf("delete saved search: %v", err)
	}
	all, err = testDB.Queries.ListSavedSearches(ctx)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(all))
	}
}

func TestApplicationCRUD(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-app", "api")
	job := mustInsertJob(t, "js-app", "app-job-1", "Rust Dev")

	events, _ := json.Marshal([]map[string]string{{"type": "created"}})
	err := testDB.Queries.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId:  job.ID,
		Status: "shortlisted",
		Events: events,
	})
	if err != nil {
		t.Fatalf("upsert application: %v", err)
	}

	app, err := testDB.Queries.GetApplicationByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get application by job ID: %v", err)
	}
	if app.Status != "shortlisted" {
		t.Fatalf("expected status shortlisted, got %s", app.Status)
	}

	got, err := testDB.Queries.GetApplicationByID(ctx, app.ID)
	if err != nil {
		t.Fatalf("get application by ID: %v", err)
	}
	if got.ID != app.ID {
		t.Fatal("ID mismatch")
	}

	apps, err := testDB.Queries.ListApplications(ctx, nil)
	if err != nil {
		t.Fatalf("list applications: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 application, got %d", len(apps))
	}
	if apps[0].JobTitle != "Rust Dev" {
		t.Fatalf("expected joined job title Rust Dev, got %s", apps[0].JobTitle)
	}

	filtered, err := testDB.Queries.ListApplications(ctx, strPtr("shortlisted"))
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered, got %d", len(filtered))
	}

	nonexistent, err := testDB.Queries.ListApplications(ctx, strPtr("hired"))
	if err != nil {
		t.Fatalf("list non-existent: %v", err)
	}
	if len(nonexistent) != 0 {
		t.Fatalf("expected 0 for non-matching filter, got %d", len(nonexistent))
	}

	notes := "Strong candidate"
	updated, err := testDB.Queries.UpdateApplication(ctx, sqlcgen.UpdateApplicationParams{
		ID:       app.ID,
		Status:   strPtr("interviewing"),
		NotesSet: boolPtr(true),
		Notes:    &notes,
	})
	if err != nil {
		t.Fatalf("update application: %v", err)
	}
	if updated.Status != "interviewing" {
		t.Fatalf("expected status interviewing, got %s", updated.Status)
	}
	if updated.Notes == nil || *updated.Notes != "Strong candidate" {
		t.Fatal("expected notes to be updated")
	}

	err = testDB.Queries.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId:  job.ID,
		Status: "hired",
		Events: []byte(`[]`),
	})
	if err != nil {
		t.Fatalf("upsert again: %v", err)
	}
	app, err = testDB.Queries.GetApplicationByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get after upsert: %v", err)
	}
	if app.Status != "hired" {
		t.Fatalf("expected status hired after upsert, got %s", app.Status)
	}
}

func mustInsertApplication(t *testing.T, sourceKey, dedupeKey string) sqlcgen.Application {
	t.Helper()
	ctx := context.Background()
	job := mustInsertJob(t, sourceKey, dedupeKey, "Outcome Test Job")
	if err := testDB.Queries.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId:  job.ID,
		Status: "shortlisted",
		Events: []byte(`[]`),
	}); err != nil {
		t.Fatalf("upsert application: %v", err)
	}
	app, err := testDB.Queries.GetApplicationByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get application by job ID: %v", err)
	}
	return app
}

func mustInsertOutcome(t *testing.T, appID pgtype.UUID, eventType string, occurredAt time.Time) sqlcgen.ApplicationOutcome {
	t.Helper()
	row, err := testDB.Queries.InsertApplicationOutcome(context.Background(), sqlcgen.InsertApplicationOutcomeParams{
		ApplicationId: appID,
		EventType:     eventType,
		OccurredAt:    pgtype.Timestamp{Time: occurredAt, Valid: true},
	})
	if err != nil {
		t.Fatalf("insert outcome %s: %v", eventType, err)
	}
	return row
}

func TestApplicationOutcomeWriteAndOrdering(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-outcome", "api")
	app := mustInsertApplication(t, "js-outcome", "outcome-job-1")

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	mustInsertOutcome(t, app.ID, "rejected", base.AddDate(0, 0, 10))
	applied := mustInsertOutcome(t, app.ID, "applied", base)
	mustInsertOutcome(t, app.ID, "screen", base.AddDate(0, 0, 4))

	if applied.EventType != "applied" {
		t.Fatalf("expected eventType applied, got %s", applied.EventType)
	}
	if !applied.OccurredAt.Valid {
		t.Fatal("expected occurredAt to be set")
	}
	if !applied.RecordedAt.Valid {
		t.Fatal("expected recordedAt to default to now()")
	}
	if applied.Note != nil {
		t.Fatalf("expected nil note, got %v", *applied.Note)
	}

	rows, err := testDB.Queries.ListApplicationOutcomes(ctx, app.ID)
	if err != nil {
		t.Fatalf("list outcomes: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 outcome events, got %d", len(rows))
	}
	want := []string{"applied", "screen", "rejected"}
	for i, w := range want {
		if rows[i].EventType != w {
			t.Fatalf("event %d: expected %s, got %s", i, w, rows[i].EventType)
		}
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].OccurredAt.Time.Before(rows[i-1].OccurredAt.Time) {
			t.Fatalf("events not ordered oldest-first at index %d", i)
		}
	}

	count, err := testDB.Queries.CountApplicationOutcomes(ctx, app.ID)
	if err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

func TestApplicationOutcomeTerminalOnceIdempotency(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-once", "api")
	app := mustInsertApplication(t, "js-once", "once-job-1")

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	for _, terminal := range []string{"applied", "offer", "rejected"} {
		mustInsertOutcome(t, app.ID, terminal, base)

		_, err := testDB.Queries.InsertApplicationOutcome(ctx, sqlcgen.InsertApplicationOutcomeParams{
			ApplicationId: app.ID,
			EventType:     terminal,
			OccurredAt:    pgtype.Timestamp{Time: base.AddDate(0, 0, 1), Valid: true},
		})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("duplicate %s: expected pgx.ErrNoRows, got %v", terminal, err)
		}
	}

	mustInsertOutcome(t, app.ID, "screen", base.AddDate(0, 0, 2))
	mustInsertOutcome(t, app.ID, "screen", base.AddDate(0, 0, 5))
	mustInsertOutcome(t, app.ID, "viewed", base.AddDate(0, 0, 3))
	mustInsertOutcome(t, app.ID, "viewed", base.AddDate(0, 0, 6))

	count, err := testDB.Queries.CountApplicationOutcomes(ctx, app.ID)
	if err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	if count != 7 {
		t.Fatalf("expected 7 outcome events, got %d", count)
	}
}

func TestApplicationOutcomeConstraints(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-constraint", "api")
	app := mustInsertApplication(t, "js-constraint", "constraint-job-1")
	now := time.Now().UTC()

	_, err := testDB.Queries.InsertApplicationOutcome(ctx, sqlcgen.InsertApplicationOutcomeParams{
		ApplicationId: app.ID,
		EventType:     "ghosted",
		OccurredAt:    pgtype.Timestamp{Time: now, Valid: true},
	})
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected CHECK violation for eventType 'ghosted', got %v", err)
	}

	orphan, perr := dbutil.ParseUUID("00000000-0000-0000-0000-0000000000ff")
	if perr != nil {
		t.Fatalf("parse uuid: %v", perr)
	}
	_, err = testDB.Queries.InsertApplicationOutcome(ctx, sqlcgen.InsertApplicationOutcomeParams{
		ApplicationId: orphan,
		EventType:     "applied",
		OccurredAt:    pgtype.Timestamp{Time: now, Valid: true},
	})
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected FK violation for unknown applicationId, got %v", err)
	}

	note := "phone screen with hiring manager"
	withNote, err := testDB.Queries.InsertApplicationOutcome(ctx, sqlcgen.InsertApplicationOutcomeParams{
		ApplicationId: app.ID,
		EventType:     "screen",
		OccurredAt:    pgtype.Timestamp{Time: now, Valid: true},
		Note:          &note,
	})
	if err != nil {
		t.Fatalf("insert outcome with note: %v", err)
	}
	if withNote.Note == nil || *withNote.Note != note {
		t.Fatal("expected note to round-trip")
	}

	if _, err := testDB.Pool.Exec(ctx, `DELETE FROM "Application" WHERE "id" = $1`, app.ID); err != nil {
		t.Fatalf("delete application: %v", err)
	}
	count, err := testDB.Queries.CountApplicationOutcomes(ctx, app.ID)
	if err != nil {
		t.Fatalf("count after cascade: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 outcomes after application delete cascade, got %d", count)
	}
}

func TestSubscriptionCRUD(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-sub", "api")

	sub, err := testDB.Queries.CreateSubscription(ctx, sqlcgen.CreateSubscriptionParams{Kind: "crawl",
		SourceKey: "js-sub",
		Name:      strPtr("Djinni Go jobs"),
		Url:       "https://djinni.example.com/jobs?lang=go",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if sub.Name == nil || *sub.Name != "Djinni Go jobs" {
		t.Fatal("expected name Djinni Go jobs")
	}
	if sub.SourceKey != "js-sub" {
		t.Fatalf("expected sourceKey js-sub, got %s", sub.SourceKey)
	}

	got, err := testDB.Queries.GetSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if got.ID != sub.ID {
		t.Fatal("ID mismatch")
	}

	all, err := testDB.Queries.ListSubscriptions(ctx)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1, got %d", len(all))
	}

	bySource, err := testDB.Queries.ListSubscriptionsBySource(ctx, "js-sub")
	if err != nil {
		t.Fatalf("list by source: %v", err)
	}
	if len(bySource) != 1 {
		t.Fatalf("expected 1, got %d", len(bySource))
	}

	newName := "Updated Sub"
	newUrl := "https://djinni.example.com/jobs?lang=rust"
	disabled := false
	updated, err := testDB.Queries.UpdateSubscription(ctx, sqlcgen.UpdateSubscriptionParams{
		ID:      sub.ID,
		Name:    &newName,
		Url:     &newUrl,
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("update subscription: %v", err)
	}
	if updated.Name == nil || *updated.Name != "Updated Sub" {
		t.Fatal("expected updated name")
	}
	if updated.Url != "https://djinni.example.com/jobs?lang=rust" {
		t.Fatalf("expected updated url, got %s", updated.Url)
	}
	if updated.Enabled {
		t.Fatal("expected enabled=false")
	}

	err = testDB.Queries.TouchSubscriptionLastRun(ctx, sub.ID)
	if err != nil {
		t.Fatalf("touch last run: %v", err)
	}
	touched, err := testDB.Queries.GetSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("get touched: %v", err)
	}
	if !touched.LastRunAt.Valid {
		t.Fatal("expected lastRunAt to be set")
	}

	err = testDB.Queries.DeleteSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	all, err = testDB.Queries.ListSubscriptions(ctx)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(all))
	}
}

func TestProfileCRUD(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	doc, _ := json.Marshal(map[string]any{
		"name":   "John Doe",
		"email":  "john@example.com",
		"skills": []string{"go", "postgres", "docker"},
	})

	prof, err := testDB.Queries.CreateProfile(ctx, sqlcgen.CreateProfileParams{
		Name:       "John Doe",
		Document:   doc,
		ExtraNotes: strPtr("Senior Go developer"),
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if prof.Name != "John Doe" {
		t.Fatalf("expected name John Doe, got %s", prof.Name)
	}
	if prof.ExtraNotes == nil || *prof.ExtraNotes != "Senior Go developer" {
		t.Fatal("expected extra notes")
	}
	if prof.Embedding != nil {
		t.Fatal("expected nil embedding initially")
	}

	got, err := testDB.Queries.GetProfile(ctx, prof.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.ID != prof.ID {
		t.Fatal("ID mismatch")
	}

	def, err := testDB.Queries.GetDefaultProfile(ctx)
	if err != nil {
		t.Fatalf("get default profile: %v", err)
	}
	if def.ID != prof.ID {
		t.Fatal("expected default to be our profile")
	}

	all, err := testDB.Queries.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1, got %d", len(all))
	}

	newDoc, _ := json.Marshal(map[string]any{"name": "Jane Doe"})
	err = testDB.Queries.UpdateProfile(ctx, sqlcgen.UpdateProfileParams{
		ID:       prof.ID,
		Name:     strPtr("Jane Doe"),
		Document: newDoc,
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	got, err = testDB.Queries.GetProfile(ctx, prof.ID)
	if err != nil {
		t.Fatalf("get updated: %v", err)
	}
	if got.Name != "Jane Doe" {
		t.Fatalf("expected updated name Jane Doe, got %s", got.Name)
	}

	err = testDB.Queries.UpdateProfileExtraNotes(ctx, sqlcgen.UpdateProfileExtraNotesParams{
		ID:         prof.ID,
		ExtraNotes: strPtr("Updated notes"),
	})
	if err != nil {
		t.Fatalf("update extra notes: %v", err)
	}
	got, err = testDB.Queries.GetProfile(ctx, prof.ID)
	if err != nil {
		t.Fatalf("get after notes update: %v", err)
	}
	if got.ExtraNotes == nil || *got.ExtraNotes != "Updated notes" {
		t.Fatal("expected updated extra notes")
	}

	has, err := testDB.Queries.ProfileHasEmbedding(ctx, prof.ID)
	if err != nil {
		t.Fatalf("has embedding: %v", err)
	}
	if has != false {
		t.Fatal("expected has_embedding=false")
	}

	err = testDB.Queries.DeleteProfile(ctx, prof.ID)
	if err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	all, err = testDB.Queries.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(all))
	}
}

func TestGeneratedDocumentCRUD(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-doc", "api")
	job := mustInsertJob(t, "js-doc", "doc-job-1", "Doc Test Job")

	content, _ := json.Marshal(map[string]string{"sections": "experience"})

	doc, err := testDB.Queries.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		JobId:   job.ID,
		Type:    "resume",
		Version: 1,
		Content: content,
		PdfPath: strPtr("/docs/resume_v1.pdf"),
		Model:   "gpt-4",
	})
	if err != nil {
		t.Fatalf("insert document: %v", err)
	}
	if doc.Type != "resume" {
		t.Fatalf("expected type resume, got %s", doc.Type)
	}
	if doc.Version != 1 {
		t.Fatalf("expected version 1, got %d", doc.Version)
	}

	got, err := testDB.Queries.GetDocumentByID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	if got.ID != doc.ID {
		t.Fatal("ID mismatch")
	}

	docs, err := testDB.Queries.ListDocumentsForJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	maxV, err := testDB.Queries.MaxDocumentVersion(ctx, sqlcgen.MaxDocumentVersionParams{
		JobId: job.ID,
		Type:  "resume",
	})
	if err != nil {
		t.Fatalf("max version: %v", err)
	}
	if maxV != 1 {
		t.Fatalf("expected max version 1, got %d", maxV)
	}

	v2Content, _ := json.Marshal(map[string]string{"sections": "experience+skills"})
	doc2, err := testDB.Queries.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		JobId:   job.ID,
		Type:    "resume",
		Version: 2,
		Content: v2Content,
		Model:   "gpt-4",
	})
	if err != nil {
		t.Fatalf("insert v2: %v", err)
	}
	maxV, err = testDB.Queries.MaxDocumentVersion(ctx, sqlcgen.MaxDocumentVersionParams{
		JobId: job.ID,
		Type:  "resume",
	})
	if err != nil {
		t.Fatalf("max version v2: %v", err)
	}
	if maxV != 2 {
		t.Fatalf("expected max version 2, got %d", maxV)
	}

	newContent, _ := json.Marshal(map[string]string{"sections": "updated"})
	pdfPath := "/docs/resume_v2_updated.pdf"
	updated, err := testDB.Queries.UpdateDocumentContent(ctx, sqlcgen.UpdateDocumentContentParams{
		ID:      doc2.ID,
		Content: newContent,
		PdfPath: &pdfPath,
	})
	if err != nil {
		t.Fatalf("update document content: %v", err)
	}
	if updated.PdfPath == nil || *updated.PdfPath != pdfPath {
		t.Fatal("expected updated pdf path")
	}

	docs, err = testDB.Queries.ListDocumentsForJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("list after v2: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}

	jobDocs, err := testDB.Queries.GetJobDocuments(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job documents: %v", err)
	}
	if len(jobDocs) != 2 {
		t.Fatalf("expected 2 job documents, got %d", len(jobDocs))
	}
}

func TestSourceRunCRUD(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	src := mustInsertJobSource(t, "js-run", "api")

	run, err := testDB.Queries.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{Trigger: "scheduled",
		SourceId: src.ID,
		SearchId: strPtr("search-abc"),
	})
	if err != nil {
		t.Fatalf("insert source run: %v", err)
	}
	if !run.StartedAt.Valid {
		t.Fatal("expected startedAt to be set")
	}
	if run.Ok != nil {
		t.Fatal("expected ok=nil before finish")
	}
	if run.Found != 0 {
		t.Fatal("expected found=0")
	}

	err = testDB.Queries.FinishSourceRunOk(ctx, sqlcgen.FinishSourceRunOkParams{
		ID:    run.ID,
		Found: 25,
		New:   5,
	})
	if err != nil {
		t.Fatalf("finish ok: %v", err)
	}

	recent, err := testDB.Queries.RecentRunsJoined(ctx, 10)
	if err != nil {
		t.Fatalf("recent runs: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent run, got %d", len(recent))
	}
	if recent[0].SourceKey != "js-run" {
		t.Fatalf("expected source_key js-run, got %s", recent[0].SourceKey)
	}
	if recent[0].Found != 25 {
		t.Fatalf("expected found=25, got %d", recent[0].Found)
	}
	if recent[0].New != 5 {
		t.Fatalf("expected new=5, got %d", recent[0].New)
	}
	if recent[0].Ok == nil || !*recent[0].Ok {
		t.Fatal("expected ok=true")
	}

	statuses, err := testDB.Queries.RecentSourceRunsForSource(ctx, sqlcgen.RecentSourceRunsForSourceParams{
		SourceId: src.ID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("recent source runs: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0] == nil || !*statuses[0] {
		t.Fatal("expected ok=true in recent")
	}

	run2, err := testDB.Queries.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{Trigger: "scheduled",
		SourceId: src.ID,
	})
	if err != nil {
		t.Fatalf("insert second run: %v", err)
	}
	errMsg := "connection timeout"
	err = testDB.Queries.FinishSourceRunError(ctx, sqlcgen.FinishSourceRunErrorParams{
		ID:    run2.ID,
		Error: &errMsg,
	})
	if err != nil {
		t.Fatalf("finish error: %v", err)
	}

	recent, err = testDB.Queries.RecentRunsJoined(ctx, 10)
	if err != nil {
		t.Fatalf("recent runs after error: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent runs, got %d", len(recent))
	}

	statuses, err = testDB.Queries.RecentSourceRunsForSource(ctx, sqlcgen.RecentSourceRunsForSourceParams{
		SourceId: src.ID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("recent status after error: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
}

func TestMatchResultCRUD(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-match", "api")
	job := mustInsertJob(t, "js-match", "match-job-1", "Matched Job")

	matched, _ := json.Marshal([]string{"go", "sql"})
	missing, _ := json.Marshal([]string{"kubernetes"})
	redFlags, _ := json.Marshal([]string{"unpaid"})

	mr, err := testDB.Queries.UpsertMatchResult(ctx, sqlcgen.UpsertMatchResultParams{
		JobId:         job.ID,
		Similarity:    0.85,
		Score:         int32Ptr(82),
		MatchedSkills: matched,
		MissingSkills: missing,
		Summary:       strPtr("Strong fit for senior Go role"),
		RedFlags:      redFlags,
		Model:         "gpt-4",
	})
	if err != nil {
		t.Fatalf("upsert match result: %v", err)
	}
	if mr.Similarity != 0.85 {
		t.Fatalf("expected similarity 0.85, got %f", mr.Similarity)
	}
	if mr.Score == nil || *mr.Score != 82 {
		t.Fatal("expected score 82")
	}

	got, err := testDB.Queries.GetMatchResultByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if got.ID != mr.ID {
		t.Fatal("ID mismatch")
	}
	if got.Summary == nil || *got.Summary != "Strong fit for senior Go role" {
		t.Fatal("expected summary to match")
	}

	newScore := int32Ptr(90)
	mr2, err := testDB.Queries.UpsertMatchResult(ctx, sqlcgen.UpsertMatchResultParams{
		JobId:         job.ID,
		Similarity:    0.92,
		Score:         newScore,
		MatchedSkills: []byte(`["go","sql","docker"]`),
		MissingSkills: []byte(`[]`),
		Summary:       strPtr("Excellent fit"),
		RedFlags:      []byte(`[]`),
		Model:         "gpt-4",
	})
	if err != nil {
		t.Fatalf("upsert again: %v", err)
	}
	if mr2.Similarity != 0.92 {
		t.Fatalf("expected updated similarity 0.92, got %f", mr2.Similarity)
	}

	got, err = testDB.Queries.GetMatchResultByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get updated: %v", err)
	}
	if got.Similarity != 0.92 {
		t.Fatalf("expected similarity 0.92 after upsert, got %f", got.Similarity)
	}
}

func TestStatsQueries(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-stats", "api")
	job := mustInsertJob(t, "js-stats", "stats-job-1", "Stats Job")

	total, err := testDB.Queries.StatsJobsTotal(ctx)
	if err != nil {
		t.Fatalf("stats total: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 total job, got %d", total)
	}

	now := dbutil.NowTimestamp()
	now.Time = now.Time.Add(-5 * time.Second)
	last24h, err := testDB.Queries.StatsJobsLast24h(ctx, now)
	if err != nil {
		t.Fatalf("stats last 24h: %v", err)
	}
	if last24h != 1 {
		t.Fatalf("expected 1 job in last 24h, got %d", last24h)
	}

	_, err = testDB.Queries.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{
		ID:     job.ID,
		Status: "hidden",
	})
	if err != nil {
		t.Fatalf("hide job: %v", err)
	}
	total, err = testDB.Queries.StatsJobsTotal(ctx)
	if err != nil {
		t.Fatalf("stats total after hide: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected 0 after hiding, got %d", total)
	}

	mustInsertJob(t, "js-stats", "stats-job-2", "Stats Job 2")
	events, _ := json.Marshal([]any{})
	_ = testDB.Queries.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId:  job.ID,
		Status: "shortlisted",
		Events: events,
	})
	_ = testDB.Queries.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId:  mustInsertJob(t, "js-stats", "stats-job-3", "Stats Job 3").ID,
		Status: "hired",
		Events: events,
	})

	pipeline, err := testDB.Queries.StatsPipeline(ctx)
	if err != nil {
		t.Fatalf("stats pipeline: %v", err)
	}
	if len(pipeline) < 2 {
		t.Fatalf("expected at least 2 pipeline groups, got %d", len(pipeline))
	}

	score := int32Ptr(80)
	_, _ = testDB.Queries.UpsertMatchResult(ctx, sqlcgen.UpsertMatchResultParams{
		JobId:      job.ID,
		Similarity: 0.9,
		Score:      score,
		Model:      "test",
	})
	highFit, err := testDB.Queries.StatsHighFit(ctx)
	if err != nil {
		t.Fatalf("stats high fit: %v", err)
	}
	if highFit != 1 {
		t.Fatalf("expected 1 high fit, got %d", highFit)
	}
}

func TestJobListQueries(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-list", "api")
	mustInsertJob(t, "js-list", "list-job-1", "Frontend Dev")
	mustInsertJob(t, "js-list", "list-job-2", "Backend Go Dev")

	count, err := testDB.Queries.CountJobs(ctx, sqlcgen.CountJobsParams{})
	if err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 jobs, got %d", count)
	}

	src := "js-list"
	count, err = testDB.Queries.CountJobs(ctx, sqlcgen.CountJobsParams{Source: &src})
	if err != nil {
		t.Fatalf("count by source: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 for source, got %d", count)
	}

	q := "%Go%"
	count, err = testDB.Queries.CountJobs(ctx, sqlcgen.CountJobsParams{Q: &q})
	if err != nil {
		t.Fatalf("count by query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 for query 'Go', got %d", count)
	}

	jobs, err := testDB.Queries.ListJobsByDate(ctx, sqlcgen.ListJobsByDateParams{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("list by date: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	scored, err := testDB.Queries.ListJobsByScore(ctx, sqlcgen.ListJobsByScoreParams{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("list by score: %v", err)
	}
	if len(scored) != 2 {
		t.Fatalf("expected 2 scored jobs, got %d", len(scored))
	}
}

func TestUUIDIntegration(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-uuid", "api")
	job := mustInsertJob(t, "js-uuid", "uuid-job-1", "UUID Test Job")

	uuidStr := dbutil.UUIDString(job.ID)
	if uuidStr == "" {
		t.Fatal("UUIDString returned empty")
	}

	parsed, err := dbutil.ParseUUID(uuidStr)
	if err != nil {
		t.Fatalf("ParseUUID: %v", err)
	}
	if parsed != job.ID {
		t.Fatal("UUID round-trip mismatch")
	}

	got, err := testDB.Queries.GetJobByID(ctx, parsed)
	if err != nil {
		t.Fatalf("get job by parsed UUID: %v", err)
	}
	if got.ID != job.ID {
		t.Fatal("get by parsed UUID returned wrong job")
	}

	ptr := dbutil.UUIDStringPtr(job.ID)
	if ptr == nil || *ptr != uuidStr {
		t.Fatal("UUIDStringPtr mismatch")
	}
	invalidPtr := dbutil.UUIDStringPtr(pgtype.UUID{})
	if invalidPtr != nil {
		t.Fatal("expected nil from UUIDStringPtr for invalid UUID")
	}

	ts := dbutil.NowTimestamp()
	if !ts.Valid {
		t.Fatal("expected valid timestamp")
	}
	tsStr := dbutil.Timestamp(ts)
	if tsStr == "" {
		t.Fatal("expected non-empty timestamp string")
	}
	tsPtr := dbutil.TimestampPtr(ts)
	if tsPtr == nil {
		t.Fatal("expected non-nil timestamp ptr")
	}
	invalidTs := dbutil.Timestamp(pgtype.Timestamp{})
	if invalidTs != "" {
		t.Fatal("expected empty for invalid timestamp")
	}
	invalidTsPtr := dbutil.TimestampPtr(pgtype.Timestamp{})
	if invalidTsPtr != nil {
		t.Fatal("expected nil for invalid timestamp ptr")
	}
}

func TestCascadeDeletes(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	src := mustInsertJobSource(t, "js-cascade", "api")
	job := mustInsertJob(t, "js-cascade", "cascade-job-1", "Cascade Job")

	events, _ := json.Marshal([]any{})
	_ = testDB.Queries.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId:  job.ID,
		Status: "shortlisted",
		Events: events,
	})
	_, _ = testDB.Queries.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		JobId:   job.ID,
		Type:    "cover_letter",
		Version: 1,
		Content: []byte(`{}`),
		Model:   "test",
	})
	_, _ = testDB.Queries.UpsertMatchResult(ctx, sqlcgen.UpsertMatchResultParams{
		JobId:      job.ID,
		Similarity: 0.5,
		Model:      "test",
	})

	apps, _ := testDB.Queries.ListApplications(ctx, nil)
	if len(apps) != 1 {
		t.Fatalf("expected 1 app before cascade, got %d", len(apps))
	}
	docs, _ := testDB.Queries.ListDocumentsForJob(ctx, job.ID)
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc before cascade, got %d", len(docs))
	}

	_, err := testDB.Pool.Exec(ctx, `DELETE FROM "Job" WHERE "id" = $1`, job.ID)
	if err != nil {
		t.Fatalf("delete job: %v", err)
	}

	apps, err = testDB.Queries.ListApplications(ctx, nil)
	if err != nil {
		t.Fatalf("list apps after cascade: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("expected 0 apps after cascade, got %d", len(apps))
	}

	docs, err = testDB.Queries.ListDocumentsForJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("list docs after cascade: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected 0 docs after cascade, got %d", len(docs))
	}

	sub, err := testDB.Queries.CreateSubscription(ctx, sqlcgen.CreateSubscriptionParams{Kind: "crawl",
		SourceKey: "js-cascade",
		Url:       "https://example.com/sub",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("create sub: %v", err)
	}
	subs, _ := testDB.Queries.ListSubscriptions(ctx)
	if len(subs) != 1 {
		t.Fatalf("expected 1 sub, got %d", len(subs))
	}

	_, err = testDB.Pool.Exec(ctx, `DELETE FROM "JobSource" WHERE "id" = $1`, src.ID)
	if err != nil {
		t.Fatalf("delete source: %v", err)
	}
	subs, err = testDB.Queries.ListSubscriptions(ctx)
	if err != nil {
		t.Fatalf("list subs after cascade: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("expected 0 subs after source delete cascade, got %d", len(subs))
	}
	_ = sub
}

func TestCompanyUpsertIdempotent(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	company, err := testDB.Queries.UpsertCompany(ctx, sqlcgen.UpsertCompanyParams{
		Name:            "Acme Corp",
		NormalizedName:  "acme-corp",
		Website:         strPtr("https://acme.example.com"),
		LastRefreshedAt: dbutil.NowTimestamp(),
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if company.Name != "Acme Corp" {
		t.Fatalf("expected name Acme Corp, got %s", company.Name)
	}
	if company.Website == nil || *company.Website != "https://acme.example.com" {
		t.Fatal("expected website to be set")
	}
	if !company.FirstSeenAt.Valid {
		t.Fatal("expected firstSeenAt to be set")
	}

	company2, err := testDB.Queries.UpsertCompany(ctx, sqlcgen.UpsertCompanyParams{
		Name:            "Acme Corporation",
		NormalizedName:  "acme-corp",
		Website:         nil,
		LastRefreshedAt: dbutil.NowTimestamp(),
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if company2.ID != company.ID {
		t.Fatal("expected same ID on upsert")
	}
	if company2.Name != "Acme Corporation" {
		t.Fatalf("expected updated name Acme Corporation, got %s", company2.Name)
	}
	if company2.Website == nil || *company2.Website != "https://acme.example.com" {
		t.Fatal("expected website to be preserved on upsert with nil")
	}
}

func TestCompanySignalUpsertReplacesInPlace(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	company, err := testDB.Queries.UpsertCompany(ctx, sqlcgen.UpsertCompanyParams{
		Name:            "TestCo",
		NormalizedName:  "testco",
		Website:         nil,
		LastRefreshedAt: dbutil.NowTimestamp(),
	})
	if err != nil {
		t.Fatalf("upsert company: %v", err)
	}

	value1, _ := json.Marshal(map[string]any{"rating": 4.5})
	raw1, _ := json.Marshal(map[string]any{"source": "glassdoor"})

	signal, err := testDB.Queries.UpsertCompanySignal(ctx, sqlcgen.UpsertCompanySignalParams{
		CompanyId: company.ID,
		Kind:      "rating",
		Value:     value1,
		Source:    strPtr("glassdoor"),
		Raw:       raw1,
	})
	if err != nil {
		t.Fatalf("first upsert signal: %v", err)
	}
	if signal.Kind != "rating" {
		t.Fatalf("expected kind rating, got %s", signal.Kind)
	}

	value2, _ := json.Marshal(map[string]any{"rating": 3.8})
	raw2, _ := json.Marshal(map[string]any{"source": "indeed"})

	signal2, err := testDB.Queries.UpsertCompanySignal(ctx, sqlcgen.UpsertCompanySignalParams{
		CompanyId: company.ID,
		Kind:      "rating",
		Value:     value2,
		Source:    strPtr("indeed"),
		Raw:       raw2,
	})
	if err != nil {
		t.Fatalf("second upsert signal: %v", err)
	}
	if signal2.ID != signal.ID {
		t.Fatal("expected same ID on upsert")
	}

	got, err := testDB.Queries.GetCompanySignalByKind(ctx, sqlcgen.GetCompanySignalByKindParams{
		CompanyId: company.ID,
		Kind:      "rating",
	})
	if err != nil {
		t.Fatalf("get signal by kind: %v", err)
	}
	var val map[string]any
	if err := json.Unmarshal(got.Value, &val); err != nil {
		t.Fatalf("unmarshal value: %v", err)
	}
	if val["rating"] != 3.8 {
		t.Fatalf("expected rating 3.8, got %v", val["rating"])
	}
	if got.Source == nil || *got.Source != "indeed" {
		t.Fatal("expected source indeed")
	}

	signals, err := testDB.Queries.GetCompanySignals(ctx, company.ID)
	if err != nil {
		t.Fatalf("get company signals: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}

	deleted, err := testDB.Queries.DeleteCompanySignals(ctx, company.ID)
	if err != nil {
		t.Fatalf("delete signals: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	signals, err = testDB.Queries.GetCompanySignals(ctx, company.ID)
	if err != nil {
		t.Fatalf("get signals after delete: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals after delete, got %d", len(signals))
	}
}

func TestJobContactUpsertIdempotent(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-contact", "api")
	job := mustInsertJob(t, "js-contact", "contact-job-1", "Contact Job")

	contact, err := testDB.Queries.UpsertJobContact(ctx, sqlcgen.UpsertJobContactParams{
		JobId:      job.ID,
		Name:       "Jane Doe",
		Title:      strPtr("Recruiter"),
		Email:      strPtr("jane@acme.com"),
		Source:     "posting",
		Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if contact.Name != "Jane Doe" {
		t.Fatalf("expected name Jane Doe, got %s", contact.Name)
	}

	contact2, err := testDB.Queries.UpsertJobContact(ctx, sqlcgen.UpsertJobContactParams{
		JobId:      job.ID,
		Name:       "Jane Doe",
		Title:      strPtr("Senior Recruiter"),
		Email:      strPtr("jane@acme.com"),
		Source:     "posting",
		Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if contact2.ID != contact.ID {
		t.Fatal("expected same ID on upsert (no duplicate row)")
	}
	if contact2.Title == nil || *contact2.Title != "Senior Recruiter" {
		t.Fatalf("expected updated title, got %v", contact2.Title)
	}

	rows, err := testDB.Queries.ListJobContactsByJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after idempotent re-run, got %d", len(rows))
	}
}

func TestJobContactOrdering(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-contact-order", "api")
	job := mustInsertJob(t, "js-contact-order", "contact-job-2", "Contact Order Job")

	if _, err := testDB.Queries.UpsertJobContact(ctx, sqlcgen.UpsertJobContactParams{
		JobId: job.ID, Name: "B Person", Source: "linkedin", Confidence: 0.5,
	}); err != nil {
		t.Fatalf("upsert linkedin contact: %v", err)
	}
	if _, err := testDB.Queries.UpsertJobContact(ctx, sqlcgen.UpsertJobContactParams{
		JobId: job.ID, Name: "A Person", Source: "posting", Confidence: 0.5,
	}); err != nil {
		t.Fatalf("upsert posting contact: %v", err)
	}
	if _, err := testDB.Queries.UpsertJobContact(ctx, sqlcgen.UpsertJobContactParams{
		JobId: job.ID, Name: "C Person", Source: "posting", Confidence: 0.9,
	}); err != nil {
		t.Fatalf("upsert high-confidence posting contact: %v", err)
	}

	rows, err := testDB.Queries.ListJobContactsByJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	want := []string{"C Person", "A Person", "B Person"}
	for i, w := range want {
		if rows[i].Name != w {
			t.Fatalf("row %d: expected %s, got %s", i, w, rows[i].Name)
		}
	}
}

func TestJobContactCascadeDelete(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	mustInsertJobSource(t, "js-contact-cascade", "api")
	job := mustInsertJob(t, "js-contact-cascade", "contact-job-3", "Contact Cascade Job")

	if _, err := testDB.Queries.UpsertJobContact(ctx, sqlcgen.UpsertJobContactParams{
		JobId: job.ID, Name: "Jane Doe", Source: "posting", Confidence: 0.9,
	}); err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	rows, err := testDB.Queries.ListJobContactsByJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("list contacts before delete: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 contact before delete, got %d", len(rows))
	}

	if _, err := testDB.Pool.Exec(ctx, `DELETE FROM "Job" WHERE "id" = $1`, job.ID); err != nil {
		t.Fatalf("delete job: %v", err)
	}

	rows, err = testDB.Queries.ListJobContactsByJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("list contacts after delete: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 contacts after cascade delete, got %d", len(rows))
	}
}

func strPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }

func int32Ptr(i int32) *int32 { return &i }
