package domain

import "testing"

func TestVerifyStructureIntegrityIgnoresRemovedOptionalSections(t *testing.T) {
	master := loadSampleMaster(t)
	merged := loadSampleMaster(t)
	sections := CvSections(merged)
	RemoveSection(sections, "skills")
	RemoveSection(sections, "projects")

	if violations := VerifyStructureIntegrity(master, merged); len(violations) != 0 {
		t.Errorf("violations = %+v, want none for removed optional sections", violations)
	}
}
