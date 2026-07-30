package salary

import (
	"github.com/job-finder/api/internal/salary/application"
	"github.com/job-finder/api/internal/salary/domain"
	salaryworker "github.com/job-finder/api/internal/salary/interfaces/worker"
)

type (
	Service         = application.Service
	LevelsFyiLoader = application.LevelsFyiLoader
	Handler         = salaryworker.Handler
	Repository      = application.Repository
	SalaryBand      = domain.SalaryBand
	SalarySource    = domain.SalarySource
	BlendedBand     = domain.BlendedBand
)

var (
	NewService         = application.NewService
	NewLevelsFyiLoader = application.NewLevelsFyiLoader
	NewHandler         = salaryworker.NewHandler
	ParseSalaryRaw     = domain.ParseSalaryRaw
)
