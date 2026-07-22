package referral

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestReferencer(t *testing.T, handler http.HandlerFunc) *GitHubCrossReferencer {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &GitHubCrossReferencer{client: srv.Client(), baseURL: srv.URL}
}

func TestFetchProfile_OK(t *testing.T) {
	g := newTestReferencer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/octocat" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("Accept header = %q", r.Header.Get("Accept"))
		}
		_ = json.NewEncoder(w).Encode(GitHubProfile{Login: "octocat", Name: "The Octocat", Followers: 100})
	})

	profile, err := g.FetchProfile(context.Background(), "octocat")
	if err != nil {
		t.Fatalf("FetchProfile() error = %v", err)
	}
	if profile == nil || profile.Login != "octocat" || profile.Followers != 100 {
		t.Errorf("FetchProfile() = %+v, unexpected", profile)
	}
}

func TestFetchProfile_NotFound(t *testing.T) {
	g := newTestReferencer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	profile, err := g.FetchProfile(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("FetchProfile() error = %v, want nil error on 404", err)
	}
	if profile != nil {
		t.Errorf("FetchProfile() = %+v, want nil on 404", profile)
	}
}

func TestFetchProfile_ServerError(t *testing.T) {
	g := newTestReferencer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := g.FetchProfile(context.Background(), "octocat")
	if err == nil {
		t.Fatal("FetchProfile() error = nil, want error on 500")
	}
}

func TestFetchFollowers_OK(t *testing.T) {
	g := newTestReferencer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/octocat/followers" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]GitHubProfile{{Login: "alice"}, {Login: "bob"}})
	})

	followers, err := g.FetchFollowers(context.Background(), "octocat")
	if err != nil {
		t.Fatalf("FetchFollowers() error = %v", err)
	}
	if len(followers) != 2 {
		t.Fatalf("FetchFollowers() got %d, want 2", len(followers))
	}
}

func TestFetchFollowing_OK(t *testing.T) {
	g := newTestReferencer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/octocat/following" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]GitHubProfile{{Login: "carol"}})
	})

	following, err := g.FetchFollowing(context.Background(), "octocat")
	if err != nil {
		t.Fatalf("FetchFollowing() error = %v", err)
	}
	if len(following) != 1 || following[0].Login != "carol" {
		t.Errorf("FetchFollowing() = %+v, unexpected", following)
	}
}

func TestFetchFollowers_ServerError(t *testing.T) {
	g := newTestReferencer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := g.FetchFollowers(context.Background(), "octocat")
	if err == nil {
		t.Fatal("FetchFollowers() error = nil, want error on 500")
	}
}
