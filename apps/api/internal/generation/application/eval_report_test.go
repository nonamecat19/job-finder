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

// Reading two comparison runs against each other (038 US4).
//
// The question US4 exists to answer is "what does the extra quality cost", and
// answering it must not require re-running anything: a comparison that can only
// be read by repeating a live run is a comparison nobody repeats.
//
// This lives behind the live build tag with the runs it reads, because a report
// over artifacts that only live mode produces has nothing to say in the
// deterministic gate.

// tradeRow is one model's quality-versus-cost position.
type tradeRow struct {
	Model string
	// Quality is the per-scorer median, kept per scorer rather than summed.
	// Summing would fold the declared grounding/drift overlap into a single
	// inflated figure and quietly weight every scorer equally.
	Quality      map[string]float64
	MedianCost   float64
	TotalCost    float64
	MedianMs     float64
	Incomplete   int
	TotalCases   int
	QualityDelta map[string]float64
	CostDelta    float64
}

// compareRuns puts two artifacts side by side. `base` is the incumbent and
// `candidate` the model being considered.
func compareRuns(base, candidate ComparisonRun) ([]tradeRow, error) {
	if base.ScorerSetVer != candidate.ScorerSetVer {
		// Same rule as the baseline comparator, for the same reason: a delta
		// measured across two instruments is not a quality signal.
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

// TestCompareArtifacts reads two artifacts named by flags and prints the trade.
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

// Two runs measured with different scorer sets, or over different corpora, are
// not comparable. Asserted because the tempting behaviour — compare anyway and
// let the reader notice — produces a table that looks authoritative and is not.
func TestCompareRunsRefusesAcrossInstrumentsAndCorpora(t *testing.T) {
	base := ComparisonRun{ScorerSetVer: 1, CorpusRevision: "rev-a"}
	if _, err := compareRuns(base, ComparisonRun{ScorerSetVer: 2, CorpusRevision: "rev-a"}); err == nil {
		t.Error("compared across scorer set versions")
	}
	if _, err := compareRuns(base, ComparisonRun{ScorerSetVer: 1, CorpusRevision: "rev-b"}); err == nil {
		t.Error("compared across corpus revisions; the two runs measured different inputs")
	}
}

// The trade is readable per scorer, never as one summed quality number.
// Summing would fold the declared grounding/drift overlap into a single
// inflated figure and weight every scorer equally without saying so.
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
	// No summed total anywhere on the row.
	if _, summed := r.Quality["total"]; summed {
		t.Error("a summed quality total appeared; the declared grounding/drift overlap makes one defect count twice in any sum")
	}
}
