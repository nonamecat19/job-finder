package retrieval

import (
	"context"

	"github.com/job-finder/api/internal/ratelimit"
)

var DefaultTransport = ratelimit.NewTransport(nil)

func ConfigureDefaultTransport(store *StateStore, overrides map[string]float64) {
	DefaultTransport.RateResolver = NewRateResolver(store, overrides)
}

func NewRateResolver(store *StateStore, overrides map[string]float64) func(host string) (float64, string, bool) {
	return func(host string) (float64, string, bool) {
		if rps, ok := overrides[host]; ok && rps > 0 {
			return rps, "override", true
		}

		ctx := context.Background()
		state, err := store.Get(ctx, host)
		if err != nil {
			return 0, "", false
		}

		if state.CrawlDelaySeconds == nil {
			return 0, "", false
		}

		delay := *state.CrawlDelaySeconds
		if delay <= 0 {
			return 0, "", false
		}

		candidate := 1 / float64(delay)
		if candidate < ratelimit.DefaultRPS {
			return candidate, "site-requested", true
		}
		return 0, "", false
	}
}
