//go:build integration

package ingestion

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

func setupMergeTest(t *testing.T) (context.Context, *sqlcgen.Queries, func()) {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	if err := db.Migrate(dsn); err != nil {
		cancel()
		t.Fatalf("migrate: %v", err)
	}
	testDB, err := db.Open(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatalf("db open: %v", err)
	}

	unlock, err := dbtest.LockSharedDB(ctx, testDB.Pool)
	if err != nil {
		cancel()
		testDB.Close()
		t.Fatalf("lock shared db: %v", err)
	}

	cleanup := func() {
		unlock()
		testDB.Close()
		cancel()
	}

	for _, tbl := range []string{"MatchResult", "Application", "Job", "JobSource"} {
		if _, err := testDB.Pool.Exec(ctx, `TRUNCATE TABLE "`+tbl+`" CASCADE`); err != nil {
			cleanup()
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}

	for _, key := range []struct{ key, kind string }{
		{"adzuna", "api"},
		{"greenhouse", "api"},
		{"lever", "api"},
	} {
		if err := testDB.Queries.UpsertJobSource(ctx, sqlcgen.UpsertJobSourceParams{
			Key: key.key, Kind: key.kind, Config: []byte(`{}`),
		}); err != nil {
			cleanup()
			t.Fatalf("upsert job source %s: %v", key.key, err)
		}
	}

	return ctx, testDB.Queries, cleanup
}

func insertJob(t *testing.T, ctx context.Context, q *sqlcgen.Queries, sourceKey, company, title, url string) sqlcgen.Job {
	t.Helper()
	dedupeKey := DedupeKey(company, title, url)
	job, err := q.InsertJob(ctx, sqlcgen.InsertJobParams{
		DedupeKey:   dedupeKey,
		SourceKey:   sourceKey,
		Title:       title,
		Company:     company,
		Url:         url,
		Description: "A " + title + " role at " + company,
		Raw:         []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return job
}

func TestMerge_AggregatorThenBoard_MergesIntoOneRow(t *testing.T) {
	ctx, q, cleanup := setupMergeTest(t)
	defer cleanup()

	aggregatorJob := insertJob(t, ctx, q, "adzuna", "Acme Corp", "Senior Go Engineer", "https://adzuna.example.com/jobs/123")

	eventsJSON, _ := json.Marshal([]map[string]string{{"status": "found", "at": time.Now().Format(time.RFC3339)}})
	if err := q.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId:  aggregatorJob.ID,
		Status: "found",
		Events: eventsJSON,
	}); err != nil {
		t.Fatalf("upsert application: %v", err)
	}

	score32 := int32(85)
	if _, err := q.UpsertMatchResult(ctx, sqlcgen.UpsertMatchResultParams{
		JobId:         aggregatorJob.ID,
		Similarity:    0.92,
		Score:         &score32,
		Summary:       &[]string{"Good fit"}[0],
		MatchedSkills: []byte("null"),
		MissingSkills: []byte("null"),
		RedFlags:      []byte("null"),
		Model:         "test-model",
	}); err != nil {
		t.Fatalf("upsert match result: %v", err)
	}

	app, err := q.GetApplicationByJobID(ctx, aggregatorJob.ID)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	if app.JobId != aggregatorJob.ID {
		t.Fatal("application FK doesn't match job ID")
	}

	boardJob := dto.NormalizedJob{
		SourceKey:   "greenhouse",
		Title:       "Senior Go Engineer",
		Company:     "Acme Corp",
		URL:         "https://boards.greenhouse.io/acme/jobs/456",
		Description: "Senior Go Engineer role at Acme Corp",
		Raw:         map[string]any{},
	}

	if !IsBoardVendor(boardJob.SourceKey) {
		t.Fatal("greenhouse should be a board vendor")
	}

	existingID, err := FindMergeCandidate(ctx, q, boardJob)
	if err != nil {
		t.Fatalf("find merge candidate: %v", err)
	}
	if !existingID.Valid {
		t.Fatal("expected a merge candidate to be found")
	}

	merged, err := q.MergeJobBoard(ctx, sqlcgen.MergeJobBoardParams{
		ID:          existingID,
		Url:         boardJob.URL,
		SourceKey:   boardJob.SourceKey,
		ArrayAppend: boardJob.SourceKey,
	})
	if err != nil {
		t.Fatalf("merge job board: %v", err)
	}

	if merged.Url != boardJob.URL {
		t.Errorf("expected merged url %q, got %q", boardJob.URL, merged.Url)
	}
	if merged.SourceKey != boardJob.SourceKey {
		t.Errorf("expected merged sourceKey %q, got %q", boardJob.SourceKey, merged.SourceKey)
	}

	foundAdzuna := false
	foundGreenhouse := false
	for _, s := range merged.SeenOnSources {
		if s == "adzuna" {
			foundAdzuna = true
		}
		if s == "greenhouse" {
			foundGreenhouse = true
		}
	}
	if !foundAdzuna {
		t.Error("seenOnSources should include adzuna")
	}
	if !foundGreenhouse {
		t.Error("seenOnSources should include greenhouse")
	}

	existingApp, err := q.GetApplicationByJobID(ctx, aggregatorJob.ID)
	if err != nil {
		t.Fatalf("get application after merge: %v", err)
	}
	if existingApp.JobId != aggregatorJob.ID {
		t.Error("application should still point to the original job ID after merge")
	}

	existingMR, err := q.GetMatchResultByJobID(ctx, aggregatorJob.ID)
	if err != nil {
		t.Fatalf("get match result after merge: %v", err)
	}
	if existingMR.JobId != aggregatorJob.ID {
		t.Error("match result should still point to the original job ID after merge")
	}
}

func TestMerge_DifferentCompanies_StaysDistinct(t *testing.T) {
	ctx, q, cleanup := setupMergeTest(t)
	defer cleanup()

	insertJob(t, ctx, q, "adzuna", "Acme Corp", "Senior Go Engineer", "https://adzuna.example.com/jobs/123")

	otherJob := dto.NormalizedJob{
		SourceKey:   "greenhouse",
		Title:       "Senior Go Engineer",
		Company:     "Beta Inc",
		URL:         "https://boards.greenhouse.io/beta/jobs/789",
		Description: "A different company",
		Raw:         map[string]any{},
	}

	existingID, err := FindMergeCandidate(ctx, q, otherJob)
	if err != nil {
		t.Fatalf("find merge candidate: %v", err)
	}
	if existingID.Valid {
		t.Fatal("should NOT find a merge candidate for a different company")
	}
}

func TestMerge_DifferentTitles_StaysDistinct(t *testing.T) {
	ctx, q, cleanup := setupMergeTest(t)
	defer cleanup()

	insertJob(t, ctx, q, "adzuna", "Acme Corp", "Senior Go Engineer", "https://adzuna.example.com/jobs/123")

	otherJob := dto.NormalizedJob{
		SourceKey:   "greenhouse",
		Title:       "Junior Frontend Developer",
		Company:     "Acme Corp",
		URL:         "https://boards.greenhouse.io/acme/jobs/999",
		Description: "Completely different role at same company",
		Raw:         map[string]any{},
	}

	existingID, err := FindMergeCandidate(ctx, q, otherJob)
	if err != nil {
		t.Fatalf("find merge candidate: %v", err)
	}
	if existingID.Valid {
		t.Fatal("should NOT find a merge candidate for an unrelated title at the same company")
	}
}

func TestMerge_NonBoardVendor_NoMerge(t *testing.T) {
	ctx, q, cleanup := setupMergeTest(t)
	defer cleanup()

	insertJob(t, ctx, q, "adzuna", "Acme Corp", "Senior Go Engineer", "https://adzuna.example.com/jobs/123")

	otherJob := dto.NormalizedJob{
		SourceKey:   "indeed",
		Title:       "Senior Go Engineer",
		Company:     "Acme Corp",
		URL:         "https://indeed.example.com/jobs/456",
		Description: "Same job, different aggregator",
		Raw:         map[string]any{},
	}

	if IsBoardVendor(otherJob.SourceKey) {
		t.Fatal("indeed should NOT be a board vendor")
	}

	existingID, err := FindMergeCandidate(ctx, q, otherJob)
	if err != nil {
		t.Fatalf("find merge candidate: %v", err)
	}
	if existingID.Valid {
		t.Fatal("non-board vendor should not produce a merge candidate")
	}
}

func TestMerge_TwoBoardPostings_SecondMergesIntoFirst(t *testing.T) {
	ctx, q, cleanup := setupMergeTest(t)
	defer cleanup()

	insertJob(t, ctx, q, "greenhouse", "Acme Corp", "Senior Go Engineer", "https://boards.greenhouse.io/acme/jobs/001")

	leverJob := dto.NormalizedJob{
		SourceKey:   "lever",
		Title:       "Senior Go Engineer",
		Company:     "Acme Corp",
		URL:         "https://jobs.lever.co/acme/002",
		Description: "Same role on Lever",
		Raw:         map[string]any{},
	}

	if !IsBoardVendor(leverJob.SourceKey) {
		t.Fatal("lever should be a board vendor")
	}

	existingID, err := FindMergeCandidate(ctx, q, leverJob)
	if err != nil {
		t.Fatalf("find merge candidate: %v", err)
	}
	if !existingID.Valid {
		t.Fatal("lever board posting should find greenhouse as merge candidate")
	}

	merged, err := q.MergeJobBoard(ctx, sqlcgen.MergeJobBoardParams{
		ID:          existingID,
		Url:         leverJob.URL,
		SourceKey:   leverJob.SourceKey,
		ArrayAppend: leverJob.SourceKey,
	})
	if err != nil {
		t.Fatalf("merge job board: %v", err)
	}

	foundGreenhouse := false
	foundLever := false
	for _, s := range merged.SeenOnSources {
		if s == "greenhouse" {
			foundGreenhouse = true
		}
		if s == "lever" {
			foundLever = true
		}
	}
	if !foundGreenhouse {
		t.Error("seenOnSources should include greenhouse")
	}
	if !foundLever {
		t.Error("seenOnSources should include lever")
	}
}

func TestIsBoardVendor(t *testing.T) {
	for _, v := range []string{"greenhouse", "lever", "ashby", "workable", "smartrecruiters"} {
		if !IsBoardVendor(v) {
			t.Errorf("%s should be a board vendor", v)
		}
	}
	for _, v := range []string{"adzuna", "indeed", "djinni", "dou", "workua", ""} {
		if IsBoardVendor(v) {
			t.Errorf("%s should NOT be a board vendor", v)
		}
	}
}

func TestTitlesOverlap(t *testing.T) {
	cases := []struct{ a, b string; want bool }{
		{"Senior Go Engineer", "Senior Go Engineer", true},
		{"Sr. Go Engineer", "Senior Go Engineer", true},
		{"Go Engineer", "Senior Go Engineer", true},
		{"Backend Engineer (Go)", "Senior Go Engineer", true},
		{"Senior Go Engineer", "Senior Backend Engineer", true},
		{"Junior Frontend Developer", "Senior Go Engineer", false},
		{"Software Engineer", "Senior Go Engineer", false},
		{"", "Senior Go Engineer", false},
		{"Senior Go Engineer", "", false},
		{"Data Scientist", "Data Engineer", false},
		{"Product Manager", "Program Manager", false},
	}
	for _, c := range cases {
		got := titlesOverlap(c.a, c.b)
		if got != c.want {
			t.Errorf("titlesOverlap(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestMerge_InsertIfNew_Integration(t *testing.T) {
	ctx, q, cleanup := setupMergeTest(t)
	defer cleanup()

	aggregatorJob := insertJob(t, ctx, q, "adzuna", "Acme Corp", "Senior Go Engineer", "https://adzuna.example.com/jobs/123")

	eventsJSON, _ := json.Marshal([]map[string]string{{"status": "found", "at": time.Now().Format(time.RFC3339)}})
	if err := q.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId:  aggregatorJob.ID,
		Status: "found",
		Events: eventsJSON,
	}); err != nil {
		t.Fatalf("upsert application: %v", err)
	}

	boardJob := dto.NormalizedJob{
		SourceKey:   "greenhouse",
		Title:       "Senior Go Engineer",
		Company:     "Acme Corp",
		URL:         "https://boards.greenhouse.io/acme/jobs/456",
		Description: "Senior Go Engineer role at Acme Corp",
		Raw:         map[string]any{},
	}

	boardDedupeKey := DedupeKey(boardJob.Company, boardJob.Title, boardJob.URL)
	if _, err := q.GetJobByDedupeKey(ctx, boardDedupeKey); err == nil {
		t.Fatal("board job should not match by dedupe key (different URL)")
	}

	existingID, err := FindMergeCandidate(ctx, q, boardJob)
	if err != nil {
		t.Fatalf("find merge candidate: %v", err)
	}
	if !existingID.Valid {
		t.Fatal("expected merge candidate")
	}
	if existingID != aggregatorJob.ID {
		t.Errorf("expected merge candidate ID %s, got %s", dbutil.UUIDString(aggregatorJob.ID), dbutil.UUIDString(existingID))
	}

	merged, err := q.MergeJobBoard(ctx, sqlcgen.MergeJobBoardParams{
		ID:          existingID,
		Url:         boardJob.URL,
		SourceKey:   boardJob.SourceKey,
		ArrayAppend: boardJob.SourceKey,
	})
	if err != nil {
		t.Fatalf("merge job board: %v", err)
	}

	if merged.Url != boardJob.URL {
		t.Errorf("expected merged url %q, got %q", boardJob.URL, merged.Url)
	}

	refetched, err := q.GetJobByID(ctx, aggregatorJob.ID)
	if err != nil {
		t.Fatalf("get job by id after merge: %v", err)
	}
	if refetched.SourceKey != "greenhouse" {
		t.Errorf("expected sourceKey greenhouse after merge, got %s", refetched.SourceKey)
	}
	if refetched.Url != boardJob.URL {
		t.Errorf("expected url %q after merge, got %q", boardJob.URL, refetched.Url)
	}
	if len(refetched.SeenOnSources) < 2 {
		t.Errorf("expected at least 2 seenOnSources, got %d: %v", len(refetched.SeenOnSources), refetched.SeenOnSources)
	}
}
