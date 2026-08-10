package application

import (
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

// T082: a rerun preserves a matched item's selected/position/edited_text and
// discards only the items that don't match anything in the freshly generated
// set — data-model.md §4's rule, exercised here as the pure function the
// service applies before every delete-and-recreate on a rerun
// (preserveMatchedSelections), with no database involved.
func TestPreserveMatchedSelectionsKeepsMatchedProfileItemState(t *testing.T) {
	old := []domain.Item{
		// The user toggled this bullet off and moved it — a decision that
		// must survive the rerun because the bullet (source_index 0) still
		// exists in the fresh set.
		{Origin: domain.OriginProfile, SourceIndex: intPtr(0), SourceText: "Shipped the payments service", Selected: false, Position: 5},
		// This bullet no longer appears in the fresh set (its entry shrank,
		// or the rerun simply re-ranked it out) — it must not leak into the
		// result at all.
		{Origin: domain.OriginProfile, SourceIndex: intPtr(9), SourceText: "A bullet that no longer exists", Selected: true, Position: 0},
	}
	fresh := []domain.Item{
		{Origin: domain.OriginProfile, SourceIndex: intPtr(0), SourceText: "Shipped the payments service", Selected: true, Position: 0},
		{Origin: domain.OriginProfile, SourceIndex: intPtr(1), SourceText: "Cut latency in half", Selected: true, Position: 1},
	}

	out := preserveMatchedSelections(old, fresh)

	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2 (exactly the fresh items, nothing extra)", len(out))
	}
	if out[0].Selected != false || out[0].Position != 5 {
		t.Errorf("matched item (sourceIndex 0) = %+v, want Selected=false Position=5 carried from the old item", out[0])
	}
	if !out[1].Selected || out[1].Position != 1 {
		t.Errorf("unmatched item (sourceIndex 1) = %+v, want the fresh default (Selected=true Position=1) untouched", out[1])
	}
}

func TestPreserveMatchedSelectionsKeepsMatchedAIItemStateAndEdit(t *testing.T) {
	old := []domain.Item{
		{Origin: domain.OriginAI, SourceText: "Led the Kubernetes migration", Selected: true, Position: 4, EditedText: strPtr("Led the Kubernetes migration end to end")},
		{Origin: domain.OriginAI, SourceText: "A suggestion the model won't repeat this time", Selected: true, Position: 0},
	}
	// The regenerated suggestion differs only in case/whitespace — still the
	// "same" suggestion under normalised-text matching (data-model.md §4).
	fresh := []domain.Item{
		{Origin: domain.OriginAI, SourceText: "  Led the Kubernetes migration  ", Selected: false, Position: 0},
		{Origin: domain.OriginAI, SourceText: "A brand new suggestion", Selected: false, Position: 1},
	}

	out := preserveMatchedSelections(old, fresh)

	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if !out[0].Selected || out[0].Position != 4 {
		t.Errorf("matched AI item = %+v, want Selected=true Position=4 carried from the old item", out[0])
	}
	if out[0].EditedText == nil || *out[0].EditedText != "Led the Kubernetes migration end to end" {
		t.Errorf("matched AI item EditedText = %v, want the preserved edit", out[0].EditedText)
	}
	if out[1].Selected || out[1].Position != 1 {
		t.Errorf("unmatched AI item = %+v, want the fresh default (Selected=false Position=1)", out[1])
	}
	if out[1].EditedText != nil {
		t.Errorf("unmatched AI item EditedText = %v, want nil", out[1].EditedText)
	}
}

// A plain run has nothing to preserve: nil oldItems must leave newItems
// exactly as given, so applyRankedSections/applyRankedSkills passing nil for
// a first-time run is a true no-op rather than a special case.
func TestPreserveMatchedSelectionsIsIdentityWithNoOldItems(t *testing.T) {
	fresh := []domain.Item{
		{Origin: domain.OriginProfile, SourceIndex: intPtr(0), SourceText: "Shipped the payments service", Selected: true, Position: 0},
	}

	out := preserveMatchedSelections(nil, fresh)

	if len(out) != 1 || out[0] != fresh[0] {
		t.Errorf("out = %+v, want fresh unchanged", out)
	}
}
