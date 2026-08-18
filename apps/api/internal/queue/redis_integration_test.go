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

func withDB(t *testing.T, base string, db int) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse %s: %v", base, err)
	}
	u.Path = fmt.Sprintf("/%d", db)
	return u.String()
}

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

func TestNewRedisClientDefaultsToLocalPort(t *testing.T) {
	base := redisURL(t)
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse %s: %v", base, err)
	}

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
