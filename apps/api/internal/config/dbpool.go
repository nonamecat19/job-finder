package config

import (
	"errors"
	"fmt"
	"log/slog"
)

// validateDBPool checks the pool settings that stand on their own, before any
// worker concurrency is known. Called from Load, so a bad value is rejected
// before a connection is ever opened (026-db-pool-capacity).
func validateDBPool(c *Config) error {
	switch {
	case c.DBMaxConns < 0:
		return errors.New("config: DB_MAX_CONNS must be >= 0 (0 derives from worker concurrency)")
	case c.DBMinConns < 0:
		return errors.New("config: DB_MIN_CONNS must be >= 0")
	case c.DBMaxConnLifetime <= 0:
		return errors.New("config: DB_MAX_CONN_LIFETIME must be > 0")
	case c.DBMaxConnIdleTime <= 0:
		return errors.New("config: DB_MAX_CONN_IDLE_TIME must be > 0")
	case c.DBAcquireTimeout <= 0:
		return errors.New("config: DB_ACQUIRE_TIMEOUT must be > 0")
	case c.DBInteractiveReserve < 1:
		return errors.New("config: DB_INTERACTIVE_RESERVE must be >= 1")
	case c.DBServerMaxConns < 1:
		return errors.New("config: DB_SERVER_MAX_CONNS must be >= 1")
	}
	if c.DBMaxConnIdleTime > c.DBMaxConnLifetime {
		slog.Warn(fmt.Sprintf("config: DB_MAX_CONN_IDLE_TIME (%s) exceeds DB_MAX_CONN_LIFETIME (%s); idle retirement will never trigger",
			c.DBMaxConnIdleTime, c.DBMaxConnLifetime))
	}
	return nil
}

// EffectiveDBMaxConns is the pool size that will be applied: the configured
// value, or the derived requirement when DB_MAX_CONNS is 0.
func (c *Config) EffectiveDBMaxConns(required int) int {
	if c.DBMaxConns > 0 {
		return c.DBMaxConns
	}
	return required
}

// ValidateDBCapacity checks configured capacity against what the workload
// needs. required comes from db.CapacityBudget.Required(); workers and
// background are its components, reported so the message names the settings
// actually in conflict.
//
// It takes the budget as arguments rather than computing it: the arithmetic
// lives in internal/db, which imports internal/queue, which imports this
// package. Passing the numbers in keeps that direction one-way.
//
// Under-capacity fails — the process can prove it on its own. Over-capacity
// only warns, because it is measured against an operator-declared server limit
// that may simply be stale (research.md R4).
func ValidateDBCapacity(c *Config, workers, background, required int) error {
	if c.DBMaxConns > 0 && c.DBMaxConns < required {
		return fmt.Errorf("config: DB_MAX_CONNS=%d is below the %d connections required by worker concurrency (workers=%d background=%d reserve=%d). Raise DB_MAX_CONNS, or lower AI_CONCURRENCY_CLOUD / INGEST_CONCURRENCY / ENRICH_CONCURRENCY",
			c.DBMaxConns, required, workers, background, c.DBInteractiveReserve)
	}
	effective := c.EffectiveDBMaxConns(required)
	if c.DBMinConns > effective {
		return fmt.Errorf("config: DB_MIN_CONNS (%d) exceeds DB_MAX_CONNS (%d)", c.DBMinConns, effective)
	}
	if effective > c.DBServerMaxConns {
		slog.Warn(fmt.Sprintf("config: DB_MAX_CONNS=%d exceeds declared DB_SERVER_MAX_CONNS=%d; connections may be refused under load",
			effective, c.DBServerMaxConns))
	}
	return nil
}
