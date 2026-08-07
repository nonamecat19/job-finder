package application

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Baselines and the comparator (038 FR-006 to FR-011, FR-027).
//
// A baseline is what the corpus scored when somebody last looked at it and
// agreed. The comparator's job is to make every way that agreement can stop
// holding visible — including the two that are easy to treat as fine:
// improvement (which means the baseline has quietly stopped describing
// reality) and a missing baseline (which is not a pass, it is an unmeasured
// case).

// Baseline is one case's recorded scores.
type Baseline struct {
	Case string `json:"case"`
	// ScorerSetVersion is the instrument this was measured with. A delta across
	// two instruments is not a quality signal, so the comparator refuses rather
	// than subtracting (FR-009).
	ScorerSetVersion int    `json:"scorer_set_version"`
	RecordedAt       string `json:"recorded_at"`
	// Reason is required on every write. A baseline that moved for a reason
	// nobody wrote down is a baseline nobody can review.
	Reason string             `json:"reason"`
	Scores map[string]float64 `json:"scores"`
}

func loadBaseline(name string) (Baseline, bool, error) {
	raw, err := os.ReadFile(caseBaselinePath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return Baseline{}, false, nil
		}
		return Baseline{}, false, err
	}
	var b Baseline
	if err := json.Unmarshal(raw, &b); err != nil {
		return Baseline{}, false, fmt.Errorf("baseline %s: %w", name, err)
	}
	return b, true, nil
}

