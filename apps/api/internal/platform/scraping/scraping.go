package scraping

import (
	"github.com/job-finder/api/internal/platform/scraping/domain"
	"github.com/job-finder/api/internal/platform/scraping/infrastructure"
)

type Scraper = domain.Scraper

type Service = infrastructure.HTTPScraper

var New = infrastructure.New
