package application

import (
	"sort"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

const ScorerSetVersion = 4

type Direction string

const (
	LowerIsBetter  Direction = "lower_is_better"
	HigherIsBetter Direction = "higher_is_better"
)

type Score struct {
	Name      string    `json:"name"`
	Value     float64   `json:"value"`
	Direction Direction `json:"direction"`
}

type scoreInput struct {
	master   domain.RendercvMaster
	result   domain.RendercvMaster
	analysis domain.VacancyAnalysis
	cfg      domain.ShapeConfig
	level    domain.GroundingLevel
	runErr   error

	report domain.CompletenessReport

	suggestions domain.SuggestionSet

	ranking domain.RankedSelection
}

type Scorer struct {
	Name      string
	Direction Direction
	Score     func(in scoreInput) float64
}

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

			return float64(len(domain.VerifyHighlightProvenance(in.master, in.result)))
		},
	},
	{
		Name:      "required_skills_missing",
		Direction: LowerIsBetter,
		Score: func(in scoreInput) float64 {

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

func rankingViolationsTotal(master domain.RendercvMaster, cfg domain.ShapeConfig, sel domain.RankedSelection) float64 {
	total := 0
	for _, e := range domain.AsSliceOfMaps(domain.CvSections(master)["experience"]) {
		company := domain.StringField(e, "company")
		available := len(domain.StringSliceField(e, "highlights"))

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

type mutation struct {
	name        string
	doc         domain.RendercvMaster
	worseScorer string

	ranking *domain.RankedSelection

	suggestions *domain.SuggestionSet
}

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

	badRanking := domain.RankedSelection{
		Experience: []domain.RankedExperience{{Company: "Acme", Ranking: []int{0, 0}}},
	}

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
