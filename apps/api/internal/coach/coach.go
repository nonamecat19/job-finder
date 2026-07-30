package coach

import (
	"github.com/job-finder/api/internal/coach/application"
	"github.com/job-finder/api/internal/coach/domain"
)

type (
	Service            = application.Service
	AssessmentService  = application.AssessmentService
	ProfileEntry       = domain.ProfileEntry
	FitGapAssessment   = domain.FitGapAssessment
	ProfileEntriesFunc = application.ProfileEntriesFunc
	DiffReader         = application.DiffReader
	RephraseModel      = application.RephraseModel
)

var (
	NewService           = application.NewService
	NewAssessmentService = application.NewAssessmentService
	ErrNoDiff            = application.ErrNoDiff
	ErrNotAssessed       = application.ErrNotAssessed
)
