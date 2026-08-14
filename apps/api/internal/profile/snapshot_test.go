package profile_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/profile"
)

type countingRepo struct {
	profile.Repository
	row               sqlcgen.Profile
	hasEmbeddingCalls int
}

func (f *countingRepo) GetDefaultProfile(ctx context.Context) (sqlcgen.Profile, error) {
	return f.row, nil
}

func (f *countingRepo) ProfileHasEmbedding(ctx context.Context, id pgtype.UUID) (interface{}, error) {
	f.hasEmbeddingCalls++
	return false, nil
}

func rendercvConfig(name string) []byte {
	return []byte(`{"cv":{"name":"` + name + `"}}`)
}

func fakeProfile(name string, updatedAt time.Time) sqlcgen.Profile {
	uid, _ := dbutil.ParseUUID("11111111-1111-1111-1111-111111111111")
	return sqlcgen.Profile{
		ID:             uid,
		RendercvConfig: rendercvConfig(name),
		UpdatedAt:      pgtype.Timestamp{Time: updatedAt, Valid: true},
	}
}

func TestSnapshotCache_HitReturnsIdenticalText(t *testing.T) {
	repo := &countingRepo{
		row: fakeProfile("Ada", time.Unix(1000, 0)),
	}
	svc := profile.NewService(repo, nil)
	cache := profile.NewSnapshotCache(svc)

	snap1, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	snap2, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap1.ProfileText != snap2.ProfileText {
		t.Errorf("expected identical text on cache hit, got %q vs %q", snap1.ProfileText, snap2.ProfileText)
	}
	if repo.hasEmbeddingCalls != 1 {
		t.Errorf("expected HasEmbedding called once (cache hit skips rebuild), got %d", repo.hasEmbeddingCalls)
	}
}

func TestSnapshotCache_NewerUpdatedAtInvalidates(t *testing.T) {
	repo := &countingRepo{
		row: fakeProfile("Ada", time.Unix(1000, 0)),
	}
	svc := profile.NewService(repo, nil)
	cache := profile.NewSnapshotCache(svc)

	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	repo.row = fakeProfile("Grace", time.Unix(2000, 0))
	snap2, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.hasEmbeddingCalls != 2 {
		t.Errorf("expected rebuild (HasEmbedding called twice) after UpdatedAt changed, got %d", repo.hasEmbeddingCalls)
	}
	if snap2.ProfileText == "" {
		t.Error("expected non-empty rebuilt text")
	}
}

func TestSnapshotCache_ExplicitInvalidateForcesRebuild(t *testing.T) {
	repo := &countingRepo{
		row: fakeProfile("Ada", time.Unix(1000, 0)),
	}
	svc := profile.NewService(repo, nil)
	cache := profile.NewSnapshotCache(svc)

	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	cache.Invalidate()
	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.hasEmbeddingCalls != 2 {
		t.Errorf("expected rebuild after explicit Invalidate, got %d HasEmbedding calls", repo.hasEmbeddingCalls)
	}
}
