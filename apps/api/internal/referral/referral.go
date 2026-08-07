package referral

import (
	"github.com/job-finder/api/internal/referral/application"
	"github.com/job-finder/api/internal/referral/domain"
	"github.com/job-finder/api/internal/referral/infrastructure/github"
)

type (
	Contact           = domain.Contact
	ContactConnection = domain.ContactConnection
	ReferralPath      = domain.ReferralPath
	ImportSummary     = domain.ImportSummary
	GithubSyncResult  = domain.GithubSyncResult
	CSVContact        = domain.CSVContact
	Repository        = domain.Repository
	JobRepository     = domain.JobRepository
	Ranker            = domain.Ranker

	Service    = application.Service
	PathFinder = application.PathFinder

	GitHubCrossReferencer = github.GitHubCrossReferencer
	GitHubProfile         = github.GitHubProfile
)

var (
	ErrContactNotFound  = domain.ErrContactNotFound
	ErrNoGithubUsername = domain.ErrNoGithubUsername

	ParseCSV  = domain.ParseCSV
	NewRanker = domain.NewRanker

	NewService    = application.NewService
	NewPathFinder = application.NewPathFinder

	NewGitHubCrossReferencer = github.NewGitHubCrossReferencer
)
