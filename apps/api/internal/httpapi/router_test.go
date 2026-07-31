package httpapi_test

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	activityhttp "github.com/job-finder/api/internal/activity/interfaces/http"
	aifeaturehttp "github.com/job-finder/api/internal/aifeature/interfaces/http"
	applicationshttp "github.com/job-finder/api/internal/applications/interfaces/http"
	coachhttp "github.com/job-finder/api/internal/coach/interfaces/http"
	companyintelhttp "github.com/job-finder/api/internal/companyintel/interfaces/http"
	generationhttp "github.com/job-finder/api/internal/generation/interfaces/http"
	ghostjobhttp "github.com/job-finder/api/internal/ghostjob/interfaces/http"
	"github.com/job-finder/api/internal/health"
	"github.com/job-finder/api/internal/httpapi"
	interviewprephttp "github.com/job-finder/api/internal/interviewprep/interfaces/http"
	jobshttp "github.com/job-finder/api/internal/jobs/interfaces/http"
	jobsourceshttp "github.com/job-finder/api/internal/jobsources/interfaces/http"
	keywordhttp "github.com/job-finder/api/internal/keyword/interfaces/http"
	llmsettingshttp "github.com/job-finder/api/internal/llmsettings/interfaces/http"
	notifierhttp "github.com/job-finder/api/internal/notifier/interfaces/http"
	outreachhttp "github.com/job-finder/api/internal/outreach/interfaces/http"
	postagehttp "github.com/job-finder/api/internal/postage/interfaces/http"
	profilehttp "github.com/job-finder/api/internal/profile/interfaces/http"
	recruiterhttp "github.com/job-finder/api/internal/recruiter/interfaces/http"
	referralhttp "github.com/job-finder/api/internal/referral/interfaces/http"
	subscriptionshttp "github.com/job-finder/api/internal/subscriptions/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

func TestHealthEndpoint(t *testing.T) {
	r := httpapi.NewRouter(0)

	w := testutil.DoRequest(r, "GET", "/api/health", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// allMounts returns every handler's Mount, zero-valued. Mount only registers
// routes — it never touches the handler's dependencies — so zero values are
// sufficient to enumerate the surface without standing up the platform.
//
// This list is the registration side of FR-006: a handler dropped during a
// move shows up here as missing routes rather than as a 404 in production.
func allMounts() []func(chi.Router) {
	return []func(chi.Router){
		(&jobsourceshttp.SourcesHandler{}).Mount,
		(&jobsourceshttp.RosterHandler{}).Mount,
		(&jobsourceshttp.SearchesHandler{}).Mount,
		(&generationhttp.DocumentsHandler{}).Mount,
		(&profilehttp.ProfilesHandler{}).Mount,
		(&jobshttp.JobsHandler{}).Mount,
		(&applicationshttp.ApplicationsHandler{}).Mount,
		(&subscriptionshttp.SubscriptionsHandler{}).Mount,
		(&activityhttp.ActivityHandler{}).Mount,
		(&keywordhttp.KeywordHandler{}).Mount,
		(&postagehttp.PostAgeHandler{}).Mount,
		(&notifierhttp.NotificationHandler{}).Mount,
		(&companyintelhttp.CompaniesHandler{}).Mount,
		(&ghostjobhttp.GhostJobHandler{}).Mount,
		(&coachhttp.CoachHandler{}).Mount,
		(&recruiterhttp.ContactsHandler{}).Mount,
		(&referralhttp.ReferralHandler{}).Mount,
		(&outreachhttp.OutreachHandler{}).Mount,
		(&llmsettingshttp.LlmSettingsHandler{}).Mount,
		(&aifeaturehttp.AiFeatureHandler{}).Mount,
		(&interviewprephttp.InterviewPrepHandler{}).Mount,
		(&health.HealthHandler{}).Mount,
		(&jobsourceshttp.HostsHandler{}).Mount,
	}
}

// TestRouteInventory prints every method+path the fully-mounted router
// exposes, under both the /api and /api/v1 prefixes. It is a standing guard,
// not scaffolding: the printed inventory is diffed across refactors to prove
// the public surface did not move (FR-006, SC-003).
func TestRouteInventory(t *testing.T) {
	r := httpapi.NewRouter(0, allMounts()...)

	var routes []string
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, fmt.Sprintf("ROUTE %s %s", method, strings.TrimSuffix(route, "/*")))
		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}

	sort.Strings(routes)
	for _, route := range routes {
		t.Log(route)
	}

	if len(routes) == 0 {
		t.Fatal("router exposed no routes")
	}

	// Both prefixes come from a single registration (FR-007), so every route
	// must appear under each.
	var v0, v1 int
	for _, route := range routes {
		switch {
		case strings.Contains(route, " /api/v1/"):
			v1++
		case strings.Contains(route, " /api/"):
			v0++
		}
	}
	if v0 != v1 {
		t.Fatalf("prefix mismatch: %d routes under /api, %d under /api/v1", v0, v1)
	}
}
