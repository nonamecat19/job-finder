package retrieval

import (
	"context"
	"time"
)

type FetchRequest struct {
	URL             string
	Headers         map[string]string
	UsesUserAccount bool
	RefererPage     string
}

type FetchResult struct {
	Outcome PageOutcome
	Body    string
}

type HostStatus struct {
	Host              string
	IdentityVersion   string
	CurrentRung       string
	LastBlockAt       *time.Time
	LastBlockReason   string
	CoolingOffUntil   *time.Time
	BudgetUsed        int
	BudgetLimit       int
	BudgetResetsAt    time.Time
	CrawlDelaySeconds *int
}

type Service interface {
	Fetch(ctx context.Context, req FetchRequest) (FetchResult, error)
	HostStatus(ctx context.Context, host string) (HostStatus, error)
	ClearRungPreference(ctx context.Context, host string) error
	ClearCookies(ctx context.Context, host string) error
	OverrideCoolingOff(ctx context.Context, host string) (time.Duration, error)
}
