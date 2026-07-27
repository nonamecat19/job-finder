package retrieval

import (
	"context"

	"github.com/job-finder/api/internal/ratelimit"
)

// DefaultTransport is the shared paced transport used by every adapter and
// enrichment fetch that doesn't carry its own client. It starts unresolved
// (DefaultRPS for every host); ConfigureDefaultTransport wires in the
// StateStore-backed resolver once composition has a store to hand it, since
// this var is initialized before that exists.
var DefaultTransport = ratelimit.NewTransport(nil)

// ConfigureDefaultTransport wires a StateStore-backed rate resolver into
// DefaultTransport, implementing the precedence in data-model.md: operator
// override → site-requested (only when slower than default) → default.
// overrides is an optional operator-configured per-host RPS map; it may be
// nil.
func ConfigureDefaultTransport(store *StateStore, overrides map[string]float64) {
	DefaultTransport.RateResolver = NewRateResolver(store, overrides)
}

// NewRateResolver builds a ratelimit.Transport.RateResolver backed by store.
// It never runs on the per-request path — the transport only calls it at
// limiter construction and TTL refresh — so the occasional extra query here
// is fine.
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
			// Never looked. Discovery is triggered from the retrieval
			// service on first contact (service_impl.go), not from here —
			// the resolver stays a pure read so it never blocks other
			// hosts' limiter lookups on a robots.txt round trip. Pace at
			// the default in the meantime.
			return 0, "", false
		}

		delay := *state.CrawlDelaySeconds
		if delay <= 0 {
			// 0 means "asked, nothing advertised" — never "no delay".
			return 0, "", false
		}

		candidate := 1 / float64(delay)
		if candidate < ratelimit.DefaultRPS {
			return candidate, "site-requested", true
		}
		// A crawl delay faster than the default is ignored; the
		// conservative default stands.
		return 0, "", false
	}
}
