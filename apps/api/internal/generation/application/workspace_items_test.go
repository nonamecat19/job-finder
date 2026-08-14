package application

import (
	"errors"
	"reflect"
	"testing"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

func skillGroupRow(text string, dropped []string) sqlcgen.GenerationItem {
	return sqlcgen.GenerationItem{Origin: "profile", SourceText: text, DroppedEntries: dropped}
}

// The stored drop set is canonical: the group's own entry order, deduped and
// trimmed, so re-sending the same skills in another order is the same write.
func TestNormalizeDroppedEntriesIsCanonical(t *testing.T) {
	row := skillGroupRow("Backend: Go, NestJS, Redis", nil)

	got, err := normalizeDroppedEntries(row, []string{" Redis ", "NestJS", "Redis"})
	if err != nil {
		t.Fatalf("normalizeDroppedEntries: %v", err)
	}
	if want := []string{"NestJS", "Redis"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v in group order", got, want)
	}
}

// An empty set restores the whole group rather than meaning "no change".
func TestNormalizeDroppedEntriesAcceptsTheEmptySet(t *testing.T) {
	got, err := normalizeDroppedEntries(skillGroupRow("Backend: Go, NestJS", []string{"Go"}), []string{})
	if err != nil {
		t.Fatalf("normalizeDroppedEntries: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want the empty drop set", got)
	}
}

// A skill the group does not contain means the client is working from a stale
// copy — a 400, not a silent no-op, because the next export would otherwise
// drop something the user never named.
func TestNormalizeDroppedEntriesRejectsAnUnknownSkill(t *testing.T) {
	_, err := normalizeDroppedEntries(skillGroupRow("Backend: Go, NestJS", nil), []string{"Rust"})
	if err == nil {
		t.Fatal("no error for a skill outside the group")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperr.KindValidation {
		t.Errorf("error = %v, want a validation error", err)
	}
}
