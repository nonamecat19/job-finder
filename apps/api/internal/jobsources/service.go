package jobsources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/job-finder/api/internal/crypto"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

var secretKeyRe = regexp.MustCompile(`(?i)cookie|key|secret|token|password`)

// Service manages per-source runtime state (enabled/config/healthy),
// encrypting/decrypting/masking config, and running health checks. Source
// identity (key/kind) lives only in the adapter registry; a JobSource row is
// created lazily, on first real use of a key, not seeded upfront.
type Service struct {
	q        *sqlcgen.Queries
	registry *Registry
	encKey   string // CONFIG_ENCRYPTION_KEY hex; "" disables encryption (dev fallback)
}

func NewService(q *sqlcgen.Queries, registry *Registry, encKey string) *Service {
	return &Service{q: q, registry: registry, encKey: encKey}
}

func (s *Service) encryptConfig(config map[string]any) ([]byte, error) {
	if !crypto.HasEncryptionKey(s.encKey) {
		return json.Marshal(config) // dev fallback: plaintext
	}
	enc, err := crypto.EncryptJSON(s.encKey, config)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"enc": enc})
}

// DecryptConfig reverses encryptConfig; on decrypt failure it logs nothing
// itself (caller decides) and returns an empty map, matching the TS
// catch-and-return-{} behavior.
func (s *Service) DecryptConfig(stored []byte) map[string]any {
	var wrapper struct {
		Enc string `json:"enc"`
	}
	if err := json.Unmarshal(stored, &wrapper); err == nil && wrapper.Enc != "" {
		var out map[string]any
		if err := crypto.DecryptJSON(s.encKey, wrapper.Enc, &out); err != nil {
			return map[string]any{}
		}
		return out
	}
	var plain map[string]any
	if err := json.Unmarshal(stored, &plain); err != nil {
		return map[string]any{}
	}
	return plain
}

func maskConfig(config map[string]any) map[string]any {
	masked := make(map[string]any, len(config))
	for k, v := range config {
		if s, ok := v.(string); ok && s != "" && secretKeyRe.MatchString(k) {
			masked[k] = "••••••"
		} else {
			masked[k] = v
		}
	}
	return masked
}

// List enumerates every source from the registry (code = identity, order),
// overlaid with its persisted runtime state if a row exists; sources never
// touched (no row) get default enabled=true, healthy=true, config={}.
func (s *Service) List(ctx context.Context) ([]dto.JobSourceDto, error) {
	rows, err := s.q.ListJobSources(ctx)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]sqlcgen.JobSource, len(rows))
	for _, r := range rows {
		byKey[r.Key] = r
	}

	adapters := s.registry.All()
	out := make([]dto.JobSourceDto, 0, len(adapters))
	for _, a := range adapters {
		if r, ok := byKey[a.Key()]; ok {
			out = append(out, dto.JobSourceDto{
				ID:      dbutil.UUIDString(r.ID),
				Key:     r.Key,
				Kind:    dto.SourceKind(r.Kind),
				Enabled: r.Enabled,
				Healthy: r.Healthy,
				Config:  maskConfig(s.DecryptConfig(r.Config)),
			})
			continue
		}
		out = append(out, dto.JobSourceDto{
			Key:     a.Key(),
			Kind:    a.Kind(),
			Enabled: true,
			Healthy: true,
			Config:  map[string]any{},
		})
	}
	return out, nil
}

// GetByKey returns the JobSource row for key, creating it with default
// runtime state on first use. The key must be a registered adapter — source
// identity is code-defined, so an unknown key is always a "not found" error
// regardless of what's in the db.
func (s *Service) GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error) {
	adapter, err := s.registry.Get(key)
	if err != nil {
		return sqlcgen.JobSource{}, fmt.Errorf("source '%s' not found", key)
	}

	row, err := s.q.GetJobSourceByKey(ctx, key)
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return sqlcgen.JobSource{}, err
	}

	cfg, err := s.encryptConfig(map[string]any{})
	if err != nil {
		return sqlcgen.JobSource{}, err
	}
	if err := s.q.UpsertJobSource(ctx, sqlcgen.UpsertJobSourceParams{
		Key:    key,
		Kind:   string(adapter.Kind()),
		Config: cfg,
	}); err != nil {
		return sqlcgen.JobSource{}, fmt.Errorf("jobsources: create '%s': %w", key, err)
	}
	row, err = s.q.GetJobSourceByKey(ctx, key)
	if err != nil {
		return sqlcgen.JobSource{}, fmt.Errorf("source '%s' not found", key)
	}
	return row, nil
}

// Config returns the decrypted (unmasked) runtime config for a source,
// lazily creating its row on first use. Used by components that need real
// secret values — e.g. the djinni session manager reading the stored cookie —
// unlike List/Update which mask secrets for the API.
func (s *Service) Config(ctx context.Context, key string) (map[string]any, error) {
	row, err := s.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	return s.DecryptConfig(row.Config), nil
}

// Update applies partial changes: enabled flag and/or config merge (masked
// "••••••" values mean "keep existing", null/"" deletes the key).
func (s *Service) Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*dto.JobSourceDto, error) {
	source, err := s.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}

	if enabled != nil {
		if err := s.q.SetJobSourceEnabled(ctx, sqlcgen.SetJobSourceEnabledParams{Key: key, Enabled: *enabled}); err != nil {
			return nil, err
		}
	}

	if configPatch != nil {
		config := s.DecryptConfig(source.Config)
		for k, v := range configPatch {
			if s, ok := v.(string); ok && s == "••••••" {
				continue
			}
			if v == nil {
				delete(config, k)
				continue
			}
			if s, ok := v.(string); ok && s == "" {
				delete(config, k)
				continue
			}
			config[k] = v
		}
		encoded, err := s.encryptConfig(config)
		if err != nil {
			return nil, err
		}
		if err := s.q.SetJobSourceConfig(ctx, sqlcgen.SetJobSourceConfigParams{Key: key, Config: encoded}); err != nil {
			return nil, err
		}
	}

	list, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Key == key {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("source '%s' not found", key)
}

// Test runs the adapter's health check (or falls back to a tiny search) and
// persists the resulting healthy flag.
func (s *Service) Test(ctx context.Context, key string) (ok bool, errMsg string) {
	source, err := s.GetByKey(ctx, key)
	if err != nil {
		return false, err.Error()
	}
	adapter, err := s.registry.Get(key)
	if err != nil {
		return false, err.Error()
	}
	config := s.DecryptConfig(source.Config)

	healthy, hcErr := adapter.HealthCheck(ctx, config)
	if hcErr != nil {
		_ = s.q.SetJobSourceHealthy(ctx, sqlcgen.SetJobSourceHealthyParams{Key: key, Healthy: false})
		return false, hcErr.Error()
	}
	_ = s.q.SetJobSourceHealthy(ctx, sqlcgen.SetJobSourceHealthyParams{Key: key, Healthy: healthy})
	return healthy, ""
}
