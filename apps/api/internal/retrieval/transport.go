package retrieval

import (
	"github.com/nonamecat19/jobscraper/ports"
	jsretrieval "github.com/nonamecat19/jobscraper/retrieval"
)

// ConfigureDefaultTransport points the library's shared paced transport at the
// app's host state, so a site-requested crawl delay becomes the per-host rate.
func ConfigureDefaultTransport(store ports.StateStore, overrides map[string]float64) {
	jsretrieval.ConfigureDefaultTransport(store, overrides)
}
