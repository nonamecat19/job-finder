package jobsources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/job-finder/api-go/internal/crypto"
	"github.com/job-finder/api-go/internal/db/sqlcgen"
	"github.com/job-finder/api-go/internal/dbutil"
	"github.com/job-finder/api-go/internal/dto"
)

var secretKeyRe = regexp.MustCompile(`(?i)cookie|key|secret|token|password`)

// Service manages JobSource rows: seeding one per registered adapter,
// encrypting/decrypting/masking their config, and running health checks.
// Mirrors job-sources.service.ts.
type Service struct {
	q        *sqlcgen.Queries
	registry *Registry
	encKey   string // CONFIG_ENCRYPTION_KEY hex; "" disables encryption (dev fallback)
}

func NewService(q *sqlcgen.Queries, registry *Registry, encKey string) *Service {
	return &Service{q: q, registry: registry, encKey: encKey}
}

// Seed inserts a JobSource row per registered adapter (idempotent), same as
// JobSourcesService.onModuleInit.
func (s *Service) Seed(ctx context.Context) error {
	for _, a := range s.registry.All() {
		cfg, err := s.encryptConfig(map[string]any{})
		if err != nil {
			return err
		}
		if err := s.q.UpsertJobSource(ctx, sqlcgen.UpsertJobSourceParams{
			Key:    a.Key(),
			Kind:   string(a.Kind()),
			Config: cfg,
		}); err != nil {
			return fmt.Errorf("jobsources: seed %s: %w", a.Key(), err)
		}
	}
	return nil
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

func (s *Service) List(ctx context.Context) ([]dto.JobSourceDto, error) {
	rows, err := s.q.ListJobSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.JobSourceDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.JobSourceDto{
			ID:      dbutil.UUIDString(r.ID),
			Key:     r.Key,
			Kind:    dto.SourceKind(r.Kind),
			Enabled: r.Enabled,
			Healthy: r.Healthy,
			Config:  maskConfig(s.DecryptConfig(r.Config)),
		})
	}
	return out, nil
}

func (s *Service) GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error) {
	row, err := s.q.GetJobSourceByKey(ctx, key)
	if err != nil {
		return sqlcgen.JobSource{}, fmt.Errorf("source '%s' not found", key)
	}
	return row, nil
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
