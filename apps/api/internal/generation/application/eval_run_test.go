package application

import (
	"context"
	"fmt"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

type evalRun struct {
	merged   domain.RendercvMaster
	analysis domain.VacancyAnalysis
	prov     *runProvenance
	err      error

	misses int

	ranking domain.RankedSelection

	suggestions domain.SuggestionSet
}

func stubRenderDeps(s *Service, pageCounts []int) renderDeps {
	deps := s.defaultRenderDeps()
	round := 0
	deps.render = func(ctx context.Context, merged domain.RendercvMaster, name string) (string, error) {
		return "/eval/" + name + ".pdf", nil
	}
	deps.countPages = func(pdfPath string) (int, error) {
		if len(pageCounts) == 0 {
			return 0, fmt.Errorf("eval: case declared no page_counts")
		}
		i := round
		round++
		if i >= len(pageCounts) {

			return 0, fmt.Errorf("eval: page-fit took more rounds than page_counts declares (%d entries); "+
				"either the loop changed or the case needs re-declaring", len(pageCounts))
		}
		return pageCounts[i], nil
	}
	return deps
}

func runCase(t *testing.T, c EvalCase) evalRun {
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
	merged, analysis, err := svc.tailorRendercvResume(
		context.Background(), c.Master, domain.VacancyTarget{}, c.Vacancy, c.Level, c.Cfg, nil, nil, run.prov)
	run.merged, run.analysis, run.err = merged, analysis, err

	if err == nil {
		deps := stubRenderDeps(svc, c.Spec.PageCounts)

		if _, rerr := svc.renderToPageTarget(
			context.Background(), deps, c.Master, merged, analysis, c.Level, c.Cfg, c.Name, nil); rerr != nil {
			run.err = fmt.Errorf("page fitting: %w", rerr)
		}
	}

	if run.err == nil {
		ranked, rerr := rankContent(context.Background(), sel, "", c.Master, analysis, c.Cfg, nil)
		if rerr != nil {
			run.err = fmt.Errorf("ranking: %w", rerr)
		} else {
			run.ranking = ranked
		}
	}

	if run.err == nil {
		suggested, serr := suggestContent(
			context.Background(), sel, "", experienceCompanies(c.Master), domain.SkillGroupLabels(c.Master), analysis)
		if serr != nil {
			run.err = fmt.Errorf("suggestion: %w", serr)
		} else {
			run.suggestions = suggested
		}
	}

	for _, p := range []*ReplayProvider{analyze, sel, premium, summary, cover} {
		run.misses += p.missCount()
	}
	return run
}

func scoreRun(c EvalCase, r evalRun) map[string]Score {
	return scoreAll(scoreInput{
		master:      c.Master,
		result:      r.merged,
		analysis:    r.analysis,
		cfg:         c.Cfg,
		level:       c.Level,
		runErr:      r.err,
		ranking:     r.ranking,
		suggestions: r.suggestions,
	})
}
