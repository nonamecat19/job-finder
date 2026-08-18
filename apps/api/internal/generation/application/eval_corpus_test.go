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

const evalDataDir = "evaldata"

var (
	casesDir     = filepath.Join(evalDataDir, "cases")
	replaysDir   = filepath.Join(evalDataDir, "replays")
	baselinesDir = filepath.Join(evalDataDir, "baselines")
)

const maxReplayFixtures = 200

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

type EvalCase struct {
	Name    string
	Dir     string
	Spec    CaseSpec
	Master  domain.RendercvMaster
	Vacancy string
	Level   domain.GroundingLevel
	Cfg     domain.ShapeConfig
}

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

var realIdentityMarkers = []*regexp.Regexp{

	regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@(?:gmail|outlook|hotmail|yahoo|proton|icloud)\.[a-z]{2,}\b`),
	regexp.MustCompile(`\+\d{1,3}[\s-]?\(?\d{2,4}\)?[\s-]?\d{3}[\s-]?\d{2,4}`),
	regexp.MustCompile(`(?i)\blinkedin\.com/in/[a-z0-9-]{4,}`),
}

var openEndedDate = regexp.MustCompile(`(?m)^\s*(?:end_date|date)\s*:\s*['"]?present['"]?\s*$`)

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

func TestCaseWhyNamesAConcreteFailure(t *testing.T) {
	for _, c := range discoverCases(t, casesDir) {
		t.Run(c.Name, func(t *testing.T) {
			why := strings.TrimSpace(c.Spec.Why)
			if len(why) < 60 {
				t.Errorf("`why` is %d characters; it must name a concrete failure mode, not describe the fixture", len(why))
			}

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
