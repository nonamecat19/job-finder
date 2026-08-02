package infrastructure

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ledongthuc/pdf"
	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/storage"
)

// RenderCvRenderer renders a (tailored) RenderCV master object to PDF by
// writing it back to YAML and shelling out to the `rendercv` CLI. Sits
// alongside HtmlPdfRenderer: the JSON-Resume path uses html/template +
// chromedp, this path uses the user's real rendercv config so the Typst
// theme/design is preserved exactly. Mirrors rendercv-renderer.ts.
type RenderCvRenderer struct {
	outDir string
	bin    string
	// Store, when set, receives the rendered YAML + PDF so all resume files are
	// persisted to object storage (MinIO) in addition to the local outDir.
	Store storage.Blobstore
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
func (r *RenderCvRenderer) Render(ctx context.Context, master domain.RendercvMaster, baseName string) (yamlPath, pdfPath string, err error) {
	outDir, err := ensureOutDir(r.outDir)
	if err != nil {
		return "", "", fmt.Errorf("rendercv: mkdir: %w", err)
	}
	yamlPath = filepath.Join(outDir, baseName+".yaml")
	pdfPath = filepath.Join(outDir, baseName+".pdf")

	ordered, err := domain.PrepareMasterForMarshal(master)
	if err != nil {
		return "", "", fmt.Errorf("rendercv: prepare sections order: %w", err)
	}
	data, err := yaml.Marshal(map[string]any(ordered))
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(yamlPath, data, 0o644); err != nil {
		return "", "", fmt.Errorf("rendercv: write yaml: %w", err)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, r.bin, "render", yamlPath, "-o", outDir, "-pdf", baseName+".pdf", "-nopng", "-nohtml", "-nomd")
	cmd.Dir = outDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("rendercv: render failed: %w: %s", err, stderr.String())
	}
	if stderr.Len() > 0 {
		slog.Debug("rendercv stderr", "output", stderr.String())
	}
	if r.Store != nil {
		if err := r.Store.Upload(ctx, filepath.Base(yamlPath), yamlPath, "application/x-yaml"); err != nil {
			return "", "", err
		}
		if err := r.Store.Upload(ctx, filepath.Base(pdfPath), pdfPath, "application/pdf"); err != nil {
			return "", "", err
		}
	}
	return yamlPath, pdfPath, nil
}

// CountPages returns the number of pages in a PDF file.
func CountPages(pdfPath string) (int, error) {
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return 0, fmt.Errorf("count pages: open: %w", err)
	}
	defer f.Close()
	return r.NumPage(), nil
}
