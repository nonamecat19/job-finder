package domain

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
)

type mockAdapter struct {
	key        string
	kind       dto.SourceKind
	searchFunc func(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)
	healthFunc func(ctx context.Context, config map[string]any) (bool, error)
}

func (m *mockAdapter) Key() string {
	return m.key
}

func (m *mockAdapter) Kind() dto.SourceKind {
	return m.kind
}

func (m *mockAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, query, config)
	}
	return []dto.NormalizedJob{}, nil
}

func (m *mockAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	if m.healthFunc != nil {
		return m.healthFunc(ctx, config)
	}
	return true, nil
}

func TestNewRegistry(t *testing.T) {
	adapter1 := &mockAdapter{key: "adapter-1", kind: dto.SourceKindAPI}
	adapter2 := &mockAdapter{key: "adapter-2", kind: dto.SourceKindScrape}

	registry := NewRegistry(adapter1, adapter2)

	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestRegistryGet(t *testing.T) {
	adapter := &mockAdapter{key: "test-adapter", kind: dto.SourceKindAPI}
	registry := NewRegistry(adapter)

	got, err := registry.Get("test-adapter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got == nil {
		t.Fatal("expected non-nil adapter")
	}

	if got.Key() != "test-adapter" {
		t.Errorf("expected key 'test-adapter', got %q", got.Key())
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.Get("non-existent")
	if err == nil {
		t.Error("expected error for non-existent adapter")
	}
}

func TestRegistryAll(t *testing.T) {
	adapter1 := &mockAdapter{key: "adapter-1", kind: dto.SourceKindAPI}
	adapter2 := &mockAdapter{key: "adapter-2", kind: dto.SourceKindScrape}
	adapter3 := &mockAdapter{key: "adapter-3", kind: dto.SourceKindSidecar}

	registry := NewRegistry(adapter1, adapter2, adapter3)

	all := registry.All()
	if len(all) != 3 {
		t.Errorf("expected 3 adapters, got %d", len(all))
	}
}

func TestRegistryKeys(t *testing.T) {
	adapter1 := &mockAdapter{key: "adapter-1", kind: dto.SourceKindAPI}
	adapter2 := &mockAdapter{key: "adapter-2", kind: dto.SourceKindScrape}

	registry := NewRegistry(adapter1, adapter2)

	keys := registry.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}

	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}

	if !keyMap["adapter-1"] {
		t.Error("expected 'adapter-1' in keys")
	}
	if !keyMap["adapter-2"] {
		t.Error("expected 'adapter-2' in keys")
	}
}

func TestRegistryRegisterDuplicate(t *testing.T) {
	adapter1 := &mockAdapter{key: "duplicate", kind: dto.SourceKindAPI}
	adapter2 := &mockAdapter{key: "duplicate", kind: dto.SourceKindScrape}

	registry := NewRegistry(adapter1, adapter2)

	all := registry.All()
	if len(all) != 2 {
		t.Errorf("expected 2 adapters (Registry does not deduplicate), got %d", len(all))
	}
}

func TestRegistryEmpty(t *testing.T) {
	registry := NewRegistry()

	all := registry.All()
	if len(all) != 0 {
		t.Errorf("expected 0 adapters, got %d", len(all))
	}

	keys := registry.Keys()
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestAdapterInterface(t *testing.T) {
	var _ Adapter = (*mockAdapter)(nil)
}

func TestRegistryConcurrentAccess(t *testing.T) {
	registry := NewRegistry()

	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(i int) {
			adapter := &mockAdapter{key: "adapter-" + string(rune('A'+i)), kind: dto.SourceKindAPI}
			registry = NewRegistry(append(registry.All(), adapter)...)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestRegistryGetAfterRegister(t *testing.T) {
	adapter := &mockAdapter{
		key:  "test",
		kind: dto.SourceKindAPI,
		searchFunc: func(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
			return []dto.NormalizedJob{
				{Title: "Test Job", SourceKey: "test"},
			}, nil
		},
	}

	registry := NewRegistry(adapter)

	got, err := registry.Get("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jobs, err := got.Search(context.Background(), dto.SearchQuery{Keywords: "test"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(jobs))
	}

	if jobs[0].Title != "Test Job" {
		t.Errorf("expected title 'Test Job', got %q", jobs[0].Title)
	}
}

func TestRegistryHealthCheck(t *testing.T) {
	adapter := &mockAdapter{
		key:  "healthy",
		kind: dto.SourceKindAPI,
		healthFunc: func(ctx context.Context, config map[string]any) (bool, error) {
			return true, nil
		},
	}

	registry := NewRegistry(adapter)

	got, err := registry.Get("healthy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	healthy, err := got.HealthCheck(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !healthy {
		t.Error("expected healthy to be true")
	}
}

func TestRegistryHealthCheckFailure(t *testing.T) {
	adapter := &mockAdapter{
		key:  "unhealthy",
		kind: dto.SourceKindAPI,
		healthFunc: func(ctx context.Context, config map[string]any) (bool, error) {
			return false, &healthError{message: "service unavailable"}
		},
	}

	registry := NewRegistry(adapter)

	got, err := registry.Get("unhealthy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	healthy, err := got.HealthCheck(context.Background(), nil)
	if err == nil {
		t.Error("expected error for unhealthy adapter")
	}

	if healthy {
		t.Error("expected healthy to be false")
	}
}

type healthError struct {
	message string
}

func (e *healthError) Error() string {
	return e.message
}

func TestRegistryMultipleAdaptersSearch(t *testing.T) {
	adapter1 := &mockAdapter{
		key:  "adapter-1",
		kind: dto.SourceKindAPI,
		searchFunc: func(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
			return []dto.NormalizedJob{
				{Title: "Job from adapter 1", SourceKey: "adapter-1"},
			}, nil
		},
	}

	adapter2 := &mockAdapter{
		key:  "adapter-2",
		kind: dto.SourceKindScrape,
		searchFunc: func(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
			return []dto.NormalizedJob{
				{Title: "Job from adapter 2", SourceKey: "adapter-2"},
			}, nil
		},
	}

	registry := NewRegistry(adapter1, adapter2)

	all := registry.All()
	var allJobs []dto.NormalizedJob

	for _, adapter := range all {
		jobs, err := adapter.Search(context.Background(), dto.SearchQuery{Keywords: "test"}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		allJobs = append(allJobs, jobs...)
	}

	if len(allJobs) != 2 {
		t.Errorf("expected 2 jobs total, got %d", len(allJobs))
	}
}

func TestRegistryWithNilSearchFunc(t *testing.T) {
	adapter := &mockAdapter{
		key:  "no-search",
		kind: dto.SourceKindAPI,
	}

	registry := NewRegistry(adapter)

	got, err := registry.Get("no-search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jobs, err := got.Search(context.Background(), dto.SearchQuery{Keywords: "test"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestRegistryWithNilHealthFunc(t *testing.T) {
	adapter := &mockAdapter{
		key:  "no-health",
		kind: dto.SourceKindAPI,
	}

	registry := NewRegistry(adapter)

	got, err := registry.Get("no-health")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	healthy, err := got.HealthCheck(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !healthy {
		t.Error("expected healthy to be true by default")
	}
}
