package domain_test

import (
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/jobsources/domain"
)

var _ domain.SearchRepository = (*sqlcgen.Queries)(nil)
