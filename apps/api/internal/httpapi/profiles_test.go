package httpapi_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/profile"
	"github.com/job-finder/api/internal/testutil"
)

type fakeProfileProvider struct{}

func (f *fakeProfileProvider) List(ctx context.Context) ([]dto.ProfileDto, error) {
	return []dto.ProfileDto{{ID: "p1", Name: "Default"}}, nil
}

func (f *fakeProfileProvider) GetDto(ctx context.Context, id string) (dto.ProfileDto, error) {
	return dto.ProfileDto{ID: id, Name: "Default"}, nil
}

func (f *fakeProfileProvider) Create(ctx context.Context, name string, document dto.JsonResume, extraNotes *string) (dto.ProfileDto, error) {
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

func (f *fakeProfileProvider) ImportPdf(ctx context.Context, data []byte) (*profile.ImportResult, error) {
	return &profile.ImportResult{TextLength: len(data)}, nil
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
		"name":     "My Profile",
		"document": map[string]any{},
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

func TestProfilesRemove(t *testing.T) {
	h := &httpapi.ProfilesHandler{Profiles: &fakeProfileProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "DELETE", "/api/profiles/p1", nil, map[string]string{"id": "p1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
