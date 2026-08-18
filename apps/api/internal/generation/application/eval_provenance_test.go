package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

func evalStages(t *testing.T, c EvalCase) (domain.TailoredSelection, *domain.TailoredSummary, domain.VacancyAnalysis, int) {
	t.Helper()

	analyze := newReplayProvider(t, "generation-analyze", c.Name)
	sel := newReplayProvider(t, "generation-select", c.Name)
	premium := newReplayProvider(t, "generation-select-premium", c.Name)
	summary := newReplayProvider(t, "generation-summary", c.Name)
	cover := newReplayProvider(t, "generation", c.Name)

	svc := NewService(nil, nil, nil, nil, GenerationRouters{
		Analyze: analyze, Select: sel, Premium: premium, Summary: summary, Cover: cover,
	}, "", "", c.Spec.GroundingLevel, nil)

	ctx := context.Background()
	prov := &runProvenance{}

	analysis, err := analyzeVacancy(ctx, svc.llm.Analyze, svc.genModel, c.Vacancy, nil)
	if err != nil {
		t.Fatalf("%s: analyze stage: %v", c.Name, err)
	}
	payload, err := svc.selectWithCompleteness(ctx, c.Master, analysis, c.Level, nil, c.Cfg, nil, prov)
	if err != nil {
		t.Fatalf("%s: select stage: %v", c.Name, err)
	}
	sum, err := svc.summarize(ctx, c.Master, payload, analysis, c.Level, c.Cfg, nil, prov, nil)
	if err != nil {
		t.Fatalf("%s: summary stage: %v", c.Name, err)
	}

	misses := 0
	for _, p := range []*ReplayProvider{analyze, sel, premium, summary, cover} {
		misses += p.missCount()
	}
	return payload, sum, analysis, misses
}

func recordedSelections(t *testing.T, c EvalCase) []domain.TailoredSelection {
	t.Helper()
	entries, err := os.ReadDir(caseReplayDir(c.Name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read replays for %s: %v", c.Name, err)
	}
	var out []domain.TailoredSelection
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(caseReplayDir(c.Name), e.Name()))
		if err != nil {
			t.Fatalf("read fixture %s: %v", e.Name(), err)
		}
		var f ReplayFixture
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("parse fixture %s: %v", e.Name(), err)
		}
		if f.RequestSummary.ModelKey != "generation-select" {
			continue
		}
		var sel domain.TailoredSelection

		if err := json.Unmarshal([]byte(f.Response), &sel); err != nil {
			continue
		}
		out = append(out, sel)
	}
	return out
}

func evalNorm(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func unguardedMerge(t *testing.T, master domain.RendercvMaster, payload domain.TailoredSelection, summary *domain.TailoredSummary) domain.RendercvMaster {
	t.Helper()
	merged, err := domain.DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	sections := domain.CvSections(merged)
	if sections == nil {
		return merged
	}
	if summary != nil {
		sections["summary"] = []any{strings.TrimSpace(summary.Summary)}
	}

	apply := func(entries []map[string]any, key string, name string, refs []domain.HighlightRef) {
		for _, e := range entries {
			if evalNorm(domain.StringField(e, key)) != evalNorm(name) {
				continue
			}
			sources := domain.StringSliceField(e, "highlights")
			out := make([]any, 0, len(refs))
			for _, ref := range refs {
				text := strings.TrimSpace(ref.Rephrased)
				if text == "" {
					if ref.SourceIndex < 0 || ref.SourceIndex >= len(sources) {
						continue
					}
					text = strings.TrimSpace(sources[ref.SourceIndex])
				}
				if text != "" {
					out = append(out, text)
				}
			}
			e["highlights"] = out
		}
	}
	for _, pe := range payload.Experience {
		apply(domain.AsSliceOfMaps(sections["experience"]), "company", pe.Company, pe.Highlights)
	}
	for _, pp := range payload.Projects {
		apply(domain.AsSliceOfMaps(sections["projects"]), "name", pp.Name, pp.Highlights)
	}
	return merged
}

func applyShape(doc domain.RendercvMaster, master domain.RendercvMaster, analysis domain.VacancyAnalysis, c EvalCase) {
	domain.ApplySectionToggles(doc, c.Cfg)
	domain.RankSkills(doc, analysis, c.Cfg)
	domain.ApplyHardLimits(master, doc, c.Cfg)
}

func applyGroundingPass(t *testing.T, master, doc domain.RendercvMaster) domain.RendercvMaster {
	t.Helper()
	guarded, err := domain.DeepCloneYAML(doc)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	domain.DropUngroundedSkillTokens(master, guarded)
	return domain.StripUngroundedHighlights(master, guarded)
}

const provenanceCaseWithRecordedDefect = "vague-vacancy"

func TestGroundingPassRemovesARecordedModelDefect(t *testing.T) {
	cases := discoverCases(t, casesDir)
	if len(cases) == 0 {
		t.Fatal("no cases discovered")
	}

	improved := map[string][]string{}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			payload, sum, analysis, misses := evalStages(t, c)
			if misses > 0 {
				t.Fatalf("%s: %d replay misses — re-record before reading this result", c.Name, misses)
			}

			for i, sel := range append([]domain.TailoredSelection{payload}, recordedSelections(t, c)...) {
				unguarded := unguardedMerge(t, c.Master, sel, sum)
				applyShape(unguarded, c.Master, analysis, c)

				guarded, err := domain.MergeTailored(c.Master, sel, sum, c.Level)
				if err != nil {
					t.Fatalf("%s: merge: %v", c.Name, err)
				}
				applyShape(guarded, c.Master, analysis, c)

				before := scoreRun(c, evalRun{merged: unguarded, analysis: analysis})
				after := scoreRun(c, evalRun{
					merged:   applyGroundingPass(t, c.Master, guarded),
					analysis: analysis,
				})

				for _, name := range scorerNames() {
					b, a := before[name], after[name]
					switch {
					case a.Value == b.Value:
						continue
					case betterThan(a, b):
						improved[c.Name] = append(improved[c.Name], name)
					default:

						t.Errorf("%s (selection %d): grounding made %s worse: %v → %v (direction %s). "+
							"Grounding must remove ungrounded content, never damage grounded content",
							c.Name, i, name, b.Value, a.Value, a.Direction)
					}
				}
			}
		})
	}

	if t.Failed() {
		return
	}

	got := improved[provenanceCaseWithRecordedDefect]
	if len(got) == 0 {
		t.Fatalf("case %q no longer carries a recorded model defect that the grounding pass removes.\n"+
			"Its fixtures were re-recorded against a model that did not misbehave, so the corpus now "+
			"proves only that the scorers work on injected defects (T015) and no longer that it holds "+
			"a real one (SC-007).\n"+
			"Fix by pointing provenanceCaseWithRecordedDefect at a case that does, or by re-recording "+
			"this one — do not delete this assertion.\n"+
			"Cases that improved this run: %v", provenanceCaseWithRecordedDefect, improved)
	}
	t.Logf("recorded model defect in %q removed by the grounding pass, on scorers: %v",
		provenanceCaseWithRecordedDefect, got)
}

func betterThan(a, b Score) bool {
	if a.Direction == HigherIsBetter {
		return a.Value > b.Value
	}
	return a.Value < b.Value
}
