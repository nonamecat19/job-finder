package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/enrichment"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/ghostjob"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/ingestion"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/jobsources/adapters"
	"github.com/job-finder/api/internal/jobsources/roster"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/llmsettings"
	"github.com/job-finder/api/internal/matching"
	"github.com/job-finder/api/internal/retrieval"
	"github.com/job-finder/api/internal/salary"
)

// App is the fully wired set of HTTP and worker handlers plus the scheduler,
// consumed by buildServers (router mounting + worker muxes) and runServers.
type App struct {
	Sources       *httpapi.SourcesHandler
	Roster        *httpapi.RosterHandler
	Searches      *httpapi.SearchesHandler
	Documents     *httpapi.DocumentsHandler
	Profiles      *httpapi.ProfilesHandler
	Jobs          *httpapi.JobsHandler
	Applications  *httpapi.ApplicationsHandler
	Subs          *httpapi.SubscriptionsHandler
	Activity      *httpapi.ActivityHandler
	Keyword       *httpapi.KeywordHandler
	PostAge       *httpapi.PostAgeHandler
	Notification  *httpapi.NotificationHandler
	Companies     *httpapi.CompaniesHandler
	GhostJob      *httpapi.GhostJobHandler
	Coach         *httpapi.CoachHandler
	ExtAuth       *httpapi.ExtAuthHandler
	ExtProfile    *httpapi.ExtProfileHandler
	Contacts      *httpapi.ContactsHandler
	Referral      *httpapi.ReferralHandler
	Outreach      *httpapi.OutreachHandler
	LlmSettings   *httpapi.LlmSettingsHandler
	AiFeatures    *httpapi.AiFeatureHandler
	InterviewPrep *httpapi.InterviewPrepHandler
	Health        *httpapi.HealthHandler
	Hosts         *httpapi.HostsHandler

	Ingestion  *ingestion.Handler
	Matching   *matching.Handler
	Generation *generation.Handler
	Enrichment *enrichment.Handler
	Salary     *salary.Handler
	Ghost      *ghostjob.Handler

	Scheduler *ingestion.Scheduler
}

type sourcesHandles struct {
	Registry  *jobsources.Registry
	Sources   *jobsources.Service
	Roster    *roster.Service
	Djinni    adapters.DjinniAdapter
	Dou       adapters.DouAdapter
	Workua    adapters.WorkUaAdapter
	Indeed    adapters.IndeedAdapter
	RemoteOK  adapters.RemoteOKAdapter
	Glassdoor adapters.GlassdoorAdapter
	JobLeads  adapters.JobLeadsAdapter
	Wellfound adapters.WellfoundAdapter
	Himalayas adapters.HimalayasAdapter
	Jobgether adapters.JobgetherAdapter
}

type ingestionHandles struct {
	Ingestion *ingestion.Service
	Handler   *ingestion.Handler
	Scheduler *ingestion.Scheduler
	Sources   *httpapi.SourcesHandler
	Searches  *httpapi.SearchesHandler
}

type llmHandles struct {
	Ollama           *llm.OllamaProvider
	MatchRouter      *llm.Router
	GenerationRouter *llm.Router
	RephraseRouter   *llm.Router
	GhostRouter      *llm.Router
	DefaultRouter    *llm.Router
	Settings         *llmsettings.Service
	SettingsHandler  *httpapi.LlmSettingsHandler
}

// redisPinger wraps a Redis client to satisfy httpapi.Pinger.
type redisPinger struct{ client redis.UniversalClient }

func (r redisPinger) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// minioPinger wraps a MinIO client + bucket to satisfy httpapi.Pinger.
type minioPinger struct {
	client *minio.Client
	bucket string
}

func (m minioPinger) Ping(ctx context.Context) error {
	_, err := m.client.BucketExists(ctx, m.bucket)
	return err
}

// extJWTSigningSecret decodes a configured 32-byte hex EXT_JWT_SECRET, or
// generates a random 32-byte secret when unset.
func extJWTSigningSecret(hexKey string) ([]byte, error) {
	if hexKey == "" {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, err
		}
		slog.Warn("EXT_JWT_SECRET not set — using an ephemeral random secret; " +
			"extension sessions will not survive a server restart")
		return secret, nil
	}
	secret, err := hex.DecodeString(hexKey)
	if err != nil || len(secret) != 32 {
		return nil, errors.New("EXT_JWT_SECRET must be a 32-byte hex string (openssl rand -hex 32)")
	}
	return secret, nil
}

// hostsAdapter maps retrieval.Service onto httpapi.HostRetrievalProvider.
type hostsAdapter struct{ retrieval retrieval.Service }

func (a hostsAdapter) HostStatus(ctx context.Context, host string) (dto.HostRetrievalStatusDto, error) {
	s, err := a.retrieval.HostStatus(ctx, host)
	if err != nil {
		return dto.HostRetrievalStatusDto{}, err
	}
	out := dto.HostRetrievalStatusDto{
		Host:              s.Host,
		IdentityVersion:   s.IdentityVersion,
		CurrentRung:       s.CurrentRung,
		LastBlockAt:       s.LastBlockAt,
		CrawlDelaySeconds: s.CrawlDelaySeconds,
		Pacing: dto.HostPacingDto{
			RequestsPerSecond: s.Pacing.RequestsPerSecond,
			IntervalSeconds:   s.Pacing.IntervalSeconds,
			Source:            s.Pacing.Source,
		},
	}
	if s.LastBlockReason != "" {
		out.LastBlockReason = &s.LastBlockReason
	}
	out.CoolingOffUntil = s.CoolingOffUntil
	return out, nil
}

func (a hostsAdapter) ClearRungPreference(ctx context.Context, host string) error {
	return a.retrieval.ClearRungPreference(ctx, host)
}

func (a hostsAdapter) ClearCookies(ctx context.Context, host string) error {
	return a.retrieval.ClearCookies(ctx, host)
}

func (a hostsAdapter) OverrideCoolingOff(ctx context.Context, host string) (time.Duration, error) {
	return a.retrieval.OverrideCoolingOff(ctx, host)
}
