package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDBPoolDefaults(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("shipped defaults must validate cleanly, got: %v", err)
	}

	if cfg.DBMaxConns != 0 {
		t.Errorf("DBMaxConns default = %d, want 0 (derive)", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 2 {
		t.Errorf("DBMinConns default = %d, want 2", cfg.DBMinConns)
	}
	if cfg.DBMaxConnLifetime != time.Hour {
		t.Errorf("DBMaxConnLifetime default = %s, want 1h", cfg.DBMaxConnLifetime)
	}
	if cfg.DBMaxConnIdleTime != 30*time.Minute {
		t.Errorf("DBMaxConnIdleTime default = %s, want 30m", cfg.DBMaxConnIdleTime)
	}
	if cfg.DBAcquireTimeout != 5*time.Second {
		t.Errorf("DBAcquireTimeout default = %s, want 5s", cfg.DBAcquireTimeout)
	}
	if cfg.DBServerMaxConns != 100 {
		t.Errorf("DBServerMaxConns default = %d, want 100", cfg.DBServerMaxConns)
	}
	if cfg.DBInteractiveReserve != 8 {
		t.Errorf("DBInteractiveReserve default = %d, want 8", cfg.DBInteractiveReserve)
	}
}

func TestLoadRejectsInvalidDBPoolValues(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		value   string
		wantMsg string
	}{
		{"negative max", "DB_MAX_CONNS", "-1", "config: DB_MAX_CONNS must be >= 0 (0 derives from worker concurrency)"},
		{"negative min", "DB_MIN_CONNS", "-1", "config: DB_MIN_CONNS must be >= 0"},
		{"zero lifetime", "DB_MAX_CONN_LIFETIME", "0s", "config: DB_MAX_CONN_LIFETIME must be > 0"},
		{"zero idle time", "DB_MAX_CONN_IDLE_TIME", "0s", "config: DB_MAX_CONN_IDLE_TIME must be > 0"},
		{"zero acquire timeout", "DB_ACQUIRE_TIMEOUT", "0s", "config: DB_ACQUIRE_TIMEOUT must be > 0"},
		{"zero reserve", "DB_INTERACTIVE_RESERVE", "0", "config: DB_INTERACTIVE_RESERVE must be >= 1"},
		{"zero server max", "DB_SERVER_MAX_CONNS", "0", "config: DB_SERVER_MAX_CONNS must be >= 1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := unsetForTest(t); err != nil {
				t.Fatal(err)
			}
			t.Setenv(tc.key, tc.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() succeeded with %s=%s, want failure", tc.key, tc.value)
			}
			if err.Error() != tc.wantMsg {
				t.Errorf("Load() error = %q, want %q", err, tc.wantMsg)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("error %q does not name the offending key %s", err, tc.key)
			}
		})
	}
}

func poolConfig() *Config {
	return &Config{
		DBMinConns:           2,
		DBMaxConnLifetime:    time.Hour,
		DBMaxConnIdleTime:    30 * time.Minute,
		DBAcquireTimeout:     5 * time.Second,
		DBServerMaxConns:     100,
		DBInteractiveReserve: 8,
	}
}

func TestValidateDBCapacityRejectsUnderCapacity(t *testing.T) {
	cfg := poolConfig()
	cfg.DBMaxConns = 6

	err := ValidateDBCapacity(cfg, 15, 2, 25)
	if err == nil {
		t.Fatal("ValidateDBCapacity() succeeded with DB_MAX_CONNS=6 against required 25, want failure")
	}
	want := "config: DB_MAX_CONNS=6 is below the 25 connections required by worker concurrency " +
		"(workers=15 background=2 reserve=8). Raise DB_MAX_CONNS, or lower " +
		"AI_CONCURRENCY_CLOUD / INGEST_CONCURRENCY / ENRICH_CONCURRENCY"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestValidateDBCapacityRejectsMinAboveEffectiveMax(t *testing.T) {
	// DB_MAX_CONNS unset, so the comparison is against the derived value.
	cfg := poolConfig()
	cfg.DBMinConns = 99

	err := ValidateDBCapacity(cfg, 15, 2, 25)
	if err == nil {
		t.Fatal("ValidateDBCapacity() succeeded with DB_MIN_CONNS=99 against effective max 25")
	}
	if want := "config: DB_MIN_CONNS (99) exceeds DB_MAX_CONNS (25)"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestValidateDBCapacityAcceptsExactAndOverDeclaredCapacity(t *testing.T) {
	cases := []struct {
		name     string
		maxConns int
	}{
		{"derived", 0},
		{"exactly required", 25},
		{"above required", 40},
		// Exceeds DB_SERVER_MAX_CONNS: a warning, not a failure, because the
		// server limit is operator-declared and may be stale (research.md R4).
		{"above declared server max", 250},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := poolConfig()
			cfg.DBMaxConns = tc.maxConns
			if err := ValidateDBCapacity(cfg, 15, 2, 25); err != nil {
				t.Errorf("ValidateDBCapacity() = %v, want nil", err)
			}
		})
	}
}

func TestEffectiveDBMaxConns(t *testing.T) {
	cfg := poolConfig()
	if got := cfg.EffectiveDBMaxConns(25); got != 25 {
		t.Errorf("EffectiveDBMaxConns(25) with DB_MAX_CONNS unset = %d, want 25", got)
	}
	cfg.DBMaxConns = 40
	if got := cfg.EffectiveDBMaxConns(25); got != 40 {
		t.Errorf("EffectiveDBMaxConns(25) with DB_MAX_CONNS=40 = %d, want 40", got)
	}
}
