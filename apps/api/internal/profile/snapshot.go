package profile

import (
	"context"
	"sync"
	"time"

	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/strutil"
)

type Snapshot struct {
	ProfileID    string
	ProfileText  string
	HasEmbedding bool
	Version      time.Time
}

type SnapshotCache struct {
	svc *Service

	mu  sync.Mutex
	cur *Snapshot
}

func NewSnapshotCache(svc *Service) *SnapshotCache {
	return &SnapshotCache{svc: svc}
}

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

func (c *SnapshotCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur = nil
}
