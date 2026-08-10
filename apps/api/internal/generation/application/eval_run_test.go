package application

import (
	"context"
	"fmt"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

// The pipeline seam (038 T005, FR-028, contracts C3-5).
//
// This is why the harness is in package `application` rather than a subpackage:
// tailorRendercvResume, renderToPageTarget and renderDeps are all unexported,
// and the two exported entry points (Generate, GenerateAdHoc) require Postgres
// and write activity rows. A subpackage could reach none of it, and driving the
// pipeline through its exported surface would mean the gate needs a database to
// measure text quality.

// evalRun is what one case produced.
type evalRun struct {
	merged   domain.RendercvMaster
	analysis domain.VacancyAnalysis
	prov     *runProvenance
	err      error
	// misses counts replay misses across all five stage providers, so a run
	// that failed for want of a fixture is distinguishable from one that failed
	// on the pipeline's own terms.
	misses int
	// ranking is the ranking stage's raw response (T045/T046, 042): a single
	// rankContent call against the same replayed `generation-select` provider
	// selectContent used, so the corpus needs a second fixture per case for
	// this request — recorded the same way every other fixture is
	// (-eval.record).
	ranking domain.RankedSelection
}

// stubRenderDeps builds render dependencies that need no PDF toolchain.
//
// The stub covers exactly two collaborators — `render` and `countPages` — and
// deliberately leaves `expand` and `condense` as production's own
// implementations, which go through the ReplayProvider. So the LLM half of page
// fitting is measured and only the PDF half is stubbed.
//
// This is what makes the gate runnable on a machine with no Python and no
// Typst. It also means, stated plainly, that **the deterministic gate does not
// measure the PDF renderer**: that stays covered by the infrastructure tests
// and by live mode.
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
			// Running past the declared sequence means the page-fit loop took
			// more rounds than the case said it would. Repeating the last value
			// would hide that; failing names it.
			return 0, fmt.Errorf("eval: page-fit took more rounds than page_counts declares (%d entries); "+
				"either the loop changed or the case needs re-declaring", len(pageCounts))
		}
		return pageCounts[i], nil
	}
	return deps
}

// runCase drives one case through the real tailoring pipeline against replayed
// responses, then through page fitting against the stubbed renderer.
func runCase(t *testing.T, c EvalCase) evalRun {
	t.Helper()

	// One provider per stage, each keyed by its own stage name, so two stages
	// that happen to send an identical conversation still resolve to different
	// fixtures.
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
		context.Background(), c.Master, c.Vacancy, c.Level, c.Cfg, nil, nil, run.prov)
	run.merged, run.analysis, run.err = merged, analysis, err

	if err == nil {
		deps := stubRenderDeps(svc, c.Spec.PageCounts)
		// renderToPageTarget expands or condenses the document in place —
		// RendercvMaster is a map, and the fit loop merges into it — so
		// run.merged already reflects what would have been rendered. The
		// outcome itself carries only the path, the page count and whether the
		// fit gave up, none of which the scorers read.
		if _, rerr := svc.renderToPageTarget(
			context.Background(), deps, c.Master, merged, analysis, c.Level, c.Cfg, c.Name, nil); rerr != nil {
			run.err = fmt.Errorf("page fitting: %w", rerr)
		}
	}

	// The ranking stage (042 T045/T046): a single call over the same replayed
	// `generation-select` provider selectContent used above, so a request-hash
	// miss here fails the case exactly the way any other stage's miss does —
	// loudly, naming the re-record command — rather than silently skipping the
	// ranking_violations scorer.
	if run.err == nil {
		ranked, rerr := rankContent(context.Background(), sel, "", c.Master, analysis, c.Cfg, nil)
		if rerr != nil {
			run.err = fmt.Errorf("ranking: %w", rerr)
		} else {
			run.ranking = ranked
		}
	}

	for _, p := range []*ReplayProvider{analyze, sel, premium, summary, cover} {
		run.misses += p.missCount()
	}
	return run
}

func scoreRun(c EvalCase, r evalRun) map[string]Score {
	return scoreAll(scoreInput{
		master:   c.Master,
		result:   r.merged,
		analysis: r.analysis,
		cfg:      c.Cfg,
		level:    c.Level,
		runErr:   r.err,
		ranking:  r.ranking,
	})
}
