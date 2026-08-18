package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/application/ingest"
	"github.com/job-finder/api/internal/manualadd/application"
	"github.com/job-finder/api/internal/manualadd/domain"
)

const fillInURL = "https://careers.example.com/jobs/42"

func completeFillIn() application.FillIn {
	return application.FillIn{
		URL:         fillInURL,
		Title:       "Senior Go Engineer",
		Company:     "NovaTech",
		Description: "Own the ledger service.",
	}
}

func TestSaveFillIn_CreatesAVacancyIndistinguishableFromAnExtractedOne(t *testing.T) {
	h := newHarness(t, djinniStub())

	result, err := h.svc.SaveFillIn(context.Background(), completeFillIn())
	if err != nil {
		t.Fatalf("SaveFillIn: %v", err)
	}
	if result.Outcome != domain.OutcomeCreated {
		t.Fatalf("outcome = %q, want created", result.Outcome)
	}
	if result.Job == nil {
		t.Fatal("expected the created vacancy back")
	}
	if len(h.repo.inserted) != 1 {
		t.Fatalf("expected exactly one insert, got %d", len(h.repo.inserted))
	}
	if len(h.repo.runs) != 1 || h.repo.runs[0].Trigger != "manual" {
		t.Errorf("expected one manual-triggered run, got %+v", h.repo.runs)
	}
}

func TestSaveFillIn_DefaultsSourceKeyToManual(t *testing.T) {
	h := newHarness(t, djinniStub())

	if _, err := h.svc.SaveFillIn(context.Background(), completeFillIn()); err != nil {
		t.Fatalf("SaveFillIn: %v", err)
	}

	if len(h.subs.ensured) != 1 || h.subs.ensured[0] != "manual" {
		t.Errorf("expected the manual source's subscription, got %v", h.subs.ensured)
	}
	if got := h.repo.inserted[0].SourceKeys[0]; got != "manual" {
		t.Errorf("sourceKey = %q, want manual", got)
	}
}

func TestSaveFillIn_KeepsAnExplicitSourceKey(t *testing.T) {
	h := newHarness(t, djinniStub())
	in := completeFillIn()
	djinni := "djinni"
	in.SourceKey = &djinni

	if _, err := h.svc.SaveFillIn(context.Background(), in); err != nil {
		t.Fatalf("SaveFillIn: %v", err)
	}

	if got := h.repo.inserted[0].SourceKeys[0]; got != "djinni" {
		t.Errorf("sourceKey = %q, want djinni", got)
	}
}

func TestSaveFillIn_NamesEveryMissingRequiredField(t *testing.T) {
	tests := []struct {
		name string
		in   application.FillIn
		want []string
	}{
		{"all missing", application.FillIn{URL: fillInURL}, []string{"title", "company", "description"}},
		{"title and description", application.FillIn{URL: fillInURL, Company: "NovaTech"}, []string{"title", "description"}},
		{"blank counts as missing", application.FillIn{URL: fillInURL, Title: "   ", Company: "NovaTech", Description: "d"}, []string{"title"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, djinniStub())

			_, err := h.svc.SaveFillIn(context.Background(), tt.in)

			var missing *application.MissingFieldsError
			if !errors.As(err, &missing) {
				t.Fatalf("expected a MissingFieldsError, got %v", err)
			}
			for _, field := range tt.want {
				if !strings.Contains(missing.Error(), field) {
					t.Errorf("expected %q to be named, got %q", field, missing.Error())
				}
			}
			if len(h.repo.inserted) != 0 {
				t.Error("a rejected fill-in must leave no vacancy behind")
			}
			if len(h.repo.runs) != 0 {
				t.Error("a rejected fill-in never reached a source, so it writes no run")
			}
		})
	}
}

func TestSaveFillIn_RejectsAnInvalidURL(t *testing.T) {
	h := newHarness(t, djinniStub())
	in := completeFillIn()
	in.URL = "not-a-url"

	result, err := h.svc.SaveFillIn(context.Background(), in)
	if err != nil {
		t.Fatalf("SaveFillIn: %v", err)
	}
	if result.Outcome != domain.OutcomeFailed || result.Kind != domain.KindInvalidURL {
		t.Fatalf("outcome = %q kind = %q, want failed/invalid_url", result.Outcome, result.Kind)
	}
	if len(h.repo.inserted) != 0 {
		t.Error("expected nothing stored")
	}
}

func TestSaveFillIn_NeverDefaultsPostedAtToNow(t *testing.T) {
	h := newHarness(t, djinniStub())

	if _, err := h.svc.SaveFillIn(context.Background(), completeFillIn()); err != nil {
		t.Fatalf("SaveFillIn: %v", err)
	}

	postedAts := h.repo.inserted[0].PostedAts
	if len(postedAts) != 1 {
		t.Fatalf("expected one row, got %d", len(postedAts))
	}
	if postedAts[0].Valid {
		t.Errorf("expected postedAt to stay unset, got %v", postedAts[0].Time)
	}
}

func TestSaveFillIn_StoresAGivenPostedAt(t *testing.T) {
	h := newHarness(t, djinniStub())
	in := completeFillIn()
	posted := "2026-07-28T00:00:00Z"
	in.PostedAt = &posted

	if _, err := h.svc.SaveFillIn(context.Background(), in); err != nil {
		t.Fatalf("SaveFillIn: %v", err)
	}

	postedAts := h.repo.inserted[0].PostedAts
	if !postedAts[0].Valid || postedAts[0].Time.Format("2006-01-02") != "2026-07-28" {
		t.Errorf("expected the given date to be stored, got %+v", postedAts[0])
	}
}

func TestSaveFillIn_ExistingDedupeKeyIsADuplicate(t *testing.T) {
	h := newHarness(t, djinniStub())
	in := completeFillIn()
	key := ingest.DedupeKey(in.Company, in.Title, in.URL)
	h.repo.knownKeys[key] = uuidOf(77)

	result, err := h.svc.SaveFillIn(context.Background(), in)
	if err != nil {
		t.Fatalf("SaveFillIn: %v", err)
	}
	if result.Outcome != domain.OutcomeDuplicate {
		t.Fatalf("outcome = %q, want duplicate", result.Outcome)
	}
	if result.Job == nil {
		t.Error("expected the existing vacancy so the client can navigate to it")
	}
	if len(h.repo.inserted) != 0 {
		t.Error("expected no second copy")
	}
}

func TestSaveFillIn_SharesTheDedupeRuleWithExtraction(t *testing.T) {
	in := completeFillIn()
	extracted := dto.NormalizedJob{Company: in.Company, Title: in.Title, URL: in.URL}

	if ingest.DedupeKey(in.Company, in.Title, in.URL) !=
		ingest.DedupeKey(extracted.Company, extracted.Title, extracted.URL) {
		t.Fatal("a hand-filled vacancy must produce the same dedupe key as an extracted one")
	}
}
