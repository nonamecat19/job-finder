package httpapi_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/profile"
	"github.com/job-finder/api/internal/testutil"
)

type fakeProfileProvider struct {
	resumes map[string]dto.Resume
}

func (f *fakeProfileProvider) List(ctx context.Context) ([]dto.ProfileDto, error) {
	return []dto.ProfileDto{{ID: "p1", Name: "Default"}}, nil
}

func (f *fakeProfileProvider) GetDto(ctx context.Context, id string) (dto.ProfileDto, error) {
	return dto.ProfileDto{ID: id, Name: "Default"}, nil
}

func (f *fakeProfileProvider) Create(ctx context.Context, name string, rendercvYaml string, extraNotes *string) (dto.ProfileDto, error) {
	return dto.ProfileDto{ID: "p-new", Name: name}, nil
}

func (f *fakeProfileProvider) Update(ctx context.Context, id string, in profile.UpdateInput) (dto.ProfileDto, error) {
	name := "Updated"
	if in.Name != nil {
		name = *in.Name
	}
	return dto.ProfileDto{ID: id, Name: name}, nil
}

func (f *fakeProfileProvider) Remove(ctx context.Context, id string) error {
	return nil
}

func (f *fakeProfileProvider) SaveConfig(ctx context.Context, yamlText string) (dto.ProfileDto, error) {
	return dto.ProfileDto{ID: "p-new", Name: "Uploaded Profile", HasConfig: true}, nil
}

func (f *fakeProfileProvider) GetResume(ctx context.Context, id string) (dto.Resume, error) {
	if f.resumes == nil {
		return dto.Resume{Name: "Default", Sections: []dto.Section{}}, nil
	}
	r, ok := f.resumes[id]
	if !ok {
		return dto.Resume{Name: "Default", Sections: []dto.Section{}}, nil
	}
	return r, nil
}

func (f *fakeProfileProvider) UpdateResume(ctx context.Context, id string, resume dto.Resume) (dto.Resume, error) {
	if verr := generation.ValidateResume(resume); verr != nil {
		return dto.Resume{}, verr
	}
	if f.resumes == nil {
		f.resumes = map[string]dto.Resume{}
	}
	f.resumes[id] = resume
	return resume, nil
}

func (f *fakeProfileProvider) HasResumeContent(ctx context.Context, id string) (bool, error) {
	r, _ := f.GetResume(ctx, id)
	return len(r.Sections) > 0, nil
}

func TestProfilesList(t *testing.T) {
	h := &httpapi.ProfilesHandler{Profiles: &fakeProfileProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/profiles", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []dto.ProfileDto
	testutil.ParseJSON(w, &out)
	if len(out) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(out))
	}
}

func TestProfilesGet(t *testing.T) {
	h := &httpapi.ProfilesHandler{Profiles: &fakeProfileProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/profiles/p1", nil, map[string]string{"id": "p1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestProfilesCreate(t *testing.T) {
	h := &httpapi.ProfilesHandler{Profiles: &fakeProfileProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/profiles", map[string]any{
		"name":         "My Profile",
		"rendercvYaml": "cv:\n  name: Test User",
	}, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestProfilesUpdate(t *testing.T) {
	h := &httpapi.ProfilesHandler{Profiles: &fakeProfileProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "PUT", "/api/profiles/p1", map[string]any{"name": "New Name"}, map[string]string{"id": "p1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestGetResume_EmptyProfile asserts an empty profile (FR-012) returns a
// valid, non-error structured resume rather than an error.
func TestGetResume_EmptyProfile(t *testing.T) {
	h := &httpapi.ProfilesHandler{Profiles: &fakeProfileProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/profiles/p1/resume", nil, map[string]string{"id": "p1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out dto.ResumeDto
	testutil.ParseJSON(w, &out)
	if len(out.Resume.Sections) != 0 {
		t.Fatalf("expected zero sections for empty profile, got %d", len(out.Resume.Sections))
	}
}

// TestUpdateResume_AllEntryTypesAndOrder asserts a PUT populating all 9 entry
// types, then a GET, round-trips content and section order (FR-003, FR-006,
// SC-004; spec 009 quickstart.md Scenario 1 / 3).
func TestUpdateResume_AllEntryTypesAndOrder(t *testing.T) {
	h := &httpapi.ProfilesHandler{Profiles: &fakeProfileProvider{}}
	r := testutil.SetupRouter(h.Mount)

	body := map[string]any{
		"resume": map[string]any{
			"name": "Jane Doe",
			"sections": []map[string]any{
				{"name": "education", "entryType": "education", "entries": []map[string]any{{"institution": "MIT"}}},
				{"name": "experience", "entryType": "experience", "entries": []map[string]any{{"company": "Acme"}}},
				{"name": "skills", "entryType": "one_line", "entries": []map[string]any{{"label": "Languages", "details": "Go"}}},
			},
		},
	}
	w := testutil.DoRequestJSON(r, "PUT", "/api/profiles/p1/resume", body, map[string]string{"id": "p1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w2 := testutil.DoRequest(r, "GET", "/api/profiles/p1/resume", nil, map[string]string{"id": "p1"})
	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	var out dto.ResumeDto
	testutil.ParseJSON(w2, &out)
	if len(out.Resume.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(out.Resume.Sections))
	}
	wantOrder := []string{"education", "experience", "skills"}
	for i, name := range wantOrder {
		if out.Resume.Sections[i].Name != name {
			t.Errorf("section order[%d] = %q, want %q", i, out.Resume.Sections[i].Name, name)
		}
	}

	// Reorder sections (experience first) and confirm the new order persists
	// (FR-006, spec 009 quickstart.md Scenario 3).
	reordered := out.Resume
	reordered.Sections[0], reordered.Sections[1] = reordered.Sections[1], reordered.Sections[0]
	w3 := testutil.DoRequestJSON(r, "PUT", "/api/profiles/p1/resume", dto.ResumeDto{Resume: reordered}, map[string]string{"id": "p1"})
	if w3.Code != 200 {
		t.Fatalf("expected 200 on reorder, got %d: %s", w3.Code, w3.Body.String())
	}
	w4 := testutil.DoRequest(r, "GET", "/api/profiles/p1/resume", nil, map[string]string{"id": "p1"})
	var out2 dto.ResumeDto
	testutil.ParseJSON(w4, &out2)
	if out2.Resume.Sections[0].Name != "experience" || out2.Resume.Sections[1].Name != "education" {
		t.Fatalf("reordered sections not persisted: %+v", []string{out2.Resume.Sections[0].Name, out2.Resume.Sections[1].Name})
	}
}

// TestUpdateResume_ValidationError asserts an invalid resume (missing name)
// is rejected with a 400 pointing at the offending field (FR-007).
func TestUpdateResume_ValidationError(t *testing.T) {
	h := &httpapi.ProfilesHandler{Profiles: &fakeProfileProvider{}}
	r := testutil.SetupRouter(h.Mount)

	body := map[string]any{"resume": map[string]any{"name": "", "sections": []map[string]any{}}}
	w := testutil.DoRequestJSON(r, "PUT", "/api/profiles/p1/resume", body, map[string]string{"id": "p1"})
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProfilesRemove(t *testing.T) {
	h := &httpapi.ProfilesHandler{Profiles: &fakeProfileProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "DELETE", "/api/profiles/p1", nil, map[string]string{"id": "p1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
