package application

import (
	"context"
	"fmt"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

// 034 T012: the chosen option decides which provider writes the summary, and
// decides nothing else.
//
// The failure this guards against is the quiet one. An option that routes
// nowhere does not error — the request simply goes to whatever provider was
// already there — so a user who deliberately picked "Premium" gets the standard
// model and no part of the system says otherwise.

type optionFixture struct {
	svc      *Service
	standard *stageProvider
	premium  *stageProvider
	analyze  *stageProvider
	sel      *stageProvider
}

func summaryOptionFixture(t *testing.T) optionFixture {
	t.Helper()
	years := domain.DeriveTotalExperienceYears(stageMaster())
	reply := summaryReply(t, fmt.Sprintf("%d+ years of experience building Go services.", years))

	analyze := &stageProvider{name: "generation-analyze", reply: analysisReply(t)}
	sel := &stageProvider{name: "generation-select", reply: selectionReply(t)}
	standard := &stageProvider{name: "generation-summary", reply: reply}
	premium := &stageProvider{name: "generation-summary-premium", reply: reply}

	svc := &Service{llm: GenerationRouters{
		Analyze: analyze, Select: sel, Premium: sel,
		Summary: standard,
		SummaryByOption: map[string]llm.Provider{
			"premium": premium,
		},
		Cover: standard,
	}}
	return optionFixture{svc: svc, standard: standard, premium: premium, analyze: analyze, sel: sel}
}

// A service with no settings provider and no per-run choice must behave exactly
// as it did before 034 existed. This is spec AC3, and it is the property that
// lets the whole feature ship without re-baselining the 038 corpus.
func TestNoChoiceRoutesToTheStandardProvider(t *testing.T) {
	f := summaryOptionFixture(t)

	if _, _, err := f.svc.tailorRendercvResume(context.Background(), stageMaster(), "Go role",
		domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, &runProvenance{}); err != nil {
		t.Fatalf("tailorRendercvResume: %v", err)
	}

	if f.standard.calls != 1 {
		t.Errorf("standard summary provider called %d times, want 1", f.standard.calls)
	}
	if f.premium.calls != 0 {
		t.Errorf("premium summary provider called %d times with no choice made, want 0", f.premium.calls)
	}
}

func TestChosenOptionRoutesTheSummaryStageAndNothingElse(t *testing.T) {
	f := summaryOptionFixture(t)

	opt, ok := domain.LookupSummaryOption("premium")
	if !ok {
		t.Fatal("catalogue lost the premium option")
	}
	ctx := WithSummaryOption(context.Background(), opt)

	if _, _, err := f.svc.tailorRendercvResume(ctx, stageMaster(), "Go role",
		domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, &runProvenance{}); err != nil {
		t.Fatalf("tailorRendercvResume: %v", err)
	}

	if f.premium.calls != 1 {
		t.Errorf("premium summary provider called %d times, want 1 — the choice did not reach the stage", f.premium.calls)
	}
	if f.standard.calls != 0 {
		t.Errorf("standard summary provider called %d times after premium was chosen, want 0", f.standard.calls)
	}
	// The whole point of exposing only the summary is that nothing else moves.
	if f.analyze.calls != 1 || f.sel.calls != 1 {
		t.Errorf("choosing a summary option disturbed other stages: analyze=%d select=%d, want 1 each",
			f.analyze.calls, f.sel.calls)
	}
}

// An option in the catalogue with no router wired — added before its deployment
// exists, or a gateway-less install — must produce a resume, not an error. An
// option is a routing preference, and a preference that can fail a run is a
// liability (plan D5).
func TestAnUnwiredOptionFallsBackRatherThanFailing(t *testing.T) {
	f := summaryOptionFixture(t)

	opt, ok := domain.LookupSummaryOption("fast") // deliberately absent from SummaryByOption
	if !ok {
		t.Fatal("catalogue lost the fast option")
	}
	ctx := WithSummaryOption(context.Background(), opt)

	if _, _, err := f.svc.tailorRendercvResume(ctx, stageMaster(), "Go role",
		domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, &runProvenance{}); err != nil {
		t.Fatalf("an unwired option failed the run: %v", err)
	}
	if f.standard.calls != 1 {
		t.Errorf("standard provider called %d times, want 1 — the fallback did not engage", f.standard.calls)
	}
}

// 044 deleted the `local` self-hosted option, but the id outlives the release:
// it sits in Document and GenerationRun rows and in the summary-model setting
// of every install that ever picked it (data-model.md §5). Those rows are read
// back on a rerun, so the id has to degrade to the default rather than fail a
// run. The miss path already does this; this pins it, because the next person
// to read LookupSummaryOption's second return value could reasonably decide a
// miss deserves an error.
func TestAPersistedLocalOptionIDResolvesToTheDefault(t *testing.T) {
	opt, ok := domain.LookupSummaryOption("local")
	if ok {
		t.Fatal(`the catalogue still offers "local"; 044 removed the self-hosted option`)
	}
	if opt.ID != domain.SummaryOptionStandard {
		t.Fatalf(`stored "local" resolved to %q, want the default %q`, opt.ID, domain.SummaryOptionStandard)
	}

	f := summaryOptionFixture(t)
	prov := &runProvenance{}
	if _, _, err := f.svc.tailorRendercvResume(WithSummaryOption(context.Background(), opt),
		stageMaster(), "Go role", domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, prov); err != nil {
		t.Fatalf(`a run carrying the retired "local" option failed: %v`, err)
	}
	if f.standard.calls != 1 {
		t.Errorf("standard provider called %d times, want 1", f.standard.calls)
	}
	if prov.summaryOption != domain.SummaryOptionStandard {
		t.Errorf("run recorded summary option %q, want %q — the retired id must not be written back",
			prov.summaryOption, domain.SummaryOptionStandard)
	}
}

type stubSummaryModelProvider struct{ opt domain.SummaryOption }

func (s stubSummaryModelProvider) SummaryOption(context.Context) domain.SummaryOption { return s.opt }

// The stored default applies when the request carries no choice; a per-run
// choice overrides it. Both directions matter: the first is spec AC4, the
// second is AC2.
func TestStoredDefaultAppliesAndAPerRunChoiceOverridesIt(t *testing.T) {
	premiumOpt, _ := domain.LookupSummaryOption("premium")
	standardOpt, _ := domain.LookupSummaryOption(domain.SummaryOptionStandard)

	t.Run("stored default is used", func(t *testing.T) {
		f := summaryOptionFixture(t)
		f.svc.SetSummaryModelProvider(stubSummaryModelProvider{opt: premiumOpt})

		if _, _, err := f.svc.tailorRendercvResume(context.Background(), stageMaster(), "Go role",
			domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, &runProvenance{}); err != nil {
			t.Fatal(err)
		}
		if f.premium.calls != 1 {
			t.Errorf("stored default did not reach the stage: premium calls = %d", f.premium.calls)
		}
	})

	t.Run("per-run choice wins", func(t *testing.T) {
		f := summaryOptionFixture(t)
		f.svc.SetSummaryModelProvider(stubSummaryModelProvider{opt: premiumOpt})
		ctx := WithSummaryOption(context.Background(), standardOpt)

		if _, _, err := f.svc.tailorRendercvResume(ctx, stageMaster(), "Go role",
			domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, &runProvenance{}); err != nil {
			t.Fatal(err)
		}
		if f.standard.calls != 1 || f.premium.calls != 0 {
			t.Errorf("per-run choice lost to the stored default: standard=%d premium=%d",
				f.standard.calls, f.premium.calls)
		}
	})
}

// The result surface has to say which option produced the resume (spec AC2),
// and the served model cannot answer that: two options can land on the same
// upstream after a fallback.
func TestTheRunRecordsWhichOptionWroteTheSummary(t *testing.T) {
	f := summaryOptionFixture(t)
	opt, _ := domain.LookupSummaryOption("premium")
	prov := &runProvenance{}

	if _, _, err := f.svc.tailorRendercvResume(WithSummaryOption(context.Background(), opt),
		stageMaster(), "Go role", domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, prov); err != nil {
		t.Fatal(err)
	}

	if prov.summaryOption != "premium" {
		t.Fatalf("run recorded summary option %q, want premium", prov.summaryOption)
	}
}
