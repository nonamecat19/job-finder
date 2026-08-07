package application

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/generation/domain"
)

// The evaluation corpus (038).
//
// The harness lives in package `application`, in _test.go files, and not in a
// subpackage — because every stage of the tailoring path is unexported
// (tailorRendercvResume, selectWithCompleteness, summarize, renderToPageTarget,
// renderDeps), and the two exported entry points require Postgres and write run
// rows. A subpackage could reach none of it. Test files keep the harness out of
// the API binary.
//
// Cases are **discovered**, never enumerated: adding a case is adding a
// directory, so a case cannot be added without the gate running it, and cannot
// be quietly dropped from a list.

const evalDataDir = "evaldata"

var (
	casesDir     = filepath.Join(evalDataDir, "cases")
	replaysDir   = filepath.Join(evalDataDir, "replays")
	baselinesDir = filepath.Join(evalDataDir, "baselines")
)

// maxReplayFixtures bounds the committed corpus.
//
// Set from the observed count plus headroom, not guessed. One case issues far
// more than one request once the retry ladders engage — analyze, up to three
// selection attempts, a summary and its re-prompt, a structure re-tailor, and
// any page-fit expand/condense rounds.
//
// A breach is a signal to shrink the corpus, not to raise this number. Fixtures
// are the part of a golden-set harness that rots: every one is a response some
// model gave once, and a corpus nobody can read is a corpus nobody maintains.
const maxReplayFixtures = 200

// CaseSpec is the on-disk case.yaml.
type CaseSpec struct {
	Name           string `yaml:"name"`
	Why            string `yaml:"why"`
	GroundingLevel string `yaml:"grounding_level"`
	Shape          struct {
		ExperienceBulletsMin *int `yaml:"experience_bullets_min"`
		ExperienceBulletsMax *int `yaml:"experience_bullets_max"`
		SkillsMaxGroups      *int `yaml:"skills_max_groups"`
		SummaryLines         *int `yaml:"summary_lines"`
	} `yaml:"shape"`
	PageCounts []int `yaml:"page_counts"`
}

// EvalCase is a loaded case.
type EvalCase struct {
	Name    string
	Dir     string
	Spec    CaseSpec
	Master  domain.RendercvMaster
	Vacancy string
	Level   domain.GroundingLevel
	Cfg     domain.ShapeConfig
}

// discoverCases walks the corpus directory. No case name appears in any Go file
// (FR-020, C5-1).
func discoverCases(t *testing.T, dir string) []EvalCase {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read corpus %s: %v", dir, err)
	}

	var cases []EvalCase
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cases = append(cases, loadCase(t, filepath.Join(dir, e.Name())))
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	return cases
}

