package retrieval

import (
	"net/http"
	"time"

	"github.com/job-finder/jobscraper/httpjson"
	jsretrieval "github.com/job-finder/jobscraper/retrieval"
)

// ConfigureDefaultTransport points the library's shared paced transport at the
// app's host state, so a site-requested crawl delay becomes the per-host rate.
func ConfigureDefaultTransport(store jsretrieval.StateStorePort, overrides map[string]float64) {
	jsretrieval.NewDefaultTransport(store, overrides)
}

// UsePacedHTTPJSONClient routes the library's default JSON client through the
// paced transport. httpjson lost that wiring when it moved into the library
// (it must not depend on retrieval there), so the app restores it at startup.
func UsePacedHTTPJSONClient() {
	httpjson.SetDefaultClient(&http.Client{
		Timeout:   30 * time.Second,
		Transport: jsretrieval.DefaultTransport,
	})
}
