package dto

import (
	"encoding/json"
	"testing"
)

func TestEditProposalDto_RoundTrip(t *testing.T) {
	dr := "grounding_violation"
	original := EditProposalDto{
		ID:            "proposal-1",
		DraftID:       "draft-1",
		FieldType:     "summary",
		FieldKey:      "summary",
		BeforeValue:   "old summary",
		AfterValue:    "new summary",
		Traceability:  TraceabilityDto{Source: "master", Path: "cv.sections.summary"},
		Status:        "pending",
		DroppedReason: &dr,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored EditProposalDto
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ID != original.ID || restored.FieldType != original.FieldType || restored.BeforeValue != original.BeforeValue {
		t.Errorf("EditProposalDto round-trip mismatch: %+v != %+v", restored, original)
	}
	if restored.DroppedReason == nil || *restored.DroppedReason != "grounding_violation" {
		t.Errorf("droppedReason not preserved")
	}
}

func TestTailoredDraftDto_RoundTrip(t *testing.T) {
	jobID := "job-1"
	original := TailoredDraftDto{
		ID:        "draft-1",
		ProfileID: "profile-1",
		JobID:     &jobID,
		State:     "review",
		Model:     "llama3",
		BaselineSummary: BaselineSummaryDto{
			ProfileName: "John Doe",
			SkillGroups: []string{"Backend", "Cloud"},
			Companies:   []string{"Acme Corp"},
		},
		Proposals: []EditProposalDto{
			{ID: "p1", FieldType: "summary", FieldKey: "summary", BeforeValue: "old", AfterValue: "new", Status: "pending"},
		},
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored TailoredDraftDto
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ID != original.ID || restored.State != original.State || restored.BaselineSummary.ProfileName != "John Doe" {
		t.Errorf("TailoredDraftDto round-trip mismatch")
	}
	if len(restored.Proposals) != 1 {
		t.Errorf("expected 1 proposal, got %d", len(restored.Proposals))
	}
}

func TestExportBlockDto_RoundTrip(t *testing.T) {
	original := ExportBlockDto{Field: "experience:Acme:3", Suggestion: "shorten this bullet"}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored ExportBlockDto
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Field != original.Field || restored.Suggestion != original.Suggestion {
		t.Errorf("ExportBlockDto round-trip mismatch")
	}
}

func TestTailorResumeRequestDto_RoundTrip(t *testing.T) {
	jobID := "job-1"
	original := TailorResumeRequestDto{
		ProfileID: "profile-1",
		JobID:     &jobID,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored TailorResumeRequestDto
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ProfileID != "profile-1" || *restored.JobID != "job-1" {
		t.Errorf("TailorResumeRequestDto round-trip mismatch")
	}
}

func TestAdhocVacancyDto_RoundTrip(t *testing.T) {
	original := AdhocVacancyDto{Company: "Acme", Title: "Engineer", Text: "job description"}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored AdhocVacancyDto
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Company != "Acme" || restored.Title != "Engineer" {
		t.Errorf("AdhocVacancyDto round-trip mismatch")
	}
}
