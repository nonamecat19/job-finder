package profile_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/domain"
	"github.com/job-finder/api/internal/profile"
)

type resumeFakeRepo struct {
	profile.Repository
	row domain.Profile
}

func (f *resumeFakeRepo) GetProfile(ctx context.Context, id pgtype.UUID) (domain.Profile, error) {
	return f.row, nil
}

func (f *resumeFakeRepo) UpdateProfile(ctx context.Context, params sqlcgen.UpdateProfileParams) error {
	if v := params.RendercvYaml; v != nil {
		f.row.RendercvYaml = v
	}
	if v := params.RendercvConfig; v != nil {
		f.row.RendercvConfig = v
	}
	return nil
}

func TestGetResume_SocialNetworksSurfaced(t *testing.T) {
	master := map[string]any{
		"cv": map[string]any{
			"name": "Test User",
			"social_networks": []map[string]any{
				{"network": "LinkedIn", "username": "testuser"},
				{"network": "GitHub", "username": "testuser"},
				{"network": "Telegram", "username": "testuser"},
			},
		},
	}
	configJSON, err := json.Marshal(master)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	repo := &resumeFakeRepo{row: domain.Profile{
		ID: "00000000-0000-0000-0000-000000000001", Name: "Test User", RendercvConfig: configJSON,
	}}
	svc := profile.NewService(repo, nil, "", "")

	resume, err := svc.GetResume(context.Background(), "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("GetResume: %v", err)
	}
	if len(resume.SocialNetworks) != 3 {
		t.Fatalf("social networks = %+v, want 3 entries", resume.SocialNetworks)
	}
}
