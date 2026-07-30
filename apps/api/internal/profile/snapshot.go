package profile

import (
	"context"
	"sync"
	"time"

	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/strutil"
)

// Snapshot is the versioned, per-process cache of the default profile's
// derived text (019-ai-job-throughput, data-model.md §5), removing the
// per-job repeat work identified in research.md R5: parsing rendercvConfig
// and rendering it to text is the same output every time until the profile
// is written again.
type Snapshot struct {
	ProfileID    string
	ProfileText  string
	HasEmbedding bool
	Version      time.Time
}

// SnapshotCache holds the current Snapshot for one process. Get rebuilds
// from Service.GetDefault only on a miss or a version change (a newer
// profile UpdatedAt); Invalidate forces a rebuild on the next Get even when
// UpdatedAt hasn't moved (e.g. RefreshEmbedding, which touches embedding
// columns only).
type SnapshotCache struct {
	svc *Service

	mu  sync.Mutex
	cur *Snapshot
}

func NewSnapshotCache(svc *Service) *SnapshotCache {
	return &SnapshotCache{svc: svc}
}

// Get returns the current snapshot, rebuilding it when the cache is empty,
// invalidated, or the default profile has changed since it was built.
func (c *SnapshotCache) Get(ctx context.Context) (Snapshot, error) {
	prof, err := c.svc.GetDefault(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	profID := dbutil.UUIDString(prof.ID)

	c.mu.Lock()
	if c.cur != nil && c.cur.ProfileID == profID && c.cur.Version.Equal(prof.UpdatedAt.Time) {
		snap := *c.cur
		c.mu.Unlock()
		return snap, nil
	}
	c.mu.Unlock()

	master, err := generation.MasterFromProfile(prof)
	if err != nil {
		return Snapshot{}, err
	}
	var extraNotes string
	if prof.ExtraNotes != nil {
		extraNotes = *prof.ExtraNotes
	}
	// Truncated to 6000 exactly as scoring.go:64 does today, so caching this
	// value changes nothing about what reaches the prompt (Constitution II).
	text := strutil.Truncate(generation.RendercvToText(master)+"\n"+extraNotes, 6000)
	hasEmbedding, err := c.svc.HasEmbedding(ctx, profID)
	if err != nil {
		return Snapshot{}, err
	}

	snap := Snapshot{
		ProfileID:    profID,
		ProfileText:  text,
		HasEmbedding: hasEmbedding,
		Version:      prof.UpdatedAt.Time,
	}
	c.mu.Lock()
	c.cur = &snap
	c.mu.Unlock()
	return snap, nil
}

// Invalidate forces the next Get to rebuild from Service.GetDefault,
// regardless of whether UpdatedAt changed.
func (c *SnapshotCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur = nil
}
