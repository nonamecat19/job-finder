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

func TestNormalizeDroppedEntriesAcceptsTheEmptySet(t *testing.T) {
	got, err := normalizeDroppedEntries(skillGroupRow("Backend: Go, NestJS", []string{"Go"}), []string{})
	if err != nil {
		t.Fatalf("normalizeDroppedEntries: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want the empty drop set", got)
	}
}

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
