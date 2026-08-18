package manualadd

import (
	"github.com/job-finder/api/internal/manualadd/application"
	"github.com/job-finder/api/internal/manualadd/domain"
)

type (
	Repository          = domain.Repository
	SourceProvider      = domain.SourceProvider
	SubscriptionEnsurer = domain.SubscriptionEnsurer
	AdapterRegistry     = domain.AdapterRegistry
	JobReader           = domain.JobReader
	Enqueuer            = domain.Enqueuer
	TxRunner            = domain.TxRunner

	Failure     = domain.Failure
	FailureKind = domain.FailureKind
	Outcome     = domain.Outcome
	Result      = domain.Result
	Draft       = domain.Draft

	Service = application.Service
)

var (
	NewService   = application.NewService
	WithTxRunner = application.WithTxRunner
	WithFillIn   = application.WithFillIn
)

const AddTimeout = application.AddTimeout
