package application

import (
	"context"
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
// The comparison is made on what the model returned, before and after the
// grounding pass, so it isolates that mechanism. Comparing a full production run
// against an unguarded merge would not: page fitting runs in between and applies
// the same drops itself, so the delta would confound the two.

// rawRun drives the same stages against the same fixtures as runCase, then
// merges **without** the grounding pass. Everything up to the merge is
// production's own code; the only thing this does differently is omit
// DropUngroundedSkillTokens and StripUngroundedHighlights, and stop before page
// fitting (which applies both again).
//
// Only the first grounding attempt is driven. In production the retry ladder
// feeds the previous attempt's violations back into the prompt; attempt 0 passes
// nil, so it is the one attempt whose request is identical to production's and
// therefore the one with a committed fixture.
func rawRun(t *testing.T, c EvalCase) evalRun {
	t.Helper()

	analyze := newReplayProvider(t, "generation-analyze", c.Name)
	sel := newReplayProvider(t, "generation-select", c.Name)
	premium := newReplayProvider(t, "generation-select-premium", c.Name)
	summary := newReplayProvider(t, "generation-summary", c.Name)
	cover := newReplayProvider(t, "generation", c.Name)

	svc := NewService(nil, nil, nil, nil, GenerationRouters{
		Analyze: analyze, Select: sel, Premium: premium, Summary: summary, Cover: cover,
	}, "", "", c.Spec.GroundingLevel, nil)

	run := evalRun{prov: &runProvenance{}}
	ctx := context.Background()

	analysis, err := analyzeVacancy(ctx, svc.llm.Analyze, svc.genModel, c.Vacancy, nil)
	if err != nil {
		t.Fatalf("%s: analyze stage: %v", c.Name, err)
	}
	payload, err := svc.selectWithCompleteness(ctx, c.Master, analysis, c.Level, nil, c.Cfg, nil, run.prov)
	if err != nil {
		t.Fatalf("%s: select stage: %v", c.Name, err)
	}
	sum, err := svc.summarize(ctx, c.Master, payload, analysis, c.Cfg, nil, run.prov, nil)
	if err != nil {
		t.Fatalf("%s: summary stage: %v", c.Name, err)
	}
	merged, err := domain.MergeTailored(c.Master, payload, sum)
	if err != nil {
		t.Fatalf("%s: merge: %v", c.Name, err)
	}
	// Toggles and hard limits are shape, not grounding, and production applies
	// them before either check runs — so they belong on both sides of the
	// comparison.
	domain.ApplySectionToggles(merged, c.Cfg)
	domain.ApplyHardLimits(c.Master, merged, c.Cfg)

	for _, p := range []*ReplayProvider{analyze, sel, premium, summary, cover} {
		run.misses += p.missCount()
	}
	run.merged, run.analysis = merged, analysis
	return run
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
// scores clean before the grounding pass as well as after. Given a posting with
// no stated requirements, the model reached for plausible-sounding skills the
// master does not list and rephrased a bullet past the overlap threshold. That
// is the genuine article, and it is why the assertion below is corpus-wide with
// one named case rather than per-case.
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
			raw := rawRun(t, c)
			if raw.misses > 0 {
				t.Fatalf("%s: %d replay misses — re-record before reading this result", c.Name, raw.misses)
			}

			before := scoreRun(c, raw)
			after := scoreRun(c, evalRun{
				merged:   applyGroundingPass(t, c.Master, raw.merged),
				analysis: raw.analysis,
			})

			for _, name := range scorerNames() {
				b, a := before[name], after[name]
				switch {
				case a.Value == b.Value:
					continue
				case betterThan(a, b):
					improved[c.Name] = append(improved[c.Name], name)
				default:
					// The grounding pass made a score worse. That is the
					// false-positive failure the `baseline` case exists to
					// catch, and it is a defect wherever it shows up.
					t.Errorf("%s: the grounding pass made %s worse: %v → %v (direction %s). "+
						"Grounding must remove ungrounded content, never damage grounded content",
						c.Name, name, b.Value, a.Value, a.Direction)
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
