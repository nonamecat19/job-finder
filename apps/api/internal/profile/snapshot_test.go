package profile_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/domain"
	"github.com/job-finder/api/internal/profile"
)

type countingRepo struct {
	profile.Repository
	row               domain.Profile
	hasEmbeddingCalls int
}

func (f *countingRepo) GetDefaultProfile(ctx context.Context) (domain.Profile, error) {
	return f.row, nil
}

func (f *countingRepo) ProfileHasEmbedding(ctx context.Context, id pgtype.UUID) (interface{}, error) {
	f.hasEmbeddingCalls++
	return false, nil
}

func rendercvConfig(name string) []byte {
	return []byte(`{"cv":{"name":"` + name + `"}}`)
}

func TestSnapshotCache_HitReturnsIdenticalText(t *testing.T) {
	repo := &countingRepo{
		row: domain.Profile{ID: "11111111-1111-1111-1111-111111111111", RendercvConfig: rendercvConfig("Ada"), UpdatedAt: time.Unix(1000, 0)},
	}
	svc := profile.NewService(repo, nil, "", "")
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
		row: domain.Profile{ID: "11111111-1111-1111-1111-111111111111", RendercvConfig: rendercvConfig("Ada"), UpdatedAt: time.Unix(1000, 0)},
	}
	svc := profile.NewService(repo, nil, "", "")
	cache := profile.NewSnapshotCache(svc)

	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	repo.row = domain.Profile{ID: "11111111-1111-1111-1111-111111111111", RendercvConfig: rendercvConfig("Grace"), UpdatedAt: time.Unix(2000, 0)}
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
		row: domain.Profile{ID: "11111111-1111-1111-1111-111111111111", RendercvConfig: rendercvConfig("Ada"), UpdatedAt: time.Unix(1000, 0)},
	}
	svc := profile.NewService(repo, nil, "", "")
	cache := profile.NewSnapshotCache(svc)

	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	cache.Invalidate()
	// UpdatedAt unchanged, but Invalidate forces a rebuild anyway (e.g. after
	// RefreshEmbedding, which doesn't touch UpdatedAt).
	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.hasEmbeddingCalls != 2 {
		t.Errorf("expected rebuild after explicit Invalidate, got %d HasEmbedding calls", repo.hasEmbeddingCalls)
	}
}
