package notifier

import (
	"github.com/job-finder/api/internal/notifier/application"
	"github.com/job-finder/api/internal/notifier/domain"
)

type (
	Repository         = domain.Repository
	NotificationReader = domain.NotificationReader
	ProfileResolver    = domain.ProfileResolver

	Service             = application.Service
	Option              = application.Option
	NotificationService = application.NotificationService
)

var (
	IsFresh = domain.IsFresh

	NewService             = application.NewService
	WithMatchThreshold     = application.WithMatchThreshold
	WithRateLimitCap       = application.WithRateLimitCap
	NewNotificationService = application.NewNotificationService
)
