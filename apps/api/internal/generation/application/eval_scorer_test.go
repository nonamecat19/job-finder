package application

import (
	"sort"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

// The scorers (038 FR-002, data-model §3).
//
// Not one of these is new logic, and that is the whole point. Every scorer
// delegates to a check production already enforces, so a green harness means
// something about production rather than about the harness. A scorer that
// invented its own rule would measure itself — and TestScorerDelegationIsExact
// makes that detectable rather than merely forbidden.
//
// There is no LLM judge anywhere here (FR-003). Every value below is
// deterministic Go over the merged document.

// ScorerSetVersion is bumped whenever any scorer's definition changes,
// **including** when an underlying domain check changes behaviour.
//
// A baseline recorded under a different version is not comparable: the delta
// would be measured across two instruments and read as a quality change. The
// comparator refuses rather than subtracting.
const ScorerSetVersion = 4

// Direction says which way is worse, so the comparator never has to infer
// whether 3 → 5 is good.
type Direction string

const (
	LowerIsBetter  Direction = "lower_is_better"
	HigherIsBetter Direction = "higher_is_better"
)

// Score is one measurement.
type Score struct {
	Name      string    `json:"name"`
	Value     float64   `json:"value"`
	Direction Direction `json:"direction"`
}

// scoreInput is everything a scorer may look at. cfg and level are here because
// VerifyRendercvGrounding and VerifyCompleteness require them — a scorer that
// guessed either would be measuring a configuration the run never used.
type scoreInput struct {
	master   domain.RendercvMaster
	result   domain.RendercvMaster
	analysis domain.VacancyAnalysis
	cfg      domain.ShapeConfig
	level    domain.GroundingLevel
	runErr   error
	// report is one VerifyCompleteness per run, shared by the three scorers
	// that read it so the verifier runs once.
	report domain.CompletenessReport
	// suggestions is the suggestion stage's raw response for this run (T064,
	// 042) — read before SuppressDuplicateSuggestions acts on it, for the
	// same reason `ranking` is read before VerifyRanking does.
	suggestions domain.SuggestionSet
	// ranking is the ranking stage's raw response for this run (T045, 042).
	// Unlike every other field here it is not read off `result` — a rejected
	// ranking never reaches the document, only the fallback does — so this is
	// the one place the corpus can see the response BEFORE VerifyRanking (and
	// StartRun's retry/fallback) act on it.
	ranking domain.RankedSelection
}

// Scorer is one measurement over a run.
type Scorer struct {
	Name      string
	Direction Direction
	Score     func(in scoreInput) float64
}

// scorers is the whole set. Each row evaluates exactly one expression against
// exactly one production check.
//
// Two measures specified for this feature are deliberately absent:
// `json_parse_failures` (CompleteStructured retries up to three times and
// discards the attempt count) and `empty_output` (no zero-content check exists
// in internal/generation/domain). Both would have to be written here, which
// would make the harness the author of a quality rule it then grades — the
// inversion FR-002 exists to prevent. They return as scorers once the
// instrumentation lands in production on its own justification.
var scorers = []Scorer{
	{
		Name:      "grounding_violations",
		Direction: LowerIsBetter,
		Score: func(in scoreInput) float64 {
			return float64(len(domain.VerifyRendercvGrounding(in.master, in.result, in.level, in.analysis)))
		},
	},
	{
		Name:      "structural_violations",
		Direction: LowerIsBetter,
		Score: func(in scoreInput) float64 {
			// VerifyStructureIntegrity, in rendercv_structure.go — NOT
			// rendercv_shape.go, which holds ApplyHardLimits: a mutator that
			// trims a document to fit configured limits and verifies nothing. A
			// scorer wired there would have measured trimming and been read as
			// measuring structural integrity.
			return float64(len(domain.VerifyStructureIntegrity(in.master, in.result)))
		},
	},
	{
		Name:      "highlight_drift",
		Direction: LowerIsBetter,
		Score: func(in scoreInput) float64 {
			return float64(len(domain.VerifyHighlightGrounding(in.master, in.result)))
		},
	},
	{
		Name:      "duplicate_provenance",
		Direction: LowerIsBetter,
		Score: func(in scoreInput) float64 {
			// VerifyHighlightProvenance, in rendercv_structure.go, wired into
			// the grounding ladder in service.go. It is the only check that
			// sees one master bullet rendered as two accomplishments — every
			// other check reads each half on its own, and each half is
			// grounded.
			return float64(len(domain.VerifyHighlightProvenance(in.master, in.result)))
		},
	},
	{
		Name:      "required_skills_missing",
		Direction: LowerIsBetter,
		Score: func(in scoreInput) float64 {
			// A count, not the retention ratio the spec first wanted. The
			// denominator — the required-skill token set — is computed inside
			// VerifyCompleteness and never escapes it, and recomputing it here
			// would mean re-implementing production's skill matching: a
			// parallel definition of quality, which is what FR-002 forbids.
			// Weaker (one missing of two scores the same as one of twenty) and
			// honest.
			return float64(len(in.report.RequiredMissing))
		},
	},
	{
		Name:      "nice_to_have_retention",
		Direction: HigherIsBetter,
		Score: func(in scoreInput) float64 {
			return in.report.NiceToHaveRetained
		},
	},
	{
		Name:      "bullet_shortfalls",
		Direction: LowerIsBetter,
		Score: func(in scoreInput) float64 {
			// LOWER is better, and this was specified as higher. BulletShortfalls
			// is populated only for companies that fell BELOW
			// cfg.ExperienceBulletsMin, and the map value is the deficient
			// count — not an achievement count. As originally declared, the gate
			// would have failed on improvement and passed on regression.
			return float64(len(in.report.BulletShortfalls))
		},
	},
	{
		Name:      "ranking_violations",
		Direction: LowerIsBetter,
		Score: func(in scoreInput) float64 {
			return rankingViolationsTotal(in.master, in.cfg, in.ranking)
		},
	},
	{
		Name:      "suggestion_duplicates",
		Direction: LowerIsBetter,
		Score: func(in scoreInput) float64 {
			return suggestionDuplicatesTotal(in.master, in.suggestions)
		},
	},
}

// suggestionDuplicatesTotal counts the suggested bullets and skills research.md
// R6's suppression had to remove: what the suggestion stage returned, minus
// what domain.SuppressDuplicateSuggestions let through (042 T064).
//
// Measured on the raw response rather than on the survivors, because the
// survivors carry no duplicates by construction — SuppressDuplicateSuggestions
// is what makes that true, so scoring them would be scoring a tautology. What
// can regress is the prompt: research.md R4 withholds the master's bullet text
// and skill tokens from the suggestion stage precisely so the model cannot
// duplicate them, and this is how much that withholding failed to prevent.
func suggestionDuplicatesTotal(master domain.RendercvMaster, suggestions domain.SuggestionSet) float64 {
	return float64(suggestionItemCount(suggestions) - suggestionItemCount(domain.SuppressDuplicateSuggestions(suggestions, master)))
}

func suggestionItemCount(s domain.SuggestionSet) int {
	n := len(s.Skills)
	for _, e := range s.Experience {
		n += len(e.Bullets)
	}
	return n
}

// rankingViolationsTotal sums domain.VerifyRanking's violations across every
// master experience entry, matching a RankedSelection response's per-entry
// ranking by company name (042 T045). The company match is the only new
// logic here — everything that decides whether a ranking is valid is
// domain.VerifyRanking itself, called once per entry exactly as
// rankExperienceSections (workspace.go) calls it in production.
func rankingViolationsTotal(master domain.RendercvMaster, cfg domain.ShapeConfig, sel domain.RankedSelection) float64 {
	total := 0
	for _, e := range domain.AsSliceOfMaps(domain.CvSections(master)["experience"]) {
		company := domain.StringField(e, "company")
		available := len(domain.StringSliceField(e, "highlights"))
		// Containment, not equality — matching rankExperienceSections'
		// findRanking (workspace.go): a model that echoes the whole "company
		// (position) | location" header line back still names the right
		// entry.
		target := strings.ToLower(strings.TrimSpace(company))
		var ranking []int
		for _, re := range sel.Experience {
			if strings.Contains(strings.ToLower(strings.TrimSpace(re.Company)), target) {
				ranking = re.Ranking
				break
			}
		}
		total += len(domain.VerifyRanking(available, cfg.ExperienceBulletsMin, ranking))
	}
	return float64(total)
}

// overlappingScorers declares pairs that move on the same defect (FR-027).
//
// VerifyRendercvGrounding performs the highlight-drift comparison inline — the
// same lcsCovered check against the same per-company master bullets — so one
// ungrounded highlight raises both. They stay separate because the *difference*
// is informative: grounding up with drift flat means a fabricated company,
// section or project, which is a different defect with a different fix.
//
// The comparator reports a co-moving pair as one defect seen twice, and never
// sums scores into a total.
var overlappingScorers = [][2]string{
	{"grounding_violations", "highlight_drift"},
}

func scoreAll(in scoreInput) map[string]Score {
	in.report = domain.VerifyCompleteness(in.master, in.result, in.analysis, in.cfg)
	out := make(map[string]Score, len(scorers))
	for _, s := range scorers {
		out[s.Name] = Score{Name: s.Name, Value: s.Score(in), Direction: s.Direction}
	}
	return out
}

func scorerNames() []string {
	names := make([]string, 0, len(scorers))
	for _, s := range scorers {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}

// --- the scorer set's own guardrails ----------------------------------------

// TestScorerSetVersionMatchesTheSet fails when a scorer is added, removed or
// renamed without bumping ScorerSetVersion.
//
// The list below is the set as of the current version. Changing the set without
// changing the version would let a baseline recorded with six scorers be
// compared against a run with seven, and the missing scorer would read as
// "unchanged" rather than "never measured".
func TestScorerSetVersionMatchesTheSet(t *testing.T) {
	const versionAtWhichThisWasWritten = 4
	wantNames := []string{
		"bullet_shortfalls",
		"duplicate_provenance",
		"grounding_violations",
		"highlight_drift",
		"nice_to_have_retention",
		"ranking_violations",
		"required_skills_missing",
		"structural_violations",
		"suggestion_duplicates",
	}

	if ScorerSetVersion != versionAtWhichThisWasWritten {
		t.Fatalf("ScorerSetVersion is %d but this test still describes version %d. "+
			"Update the expected set below in the same change that bumped the version.",
			ScorerSetVersion, versionAtWhichThisWasWritten)
	}
	got := scorerNames()
	if len(got) != len(wantNames) {
		t.Fatalf("the scorer set has %d scorers, version %d describes %d — bump ScorerSetVersion and update this list",
			len(got), ScorerSetVersion, len(wantNames))
	}
	for i := range got {
		if got[i] != wantNames[i] {
			t.Errorf("scorer %d is %q, version %d describes %q — bump ScorerSetVersion", i, got[i], ScorerSetVersion, wantNames[i])
		}
	}
}

// scoringFixture is a small master/merged pair with known properties, used by
// the determinism and defect-injection tests. It is deliberately hand-built
// rather than corpus-derived: these tests must run before any fixture exists.
func scoringFixture() (master, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, cfg domain.ShapeConfig) {
	build := func() domain.RendercvMaster {
		return domain.RendercvMaster{"cv": map[string]any{"sections": map[string]any{
			"summary": []any{"Backend engineer with payments experience."},
			"skills": []any{
				map[string]any{"label": "Backend", "details": "Go, Postgres, Docker"},
				map[string]any{"label": "Frontend", "details": "React, TypeScript"},
			},
			"experience": []any{map[string]any{
				"company":    "Acme",
				"position":   "Engineer",
				"start_date": "2018-01",
				"end_date":   "2024-01",
				"highlights": []any{"Shipped the payments service", "Cut latency in half"},
			}},
		}}}
	}
	analysis = domain.VacancyAnalysis{
		RequiredSkills:   []string{"Go", "Docker"},
		NiceToHaveSkills: []string{"React"},
		ExperienceLevel:  "senior",
	}
	cfg = domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 2
	return build(), build(), analysis, cfg
}

// healthyRanking is a valid ranking response for scoringFixture's single
// "Acme" entry: K = min(2*2, 2) = 2, so both of its bullets, in any order.
func healthyRanking() domain.RankedSelection {
	return domain.RankedSelection{
		Experience: []domain.RankedExperience{{Company: "Acme", Ranking: []int{0, 1}}},
	}
}

func baseScoreInput() scoreInput {
	master, merged, analysis, cfg := scoringFixture()
	return scoreInput{
		master:   master,
		result:   merged,
		analysis: analysis,
		cfg:      cfg,
		level:    domain.GroundingModerate,
		ranking:  healthyRanking(),
	}
}

// C1-3: the same input scores identically every time. A scorer reading a map in
// iteration order, or consulting the clock, would fail here — and would make
// every baseline comparison a coin flip.
func TestScorersAreDeterministic(t *testing.T) {
	in := baseScoreInput()
	first := scoreAll(in)
	for i := 0; i < 5; i++ {
		again := scoreAll(in)
		for name, s := range first {
			if again[name].Value != s.Value {
				t.Errorf("scorer %q returned %v then %v on identical input", name, s.Value, again[name].Value)
			}
		}
	}
}

// --- tripwire: delegation ---------------------------------------------------

// TestScorerDelegationIsExact is a tripwire (FR-029, C1-1a).
//
// For each scorer it independently calls the domain function that scorer names
// and asserts **equality** — not correlation, not "close enough". A scorer that
// quietly adds a threshold, a filter or a cap breaks the equality.
//
// Without this, FR-002's delegation rule is a rule with no detector. That is
// how `structural_violations` came to be documented as wrapping a file that
// contains no verifier at all: nothing was checking that the scorer and the
// name agreed.
func TestScorerDelegationIsExact(t *testing.T) {
	inputs := []scoreInput{baseScoreInput()}
	for _, m := range mutatedDocuments() {
		in := baseScoreInput()
		in.result = m.doc
		if m.ranking != nil {
			in.ranking = *m.ranking
		}
		if m.suggestions != nil {
			in.suggestions = *m.suggestions
		}
		inputs = append(inputs, in)
	}

	for i, in := range inputs {
		in.report = domain.VerifyCompleteness(in.master, in.result, in.analysis, in.cfg)
		got := scoreAll(in)

		want := map[string]float64{
			"grounding_violations":    float64(len(domain.VerifyRendercvGrounding(in.master, in.result, in.level, in.analysis))),
			"structural_violations":   float64(len(domain.VerifyStructureIntegrity(in.master, in.result))),
			"highlight_drift":         float64(len(domain.VerifyHighlightGrounding(in.master, in.result))),
			"duplicate_provenance":    float64(len(domain.VerifyHighlightProvenance(in.master, in.result))),
			"required_skills_missing": float64(len(in.report.RequiredMissing)),
			"nice_to_have_retention":  in.report.NiceToHaveRetained,
			"bullet_shortfalls":       float64(len(in.report.BulletShortfalls)),
			"ranking_violations":      rankingViolationsTotal(in.master, in.cfg, in.ranking),
			"suggestion_duplicates":   suggestionDuplicatesTotal(in.master, in.suggestions),
		}

		for name, wantVal := range want {
			if got[name].Value != wantVal {
				t.Errorf("input %d: scorer %q returned %v; calling %s directly returns %v. "+
					"A scorer must delegate exactly — a threshold, filter or cap added here becomes a "+
					"definition of quality that production does not enforce (FR-002).",
					i, name, got[name].Value, name, wantVal)
			}
		}
		if len(got) != len(want) {
			t.Errorf("input %d: %d scores produced for %d checked expressions; a scorer has no delegation assertion", i, len(got), len(want))
		}
	}
}

// --- tripwire: injected defects ---------------------------------------------

type mutation struct {
	name        string
	doc         domain.RendercvMaster
	worseScorer string
	// ranking overrides scoreInput.ranking for this mutation. Every other
	// scorer's defect lives entirely in `doc`; ranking_violations' does not —
	// a rejected ranking never reaches the document — so this is the one
	// mutation that needs a second axis. nil means "leave baseScoreInput's
	// healthy ranking as-is".
	ranking *domain.RankedSelection
	// suggestions overrides scoreInput.suggestions, for the same reason
	// `ranking` overrides scoreInput.ranking: a suppressed suggestion never
	// reaches the document either.
	suggestions *domain.SuggestionSet
}

// mutatedDocuments injects known defects into a copy of a healthy document.
// Each one is a defect that has actually occurred in this pipeline.
func mutatedDocuments() []mutation {
	sections := func(d domain.RendercvMaster) map[string]any {
		return d["cv"].(map[string]any)["sections"].(map[string]any)
	}

	fabricatedCompany := func() domain.RendercvMaster {
		_, d, _, _ := scoringFixture()
		s := sections(d)
		s["experience"] = append(s["experience"].([]any), map[string]any{
			"company":    "Globex",
			"position":   "Principal Engineer",
			"start_date": "2015-01",
			"end_date":   "2018-01",
			"highlights": []any{"Ran the platform team", "Owned the billing rewrite"},
		})
		return d
	}()

	driftedHighlight := func() domain.RendercvMaster {
		_, d, _, _ := scoringFixture()
		s := sections(d)
		exp := s["experience"].([]any)[0].(map[string]any)
		exp["highlights"] = []any{"Architected quantum blockchain synergy frameworks", "Cut latency in half"}
		return d
	}()

	strippedBullets := func() domain.RendercvMaster {
		_, d, _, _ := scoringFixture()
		s := sections(d)
		exp := s["experience"].([]any)[0].(map[string]any)
		exp["highlights"] = []any{"Shipped the payments service"}
		return d
	}()

	removedRequiredSkill := func() domain.RendercvMaster {
		_, d, _, _ := scoringFixture()
		s := sections(d)
		s["skills"] = []any{
			map[string]any{"label": "Backend", "details": "Postgres"},
			map[string]any{"label": "Frontend", "details": "React, TypeScript"},
		}
		return d
	}()

	droppedNiceToHave := func() domain.RendercvMaster {
		_, d, _, _ := scoringFixture()
		s := sections(d)
		s["skills"] = []any{
			map[string]any{"label": "Backend", "details": "Go, Postgres, Docker"},
			map[string]any{"label": "Frontend", "details": "TypeScript"},
		}
		return d
	}()

	// One master bullet spun into two wordings. Both halves keep enough of the
	// original to pass lcsCovered — that is what makes this defect invisible to
	// every scorer but its own.
	oneBulletTwice := func() domain.RendercvMaster {
		_, d, _, _ := scoringFixture()
		s := sections(d)
		exp := s["experience"].([]any)[0].(map[string]any)
		exp["highlights"] = []any{
			"Shipped the payments service",
			"Shipped the payments service to production",
		}
		return d
	}()

	// badRanking duplicates an index — one of the three VerifyRanking
	// violations — so the ranking-stage response, not the merged document,
	// carries this defect.
	badRanking := domain.RankedSelection{
		Experience: []domain.RankedExperience{{Company: "Acme", Ranking: []int{0, 0}}},
	}
	// duplicatedSuggestion is a suggestion stage that echoed a master bullet
	// back — the FR-017 case R6 exists for, and the one R4's prompt is meant
	// to make unlikely rather than impossible.
	duplicatedSuggestion := domain.SuggestionSet{
		Experience: []domain.ExperienceSuggestions{{Company: "Acme", Bullets: []string{"Shipped the payments service"}}},
	}
	_, healthyDoc, _, _ := scoringFixture()

	return []mutation{
		{"a company absent from master", fabricatedCompany, "grounding_violations", nil, nil},
		{"one master bullet rendered as two accomplishments", oneBulletTwice, "duplicate_provenance", nil, nil},
		{"a highlight sharing no words with any master bullet", driftedHighlight, "highlight_drift", nil, nil},
		{"highlights stripped below ExperienceBulletsMin", strippedBullets, "bullet_shortfalls", nil, nil},
		{"a required skill removed", removedRequiredSkill, "required_skills_missing", nil, nil},
		{"a nice-to-have skill dropped", droppedNiceToHave, "nice_to_have_retention", nil, nil},
		{"a ranking response with a duplicated index", healthyDoc, "ranking_violations", &badRanking, nil},
		{"a suggestion echoing a master bullet back", healthyDoc, "suggestion_duplicates", nil, &duplicatedSuggestion},
	}
}

// TestScorersDetectInjectedDefects is the second tripwire (FR-029).
//
// It catches the three ways a scorer can be broken while every other test still
// passes: returning a constant, being wired to the wrong domain function, or
// never being called at all. Each mutation is a defect this pipeline has
// actually produced, and the assertion is that the *relevant* scorer moves in
// its worse direction.
func TestScorersDetectInjectedDefects(t *testing.T) {
	healthy := scoreAll(baseScoreInput())

	for _, m := range mutatedDocuments() {
		t.Run(m.name, func(t *testing.T) {
			in := baseScoreInput()
			in.result = m.doc
			if m.suggestions != nil {
				in.suggestions = *m.suggestions
			}
			if m.ranking != nil {
				in.ranking = *m.ranking
			}
			got := scoreAll(in)

			s := got[m.worseScorer]
			base := healthy[m.worseScorer]
			worse := s.Value > base.Value
			if s.Direction == HigherIsBetter {
				worse = s.Value < base.Value
			}
			if !worse {
				t.Errorf("injecting %q left %q at %v (healthy: %v, direction %s). "+
					"A scorer that does not move on its own defect is a scorer returning a constant, "+
					"wired to the wrong check, or never called.",
					m.name, m.worseScorer, s.Value, base.Value, s.Direction)
			}
		})
	}
}

// The healthy document must score clean. Stated on its own because a gate made
// only of failure cases cannot tell "stricter" from "broken", and a verifier
// that starts firing on good output is how a gate gets skipped.
func TestHealthyDocumentScoresClean(t *testing.T) {
	got := scoreAll(baseScoreInput())
	for name, s := range got {
		if s.Direction == LowerIsBetter && s.Value != 0 {
			t.Errorf("scorer %q reports %v on a healthy document, want 0", name, s.Value)
		}
		if s.Direction == HigherIsBetter && s.Value != 1 {
			t.Errorf("scorer %q reports %v on a healthy document, want 1", name, s.Value)
		}
	}
}
