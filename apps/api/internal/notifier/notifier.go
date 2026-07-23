// Package notifier is the facade for the fresh-match notification bounded
// context: the Repository/NotificationReader/ProfileResolver outbound
// ports and the freshness rule live in domain/; the write-path
// (MaybeNotify) and read-path (NotificationService) orchestration live in
// application/. This file re-exports the shape callers already depend on
// (compose.go, matching/handler.go) so relocating the package required no
// changes at call sites beyond the import path.
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
