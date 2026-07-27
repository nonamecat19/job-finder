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
	CrawlDelaySeconds *int
	Pacing            HostPacing
}

// HostPacing is the rate currently in force for a host and where it came
// from. Derived on demand from the pacing transport; never persisted.
type HostPacing struct {
	RequestsPerSecond float64
	IntervalSeconds   float64
	Source            string
}

type Service interface {
	Fetch(ctx context.Context, req FetchRequest) (FetchResult, error)
	HostStatus(ctx context.Context, host string) (HostStatus, error)
	ClearRungPreference(ctx context.Context, host string) error
	ClearCookies(ctx context.Context, host string) error
	OverrideCoolingOff(ctx context.Context, host string) (time.Duration, error)
}
