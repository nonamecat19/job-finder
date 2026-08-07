package domain

type Registry struct {
	scrapers []Scraper
}

func NewRegistry(scrapers ...Scraper) *Registry {
	return &Registry{scrapers: scrapers}
}

func (r *Registry) All() []Scraper {
	out := make([]Scraper, len(r.scrapers))
	copy(out, r.scrapers)
	return out
}

func (r *Registry) ByDomain() map[string][]Scraper {
	groups := make(map[string][]Scraper)
	for _, s := range r.scrapers {
		groups[s.Domain()] = append(groups[s.Domain()], s)
	}
	return groups
}
