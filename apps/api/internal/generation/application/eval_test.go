package application

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

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

		if elapsed := time.Since(started); elapsed > 60*time.Second {
			t.Errorf("the deterministic corpus took %s, budget is 60s. Parallelise cases rather than raising the budget.", elapsed)
		}
	})
}

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

func TestGateRunsWithNoCredentials(t *testing.T) {
	for _, key := range []string{
		"GROQ_API_KEY", "COHERE_API_KEY", "OPENROUTER_API_KEY",
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

func TestGateRunsWithNoRenderToolchain(t *testing.T) {

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
