//go:build integration

package domain

import (
	"testing"
)

// certificationsIntegrationMasterYAML is a full RenderCV master document —
// parsed the same way a real master profile is (ParseRendercv), so the
// synthetic `_order` section list is derived from the YAML itself rather
// than hand-built — with a certifications section among several others, for
// US1's end-to-end assertion that disabling certifications removes it from
// both cv.sections and the enforced order while leaving the remaining
// sections' relative order intact (FR-004).
const certificationsIntegrationMasterYAML = `
cv:
  name: Jane Doe
  sections:
    summary:
      - Old summary line.
    skills:
      - label: Backend
        details: Go, Postgres, Docker
    experience:
      - company: Acme Corp
        position: Senior Engineer
        start_date: 2020-01
        end_date: present
        highlights:
          - Did a thing
    certifications:
      - label: AWS Certified Solutions Architect
        details: Amazon Web Services
      - label: Certified Kubernetes Administrator
        details: CNCF
    education:
      - institution: MIT
        area: Computer Science
        studyType: Bachelor
        start_date: 2014-09
        end_date: 2018-05
design:
  theme: sb2nov
`

// TestIntegration_ApplySectionTogglesRemovesCertificationsFromRenderedResume
// exercises the real parse → merge → toggle pipeline (ParseRendercv,
// MergeTailored, ApplySectionToggles) end to end, standing in for the render
// step: rendercv renders exactly what cv.sections + _order describe, so
// asserting on those after the pipeline is equivalent to asserting on the
// rendered document's section list for this feature's purposes (T016).
func TestIntegration_ApplySectionTogglesRemovesCertificationsFromRenderedResume(t *testing.T) {
	master, err := ParseRendercv(certificationsIntegrationMasterYAML)
	if err != nil {
		t.Fatalf("ParseRendercv: %v", err)
	}

	masterOrderBefore := sectionOrder(master)
	masterCertsBefore := certificationLabels(master)
	if len(masterCertsBefore) != 2 {
		t.Fatalf("fixture setup: master certifications = %v, want 2 entries", masterCertsBefore)
	}

	merged, err := MergeTailored(master, TailoredSections{Summary: "New summary."})
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	cfg := DefaultShapeConfig()
	cfg.CertificationsEnabled = false
	ApplySectionToggles(merged, cfg)

	// The rendered resume (merged) must have no certifications section left,
	// in either cv.sections or the enforced _order list.
	if _, present := CvSections(merged)["certifications"]; present {
		t.Error("merged certifications section still present, want it removed from the rendered resume")
	}
	wantOrder := []string{"summary", "skills", "experience", "education"}
	if got := sectionOrder(merged); !equalStringSlices(got, wantOrder) {
		t.Errorf("merged _order = %v, want %v with relative order of remaining sections preserved", got, wantOrder)
	}

	// FR-004: the source master profile must be untouched by disabling the
	// section on the generated resume.
	if got := certificationLabels(master); !equalStringSlices(got, masterCertsBefore) {
		t.Errorf("master certifications = %v, want unchanged %v", got, masterCertsBefore)
	}
	if got := sectionOrder(master); !equalStringSlices(got, masterOrderBefore) {
		t.Errorf("master _order = %v, want unchanged %v", got, masterOrderBefore)
	}
	if _, present := CvSections(master)["certifications"]; !present {
		t.Error("master certifications section removed, want it left in place on the master profile")
	}
}

// TestIntegration_ApplySectionTogglesCertificationsEnabledRendersSection
// covers the toggle-back-on side of US1's independent test: with the
// default config (certifications enabled), the rendered resume keeps the
// certifications section and its authored order.
func TestIntegration_ApplySectionTogglesCertificationsEnabledRendersSection(t *testing.T) {
	master, err := ParseRendercv(certificationsIntegrationMasterYAML)
	if err != nil {
		t.Fatalf("ParseRendercv: %v", err)
	}

	merged, err := MergeTailored(master, TailoredSections{Summary: "New summary."})
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	ApplySectionToggles(merged, DefaultShapeConfig())

	wantOrder := []string{"summary", "skills", "experience", "certifications", "education"}
	if got := sectionOrder(merged); !equalStringSlices(got, wantOrder) {
		t.Errorf("merged _order = %v, want %v", got, wantOrder)
	}
	if got := certificationLabels(merged); len(got) != 2 {
		t.Errorf("merged certifications = %v, want both entries kept", got)
	}
}

// TestIntegration_ApplySectionTogglesNoCertificationsSectionRendersCleanly
// covers FR-004's "no panic or error" clause end to end: a master profile
// authored before this feature existed, with no certifications section at
// all, must generate successfully under both toggle states.
func TestIntegration_ApplySectionTogglesNoCertificationsSectionRendersCleanly(t *testing.T) {
	const noCertsYAML = `
cv:
  name: Jane Doe
  sections:
    summary:
      - Old summary line.
    experience:
      - company: Acme Corp
        position: Senior Engineer
        start_date: 2020-01
        end_date: present
        highlights:
          - Did a thing
`
	master, err := ParseRendercv(noCertsYAML)
	if err != nil {
		t.Fatalf("ParseRendercv: %v", err)
	}
	merged, err := MergeTailored(master, TailoredSections{Summary: "New summary."})
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	for _, enabled := range []bool{true, false} {
		cfg := DefaultShapeConfig()
		cfg.CertificationsEnabled = enabled
		ApplySectionToggles(merged, cfg)

		if _, present := CvSections(merged)["certifications"]; present {
			t.Errorf("certificationsEnabled=%v: certifications section present, want it absent as it always was", enabled)
		}
	}
}
