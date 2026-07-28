package main

import (
	"github.com/job-finder/api/internal/applications"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/ingestion"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/notifier"
	"github.com/job-finder/api/internal/postage"
	"github.com/job-finder/api/internal/subscriptions"
)

func composeApplications(p *Platform) *httpapi.ApplicationsHandler {
	svc := applications.NewService(p.DB.Queries, p.DB)
	return &httpapi.ApplicationsHandler{Applications: svc}
}

func composeSubscriptions(p *Platform, sourcesSvc *jobsources.Service, ingestionSvc *ingestion.Service) *httpapi.SubscriptionsHandler {
	svc := subscriptions.NewService(p.DB.Queries, sourcesSvc)
	return &httpapi.SubscriptionsHandler{Subs: svc, Ingestion: ingestionSvc}
}

func composePostAge(p *Platform) *httpapi.PostAgeHandler {
	return &httpapi.PostAgeHandler{PostAge: postage.NewService(p.DB.Queries)}
}

func composeNotifications(p *Platform) *httpapi.NotificationHandler {
	return &httpapi.NotificationHandler{Provider: notifier.NewNotificationService(p.DB.Queries, p.DB.Queries)}
}

func composeHealth(p *Platform) *httpapi.HealthHandler {
	var minioPing httpapi.Pinger
	if p.MinioReady != nil {
		minioPing = minioPinger{client: p.MinioReady, bucket: p.Config.MinioBucket}
	}
	return &httpapi.HealthHandler{
		Postgres: p.DB.Pool,
		Redis:    redisPinger{client: p.RedisClient},
		Minio:    minioPing,
	}
}

func composeHosts(p *Platform) *httpapi.HostsHandler {
	return &httpapi.HostsHandler{Retrieval: hostsAdapter{p.Retrieval}}
}
