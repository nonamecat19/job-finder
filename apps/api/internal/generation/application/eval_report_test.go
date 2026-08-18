//go:build eval_live

package application

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

type tradeRow struct {
	Model string

	Quality      map[string]float64
	MedianCost   float64
	TotalCost    float64
	MedianMs     float64
	Incomplete   int
	TotalCases   int
	QualityDelta map[string]float64
	CostDelta    float64
}

func compareRuns(base, candidate ComparisonRun) ([]tradeRow, error) {
	if base.ScorerSetVer != candidate.ScorerSetVer {

		return nil, fmt.Errorf("scorer set versions differ (%d vs %d); refusing to compare across instruments",
			base.ScorerSetVer, candidate.ScorerSetVer)
	}
	if base.CorpusRevision != candidate.CorpusRevision {
		return nil, fmt.Errorf("corpus revisions differ; the two runs measured different inputs and their scores are not comparable")
	}

	incumbent := map[string]ModelResult{}
	for _, m := range base.Models {
		incumbent[m.Model] = m
	}

	var rows []tradeRow
	for _, m := range candidate.Models {
		row := tradeRow{
			Model:        m.Model,
			Quality:      m.MedianScores,
			MedianCost:   m.MedianCostUSD,
			TotalCost:    m.TotalCostUSD,
			MedianMs:     m.MedianDurationMs,
			Incomplete:   m.IncompleteCases,
			TotalCases:   len(m.Cases),
			QualityDelta: map[string]float64{},
		}
		if prev, ok := incumbent[m.Model]; ok {
			for name, v := range m.MedianScores {
				row.QualityDelta[name] = v - prev.MedianScores[name]
			}
			row.CostDelta = m.MedianCostUSD - prev.MedianCostUSD
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].MedianCost < rows[j].MedianCost })
	return rows, nil
}

func loadComparison(path string) (ComparisonRun, error) {
	var run ComparisonRun
	raw, err := os.ReadFile(path)
	if err != nil {
		return run, err
	}
	return run, json.Unmarshal(raw, &run)
}

func TestCompareArtifacts(t *testing.T) {
	basePath := os.Getenv("EVAL_BASE_ARTIFACT")
	candPath := os.Getenv("EVAL_CANDIDATE_ARTIFACT")
	if basePath == "" || candPath == "" {
		t.Skip("set EVAL_BASE_ARTIFACT and EVAL_CANDIDATE_ARTIFACT to compare two runs")
	}

	base, err := loadComparison(basePath)
	if err != nil {
		t.Fatalf("read %s: %v", basePath, err)
	}
	cand, err := loadComparison(candPath)
	if err != nil {
		t.Fatalf("read %s: %v", candPath, err)
	}

	rows, err := compareRuns(base, cand)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	names := scorerNames()
	header := append([]string{"Model", "Complete", "Median $", "Δ$", "Median ms"}, names...)
	t.Log(strings.Join(header, " | "))
	for _, r := range rows {
		line := []string{
			r.Model,
			fmt.Sprintf("%d/%d", r.TotalCases-r.Incomplete, r.TotalCases),
			fmt.Sprintf("%.6f", r.MedianCost),
			fmt.Sprintf("%+.6f", r.CostDelta),
			fmt.Sprintf("%.0f", r.MedianMs),
		}
		for _, n := range names {
			line = append(line, fmt.Sprintf("%.3f (%+.3f)", r.Quality[n], r.QualityDelta[n]))
		}
		t.Log(strings.Join(line, " | "))
	}
}

func TestCompareRunsRefusesAcrossInstrumentsAndCorpora(t *testing.T) {
	base := ComparisonRun{ScorerSetVer: 1, CorpusRevision: "rev-a"}
	if _, err := compareRuns(base, ComparisonRun{ScorerSetVer: 2, CorpusRevision: "rev-a"}); err == nil {
		t.Error("compared across scorer set versions")
	}
	if _, err := compareRuns(base, ComparisonRun{ScorerSetVer: 1, CorpusRevision: "rev-b"}); err == nil {
		t.Error("compared across corpus revisions; the two runs measured different inputs")
	}
}

func TestQualityIsReportedPerScorerNotSummed(t *testing.T) {
	rev := "rev-a"
	base := ComparisonRun{ScorerSetVer: ScorerSetVersion, CorpusRevision: rev, Models: []ModelResult{{
		Model:         "generation-select",
		MedianScores:  map[string]float64{"grounding_violations": 2, "highlight_drift": 2},
		MedianCostUSD: 0.01,
	}}}
	cand := ComparisonRun{ScorerSetVer: ScorerSetVersion, CorpusRevision: rev, Models: []ModelResult{{
		Model:         "generation-select",
		MedianScores:  map[string]float64{"grounding_violations": 0, "highlight_drift": 0},
		MedianCostUSD: 0.04,
		Cases:         []CaseResult{{}, {}},
	}}}

	rows, err := compareRuns(base, cand)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if len(r.QualityDelta) != 2 {
		t.Errorf("quality deltas = %v, want one per scorer", r.QualityDelta)
	}
	if r.QualityDelta["grounding_violations"] != -2 || r.QualityDelta["highlight_drift"] != -2 {
		t.Errorf("per-scorer deltas wrong: %v", r.QualityDelta)
	}
	if r.CostDelta < 0.0299 || r.CostDelta > 0.0301 {
		t.Errorf("cost delta = %v, want ~0.03", r.CostDelta)
	}

	if _, summed := r.Quality["total"]; summed {
		t.Error("a summed quality total appeared; the declared grounding/drift overlap makes one defect count twice in any sum")
	}
}
