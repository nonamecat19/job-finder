package retrieval

import "github.com/job-finder/api/internal/ratelimit"

var DefaultTransport = ratelimit.NewTransport(nil)
