//go:build integration

package domain

import (
	"testing"
)

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

	if _, present := CvSections(merged)["certifications"]; present {
		t.Error("merged certifications section still present, want it removed from the rendered resume")
	}
	wantOrder := []string{"summary", "skills", "experience", "education"}
	if got := sectionOrder(merged); !equalStringSlices(got, wantOrder) {
		t.Errorf("merged _order = %v, want %v with relative order of remaining sections preserved", got, wantOrder)
	}

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
