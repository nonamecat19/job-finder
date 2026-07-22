package profile_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/profile"
)

const testMasterYAMLAsJSON = `{
  "cv": {
    "name": "Jane Doe",
    "headline": "Senior Frontend Engineer",
    "location": "San Francisco, CA",
    "email": "jane@example.com",
    "phone": "+1-555-1234",
    "custom_connections": [
      {"placeholder": "Portfolio", "url": "https://jane.dev"}
    ],
    "sections": {
      "skills": [
        {"label": "Languages", "details": "TypeScript, Go"}
      ],
      "experience": [
        {"company": "Acme Corp", "position": "Senior Engineer", "start_date": "2020-03", "end_date": "", "highlights": ["Built things"]}
      ],
      "education": [
        {"institution": "MIT", "degree": "B.S.", "area": "Computer Science", "start_date": "2012-09", "end_date": "2016-06"}
      ]
    }
  }
}`

type extProfileFakeRepo struct {
	profile.Repository
	row sqlcgen.Profile
}

func (f *extProfileFakeRepo) GetDefaultProfile(ctx context.Context) (sqlcgen.Profile, error) {
	return f.row, nil
}

func (f *extProfileFakeRepo) GetProfile(ctx context.Context, id pgtype.UUID) (sqlcgen.Profile, error) {
	return f.row, nil
}

func testProfileRow(t *testing.T) sqlcgen.Profile {
	t.Helper()
	var master map[string]any
	if err := json.Unmarshal([]byte(testMasterYAMLAsJSON), &master); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	raw, err := json.Marshal(master)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return sqlcgen.Profile{
		ID:             pgtype.UUID{Bytes: uuid.New(), Valid: true},
		Name:           "Default",
		RendercvConfig: raw,
	}
}

func TestExtProfile_MapsFields(t *testing.T) {
	repo := &extProfileFakeRepo{row: testProfileRow(t)}
	svc := profile.NewService(repo, nil, "", "")

	out, err := svc.ExtProfile(context.Background())
	if err != nil {
		t.Fatalf("ExtProfile: %v", err)
	}

	if out.FullName != "Jane Doe" {
		t.Errorf("FullName = %q, want %q", out.FullName, "Jane Doe")
	}
	if out.Email != "jane@example.com" {
		t.Errorf("Email = %q", out.Email)
	}
	if out.Phone != "+1-555-1234" {
		t.Errorf("Phone = %q", out.Phone)
	}
	if out.Location != "San Francisco, CA" {
		t.Errorf("Location = %q", out.Location)
	}
	if out.Headline != "Senior Frontend Engineer" {
		t.Errorf("Headline = %q", out.Headline)
	}
	if len(out.Skills) != 2 || out.Skills[0] != "TypeScript" || out.Skills[1] != "Go" {
		t.Errorf("Skills = %v, want [TypeScript Go]", out.Skills)
	}
	if len(out.Links) != 1 || out.Links[0].URL != "https://jane.dev" || out.Links[0].Label != "Portfolio" {
		t.Errorf("Links = %+v", out.Links)
	}
	if len(out.WorkHistory) != 1 {
		t.Fatalf("WorkHistory len = %d, want 1", len(out.WorkHistory))
	}
	we := out.WorkHistory[0]
	if we.Employer != "Acme Corp" || we.Role != "Senior Engineer" || we.StartDate != "2020-03" {
		t.Errorf("WorkHistory[0] = %+v", we)
	}
	if !we.Current || we.EndDate != nil {
		t.Errorf("expected current=true, endDate=nil for an empty end_date, got current=%v endDate=%v", we.Current, we.EndDate)
	}
	if len(out.Education) != 1 {
		t.Fatalf("Education len = %d, want 1", len(out.Education))
	}
	ed := out.Education[0]
	if ed.Institution != "MIT" || ed.Degree != "B.S., Computer Science" {
		t.Errorf("Education[0] = %+v", ed)
	}
}

func TestExtProfile_NoConfig(t *testing.T) {
	repo := &extProfileFakeRepo{row: sqlcgen.Profile{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Name: "Empty"}}
	svc := profile.NewService(repo, nil, "", "")

	out, err := svc.ExtProfile(context.Background())
	if err != nil {
		t.Fatalf("ExtProfile: %v", err)
	}
	if out.FullName != "" {
		t.Errorf("expected empty FullName for a profile with no rendercv config, got %q", out.FullName)
	}
	if out.Skills == nil || len(out.Skills) != 0 {
		t.Errorf("expected an empty (non-nil) Skills slice, got %v", out.Skills)
	}
}
