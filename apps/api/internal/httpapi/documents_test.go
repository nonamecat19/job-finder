package httpapi_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"
)

type fakeDocGenerator struct {
	doc sqlcgen.GeneratedDocument
}

func (f *fakeDocGenerator) GenerateRendercvFromText(ctx context.Context, in generation.RendercvFromTextInput) (*generation.RendercvFromTextResult, error) {
	return &generation.RendercvFromTextResult{
		YamlPath:       "/tmp/test.yaml",
		PdfPath:        "/tmp/test.pdf",
		GroundingLevel: generation.GroundingModerate,
	}, nil
}

func (f *fakeDocGenerator) GetDocumentDto(ctx context.Context, id string) (dto.GeneratedDocumentDto, error) {
	return dto.GeneratedDocumentDto{ID: id, Type: "rendercv"}, nil
}

func (f *fakeDocGenerator) UpdateDocument(ctx context.Context, id, text string) (dto.GeneratedDocumentDto, error) {
	return dto.GeneratedDocumentDto{ID: id, Type: "rendercv"}, nil
}

func (f *fakeDocGenerator) GetDocument(ctx context.Context, id string) (sqlcgen.GeneratedDocument, error) {
	return f.doc, nil
}

func TestDocumentsTailor(t *testing.T) {
	h := &httpapi.DocumentsHandler{Generation: &fakeDocGenerator{}}
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
	h := &httpapi.DocumentsHandler{Generation: &fakeDocGenerator{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/documents/doc-1", nil, map[string]string{"id": "doc-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDocumentsUpdate(t *testing.T) {
	h := &httpapi.DocumentsHandler{Generation: &fakeDocGenerator{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "PUT", "/api/documents/doc-1", map[string]any{"text": "updated resume text"}, map[string]string{"id": "doc-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDocumentsPdfNotFound(t *testing.T) {
	fake := &fakeDocGenerator{doc: sqlcgen.GeneratedDocument{PdfPath: nil}}
	h := &httpapi.DocumentsHandler{Generation: fake}
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

	fake := &fakeDocGenerator{doc: sqlcgen.GeneratedDocument{PdfPath: &pdfPath}}
	h := &httpapi.DocumentsHandler{Generation: fake}
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
