package companyintel

// Registry holds every registered Scraper, grouped by the external domain
// they hit so the caller can pace/parallelize per FR-012 ("no faster than
// one request per 2 seconds per source domain") while still running
// distinct domains concurrently.
type Registry struct {
	scrapers []Scraper
}

// NewRegistry builds a registry from the given scrapers. Order is
// preserved for deterministic logging/testing but otherwise unused —
// scrapers are grouped by domain at refresh time.
func NewRegistry(scrapers ...Scraper) *Registry {
	return &Registry{scrapers: scrapers}
}

// All returns every registered scraper.
func (r *Registry) All() []Scraper {
	out := make([]Scraper, len(r.scrapers))
	copy(out, r.scrapers)
	return out
}

// ByDomain groups the registered scrapers by Domain(), so the service can
// run each domain's scrapers sequentially (respecting its pacing) while
// different domains run concurrently.
func (r *Registry) ByDomain() map[string][]Scraper {
	groups := make(map[string][]Scraper)
	for _, s := range r.scrapers {
		groups[s.Domain()] = append(groups[s.Domain()], s)
	}
	return groups
}
