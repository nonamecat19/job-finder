package dto

// The scraping stack lives in the jobscraper library; these aliases keep every
// existing dto.X reference in the app working against the library's canonical
// types.
//
// This file is excluded from tygo generation (see apps/api/tygo.yaml): tygo
// cannot follow aliases across the module boundary, so the TypeScript for these
// four types is generated from the library package into generated-model.ts and
// re-exported from generated.ts.

import "github.com/job-finder/jobscraper/model"

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
	// SourceKindManual backs hand-entered vacancies on hosts no adapter reads.
	// It is never crawled — its adapter's Search fails permanently (041 D4).
	SourceKindManual = model.SourceKindManual
)
