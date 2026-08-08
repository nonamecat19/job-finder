package domain

import "testing"

// The catalogue is a menu a user reads, so its invariants are about the menu
// being usable, not about Go compiling. Spec AC1 asks for 3-5 options plus the
// always-available self-hosted one, with exactly one preselected.
func TestSummaryCatalogueIsAUsableMenu(t *testing.T) {
	opts := SummaryOptions()

	if len(opts) < 3 || len(opts) > 5 {
		t.Fatalf("catalogue has %d options, want 3-5 (spec AC1); a longer menu is not more value "+
			"when the price points do not change", len(opts))
	}

	seen := map[string]bool{}
	defaults, selfHosted := 0, 0
	for _, o := range opts {
		if seen[o.ID] {
			t.Errorf("duplicate option id %q; ids are persisted, so a collision silently reroutes a user", o.ID)
		}
		seen[o.ID] = true

		if o.ID == "" || o.Label == "" || o.Description == "" || o.Cost == "" {
			t.Errorf("option %q has an empty field; every one of them is rendered to the user", o.ID)
		}
		if o.Default {
			defaults++
		}
		if o.SelfHosted() {
			selfHosted++
		}
	}

	if defaults != 1 {
		t.Errorf("catalogue has %d defaults, want exactly 1 (spec AC1)", defaults)
	}
	if selfHosted != 1 {
		t.Errorf("catalogue offers %d self-hosted options, want exactly 1; it must always be "+
			"available (Constitution V)", selfHosted)
	}
}

// The default must stay on the task key the pipeline used before this feature,
// which is what makes spec AC3 — a user who never opens the selector sees
// today's behaviour — true by construction rather than by vigilance.
func TestDefaultSummaryOptionKeepsThePreFeatureTaskKey(t *testing.T) {
	d := DefaultSummaryOption()
	if d.ID != SummaryOptionStandard {
		t.Fatalf("default option is %q, want %q", d.ID, SummaryOptionStandard)
	}
	if d.TaskKey != "generation-summary" {
		t.Fatalf("default option routes to %q, want %q. Changing this breaks spec AC3: a user who "+
			"never touches the selector would silently move to a different chain",
			d.TaskKey, "generation-summary")
	}
}

func TestLookupSummaryOptionFallsBackToTheDefault(t *testing.T) {
	got, ok := LookupSummaryOption("premium")
	if !ok || got.ID != "premium" {
		t.Fatalf("lookup(premium) = %v, %v", got.ID, ok)
	}

	// A stale dashboard, a hand-edited setting, or an option removed between
	// releases. The run must carry on.
	got, ok = LookupSummaryOption("no-such-option")
	if ok {
		t.Fatal("lookup of an unknown id reported success")
	}
	if got.ID != DefaultSummaryOption().ID {
		t.Fatalf("unknown id resolved to %q, want the default %q", got.ID, DefaultSummaryOption().ID)
	}
}

func TestSelfHostedOptionIsTheOneWithNoTaskKey(t *testing.T) {
	for _, o := range SummaryOptions() {
		if o.SelfHosted() != (o.TaskKey == "") {
			t.Errorf("option %q disagrees with itself about being self-hosted", o.ID)
		}
	}
}