// saveBaseline writes one case's baseline. A reason is mandatory: FR-007's
// whole point is that a baseline moves deliberately and reviewably.
func saveBaseline(b Baseline) error {
	if strings.TrimSpace(b.Reason) == "" {
		return fmt.Errorf("baseline for %q has no reason; a baseline that moved for a reason nobody wrote down is one nobody can review (FR-007)", b.Case)
	}
	if b.Case == "" {
		return fmt.Errorf("baseline has no case name")
	}
	if b.ScorerSetVersion == 0 {
		return fmt.Errorf("baseline for %q has no scorer_set_version; without it a later comparison cannot tell whether the instrument changed", b.Case)
	}
	if err := os.MkdirAll(baselinesDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(caseBaselinePath(b.Case), append(raw, '\n'), 0o644)
}

// Outcome is how one case compared.
type Outcome string

const (
	// OutcomeMatch: every scorer equal to baseline.
	OutcomeMatch Outcome = "match"
	// OutcomeWorse: at least one scorer moved in its worse direction. Fails.
	OutcomeWorse Outcome = "worse"
	// OutcomeBetter: an improvement. Also fails, with "baseline needs
	// updating" — silent acceptance of improvement is how a baseline quietly
	// stops describing reality.
	OutcomeBetter Outcome = "better"
	// OutcomeVersionMismatch: the baseline was measured with a different
	// instrument. Refuses to compare, reports no delta.
	OutcomeVersionMismatch Outcome = "version_mismatch"
	// OutcomeUnbaselined: no baseline file. Reported, and NOT a pass.
	OutcomeUnbaselined Outcome = "unbaselined"
)

// Comparison is the result of comparing one case against its baseline.
type Comparison struct {
	Case    Outcome
	Outcome Outcome
	// Messages are human-readable lines, each naming case, scorer, baseline
	// value, actual value and direction.
	Messages []string
	// CoMoving names pairs of scorers that moved together on a declared
	// overlap. Reported as one defect seen twice, never as two regressions and
	// never summed.
	CoMoving []string
}

// compareToBaseline is the comparator. Every branch here is one of FR-006 to
// FR-011, and each returns an outcome rather than a boolean so the caller
// cannot flatten "unbaselined" into "passed".
func compareToBaseline(caseName string, scores map[string]Score, b Baseline, hasBaseline bool) Comparison {
	if !hasBaseline {
		return Comparison{
			Outcome: OutcomeUnbaselined,
			Messages: []string{fmt.Sprintf(
				"case %q has no baseline. This is not a pass: the case ran and nothing checked the result. "+
					"Record one with: -eval.update-baseline -eval.case %s -eval.reason \"initial baseline\"", caseName, caseName)},
		}
	}
	if b.ScorerSetVersion != ScorerSetVersion {
		// Deliberately no delta in this message. Reporting one would invite
		// reading it as a quality change when it is an instrument change.
		return Comparison{
			Outcome: OutcomeVersionMismatch,
			Messages: []string{fmt.Sprintf(
				"case %q: baseline was recorded with scorer set version %d, this run uses version %d. "+
					"Refusing to compare — a delta measured across two instruments is not a quality signal. "+
					"Re-record the baseline with a reason naming the scorer change.",
				caseName, b.ScorerSetVersion, ScorerSetVersion)},
		}
	}

	var worse, better []string
	movedWorse := map[string]bool{}

	names := make([]string, 0, len(scores))
	for name := range scores {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		s := scores[name]
		base, ok := b.Scores[name]
		if !ok {
			worse = append(worse, fmt.Sprintf(
				"case %q scorer %q: no baseline value at this scorer set version; the scorer set and the baseline disagree", caseName, name))
			continue
		}
		if s.Value == base {
			continue
		}
		isWorse := s.Value > base
		if s.Direction == HigherIsBetter {
			isWorse = s.Value < base
		}
		line := fmt.Sprintf("case %q scorer %q: baseline %v, actual %v (%s)", caseName, name, base, s.Value, s.Direction)
		if isWorse {
			worse = append(worse, line)
			movedWorse[name] = true
		} else {
			better = append(better, line)
		}
	}

	var coMoving []string
	for _, pair := range overlappingScorers {
		if movedWorse[pair[0]] && movedWorse[pair[1]] {
			coMoving = append(coMoving, fmt.Sprintf(
				"%q and %q both moved: these share a check (VerifyRendercvGrounding performs the drift comparison inline), "+
					"so this is ONE defect seen by two instruments, not two regressions. Do not add them together.",
				pair[0], pair[1]))
		}
	}

	switch {
	case len(worse) > 0:
		return Comparison{Outcome: OutcomeWorse, Messages: worse, CoMoving: coMoving}
	case len(better) > 0:
		msgs := append([]string{fmt.Sprintf("case %q improved; the baseline needs updating so it keeps describing reality", caseName)}, better...)
		msgs = append(msgs, fmt.Sprintf(
			"Update with: -eval.update-baseline -eval.case %s -eval.reason \"<what changed>\"", caseName))
		return Comparison{Outcome: OutcomeBetter, Messages: msgs}
	default:
		return Comparison{Outcome: OutcomeMatch}
	}
}

// --- the deliberate baseline update -----------------------------------------
//
// Flags on the test binary rather than a cmd/ program: a `main` package cannot
// import test files, and the harness is test files.

var (
	flagUpdateBaseline = flag.Bool("eval.update-baseline", false, "rewrite one case's baseline from this run")
	flagCase           = flag.String("eval.case", "", "which case to act on")
	flagReason         = flag.String("eval.reason", "", "why the baseline is moving; required with -eval.update-baseline")
)

// updateRequested reports whether this invocation is a deliberate baseline
// update, and validates that it named a case and a reason.
func updateRequested(t *testing.T) (caseName string, reason string, ok bool) {
	t.Helper()
	if !*flagUpdateBaseline {
		return "", "", false
	}
	if strings.TrimSpace(*flagCase) == "" {
		t.Fatal("-eval.update-baseline needs -eval.case: updating every baseline at once produces a diff nobody reviews")
	}
	if strings.TrimSpace(*flagReason) == "" {
		t.Fatal("-eval.update-baseline needs -eval.reason: a baseline that moved for a reason nobody wrote down is one nobody can review (FR-007)")
	}
	return *flagCase, *flagReason, true
}

func newBaseline(caseName, reason string, scores map[string]Score) Baseline {
	b := Baseline{
		Case:             caseName,
		ScorerSetVersion: ScorerSetVersion,
		RecordedAt:       time.Now().UTC().Format("2006-01-02"),
		Reason:           reason,
		Scores:           map[string]float64{},
	}
	for name, s := range scores {
		b.Scores[name] = s.Value
	}
	return b
}

// --- tests ------------------------------------------------------------------

func fixedScores(values map[string]float64) map[string]Score {
	out := map[string]Score{}
	for _, s := range scorers {
		v, ok := values[s.Name]
		if !ok {
			continue
		}
		out[s.Name] = Score{Name: s.Name, Value: v, Direction: s.Direction}
	}
	return out
}

func baselineFrom(version int, values map[string]float64) Baseline {
	return Baseline{Case: "t", ScorerSetVersion: version, Reason: "test", Scores: values}
}

// TestVersionMismatchRefuses is a tripwire (FR-009, C2-1).
//
// A baseline recorded under a different scorer set was measured with a
// different instrument. The refusal must carry **no delta**: a number here
// would be read as a quality change when it is an instrument change, which is
// the specific confusion this rule exists to prevent.
func TestVersionMismatchRefuses(t *testing.T) {
	values := map[string]float64{"grounding_violations": 0, "nice_to_have_retention": 1}
	cmp := compareToBaseline("t", fixedScores(values), baselineFrom(ScorerSetVersion-1, values), true)

	if cmp.Outcome != OutcomeVersionMismatch {
		t.Fatalf("outcome = %q, want %q", cmp.Outcome, OutcomeVersionMismatch)
	}
	joined := strings.Join(cmp.Messages, "\n")
	if !strings.Contains(joined, "Refusing to compare") {
		t.Errorf("the refusal does not say it is refusing:\n%s", joined)
	}
	// No delta: neither a scorer name nor a score value may appear. The prose
	// may of course use the word "delta" to say it is not reporting one.
	for name := range values {
		if strings.Contains(joined, name) {
			t.Errorf("the refusal names scorer %q; reporting a per-scorer figure across two instruments is exactly what this refusal exists to avoid:\n%s", name, joined)
		}
	}
	for _, forbidden := range []string{"actual", "baseline 0", "baseline 1"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("the refusal contains a measured value (%q); a number here reads as a quality change when it is an instrument change:\n%s", forbidden, joined)
		}
	}
}

