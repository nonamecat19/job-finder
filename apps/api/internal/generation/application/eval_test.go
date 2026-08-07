package application

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The gate (038 FR-005, contracts C4-1/C4-2).
//
// TestEvalCorpus runs in the ordinary suite: no build tag, no environment
// variable, no credentials, no network, no PDF toolchain, no database. Every
// one of those is a reason a gate gets skipped, and a gate that is skipped is
// not a gate.
//
// What it does: run every discovered case against replayed responses, score the
// result with production's own checks, and compare to a recorded baseline. A
// regression fails the build. So does an improvement, and so does a case with
// no baseline — see the comparator for why neither is a pass.

func TestEvalCorpus(t *testing.T) {
	started := time.Now()
	cases := discoverCases(t, casesDir)
	if len(cases) == 0 {
		t.Fatal("the corpus is empty; a gate over no cases gates nothing")
	}

	updateCase, reason, updating := updateRequested(t)

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			run := runCase(t, c)
			if run.err != nil {
				if run.misses > 0 {
					t.Fatalf("case %q could not run: %d replay miss(es). The requests changed, so the recorded "+
						"responses no longer describe them — re-record rather than editing a fixture.\n%v", c.Name, run.misses, run.err)
				}
				t.Fatalf("case %q failed in the pipeline: %v", c.Name, run.err)
			}

			scores := scoreRun(c, run)

			if updating && c.Name == updateCase {
				if err := saveBaseline(newBaseline(c.Name, reason, scores)); err != nil {
					t.Fatalf("update baseline: %v", err)
				}
				t.Logf("baseline for %q rewritten: %s", c.Name, reason)
				return
			}

			base, has, err := loadBaseline(c.Name)
			if err != nil {
				t.Fatalf("load baseline: %v", err)
			}
			cmp := compareToBaseline(c.Name, scores, base, has)
			for _, note := range cmp.CoMoving {
				t.Log(note)
			}
			if cmp.Outcome != OutcomeMatch {
				t.Errorf("%s\n%s", cmp.Outcome, strings.Join(cmp.Messages, "\n"))
			}
		})
	}

	t.Cleanup(func() {
		// SC-002: the whole deterministic run under 60 seconds. If this ever
		// fails, parallelise cases rather than raising the budget — a slow gate
		// is a skipped gate, and the fix for slow is not a bigger number.
		if elapsed := time.Since(started); elapsed > 60*time.Second {
			t.Errorf("the deterministic corpus took %s, budget is 60s. Parallelise cases rather than raising the budget.", elapsed)
		}
	})
}

// FR-005 / C4-1: the gate must need no build tag and no environment variable.
//
// Asserted by construction — this file has no build constraint — and checked
// here so a later edit that adds one fails rather than silently removing the
// gate from every ordinary run.
func TestGateRunsWithNoTagAndNoEnvVar(t *testing.T) {
	raw, err := os.ReadFile("eval_test.go")
	if err != nil {
		t.Fatalf("read own source: %v", err)
	}
	first := strings.SplitN(string(raw), "\n", 2)[0]
	if strings.HasPrefix(strings.TrimSpace(first), "//go:build") {
		t.Errorf("eval_test.go has a build constraint (%q); the gate would then not run in the ordinary suite", first)
	}
	for _, name := range []string{"EVAL", "EVAL_LIVE", "EVAL_ENABLED", "GENERATION_BENCHMARK"} {
		if strings.Contains(string(raw), "os.Getenv(\""+name+"\")") {
			t.Errorf("the gate consults %s; an opt-in environment variable is a gate that is off by default", name)
		}
	}
}

// FR-004 / SC-002 / C4-4: no credentials, no network.
//
// The ReplayProvider is the whole mechanism — it holds no HTTP client and has
// no live fallback compiled into this binary. This test unsets every provider
// credential and runs a case, so a future change that reintroduces a live path
// fails here rather than in CI on a runner without keys.
func TestGateRunsWithNoCredentials(t *testing.T) {
	for _, key := range []string{
		"CEREBRAS_API_KEY", "GROQ_API_KEY", "COHERE_API_KEY", "OPENROUTER_API_KEY",
		"GATEWAY_URL", "LITELLM_MASTER_KEY", "OLLAMA_URL", "OLLAMA_KEY",
	} {
		t.Setenv(key, "")
	}
	cases := discoverCases(t, casesDir)
	if len(cases) == 0 {
		t.Skip("no cases")
	}
	run := runCase(t, cases[0])
	if run.misses > 0 {
		t.Fatalf("case %q had %d replay miss(es) with credentials unset", cases[0].Name, run.misses)
	}
	if run.err != nil {
		t.Fatalf("case %q failed with credentials unset: %v", cases[0].Name, run.err)
	}
}

// FR-028 / SC-002 / C4-4a: no PDF toolchain.
//
// This is the risk nothing previously measured. The 60-second budget was never
// in danger — a render is under a second — but a CI runner without Python and
// Typst could not have run the gate at all. The stubbed render and countPages
// are what remove that dependency; this test confirms nothing reintroduces it.
func TestGateRunsWithNoRenderToolchain(t *testing.T) {
	// A PATH with nothing on it: rendercv, python and typst are all
	// unreachable for the duration of this test.
	t.Setenv("PATH", t.TempDir())
	for _, bin := range []string{"rendercv", "python", "python3", "typst"} {
		if path, err := exec.LookPath(bin); err == nil {
			t.Fatalf("%s is still resolvable at %s; this test cannot prove the gate runs without it", bin, path)
		}
	}

	cases := discoverCases(t, casesDir)
	if len(cases) == 0 {
		t.Skip("no cases")
	}
	run := runCase(t, cases[0])
	if run.err != nil {
		t.Fatalf("case %q failed with no render toolchain on PATH: %v", cases[0].Name, run.err)
	}
}
