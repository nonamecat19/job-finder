// Package scraping is the facade for the scraping platform kernel: the
// Scraper port lives in domain/, the HTTP+chromedp implementation in
// infrastructure/. This file re-exports the shape callers already depend on
// so relocating the package into internal/platform/scraping required no
// changes at call sites beyond the import path.
package scraping

import (
	"github.com/job-finder/api/internal/platform/scraping/domain"
	"github.com/job-finder/api/internal/platform/scraping/infrastructure"
)

// Scraper is the port consumers should depend on instead of *Service, so a
// fake/mock can be substituted in tests without a concrete dependency.
type Scraper = domain.Scraper

// Service is kept as an alias to the concrete implementation for existing
// call sites that still declare a *scraping.Service field; new code should
// prefer the Scraper interface.
type Service = infrastructure.HTTPScraper

var New = infrastructure.New
