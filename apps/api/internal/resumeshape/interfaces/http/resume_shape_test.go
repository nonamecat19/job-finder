package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
)

// fakeSettings is the handler's view of resumeshape.Service, with the same
// validate-before-write ordering: a rejected config must leave state untouched.
type fakeSettings struct {
	cfg domain.ShapeConfig
}

func (f *fakeSettings) Get() domain.ShapeConfig { return f.cfg }

func (f *fakeSettings) Update(_ context.Context, cfg domain.ShapeConfig) (domain.ShapeConfig, error) {
	if err := cfg.Validate(); err != nil {
		return domain.ShapeConfig{}, err
	}
	f.cfg = cfg
	return f.cfg, nil
}

func (f *fakeSettings) Reset(context.Context) (domain.ShapeConfig, error) {
	f.cfg = domain.DefaultShapeConfig()
	return f.cfg, nil
}

func newTestRouter() (chi.Router, *fakeSettings) {
	settings := &fakeSettings{cfg: domain.DefaultShapeConfig()}
	r := chi.NewRouter()
	(&ResumeShapeHandler{Settings: settings}).Mount(r)
	return r, settings
}

func do(t *testing.T, r chi.Router, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeConfig(t *testing.T, rec *httptest.ResponseRecorder) dto.ResumeShapeConfigDto {
	t.Helper()
	var out dto.ResumeShapeConfigDto
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return out
}

func errMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	msg, _ := out["message"].(string)
	return msg
}

func defaultDto() dto.ResumeShapeConfigDto { return configToDto(domain.DefaultShapeConfig()) }

func TestGetReturnsDefaults(t *testing.T) {
	r, _ := newTestRouter()

	rec := do(t, r, http.MethodGet, "/settings/resume-shape", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decodeConfig(t, rec); got != defaultDto() {
		t.Errorf("body = %+v, want the defaults %+v", got, defaultDto())
	}
}

func TestPutValidConfigRoundTrips(t *testing.T) {
	r, _ := newTestRouter()

	want := dto.ResumeShapeConfigDto{
		SummaryLines: 2, SkillsEnabled: true, SkillsMaxGroups: 3,
		ExperienceBulletsMin: 4, ExperienceBulletsMax: 5, TargetPages: 1,
		ProjectsEnabled: true, ProjectsMin: 1, ProjectsMax: 2, ProjectBulletsMax: 3,
	}
	rec := do(t, r, http.MethodPut, "/settings/resume-shape", want)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	if got := decodeConfig(t, rec); got != want {
		t.Errorf("PUT body = %+v, want %+v", got, want)
	}

	rec = do(t, r, http.MethodGet, "/settings/resume-shape", nil)
	if got := decodeConfig(t, rec); got != want {
		t.Errorf("GET after PUT = %+v, want %+v", got, want)
	}
}

func TestPutFullNonDefaultRoundTrip(t *testing.T) {
	r, _ := newTestRouter()

	// Every field set to an in-range value different from its default.
	want := dto.ResumeShapeConfigDto{
		SummaryLines: 12, SkillsEnabled: false, SkillsMaxGroups: 20,
		ExperienceBulletsMin: 1, ExperienceBulletsMax: 20, TargetPages: 3,
		ProjectsEnabled: true, ProjectsMin: 2, ProjectsMax: 4, ProjectBulletsMax: 10,
	}
	if rec := do(t, r, http.MethodPut, "/settings/resume-shape", want); rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	rec := do(t, r, http.MethodGet, "/settings/resume-shape", nil)
	if got := decodeConfig(t, rec); got != want {
		t.Errorf("GET after PUT = %+v, want %+v", got, want)
	}
}

func TestPutRejectsInvalidConfigs(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*dto.ResumeShapeConfigDto)
		wantMsgs []string
	}{
		{
			name:     "targetPages out of range",
			mutate:   func(d *dto.ResumeShapeConfigDto) { d.TargetPages = 4 },
			wantMsgs: []string{"targetPages", "1 and 3"},
		},
		{
			name: "experience bullets min above max",
			mutate: func(d *dto.ResumeShapeConfigDto) {
				d.ExperienceBulletsMin = 12
				d.ExperienceBulletsMax = 8
			},
			wantMsgs: []string{"experienceBulletsMin", "experienceBulletsMax"},
		},
		{
			name: "projects minimum without projects enabled",
			mutate: func(d *dto.ResumeShapeConfigDto) {
				d.ProjectsEnabled = false
				d.ProjectsMin = 2
			},
			wantMsgs: []string{"projectsMin", "projectsEnabled"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newTestRouter()
			body := defaultDto()
			tt.mutate(&body)

			rec := do(t, r, http.MethodPut, "/settings/resume-shape", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d (%s), want 400", rec.Code, rec.Body.String())
			}
			msg := errMessage(t, rec)
			for _, want := range tt.wantMsgs {
				if !strings.Contains(msg, want) {
					t.Errorf("error message %q does not name %q", msg, want)
				}
			}

			// Nothing was stored: the config is still what it was.
			rec = do(t, r, http.MethodGet, "/settings/resume-shape", nil)
			if got := decodeConfig(t, rec); got != defaultDto() {
				t.Errorf("GET after rejected PUT = %+v, want the previous values %+v", got, defaultDto())
			}
		})
	}
}

func TestPutRejectsUnparseableBody(t *testing.T) {
	r, _ := newTestRouter()

	req := httptest.NewRequest(http.MethodPut, "/settings/resume-shape", strings.NewReader("{not json"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestDeleteResetsToDefaults(t *testing.T) {
	r, _ := newTestRouter()

	changed := defaultDto()
	changed.TargetPages = 1
	changed.SummaryLines = 8
	if rec := do(t, r, http.MethodPut, "/settings/resume-shape", changed); rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d (%s), want 200", rec.Code, rec.Body.String())
	}

	rec := do(t, r, http.MethodDelete, "/settings/resume-shape", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	if got := decodeConfig(t, rec); got != defaultDto() {
		t.Errorf("DELETE body = %+v, want the defaults", got)
	}

	rec = do(t, r, http.MethodGet, "/settings/resume-shape", nil)
	if got := decodeConfig(t, rec); got != defaultDto() {
		t.Errorf("GET after DELETE = %+v, want the defaults", got)
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	r, _ := newTestRouter()

	first := do(t, r, http.MethodDelete, "/settings/resume-shape", nil)
	second := do(t, r, http.MethodDelete, "/settings/resume-shape", nil)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses = %d and %d, want 200 both", first.Code, second.Code)
	}
	if decodeConfig(t, first) != decodeConfig(t, second) {
		t.Error("a second DELETE returned a different body, want the same defaults")
	}
}
