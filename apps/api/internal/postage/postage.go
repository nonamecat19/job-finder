// Package postage is the facade for the post-age-at-apply response-rate
// signal: the Repository port and cold-start threshold constants live in
// domain/, the SQL-aggregation-to-DTO orchestration in application/. This
// file re-exports the shape callers already depend on (compose.go) so
// relocating the package required no changes at call sites beyond the
// import path.
package postage

import (
	"github.com/job-finder/api/internal/postage/application"
	"github.com/job-finder/api/internal/postage/domain"
)

type (
	Repository = domain.Repository
	Service    = application.Service
)

const (
	GlobalColdStartThreshold = domain.GlobalColdStartThreshold
	PerBucketMinSample       = domain.PerBucketMinSample
	DocumentedPriorRate      = domain.DocumentedPriorRate
	DocumentedPriorLabel     = domain.DocumentedPriorLabel
)

var NewService = application.NewService
