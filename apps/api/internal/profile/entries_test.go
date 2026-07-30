package profile_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/profile"
)

type entriesFakeRepo struct {
	profile.Repository
	row    sqlcgen.Profile
	getErr error
}

func (f *entriesFakeRepo) GetDefaultProfile(ctx context.Context) (sqlcgen.Profile, error) {
	if f.getErr != nil {
		return sqlcgen.Profile{}, f.getErr
	}
	return f.row, nil
}

func rendercvConfigWithExperience(exp []map[string]any) []byte {
	master := map[string]any{
		"cv": map[string]any{
			"name": "Test User",
			"sections": map[string]any{
				"experience": exp,
			},
		},
	}
	b, _ := json.Marshal(master)
	return b
}

func TestProfileEntries_NoProfile(t *testing.T) {
	svc := profile.NewService(&entriesFakeRepo{getErr: context.DeadlineExceeded}, nil, "", "")
	out, err := svc.ProfileEntries(context.Background())
	if err != nil {
		t.Fatalf("ProfileEntries: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil entries, got %+v", out)
	}
}

func TestProfileEntries_BuildsSourceLabel(t *testing.T) {
	config := rendercvConfigWithExperience([]map[string]any{
		{
			"company":    "CloudScale",
			"position":   "Senior Backend Engineer",
			"start_date": "2022-03-01",
			"end_date":   "2024-01-01",
			"highlights": []any{"Reduced API latency by 40%", "Mentored 3 junior developers"},
		},
		{
			"company":    "StartupLab",
			"position":   "Junior Developer",
			"startDate":  "2017-01-15",
			"highlights": []any{"Built MVPs for 4 early-stage startups"},
		},
	})

	svc := profile.NewService(&entriesFakeRepo{row: sqlcgen.Profile{RendercvConfig: config}}, nil, "", "")
	out, err := svc.ProfileEntries(context.Background())
	if err != nil {
		t.Fatalf("ProfileEntries: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(out), out)
	}

	if out[0].SourceLabel != "Senior Backend Engineer, CloudScale (2022-03-01–2024-01-01)" {
		t.Errorf("unexpected label: %q", out[0].SourceLabel)
	}
	if out[0].Bullet != "Reduced API latency by 40%" {
		t.Errorf("unexpected bullet: %q", out[0].Bullet)
	}
	if out[1].SourceLabel != out[0].SourceLabel {
		t.Errorf("expected both highlights from the same entry to share a label")
	}

	if out[2].SourceLabel != "Junior Developer, StartupLab (2017-01-15–Present)" {
		t.Errorf("unexpected label: %q", out[2].SourceLabel)
	}
}

func TestProfileEntries_MissingConfig(t *testing.T) {
	svc := profile.NewService(&entriesFakeRepo{row: sqlcgen.Profile{}}, nil, "", "")
	out, err := svc.ProfileEntries(context.Background())
	if err != nil {
		t.Fatalf("ProfileEntries: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil entries for empty config, got %+v", out)
	}
}
