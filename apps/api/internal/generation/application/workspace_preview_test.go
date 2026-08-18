package application

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/generation/domain"
)

func TestPreviewDocument_MatchesExportAssembly(t *testing.T) {
	master := exportMaster()
	sections := exportSections()

	got, err := previewDocumentFromMaster(master, sections)
	if err != nil {
		t.Fatalf("previewDocumentFromMaster: %v", err)
	}

	wantDoc, err := domain.Assemble(master, sections)
	if err != nil {
		t.Fatalf("domain.Assemble: %v", err)
	}
	wantOrdered, err := domain.PrepareMasterForMarshal(wantDoc)
	if err != nil {
		t.Fatalf("domain.PrepareMasterForMarshal: %v", err)
	}
	wantBytes, err := yaml.Marshal(map[string]any(wantOrdered))
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	if got.Yaml != string(wantBytes) {
		t.Fatalf("preview YAML diverged from export's own assembly:\n--- got ---\n%s\n--- want ---\n%s", got.Yaml, string(wantBytes))
	}
	if got.SectionsHash == "" {
		t.Fatal("SectionsHash is empty")
	}
}

func TestPreviewDocument_SectionsHashStable(t *testing.T) {
	master := exportMaster()
	sections := exportSections()

	first, err := previewDocumentFromMaster(master, sections)
	if err != nil {
		t.Fatalf("previewDocumentFromMaster: %v", err)
	}
	second, err := previewDocumentFromMaster(master, sections)
	if err != nil {
		t.Fatalf("previewDocumentFromMaster: %v", err)
	}
	if first.SectionsHash != second.SectionsHash {
		t.Fatalf("SectionsHash not stable across identical calls: %q vs %q", first.SectionsHash, second.SectionsHash)
	}
}