func loadCase(t *testing.T, dir string) EvalCase {
	t.Helper()
	c := EvalCase{Name: filepath.Base(dir), Dir: dir}

	specRaw, err := os.ReadFile(filepath.Join(dir, "case.yaml"))
	if err != nil {
		t.Fatalf("case %s: %v", c.Name, err)
	}
	if err := yaml.Unmarshal(specRaw, &c.Spec); err != nil {
		t.Fatalf("case %s: case.yaml: %v", c.Name, err)
	}

	masterRaw, err := os.ReadFile(filepath.Join(dir, "master.yaml"))
	if err != nil {
		t.Fatalf("case %s: %v", c.Name, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(masterRaw, &m); err != nil {
		t.Fatalf("case %s: master.yaml: %v", c.Name, err)
	}
	c.Master = domain.RendercvMaster(domain.NormalizeYAMLMap(m).(map[string]any))

	vacancyRaw, err := os.ReadFile(filepath.Join(dir, "vacancy.txt"))
	if err != nil {
		t.Fatalf("case %s: %v", c.Name, err)
	}
	c.Vacancy = string(vacancyRaw)

	c.Level = domain.ParseGroundingLevel(c.Spec.GroundingLevel)
	c.Cfg = domain.DefaultShapeConfig()
	if v := c.Spec.Shape.ExperienceBulletsMin; v != nil {
		c.Cfg.ExperienceBulletsMin = *v
	}
	if v := c.Spec.Shape.ExperienceBulletsMax; v != nil {
		c.Cfg.ExperienceBulletsMax = *v
	}
	if v := c.Spec.Shape.SkillsMaxGroups; v != nil {
		c.Cfg.SkillsMaxGroups = *v
	}
	if v := c.Spec.Shape.SummaryLines; v != nil {
		c.Cfg.SummaryLines = *v
	}
	return c
}

// realIdentityMarkers are patterns whose presence in a fixture suggests a real
// person's data was committed. Every fixture must be synthetic (FR-019,
// SC-011): the convenient fixture is the developer's own résumé, and committing
// a real employment history and contact details to test a grounding checker is
// not a trade worth making — the checks are structural, so real data buys
// nothing.
var realIdentityMarkers = []*regexp.Regexp{
	// Contact details that would identify someone. The demo document's
	// rendercv.com addresses are explicitly allowed by the exceptions below.
	regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@(?:gmail|outlook|hotmail|yahoo|proton|icloud)\.[a-z]{2,}\b`),
	regexp.MustCompile(`\+\d{1,3}[\s-]?\(?\d{2,4}\)?[\s-]?\d{3}[\s-]?\d{2,4}`),
	regexp.MustCompile(`(?i)\blinkedin\.com/in/[a-z0-9-]{4,}`),
}

// openEndedDate is the pattern FR-030 forbids.
//
// DeriveTotalExperienceYears resolves `present` against time.Now().Year(), and
// production calls it at four sites in the tailoring path. An open-ended role
// therefore changes the derived experience figure — and so the summary prompt,
// and so the request hash — on 1 January, which would expire every replay
// fixture in the corpus overnight for no reason anybody would connect to the
// date.
var openEndedDate = regexp.MustCompile(`(?m)^\s*(?:end_date|date)\s*:\s*['"]?present['"]?\s*$`)

// TestCorpusDiscipline is the rule set every case must satisfy. It runs in the
// ordinary suite, so a malformed, non-synthetic or open-endedly dated case
// fails the build rather than being discovered when somebody next reads it.
func TestCorpusDiscipline(t *testing.T) {
	cases := discoverCases(t, casesDir)
	if len(cases) == 0 {
		t.Fatal("the corpus is empty; a gate over no cases gates nothing")
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			for _, f := range []string{"case.yaml", "master.yaml", "vacancy.txt"} {
				if _, err := os.Stat(filepath.Join(c.Dir, f)); err != nil {
					t.Errorf("missing %s", f)
				}
			}
			if strings.TrimSpace(c.Spec.Why) == "" {
				t.Error("case.yaml has no `why`; a case whose reason for existing is not written down becomes one nobody dares delete and nobody understands")
			}
			if c.Spec.Name != "" && c.Spec.Name != c.Name {
				t.Errorf("case.yaml name %q does not match its directory %q", c.Spec.Name, c.Name)
			}
			if len(c.Spec.PageCounts) == 0 {
				t.Error("case.yaml declares no `page_counts`; without it the page-fit loop has no deterministic answer and the case cannot run without a PDF toolchain")
			}
			for _, n := range c.Spec.PageCounts {
				if n <= 0 {
					t.Errorf("page_counts contains %d; a rendered document has at least one page", n)
				}
			}
			if strings.TrimSpace(c.Vacancy) == "" {
				t.Error("vacancy.txt is empty")
			}

			for _, f := range []string{"master.yaml", "vacancy.txt", "case.yaml"} {
				raw, err := os.ReadFile(filepath.Join(c.Dir, f))
				if err != nil {
					continue
				}
				text := string(raw)
				for _, re := range realIdentityMarkers {
					if m := re.FindString(text); m != "" {
						t.Errorf("%s contains what looks like real contact detail (%q); every fixture must be synthetic (FR-019)", f, m)
					}
				}
				if m := openEndedDate.FindString(text); m != "" {
					t.Errorf("%s contains an open-ended date (%q); a role ending in `present` makes the derived experience figure — and therefore every replay fixture for this case — change on 1 January (FR-030)",
						f, strings.TrimSpace(m))
				}
			}
		})
	}
}

// TestCaseWhyNamesAConcreteFailure guards against the `why` field decaying into
// a description of the case's contents. "A vacancy and a profile" is not a
// reason; "the silent selection truncation 035 found" is.
func TestCaseWhyNamesAConcreteFailure(t *testing.T) {
	for _, c := range discoverCases(t, casesDir) {
		t.Run(c.Name, func(t *testing.T) {
			why := strings.TrimSpace(c.Spec.Why)
			if len(why) < 60 {
				t.Errorf("`why` is %d characters; it must name a concrete failure mode, not describe the fixture", len(why))
			}
			// A concrete failure mode says what goes wrong. These are the verbs
			// and nouns that distinguish "this catches X" from "this is a case".
			concrete := []string{"fail", "wrong", "missing", "truncat", "fabricat", "drift", "silent",
				"contradict", "shortfall", "regress", "expire", "invalid", "empty", "break", "catch", "must not"}
			found := false
			for _, w := range concrete {
				if strings.Contains(strings.ToLower(why), w) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("`why` does not name a failure mode. It reads as a description of the fixture:\n%s", why)
			}
		})
	}
}

// TestReplayFixtureCountIsBounded keeps the corpus readable. See
// maxReplayFixtures for why a breach means shrinking the corpus.
func TestReplayFixtureCountIsBounded(t *testing.T) {
	count := 0
	err := filepath.WalkDir(replaysDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".json") {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", replaysDir, err)
	}
	if count > maxReplayFixtures {
		t.Errorf("%d replay fixtures, bound is %d. Shrink the corpus rather than raising the bound: "+
			"every fixture is a response some model gave once, and a corpus nobody can read is a corpus nobody maintains.",
			count, maxReplayFixtures)
	}
}

func caseReplayDir(name string) string { return filepath.Join(replaysDir, name) }

func caseBaselinePath(name string) string {
	return filepath.Join(baselinesDir, fmt.Sprintf("%s.json", name))
}
