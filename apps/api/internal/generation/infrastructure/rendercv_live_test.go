//go:build live

package infrastructure

import (
	"context"
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/generation/domain"
)

func TestLive_RenderCvRenderer_ProducesRealPDF(t *testing.T) {
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

	yamlPath, pdfPath, err := r.Render(ctx, master, "smoke-test")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, err := os.Stat(yamlPath); err != nil {
		t.Errorf("expected yaml file at %s: %v", yamlPath, err)
	}
	info, err := os.Stat(pdfPath)
	if err != nil {
		t.Fatalf("expected pdf file at %s: %v", pdfPath, err)
	}
	if info.Size() == 0 {
		t.Fatal("pdf file is empty")
	}
	t.Logf("produced PDF: %s (%d bytes)", pdfPath, info.Size())
}
