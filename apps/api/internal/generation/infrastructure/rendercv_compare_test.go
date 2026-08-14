//go:build live

package infrastructure

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/generation/domain"
)

func TestCompare_RenderCvRenderer_MatchesGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_rendercv.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	master := domain.RendercvMaster(domain.NormalizeYAMLMap(m).(map[string]any))

	prepared, err := domain.PrepareMasterForMarshal(master)
	if err != nil {
		t.Fatalf("PrepareMasterForMarshal: %v", err)
	}

	outDir := t.TempDir()
	r := NewRenderCvRenderer(outDir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, pdfPath, err := r.Render(ctx, prepared, "compare-test")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	cmd := exec.Command("pdftotext", "-layout", pdfPath, "-")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	gotText := strings.TrimSpace(string(out))

	goldenText, err := os.ReadFile("testdata/compare/golden/sample_rendercv.txt")
	if err != nil {
		t.Fatalf("read golden text: %v", err)
	}
	wantText := strings.TrimSpace(string(goldenText))

	if gotText != wantText {
		t.Errorf("text mismatch:\n%s", lineDiff(wantText, gotText))
	}

	pages, err := CountPages(pdfPath)
	if err != nil {
		t.Fatalf("CountPages: %v", err)
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

func lineDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	maxLen := len(wantLines)
	if len(gotLines) > maxLen {
		maxLen = len(gotLines)
	}
	var b strings.Builder
	for i := 0; i < maxLen; i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			fmt.Fprintf(&b, "line %d:\n  want: %q\n  got:  %q\n", i+1, w, g)
		}
	}
	return b.String()
}
