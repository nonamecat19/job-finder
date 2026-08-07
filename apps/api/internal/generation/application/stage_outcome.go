package application

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/platform/llm"
)

// Stage names recorded on a generation run.
const (
	stageAnalyze = "analyze"
	stageSelect  = "select"
	stageSummary = "summary"
	stagePageFit = "page-fit"
)

// StageOutcome is what one stage of a generation run cost and who served it.
// The served model is an observation, not an input: the application asks for a
// task key and reads back which upstream the proxy actually used (035 FR-017).
type StageOutcome struct {
	Stage            string
	ServedModel      string
	ServedGroup      string
	Substituted      bool
	Escalated        bool
	DurationMs       int64
	CostUSD          float64
	PromptTokens     int
	CompletionTokens int
}

// runProvenance accumulates the stage outcomes of a single run so the finished
// document can say which model wrote which part and what the run cost — from
// measurement rather than estimate.
type runProvenance struct {
	stages []StageOutcome
}

// observe runs fn with served-model and usage capture attached, then records
// what came back. The stage's result is returned untouched; provenance is a
// side-channel so a stage's own signature never grows a reporting parameter.
func observe[T any](ctx context.Context, prov *runProvenance, stage string, escalated bool, fn func(context.Context) (T, error)) (T, error) {
	if prov == nil {
		return fn(ctx)
	}
	ctx, served := llm.WithServedModelCapture(ctx)
	ctx, usage := llm.WithUsageCapture(ctx)
	started := time.Now()
	out, err := fn(ctx)
	prov.stages = append(prov.stages, StageOutcome{
		Stage:            stage,
		ServedModel:      *served,
		ServedGroup:      usage.ServedGroup,
		Substituted:      usage.Substituted,
		Escalated:        escalated,
		DurationMs:       time.Since(started).Milliseconds(),
		CostUSD:          usage.CostUSD,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
	})
	return out, err
}

func (p *runProvenance) last(stage string) *StageOutcome {
	for i := len(p.stages) - 1; i >= 0; i-- {
		if p.stages[i].Stage == stage {
			return &p.stages[i]
		}
	}
	return nil
}

func (p *runProvenance) summaryModel() *string {
	if o := p.last(stageSummary); o != nil && o.ServedModel != "" {
		m := o.ServedModel
		return &m
	}
	return nil
}

// summarySubstituted reports whether the premium summary was served by a
// fallback. This is the one substitution a user is shown, because it is the
// one that changes what they are reading (FR-012).
func (p *runProvenance) summarySubstituted() bool {
	o := p.last(stageSummary)
	return o != nil && o.Substituted
}

func (p *runProvenance) selectionModel() *string {
	if o := p.last(stageSelect); o != nil && o.ServedModel != "" {
		m := o.ServedModel
		return &m
	}
	return nil
}

func (p *runProvenance) selectionEscalated() bool {
	for _, o := range p.stages {
		if o.Stage == stageSelect && o.Escalated {
			return true
		}
	}
	return false
}

func (p *runProvenance) totalCostUSD() float64 {
	var total float64
	for _, o := range p.stages {
		total += o.CostUSD
	}
	return total
}

// meta renders the run's stages as activity metadata, so an operator can see
// per-stage timing, cost and substitution without a database query.
func (p *runProvenance) meta() map[string]any {
	stages := make([]map[string]any, 0, len(p.stages))
	for _, o := range p.stages {
		stages = append(stages, map[string]any{
			"stage":            o.Stage,
			"servedModel":      o.ServedModel,
			"servedGroup":      o.ServedGroup,
			"substituted":      o.Substituted,
			"escalated":        o.Escalated,
			"durationMs":       o.DurationMs,
			"costUsd":          o.CostUSD,
			"promptTokens":     o.PromptTokens,
			"completionTokens": o.CompletionTokens,
		})
	}
	return map[string]any{"stages": stages, "totalCostUsd": p.totalCostUSD()}
}

// withProvenance stamps a run's stage provenance onto the document row. Cost is
// carried as a string because pgtype.Numeric scans from one; a run with no
// measured cost (every stage local, or a proxy that reported none) leaves the
// column null rather than claiming the run was free.
func withProvenance(params sqlcgen.InsertGeneratedDocumentParams, prov *runProvenance) sqlcgen.InsertGeneratedDocumentParams {
	if prov == nil {
		return params
	}
	params.SummaryModel = prov.summaryModel()
	params.SummarySubstituted = prov.summarySubstituted()
	params.SelectionModel = prov.selectionModel()
	params.SelectionEscalated = prov.selectionEscalated()
	if cost := prov.totalCostUSD(); cost > 0 {
		var n pgtype.Numeric
		if err := n.Scan(strconv.FormatFloat(cost, 'f', 6, 64)); err == nil {
			params.StageCostUsd = n
		}
	}
	return params
}
