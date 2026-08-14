package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ledongthuc/pdf"
	"github.com/nonamecat19/rendercv-go/pkg/rendercv"
	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/storage"
)

type RenderCvRenderer struct {
	outDir string
	Store  storage.Blobstore
}

func NewRenderCvRenderer(outDir string) *RenderCvRenderer {
	if outDir == "" {
		outDir = "/data/documents"
	}
	return &RenderCvRenderer{outDir: outDir}
}

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

	tmpFile, err := os.CreateTemp(outDir, baseName+".pdf.tmp.*")
	if err != nil {
		return "", "", fmt.Errorf("rendercv: internal error: create temp file: %w", err)
	}
	tmpPdfPath := tmpFile.Name()
	tmpFile.Close()
	os.Remove(tmpPdfPath)

	tmpTypstPath := filepath.Join(outDir, baseName+".typ.tmp."+filepath.Base(tmpPdfPath))

	cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	type renderResult struct {
		typstPath string
		pdfPath   string
		err       error
	}
	resultCh := make(chan renderResult, 1)

	go func() {
		var res renderResult
		defer func() {
			resultCh <- res
		}()

		yamlContent, readErr := os.ReadFile(yamlPath)
		if readErr != nil {
			res.err = fmt.Errorf("rendercv: internal error: read yaml: %w", readErr)
			return
		}

		opts := rendercv.BuildOptions{
			InputFilePath:        yamlPath,
			PDFPath:              tmpPdfPath,
			TypstPath:            tmpTypstPath,
			DontGenerateMarkdown: true,
			DontGenerateHTML:     true,
			DontGeneratePNG:      true,
		}

		_, model, buildErr := rendercv.Build(string(yamlContent), opts)
		if buildErr != nil {
			res.err = classifyRenderError(buildErr)
			return
		}

		typstPath, typstErr := rendercv.GenerateTypst(model)
		if typstErr != nil {
			res.err = fmt.Errorf("rendercv: internal error: %w", typstErr)
			return
		}

		pdfPath, pdfErr := rendercv.GeneratePDF(model, typstPath)
		if pdfErr != nil {
			res.err = fmt.Errorf("rendercv: internal error: %w", pdfErr)
			return
		}

		res.typstPath = typstPath
		res.pdfPath = pdfPath
	}()

	var res renderResult
	select {
	case res = <-resultCh:
	case <-cmdCtx.Done():
		bestEffortRemove(tmpPdfPath)
		bestEffortRemove(tmpTypstPath)
		return "", "", fmt.Errorf("rendercv: render cancelled: %w", ctx.Err())
	}

	if res.err != nil {
		bestEffortRemove(tmpPdfPath)
		bestEffortRemove(tmpTypstPath)
		return "", "", res.err
	}

	if err := os.Rename(res.pdfPath, pdfPath); err != nil {
		bestEffortRemove(tmpPdfPath)
		bestEffortRemove(tmpTypstPath)
		return "", "", fmt.Errorf("rendercv: internal error: rename pdf: %w", err)
	}

	bestEffortRemove(res.typstPath)

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

func classifyRenderError(err error) error {
	var valErr *rendercv.UserValidationError
	if errors.As(err, &valErr) {
		var details []string
		for _, ve := range valErr.Errors {
			loc := joinLocation(ve.SchemaLocation)
			details = append(details, fmt.Sprintf("%s: %s", loc, ve.Message))
		}
		return fmt.Errorf("rendercv: invalid document: %s: %w", joinDetails(details), err)
	}
	var userErr *rendercv.UserError
	if errors.As(err, &userErr) {
		return fmt.Errorf("rendercv: invalid document: %s: %w", userErr.Message, err)
	}
	var intErr *rendercv.InternalError
	if errors.As(err, &intErr) {
		return fmt.Errorf("rendercv: internal error: %w", err)
	}
	return fmt.Errorf("rendercv: internal error: %w", err)
}

func joinLocation(loc []string) string {
	if len(loc) == 0 {
		return ""
	}
	s := "["
	for i, p := range loc {
		if i > 0 {
			s += " "
		}
		s += p
	}
	s += "]"
	return s
}

func joinDetails(details []string) string {
	if len(details) == 0 {
		return ""
	}
	s := details[0]
	for _, d := range details[1:] {
		s += "; " + d
	}
	return s
}

func bestEffortRemove(path string) {
	if path != "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.Debug("rendercv: failed to remove temp file", "path", path, "error", err)
		}
	}
}

func CountPages(pdfPath string) (int, error) {
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return 0, fmt.Errorf("count pages: open: %w", err)
	}
	defer f.Close()
	return r.NumPage(), nil
}
