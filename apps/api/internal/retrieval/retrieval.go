// Package retrieval is the single shared HTTP retrieval interface for all
// job-source adapters (014-browser-fidelity-fetch-ladder). Every scraped
// adapter fetches through this package so no source implements its own
// request strategy or challenge handling (FR-020).
package retrieval
