//go:build integration

package queue

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/job-finder/api/internal/testinfra"
)

// NewRedisClient's URL handling is unit-tested against its own parsing, which
// cannot tell whether the options it produces actually connect: a wrong
// address form, a database index the server rejects, or a password field the
// client never sends all parse fine and fail only against a server. These
// tests point it at the Redis image docker-compose.yml runs.
//
// Redis is caching and rate-limit state now, not a queue backend (047), and
// cmd/server's readiness check pings it — so "the client this constructor
// returns can talk to a real Redis" is the whole contract.

func redisURL(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	url, err := testinfra.RedisURL(ctx)
	if err != nil {
		t.Fatalf("start redis: %v", err)
	}
	return url
}

// withDB rewrites a redis:// URL's path to select a database index, the form
// REDIS_URL takes in docker-compose.yml and CI (redis://host:6379/1).
func withDB(t *testing.T, base string, db int) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse %s: %v", base, err)
	}
	u.Path = fmt.Sprintf("/%d", db)
	return u.String()
}

// TestNewRedisClientConnects is what the readiness endpoint depends on:
// the client the constructor returns answers PING.
func TestNewRedisClientConnects(t *testing.T) {
	client, err := NewRedisClient(redisURL(t))
	if err != nil {
		t.Fatalf("NewRedisClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("PING: %v", err)
	}
}

// TestRedisURLDatabaseIndexIsHonoured proves the `/1` in REDIS_URL reaches
// the server as a SELECT, not just as a parsed integer. The deployment
// relies on this: tests and the dev stack share one Redis and separate
// themselves by database index alone, so a client that silently stayed on
// database 0 would have them writing over each other.
func TestRedisURLDatabaseIndexIsHonoured(t *testing.T) {
	base := redisURL(t)

	one, err := NewRedisClient(withDB(t, base, 1))
	if err != nil {
		t.Fatalf("NewRedisClient(db 1): %v", err)
	}
	defer one.Close()
	zero, err := NewRedisClient(withDB(t, base, 0))
	if err != nil {
		t.Fatalf("NewRedisClient(db 0): %v", err)
	}
	defer zero.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const key = "testinfra:db-index"
	if err := one.Set(ctx, key, "written-to-db-1", time.Minute).Err(); err != nil {
		t.Fatalf("SET on db 1: %v", err)
	}
	t.Cleanup(func() { _ = one.Del(context.Background(), key).Err() })

	got, err := one.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("GET on db 1: %v", err)
	}
	if got != "written-to-db-1" {
		t.Fatalf("db 1 returned %q", got)
	}

	if _, err := zero.Get(ctx, key).Result(); err == nil {
		t.Fatal("a key written to database 1 is readable from database 0: the index in REDIS_URL is not being applied")
	}
}

// TestNewRedisClientDefaultsToLocalPort proves the empty-URL default still
// produces a usable client shape — the constructor's fallback is
// redis://localhost:6379, so pointing it at the container's port through the
// same code path is the closest a test can get to exercising that branch.
func TestNewRedisClientDefaultsToLocalPort(t *testing.T) {
	base := redisURL(t)
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse %s: %v", base, err)
	}

	// No path, no credentials: the minimal form the fallback produces.
	client, err := NewRedisClient("redis://" + u.Host)
	if err != nil {
		t.Fatalf("NewRedisClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("PING: %v", err)
	}
}

// TestRedisURLPasswordIsSent proves the password in a redis://user:pass@host
// URL reaches the server, by turning authentication on for the duration of
// the test: with requirepass set, a client built from a URL carrying the
// right password must work and one carrying the wrong password must not. A
// constructor that dropped the credential would fail both.
func TestRedisURLPasswordIsSent(t *testing.T) {
	base := redisURL(t)
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse %s: %v", base, err)
	}

	admin, err := NewRedisClient(base)
	if err != nil {
		t.Fatalf("NewRedisClient: %v", err)
	}
	defer admin.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const password = "testinfra-secret"
	if err := admin.ConfigSet(ctx, "requirepass", password).Err(); err != nil {
		t.Fatalf("enable requirepass: %v", err)
	}
	t.Cleanup(func() {
		// The admin client authenticated before requirepass existed, so it
		// keeps its connection; a fresh authenticated client is what can
		// turn it back off for the rest of the package.
		restore, err := NewRedisClient("redis://default:" + password + "@" + u.Host)
		if err != nil {
			t.Fatalf("build client to restore requirepass: %v", err)
		}
		defer restore.Close()
		if err := restore.ConfigSet(context.Background(), "requirepass", "").Err(); err != nil {
			t.Fatalf("restore requirepass: %v", err)
		}
	})

	authenticated, err := NewRedisClient("redis://default:" + password + "@" + u.Host)
	if err != nil {
		t.Fatalf("NewRedisClient(with password): %v", err)
	}
	defer authenticated.Close()
	if err := authenticated.Ping(ctx).Err(); err != nil {
		t.Fatalf("PING with the right password: %v — the credential in REDIS_URL is not reaching the server", err)
	}

	wrong, err := NewRedisClient("redis://default:not-the-password@" + u.Host)
	if err != nil {
		t.Fatalf("NewRedisClient(wrong password): %v", err)
	}
	defer wrong.Close()
	if err := wrong.Ping(ctx).Err(); err == nil {
		t.Fatal("PING succeeded with the wrong password")
	}
}
