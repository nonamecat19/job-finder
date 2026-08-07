package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation"
	generationhttp "github.com/job-finder/api/internal/generation/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

type fakeDocGenerator struct {
	pdfPath      *string
	downloadName string

	coverLetter    dto.GeneratedDocumentDto
	coverLetterErr error
	coverLetterFor []string
}

func (f *fakeDocGenerator) GenerateAdHoc(ctx context.Context, in generation.AdHocInput) (dto.GeneratedDocumentDto, error) {
	return dto.GeneratedDocumentDto{ID: "resume-1", Type: "resume"}, nil
}

func (f *fakeDocGenerator) GenerateCoverLetterFor(ctx context.Context, resumeID string) (dto.GeneratedDocumentDto, error) {
	f.coverLetterFor = append(f.coverLetterFor, resumeID)
	if f.coverLetterErr != nil {
		return dto.GeneratedDocumentDto{}, f.coverLetterErr
	}
	return f.coverLetter, nil
}

func (f *fakeDocGenerator) ListAdHocDocuments(ctx context.Context) ([]dto.GeneratedDocumentDto, error) {
	return nil, nil
}

func (f *fakeDocGenerator) GetDocumentDto(ctx context.Context, id string) (dto.GeneratedDocumentDto, error) {
	return dto.GeneratedDocumentDto{ID: id, Type: "rendercv"}, nil
}

func (f *fakeDocGenerator) UpdateDocument(ctx context.Context, id, text string) (dto.GeneratedDocumentDto, error) {
	return dto.GeneratedDocumentDto{ID: id, Type: "rendercv"}, nil
}

func (f *fakeDocGenerator) GetDocumentDownload(ctx context.Context, id string) (*string, string, error) {
	return f.pdfPath, f.downloadName, nil
}

func TestDocumentsTailor(t *testing.T) {
	h := &generationhttp.DocumentsHandler{Generation: &fakeDocGenerator{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/documents/tailor", map[string]any{
		"vacancy": "Looking for a Go developer",
		"company": "Acme",
		"title":   "Senior Engineer",
	}, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	cover, ok := body["coverLetter"]
	if !ok {
		t.Fatal("expected the response to keep its coverLetter key")
	}
	if string(cover) != "null" {
		t.Fatalf("expected a null cover letter, got %s", cover)
	}
	if _, ok := body["resume"]; !ok {
		t.Fatal("expected a resume in the response")
	}
}

func TestDocumentsCoverLetter(t *testing.T) {
	fake := &fakeDocGenerator{coverLetter: dto.GeneratedDocumentDto{ID: "cover-1", Type: "cover_letter", Version: 1}}
	h := &generationhttp.DocumentsHandler{Generation: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/documents/resume-1/cover-letter", nil, map[string]string{"id": "resume-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out dto.GeneratedDocumentDto
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.ID != "cover-1" || out.Type != "cover_letter" {
		t.Fatalf("expected the generated cover letter, got %+v", out)
	}
	if len(fake.coverLetterFor) != 1 || fake.coverLetterFor[0] != "resume-1" {
		t.Fatalf("expected generation for resume-1, got %v", fake.coverLetterFor)
	}
}

func TestDocumentsCoverLetterUnknownID(t *testing.T) {
	fake := &fakeDocGenerator{coverLetterErr: apperr.NotFound("document", "missing")}
	h := &generationhttp.DocumentsHandler{Generation: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/documents/missing/cover-letter", nil, map[string]string{"id": "missing"})
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDocumentsCoverLetterNonResumeTarget(t *testing.T) {
	fake := &fakeDocGenerator{coverLetterErr: apperr.Conflict("document cover-9 is not a resume")}
	h := &generationhttp.DocumentsHandler{Generation: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/documents/cover-9/cover-letter", nil, map[string]string{"id": "cover-9"})
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDocumentsCoverLetterGenerationFailure(t *testing.T) {
	fake := &fakeDocGenerator{coverLetterErr: errors.New("gateway unreachable")}
	h := &generationhttp.DocumentsHandler{Generation: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/documents/resume-1/cover-letter", nil, map[string]string{"id": "resume-1"})
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDocumentsGet(t *testing.T) {
	h := &generationhttp.DocumentsHandler{Generation: &fakeDocGenerator{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/documents/doc-1", nil, map[string]string{"id": "doc-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDocumentsUpdate(t *testing.T) {
	h := &generationhttp.DocumentsHandler{Generation: &fakeDocGenerator{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "PUT", "/api/documents/doc-1", map[string]any{"text": "updated resume text"}, map[string]string{"id": "doc-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDocumentsPdfNotFound(t *testing.T) {
	fake := &fakeDocGenerator{pdfPath: nil}
	h := &generationhttp.DocumentsHandler{Generation: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/documents/doc-1/pdf", nil, map[string]string{"id": "doc-1"})
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDocumentsPdfServesFile(t *testing.T) {
	tmp := t.TempDir()
	pdfPath := filepath.Join(tmp, "resume.pdf")
	os.WriteFile(pdfPath, []byte("%PDF-1.4 fake"), 0644)

	fake := &fakeDocGenerator{pdfPath: &pdfPath}
	h := &generationhttp.DocumentsHandler{Generation: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/documents/doc-1/pdf", nil, map[string]string{"id": "doc-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/pdf" {
		t.Fatalf("expected application/pdf, got %s", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="resume.pdf"` {
		t.Fatalf("expected the on-disk base name when no download name is set, got %s", cd)
	}
}

func TestDocumentsPdfUsesDownloadName(t *testing.T) {
	tmp := t.TempDir()
	pdfPath := filepath.Join(tmp, "acme-senior-engineer-resume-1730000000000.pdf")
	os.WriteFile(pdfPath, []byte("%PDF-1.4 fake"), 0644)

	fake := &fakeDocGenerator{pdfPath: &pdfPath, downloadName: "CV_Ada_Lovelace.pdf"}
	h := &generationhttp.DocumentsHandler{Generation: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/documents/doc-1/pdf", nil, map[string]string{"id": "doc-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="CV_Ada_Lovelace.pdf"` {
		t.Fatalf("expected the download name, got %s", cd)
	}
}
