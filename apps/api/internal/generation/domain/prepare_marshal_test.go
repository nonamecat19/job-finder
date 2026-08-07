package domain

import "testing"

func entriesFromPrepared(t *testing.T, out RendercvMaster, sectionKey string) []map[string]any {
	t.Helper()
	cv, _ := out["cv"].(map[string]any)
	ordered, ok := cv["sections"].(OrderedYAMLMap)
	if !ok {
		t.Fatalf("cv.sections is %T, want OrderedYAMLMap", cv["sections"])
	}
	return AsSliceOfMaps(ordered.Values[sectionKey])
}

func projectsFromPrepared(t *testing.T, out RendercvMaster) []map[string]any {
	return entriesFromPrepared(t, out, "projects")
}

func TestPrepareMasterForMarshalEmbedsProjectURLIntoName(t *testing.T) {
	master := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"projects": []any{
			map[string]any{"name": "job-finder", "url": "https://github.com/example/job-finder"},
		},
	}}}

	out, err := PrepareMasterForMarshal(master)
	if err != nil {
		t.Fatalf("PrepareMasterForMarshal: %v", err)
	}

	projects := projectsFromPrepared(t, out)
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	got := StringField(projects[0], "name")
	want := "[job-finder](https://github.com/example/job-finder)"
	if got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if _, hasURL := projects[0]["url"]; hasURL {
		t.Error("url key should be removed once folded into name")
	}
}

func TestPrepareMasterForMarshalLeavesProjectWithoutURLUntouched(t *testing.T) {
	master := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"projects": []any{
			map[string]any{"name": "job-finder"},
		},
	}}}

	out, err := PrepareMasterForMarshal(master)
	if err != nil {
		t.Fatalf("PrepareMasterForMarshal: %v", err)
	}

	projects := projectsFromPrepared(t, out)
	if got := StringField(projects[0], "name"); got != "job-finder" {
		t.Errorf("name = %q, want unchanged %q", got, "job-finder")
	}
}

func TestPrepareMasterForMarshalDoesNotDoubleWrapAlreadyMarkdownName(t *testing.T) {
	master := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"projects": []any{
			map[string]any{"name": "[job-finder](https://example.com/already)", "url": "https://github.com/example/job-finder"},
		},
	}}}

	out, err := PrepareMasterForMarshal(master)
	if err != nil {
		t.Fatalf("PrepareMasterForMarshal: %v", err)
	}

	projects := projectsFromPrepared(t, out)
	want := "[job-finder](https://example.com/already)"
	if got := StringField(projects[0], "name"); got != want {
		t.Errorf("name = %q, want unchanged %q", got, want)
	}
}

func TestPrepareMasterForMarshalEmbedsCertificationURLIntoName(t *testing.T) {
	master := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"certifications": []any{
			map[string]any{"name": "Microservice Architecture", "url": "https://robotdreams.io/cert/123", "date": "Mar 2026"},
		},
	}}}

	out, err := PrepareMasterForMarshal(master)
	if err != nil {
		t.Fatalf("PrepareMasterForMarshal: %v", err)
	}

	certs := entriesFromPrepared(t, out, "certifications")
	if len(certs) != 1 {
		t.Fatalf("got %d certifications, want 1", len(certs))
	}
	got := StringField(certs[0], "name")
	want := "[Microservice Architecture](https://robotdreams.io/cert/123)"
	if got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if _, hasURL := certs[0]["url"]; hasURL {
		t.Error("url key should be removed once folded into name")
	}
	if got := StringField(certs[0], "date"); got != "Mar 2026" {
		t.Errorf("date = %q, want unchanged %q", got, "Mar 2026")
	}
}
