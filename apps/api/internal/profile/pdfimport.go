package profile

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/llm"
)

// ImportResult mirrors ProfileService.importPdf's return shape.
type ImportResult struct {
	Draft      dto.JsonResume
	TextLength int
}

// ImportPdf extracts text from an uploaded resume PDF and asks the LLM to
// convert it into a structured JSON Resume draft. Mirrors
// ProfileService.importPdf: the caller reviews and saves the draft
// explicitly — nothing is persisted here.
func (s *Service) ImportPdf(ctx context.Context, data []byte) (*ImportResult, error) {
	text, err := extractPdfText(data)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 50 {
		return nil, fmt.Errorf("could not extract text from PDF (scanned image? try a text-based PDF)")
	}
	if len(trimmed) > 15000 {
		trimmed = trimmed[:15000]
	}

	draft, err := llm.CompleteStructured[dto.JsonResume](ctx, s.llmc,
		"Extract a structured resume from the following raw text of a PDF resume.\n"+
			"Rules: copy facts exactly as written — do NOT invent, embellish, or guess missing employers, dates, or degrees. "+
			"Omit fields that are not present in the text.\n\nRESUME TEXT:\n"+trimmed,
		&llm.CompleteOptions{System: "You convert resume text into JSON Resume format. You never fabricate information."},
	)
	if err != nil {
		return nil, err
	}
	return &ImportResult{Draft: draft, TextLength: len(text)}, nil
}

// extractPdfText tries the pure-Go ledongthuc/pdf reader first (fast, no
// subprocess), and falls back to shelling out to poppler's `pdftotext` —
// mirrors the plan's "ledongthuc/pdf + fallback shell pdftotext" decision.
func extractPdfText(data []byte) (string, error) {
	if text, err := extractWithLedongthuc(data); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}
	return extractWithPdftotext(data)
}

func extractWithLedongthuc(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n")
	}
	return buf.String(), nil
}

func extractWithPdftotext(data []byte) (string, error) {
	tmp, err := os.CreateTemp("", "resume-*.pdf")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()

	out, err := exec.Command("pdftotext", tmp.Name(), "-").Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext failed: %w", err)
	}
	return string(out), nil
}
