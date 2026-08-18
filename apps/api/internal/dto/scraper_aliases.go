package dto

import "github.com/nonamecat19/job-scraper/model"

type (
	NormalizedJob = model.NormalizedJob
	SearchQuery   = model.SearchQuery
	JobSourceDto  = model.JobSourceDto
	SourceKind    = model.SourceKind
)

const (
	SourceKindAPI     = model.SourceKindAPI
	SourceKindScrape  = model.SourceKindScrape
	SourceKindSidecar = model.SourceKindSidecar

	SourceKindManual = model.SourceKindManual
)
