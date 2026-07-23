package domain_test

import (
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/jobsources/domain"
)

// *sqlcgen.Queries must satisfy the SearchRepository port structurally.
var _ domain.SearchRepository = (*sqlcgen.Queries)(nil)
