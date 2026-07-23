// Package referral is the facade for the Referral bounded context (plan
// 011): the Contact/ContactConnection/ReferralPath model, CSV parsing, and
// outbound ports live in domain/; CSV-import/GitHub-sync/path-finding
// orchestration lives in application/; the GitHub REST client lives in
// infrastructure/github/. This file re-exports the shape callers already
// depend on (compose.go, httpapi/referral.go) so relocating the package
// required no changes at call sites beyond the import path.
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
