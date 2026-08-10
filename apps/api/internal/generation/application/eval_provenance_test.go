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

// Corpus provenance (038 T037, SC-007).
//
// T037 asked for each case to be run against the code as it was when its
// failure occurred, proving the case would have caught its own bug. That is not
// doable as written and the reason is structural, not incidental: replay
// fixtures are keyed by a hash over the request, and older code builds
// different prompts. Checking out the commit a failure happened at yields
// replay misses, not scorer movements. The case would "fail", but for the wrong
// reason — which proves nothing about whether it catches its bug. Making it
// work would need fixtures recorded per historical commit, a corpus this design
// deliberately does not carry.
//
// What is done instead is narrower and, for the property SC-007 actually cares
// about, stronger. TestScorersDetectInjectedDefects (T015) proves the scorers
// move on defects *the test injected*. The test below proves the corpus holds a
// defect **the model genuinely emitted**, recorded in a committed fixture, that
// production's grounding pass removes. An injected defect proves the scorer
// works; a recorded one proves the corpus is still worth running.
//
// The comparison is made on what the model returned, before and after
// production's grounding defenses, so it isolates that mechanism. Comparing a
// full production run against an unguarded merge would not: page fitting runs in
// between and applies the same drops itself, so the delta would confound the two.
//
// Where the boundary sits moved once, and the move is the whole reason this
// comment is long. It used to sit after MergeTailored: the model returned bullet
// text, the merge copied it in, and DropUngroundedSkillTokens and
// StripUngroundedHighlights cleaned up afterwards — so "unguarded" meant "skip
// those two calls". Selection now returns references ({sourceIndex, rephrased})
// rather than prose, and ResolveHighlights repairs inside the merge: an index
// nothing points at is dropped, a repeat is dropped, and a rewording that drifts
// from the bullet it names is replaced by the original. The defect never reaches
// the post-merge pass, so measuring across that pass alone measures nothing, and
// this test failed with "no longer carries a recorded model defect" against a
// fixture that plainly still carries one.
//
// So the unguarded side is now built by unguardedMerge below, which takes the
// model at its word — every rewording verbatim, every repeated index kept — and
// the guarded side is production's own merge followed by the post-merge pass.
// The delta is what the two defenses remove together, which is what SC-007 is
// about: not which function does the removing, but that the corpus still holds
// something real to remove.

// evalStages drives the analyze/select/summary stages against a case's committed
// fixtures and returns what they produced, so the guarded and unguarded docs are
// built from one set of model responses rather than two replays of them.
//
// Only the first grounding attempt is driven. In production the retry ladder
// feeds the previous attempt's violations back into the prompt; attempt 0 passes
// nil, so it is the one attempt whose request is identical to production's and
// therefore the one with a committed fixture.
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

// recordedSelections returns every committed select-model response for a case
// that parses as a selection, in filename order.
//
// It reads the fixtures directly rather than driving the stages that would
// request them, because some of them are only requested under conditions the
// deterministic harness cannot reproduce here — the expand request exists
// because a rendered PDF came up short, and rendering is stubbed. The response
// is a recorded model output either way, which is all this test needs from it:
// what the model said, to be merged twice and compared.
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
		// A response that does not parse as a selection is not a defect this
		// test can speak to — the stage that requested it reports that itself.
		if err := json.Unmarshal([]byte(f.Response), &sel); err != nil {
			continue
		}
		out = append(out, sel)
	}
	return out
}

// evalNorm matches domain's own company matching, which is unexported. Company
// names are matched the same way on both sides of the comparison or the
// unguarded doc would differ from the guarded one for a reason that has nothing
// to do with grounding.
func evalNorm(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// unguardedMerge is MergeTailored with every repair removed: each reference
// contributes its rewording verbatim when it carries one, and a repeated
// sourceIndex contributes a second time instead of being dropped. It is
// deliberately not a call into domain with a flag — a "trust the model" mode has
// no business existing in production code, and a flag that only tests pass is a
// flag production can pass by mistake.
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

// applyShape applies what production applies between the merge and the grounding
// pass. Toggles, ranking and hard limits are shape, not grounding, so they
// belong on both sides of the comparison.
func applyShape(doc domain.RendercvMaster, master domain.RendercvMaster, analysis domain.VacancyAnalysis, c EvalCase) {
	domain.ApplySectionToggles(doc, c.Cfg)
	domain.RankSkills(doc, analysis, c.Cfg)
	domain.ApplyHardLimits(master, doc, c.Cfg)
}

// applyGroundingPass is the production grounding post-processing, and nothing
// else, applied to a copy.
func applyGroundingPass(t *testing.T, master, doc domain.RendercvMaster) domain.RendercvMaster {
	t.Helper()
	guarded, err := domain.DeepCloneYAML(doc)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	domain.DropUngroundedSkillTokens(master, guarded)
	return domain.StripUngroundedHighlights(master, guarded)
}

// The case that currently carries a recorded model defect. Named rather than
// discovered, so that a re-record which cleans it up fails this test loudly
// instead of quietly leaving the corpus with nothing real in it.
//
// It is `vague-vacancy` and not, as one might guess, `absent-skills`: asked for
// skills the candidate lacks, the model declined to invent them, so that case
// scores clean unguarded as well as guarded. Given a posting with no stated
// requirements, the model reworded every bullet it selected into
// achievement-speak the master never wrote and pointed three separate
// references at the same source bullet. That is the genuine article, and it is
// why the assertion below is corpus-wide with one named case rather than
// per-case.
const provenanceCaseWithRecordedDefect = "vague-vacancy"

// TestGroundingPassRemovesARecordedModelDefect is the T037 substitute: proof
// that the corpus holds a real defect and that production removes it.
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

			// Every selection the case recorded, not only the primary one.
			// Page fitting asks the select model for more bullets when a
			// document comes up short, and that request is where this corpus's
			// surviving defect actually lives: given a one-page resume to fill,
			// the model pointed three references at the same master bullet.
			// Scoring only the primary selection would have reported a clean
			// corpus while a committed fixture held that.
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
						// Grounding made a score worse. That is the
						// false-positive failure the `baseline` case exists to
						// catch, and it is a defect wherever it shows up.
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

// betterThan reports whether a is a better score than b, per the scorer's
// declared direction. Both scores must be the same scorer.
func betterThan(a, b Score) bool {
	if a.Direction == HigherIsBetter {
		return a.Value > b.Value
	}
	return a.Value < b.Value
}
