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
