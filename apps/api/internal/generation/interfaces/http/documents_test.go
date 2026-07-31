package http_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation"
	generationhttp "github.com/job-finder/api/internal/generation/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

type fakeDocGenerator struct {
	pdfPath *string
}

func (f *fakeDocGenerator) GenerateAdHoc(ctx context.Context, in generation.AdHocInput) (dto.GeneratedDocumentDto, dto.GeneratedDocumentDto, error) {
	return dto.GeneratedDocumentDto{ID: "resume-1", Type: "resume"},
		dto.GeneratedDocumentDto{ID: "cover-1", Type: "cover_letter"}, nil
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

func (f *fakeDocGenerator) GetDocumentPdfPath(ctx context.Context, id string) (*string, error) {
	return f.pdfPath, nil
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
}
