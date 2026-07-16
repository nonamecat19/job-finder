package adapters

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
)

func TestRobotaKey(t *testing.T) {
	a := RobotaAdapter{}
	if a.Key() != "robota" {
		t.Errorf("expected key 'robota', got %q", a.Key())
	}
}

func TestRobotaKind(t *testing.T) {
	a := RobotaAdapter{}
	if a.Kind() != dto.SourceKindAPI {
		t.Errorf("expected kind %q, got %q", dto.SourceKindAPI, a.Kind())
	}
}

func TestRobotaSearchRequiresKeywords(t *testing.T) {
	a := RobotaAdapter{}
	_, err := a.Search(context.Background(), dto.SearchQuery{}, nil)
	if err == nil {
		t.Error("expected error for empty keywords")
	}
}

func TestRobotaHealthCheck(t *testing.T) {
	a := RobotaAdapter{}
	healthy, err := a.HealthCheck(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !healthy {
		t.Error("expected healthy to be true")
	}
}
