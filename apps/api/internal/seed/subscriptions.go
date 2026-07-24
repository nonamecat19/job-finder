package seed

import (
	"context"
	"log/slog"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type subscriptionSeed struct {
	sourceKey string
	name      string
	url       string
	enabled   bool
}

var subscriptions = []subscriptionSeed{
	{
		sourceKey: "djinni",
		name:      "Djinni Go Jobs",
		url:       "https://djinni.io/jobs?technology=golang&remote=true",
		enabled:   true,
	},
	{
		sourceKey: "remotive",
		name:      "Remotive Remote Jobs",
		url:       "https://remotive.com/remote-jobs?category=software-dev",
		enabled:   true,
	},
	{
		sourceKey: "arbeitnow",
		name:      "Arbeitnow EU Jobs",
		url:       "https://arbeitnow.com/api/job-board-api",
		enabled:   true,
	},
	{
		sourceKey: "indeed",
		name:      "Indeed Go Remote",
		url:       "https://www.indeed.com/jobs?q=golang&l=remote",
		enabled:   true,
	},
	{
		sourceKey: "remoteok",
		name:      "RemoteOK Golang",
		url:       "https://remoteok.com/remote-golang-jobs",
		enabled:   true,
	},
	{
		sourceKey: "glassdoor",
		name:      "Glassdoor Go Remote",
		url:       "https://www.glassdoor.com/Job/remote-golang-jobs-SRCH_KO0,14.htm",
		enabled:   true,
	},
	{
		sourceKey: "jobleads",
		name:      "JobLeads Golang",
		url:       "https://www.jobleads.com/job-search?q=golang",
		enabled:   true,
	},
}

func seedSubscriptions(ctx context.Context, q *sqlcgen.Queries) error {
	existing, err := q.ListSubscriptions(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		slog.Info("seed: subscriptions already exist, skipping")
		return nil
	}

	for _, s := range subscriptions {
		_, err := q.CreateSubscription(ctx, sqlcgen.CreateSubscriptionParams{
			SourceKey: s.sourceKey,
			Name:      &s.name,
			Url:       s.url,
			Enabled:   s.enabled,
		})
		if err != nil {
			return err
		}
	}

	slog.Info("seed: created subscriptions", "count", len(subscriptions))
	return nil
}
