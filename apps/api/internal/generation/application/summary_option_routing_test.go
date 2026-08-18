package application

import (
	"context"
	"fmt"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

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

	if f.analyze.calls != 1 || f.sel.calls != 1 {
		t.Errorf("choosing a summary option disturbed other stages: analyze=%d select=%d, want 1 each",
			f.analyze.calls, f.sel.calls)
	}
}

func TestAnUnwiredOptionFallsBackRatherThanFailing(t *testing.T) {
	f := summaryOptionFixture(t)

	opt, ok := domain.LookupSummaryOption("fast")
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
