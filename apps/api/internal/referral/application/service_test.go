package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/referral/domain"
	"github.com/job-finder/api/internal/referral/infrastructure/github"

	"github.com/jackc/pgx/v5/pgtype"
)

type fakeJobRepo struct {
	jobs map[string]sqlcgen.Job
}

func (f *fakeJobRepo) GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	j, ok := f.jobs[dbutil.UUIDString(id)]
	if !ok {
		return sqlcgen.Job{}, errNotFound
	}
	return j, nil
}

func TestService_ImportCSV(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeJobRepo{}, github.NewGitHubCrossReferencer())

	csvBody := "Name,Company\nJane Doe,Acme\nJohn Smith,Acme\n,Skipped Co\n"
	summary, err := svc.ImportCSV(context.Background(), strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("ImportCSV() error = %v", err)
	}
	if summary.Imported != 2 {
		t.Errorf("Imported = %d, want 2", summary.Imported)
	}
	if summary.Total != 2 {
		t.Errorf("Total = %d, want 2 (blank-name row dropped before counting)", summary.Total)
	}
	if len(repo.contacts) != 2 {
		t.Errorf("repo has %d contacts, want 2", len(repo.contacts))
	}
}

func TestService_ImportCSV_InvalidCSV(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeJobRepo{}, github.NewGitHubCrossReferencer())

	_, err := svc.ImportCSV(context.Background(), strings.NewReader(""))
	if err == nil {
		t.Fatal("ImportCSV() error = nil, want error for empty CSV (no header)")
	}
}

func TestService_SyncGithub_NoUsername(t *testing.T) {
	repo := newFakeRepo()
	c := repo.addContact("No Github", "")
	svc := NewService(repo, &fakeJobRepo{}, github.NewGitHubCrossReferencer())

	_, err := svc.SyncGithub(context.Background(), dbutil.UUIDString(c.ID))
	if !errors.Is(err, domain.ErrNoGithubUsername) {
		t.Fatalf("SyncGithub() error = %v, want domain.ErrNoGithubUsername", err)
	}
}

func TestService_SyncGithub_ContactNotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeJobRepo{}, github.NewGitHubCrossReferencer())

	_, err := svc.SyncGithub(context.Background(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, domain.ErrContactNotFound) {
		t.Fatalf("SyncGithub() error = %v, want domain.ErrContactNotFound", err)
	}
}

func TestService_SyncGithub_CreatesConnectionsForKnownContacts(t *testing.T) {
	repo := newFakeRepo()
	me := repo.addContact("Me", "")
	meUsername := "me-gh"
	me.GithubUsername = &meUsername
	repo.contacts[idKey(me.ID)] = me

	knownLogin := "alice"
	known := repo.addContact("Alice", "Acme")
	known.GithubUsername = &knownLogin
	repo.contacts[idKey(known.ID)] = known

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/me-gh/followers":
			_ = json.NewEncoder(w).Encode([]github.GitHubProfile{{Login: "alice"}, {Login: "stranger"}})
		case "/users/me-gh/following":
			_ = json.NewEncoder(w).Encode([]github.GitHubProfile{{Login: "alice"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	gh := github.NewWithClient(srv.Client(), srv.URL)
	svc := NewService(repo, &fakeJobRepo{}, gh)

	result, err := svc.SyncGithub(context.Background(), dbutil.UUIDString(me.ID))
	if err != nil {
		t.Fatalf("SyncGithub() error = %v", err)
	}
	if result.FollowersScanned != 2 || result.FollowingScanned != 1 {
		t.Errorf("scanned = followers:%d following:%d, want 2/1", result.FollowersScanned, result.FollowingScanned)
	}
	// alice appears in both followers and following (mutual) — one connection,
	// deduped, not one per list.
	if result.ConnectionsMade != 1 {
		t.Errorf("ConnectionsMade = %d, want 1 (deduped mutual match)", result.ConnectionsMade)
	}
	if len(repo.connections) != 1 {
		t.Fatalf("repo has %d connections, want 1", len(repo.connections))
	}
	if repo.connections[0].Strength != 0.8 {
		t.Errorf("connection strength = %v, want 0.8 for a mutual follow", repo.connections[0].Strength)
	}
}

func TestService_FindReferralPaths(t *testing.T) {
	repo := newFakeRepo()
	me := repo.addContact("Me", "")
	target := repo.addContact("Target", "Acme")
	repo.connect(me, target, 0.9)

	jobID := "11111111-1111-1111-1111-111111111111"
	uid, _ := dbutil.ParseUUID(jobID)
	jobs := &fakeJobRepo{jobs: map[string]sqlcgen.Job{
		jobID: {ID: uid, Company: "Acme"},
	}}
	svc := NewService(repo, jobs, github.NewGitHubCrossReferencer())

	paths, err := svc.FindReferralPaths(context.Background(), jobID, 3, 10)
	if err != nil {
		t.Fatalf("FindReferralPaths() error = %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d paths, want 1", len(paths))
	}
}

func TestService_FindReferralPaths_JobNotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeJobRepo{}, github.NewGitHubCrossReferencer())

	_, err := svc.FindReferralPaths(context.Background(), "22222222-2222-2222-2222-222222222222", 3, 10)
	if err == nil {
		t.Fatal("FindReferralPaths() error = nil, want error for unknown job")
	}
}

func TestService_FindReferralPaths_NoCompany(t *testing.T) {
	repo := newFakeRepo()
	jobID := "33333333-3333-3333-3333-333333333333"
	uid, _ := dbutil.ParseUUID(jobID)
	jobs := &fakeJobRepo{jobs: map[string]sqlcgen.Job{jobID: {ID: uid, Company: ""}}}
	svc := NewService(repo, jobs, github.NewGitHubCrossReferencer())

	paths, err := svc.FindReferralPaths(context.Background(), jobID, 3, 10)
	if err != nil {
		t.Fatalf("FindReferralPaths() error = %v", err)
	}
	if paths != nil {
		t.Errorf("got %+v, want nil for a job with no company", paths)
	}
}
