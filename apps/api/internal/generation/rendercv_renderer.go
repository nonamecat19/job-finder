package generation

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// RenderCvRenderer renders a (tailored) RenderCV master object to PDF by
// writing it back to YAML and shelling out to the `rendercv` CLI. Sits
// alongside HtmlPdfRenderer: the JSON-Resume path uses html/template +
// chromedp, this path uses the user's real rendercv config so the Typst
// theme/design is preserved exactly. Mirrors rendercv-renderer.ts.
type RenderCvRenderer struct {
	outDir string
	bin    string
}

func NewRenderCvRenderer(outDir, bin string) *RenderCvRenderer {
	if outDir == "" {
		outDir = "/data/documents"
	}
	if bin == "" {
		bin = "rendercv"
	}
	return &RenderCvRenderer{outDir: outDir, bin: bin}
}

// Render writes master to <outDir>/<baseName>.yaml and runs
// `rendercv render <yaml> -o <outDir> -pdf <baseName>.pdf -nopng -nohtml -nomd`,
// returning both file paths.
func (r *RenderCvRenderer) Render(ctx context.Context, master RendercvMaster, baseName string) (yamlPath, pdfPath string, err error) {
	if err := os.MkdirAll(r.outDir, 0o755); err != nil {
		return "", "", fmt.Errorf("rendercv: mkdir: %w", err)
	}
	yamlPath = filepath.Join(r.outDir, baseName+".yaml")
	pdfPath = filepath.Join(r.outDir, baseName+".pdf")

	data, err := yaml.Marshal(map[string]any(master))
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(yamlPath, data, 0o644); err != nil {
		return "", "", fmt.Errorf("rendercv: write yaml: %w", err)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, r.bin, "render", yamlPath, "-o", r.outDir, "-pdf", baseName+".pdf", "-nopng", "-nohtml", "-nomd")
	cmd.Dir = r.outDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("rendercv: render failed: %w: %s", err, stderr.String())
	}
	if stderr.Len() > 0 {
		slog.Debug("rendercv stderr", "output", stderr.String())
	}
	return yamlPath, pdfPath, nil
}
