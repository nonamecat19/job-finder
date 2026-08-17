package http_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/job-finder/api/internal/dto"
	generationhttp "github.com/job-finder/api/internal/generation/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

type fakeDocGenerator struct {
	pdfPath      *string
	downloadName string
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