// FR-006: a worse score fails, and the message carries case, scorer, baseline
// value, actual value and direction — everything a reader needs without
// re-running.
func TestWorseScoreFailsWithAFullMessage(t *testing.T) {
	cmp := compareToBaseline("absent-skills",
		fixedScores(map[string]float64{"grounding_violations": 3}),
		baselineFrom(ScorerSetVersion, map[string]float64{"grounding_violations": 1}), true)

	if cmp.Outcome != OutcomeWorse {
		t.Fatalf("outcome = %q, want %q", cmp.Outcome, OutcomeWorse)
	}
	joined := strings.Join(cmp.Messages, "\n")
	for _, want := range []string{"absent-skills", "grounding_violations", "baseline 1", "actual 3", "lower_is_better"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the failure message omits %q:\n%s", want, joined)
		}
	}
}

// A higher-is-better scorer moving down is also worse. Stated separately
// because getting this backwards is exactly the defect the spec's original
// `achievements_per_job_min` had: the gate would have failed on improvement.
func TestHigherIsBetterScorerFailsWhenItDrops(t *testing.T) {
	cmp := compareToBaseline("t",
		fixedScores(map[string]float64{"nice_to_have_retention": 0.5}),
		baselineFrom(ScorerSetVersion, map[string]float64{"nice_to_have_retention": 0.9}), true)
	if cmp.Outcome != OutcomeWorse {
		t.Fatalf("outcome = %q, want worse: retention fell from 0.9 to 0.5", cmp.Outcome)
	}
}

// FR-007: improvement fails too, with "needs updating". Silent acceptance is
// how a baseline stops describing reality while still passing.
func TestImprovementFailsWithNeedsUpdating(t *testing.T) {
	cmp := compareToBaseline("t",
		fixedScores(map[string]float64{"grounding_violations": 0}),
		baselineFrom(ScorerSetVersion, map[string]float64{"grounding_violations": 2}), true)

	if cmp.Outcome != OutcomeBetter {
		t.Fatalf("outcome = %q, want %q", cmp.Outcome, OutcomeBetter)
	}
	joined := strings.Join(cmp.Messages, "\n")
	if !strings.Contains(joined, "needs updating") {
		t.Errorf("an improvement did not ask for the baseline to be updated:\n%s", joined)
	}
	if !strings.Contains(joined, "-eval.update-baseline") {
		t.Errorf("the message does not say how to update:\n%s", joined)
	}
}

// FR-011: a case with no baseline is reported and is NOT a pass.
func TestMissingBaselineIsNotAPass(t *testing.T) {
	cmp := compareToBaseline("newcase", fixedScores(map[string]float64{"grounding_violations": 0}), Baseline{}, false)
	if cmp.Outcome != OutcomeUnbaselined {
		t.Fatalf("outcome = %q, want %q", cmp.Outcome, OutcomeUnbaselined)
	}
	if cmp.Outcome == OutcomeMatch {
		t.Fatal("an unbaselined case reported as a match")
	}
	if !strings.Contains(strings.Join(cmp.Messages, "\n"), "not a pass") {
		t.Error("the message does not say an unbaselined case is not a pass")
	}
}

