//go:build live

package infrastructure

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nonamecat19/rendercv-go/pkg/rendercv"

	"github.com/job-finder/api/internal/generation/domain"
)

func TestRenderCvRenderer_CountPages(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_rendercv.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	master := domain.RendercvMaster(domain.NormalizeYAMLMap(m).(map[string]any))

	outDir := t.TempDir()
	r := NewRenderCvRenderer(outDir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, pdfPath, err := r.Render(ctx, master, "page-count-test")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	pages, err := CountPages(pdfPath)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if pages <= 0 {
		t.Fatalf("expected positive page count, got %d", pages)
	}

	goldenPages, err := os.ReadFile("testdata/compare/golden/sample_rendercv.pages")
	if err != nil {
		t.Fatalf("read golden pages: %v", err)
	}
	wantPages, err := strconv.Atoi(strings.TrimSpace(string(goldenPages)))
	if err != nil {
		t.Fatalf("parse golden pages: %v", err)
	}

	if pages != wantPages {
		t.Errorf("page count mismatch: got %d, want %d", pages, wantPages)
	}
}

func TestRenderCvRenderer_ValidationFailure(t *testing.T) {
	master := domain.RendercvMaster{"cv": map[string]any{"name": 5}}
	r := NewRenderCvRenderer(t.TempDir())
	ctx := context.Background()

	_, _, err := r.Render(ctx, master, "validation-fail")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rendercv: invalid document:") {
		t.Errorf("error should contain 'rendercv: invalid document:', got: %v", err)
	}
	if !strings.Contains(err.Error(), "[cv name]") {
		t.Errorf("error should contain '[cv name]', got: %v", err)
	}
	var valErr *rendercv.UserValidationError
	if !errors.As(err, &valErr) {
		t.Errorf("error should wrap *rendercv.UserValidationError, got: %T", err)
	}
}

func TestRenderCvRenderer_InternalError(t *testing.T) {
	master := domain.RendercvMaster{"cv": map[string]any{"name": "Test"}}
	r := NewRenderCvRenderer("/dev/null/subdir")
	ctx := context.Background()

	_, _, err := r.Render(ctx, master, "internal-error")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "rendercv: internal error:") && !strings.Contains(msg, "rendercv: mkdir:") {
		t.Errorf("error should contain 'rendercv: internal error:' or 'rendercv: mkdir:', got: %v", err)
	}
}

func TestRenderCvRenderer_Cancellation(t *testing.T) {
	master := domain.RendercvMaster{"cv": map[string]any{"name": "Test"}}
	r := NewRenderCvRenderer(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, pdfPath, err := r.Render(ctx, master, "cancel-test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error should wrap context.Canceled, got: %v", err)
	}
	if _, statErr := os.Stat(pdfPath); statErr == nil {
		t.Errorf("expected no file at %s, but it exists", pdfPath)
	}
}