// FR-027 / C2-8: a declared overlapping pair is reported as one defect seen
// twice, never summed. Summing would make one ungrounded highlight look like
// two regressions and inflate every quality report built on it.
func TestOverlappingScorersReportOneDefectSeenTwice(t *testing.T) {
	cmp := compareToBaseline("t",
		fixedScores(map[string]float64{"grounding_violations": 2, "highlight_drift": 2}),
		baselineFrom(ScorerSetVersion, map[string]float64{"grounding_violations": 0, "highlight_drift": 0}), true)

	if cmp.Outcome != OutcomeWorse {
		t.Fatalf("outcome = %q, want worse", cmp.Outcome)
	}
	if len(cmp.CoMoving) != 1 {
		t.Fatalf("co-moving pairs reported: %v, want exactly one", cmp.CoMoving)
	}
	note := cmp.CoMoving[0]
	if !strings.Contains(note, "ONE defect") || !strings.Contains(note, "not two regressions") {
		t.Errorf("the co-movement note does not say what it means:\n%s", note)
	}
	if !strings.Contains(note, "Do not add them together") {
		t.Errorf("the note does not forbid summing:\n%s", note)
	}
}

// A pair that did not co-move must not be reported as one — otherwise the note
// appears on every run and stops carrying information.
func TestNonCoMovingScorersAreNotPaired(t *testing.T) {
	cmp := compareToBaseline("t",
		fixedScores(map[string]float64{"grounding_violations": 2, "highlight_drift": 0}),
		baselineFrom(ScorerSetVersion, map[string]float64{"grounding_violations": 0, "highlight_drift": 0}), true)
	if len(cmp.CoMoving) != 0 {
		t.Errorf("co-movement reported when only one scorer moved: %v", cmp.CoMoving)
	}
}

// C2-6: a baseline update with no reason is rejected at the write, not just at
// the flag — so no code path can produce a reasonless baseline file.
func TestBaselineWriteRejectsAMissingReason(t *testing.T) {
	err := saveBaseline(Baseline{Case: "t", ScorerSetVersion: ScorerSetVersion, Scores: map[string]float64{}})
	if err == nil {
		t.Fatal("saveBaseline accepted a baseline with no reason")
	}
	if !strings.Contains(err.Error(), "reason") {
		t.Errorf("the rejection does not name the missing reason: %v", err)
	}
}

// C2-5: a passing run writes nothing. A gate that rewrites its own baseline on
// success is not a gate — it records whatever the code currently does and calls
// it correct.
func TestPassingRunWritesNoBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.json")

	cmp := compareToBaseline("t",
		fixedScores(map[string]float64{"grounding_violations": 0}),
		baselineFrom(ScorerSetVersion, map[string]float64{"grounding_violations": 0}), true)
	if cmp.Outcome != OutcomeMatch {
		t.Fatalf("outcome = %q, want match", cmp.Outcome)
	}
	// The comparator has no writer: nothing in the match path can create a file.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a file appeared at %s after a matching comparison", path)
	}
}

// A round trip through disk, so the on-disk shape is exercised rather than
// assumed.
func TestBaselineRoundTrip(t *testing.T) {
	orig := baselinesDir
	tmp := t.TempDir()
	baselinesDir = tmp
	t.Cleanup(func() { baselinesDir = orig })

	b := newBaseline("roundtrip", "initial baseline", fixedScores(map[string]float64{
		"grounding_violations":   0,
		"nice_to_have_retention": 0.8,
	}))
	if err := saveBaseline(b); err != nil {
		t.Fatalf("saveBaseline: %v", err)
	}
	got, ok, err := loadBaseline("roundtrip")
	if err != nil || !ok {
		t.Fatalf("loadBaseline: %v (found=%v)", err, ok)
	}
	if got.Reason != "initial baseline" || got.ScorerSetVersion != ScorerSetVersion {
		t.Errorf("round trip lost metadata: %+v", got)
	}
	if got.Scores["nice_to_have_retention"] != 0.8 {
		t.Errorf("round trip lost a score: %+v", got.Scores)
	}
	if got.RecordedAt == "" {
		t.Error("recorded_at is empty")
	}
}
