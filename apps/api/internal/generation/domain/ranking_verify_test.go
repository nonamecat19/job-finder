package domain

import "testing"

func hasViolation(vs []RankingViolation, kind RankingViolationKind) bool {
	for _, v := range vs {
		if v.Kind == kind {
			return true
		}
	}
	return false
}

func TestVerifyRanking(t *testing.T) {

	full := []int{0, 1, 2, 3, 4, 5, 6, 7}

	t.Run("valid ranking of exactly K has no violations", func(t *testing.T) {
		if got := VerifyRanking(10, 4, full); len(got) != 0 {
			t.Errorf("VerifyRanking() = %+v, want none", got)
		}
	})

	t.Run("out_of_range detected for a negative index", func(t *testing.T) {
		ranking := append([]int{-1}, full[1:]...)
		got := VerifyRanking(10, 4, ranking)
		if !hasViolation(got, RankingOutOfRange) {
			t.Errorf("VerifyRanking() = %+v, want out_of_range", got)
		}
	})

	t.Run("out_of_range detected for an index >= available", func(t *testing.T) {
		ranking := append([]int{10}, full[1:]...)
		got := VerifyRanking(10, 4, ranking)
		if !hasViolation(got, RankingOutOfRange) {
			t.Errorf("VerifyRanking() = %+v, want out_of_range", got)
		}
	})

	t.Run("duplicate detected", func(t *testing.T) {
		ranking := []int{0, 1, 2, 3, 4, 5, 6, 6}
		got := VerifyRanking(10, 4, ranking)
		if !hasViolation(got, RankingDuplicate) {
			t.Errorf("VerifyRanking() = %+v, want duplicate", got)
		}
	})

	t.Run("short detected when fewer than K entries returned", func(t *testing.T) {
		ranking := []int{0, 1, 2}
		got := VerifyRanking(10, 4, ranking)
		if !hasViolation(got, RankingShort) {
			t.Errorf("VerifyRanking() = %+v, want short", got)
		}
	})

	t.Run("len(ranking) > K is accepted, not a violation", func(t *testing.T) {
		ranking := append(append([]int{}, full...), 8, 9)
		got := VerifyRanking(10, 4, ranking)
		if len(got) != 0 {
			t.Errorf("VerifyRanking() = %+v, want none (extra ranked indices are accepted)", got)
		}
	})

	t.Run("K = min(2*target, available)", func(t *testing.T) {

		if got := VerifyRanking(3, 4, []int{0, 1, 2}); len(got) != 0 {
			t.Errorf("VerifyRanking() = %+v, want none when K is capped by availability", got)
		}
		if got := RankingK(3, 4); got != 3 {
			t.Errorf("RankingK(3, 4) = %d, want 3", got)
		}
		if got := RankingK(10, 4); got != 8 {
			t.Errorf("RankingK(10, 4) = %d, want 8", got)
		}
	})

	t.Run("short still fires when available caps K below the naive count", func(t *testing.T) {

		got := VerifyRanking(3, 4, []int{0, 1})
		if !hasViolation(got, RankingShort) {
			t.Errorf("VerifyRanking() = %+v, want short", got)
		}
	})
}

func TestMasterOrderRanking(t *testing.T) {
	got := MasterOrderRanking(10, 4)
	if len(got) != 8 {
		t.Fatalf("MasterOrderRanking() has %d entries, want 8", len(got))
	}
	for i, v := range got {
		if v != i {
			t.Errorf("MasterOrderRanking()[%d] = %d, want %d (master order)", i, v, i)
		}
	}
	if violations := VerifyRanking(10, 4, got); len(violations) != 0 {
		t.Errorf("MasterOrderRanking() itself fails verification: %+v", violations)
	}
}

func TestSeedRankedItems(t *testing.T) {
	highlights := []string{"a", "b", "c", "d", "e"}

	t.Run("top N selected, rest of the ranking unselected, in ranked order", func(t *testing.T) {
		items := SeedRankedItems(highlights, 2, []int{3, 1, 0, 4})
		if len(items) != 5 {
			t.Fatalf("len(items) = %d, want 5 (the unranked tail must still appear)", len(items))
		}
		wantOrder := []int{3, 1, 0, 4, 2}
		wantSelected := []bool{true, true, false, false, false}
		for i, it := range items {
			if it.SourceIndex == nil || *it.SourceIndex != wantOrder[i] {
				t.Errorf("items[%d].SourceIndex = %v, want %d", i, it.SourceIndex, wantOrder[i])
			}
			if it.Selected != wantSelected[i] {
				t.Errorf("items[%d].Selected = %v, want %v", i, it.Selected, wantSelected[i])
			}
			if it.SourceText != highlights[wantOrder[i]] {
				t.Errorf("items[%d].SourceText = %q, want %q", i, it.SourceText, highlights[wantOrder[i]])
			}
			if it.Origin != OriginProfile {
				t.Errorf("items[%d].Origin = %q, want profile", i, it.Origin)
			}
		}
	})

	t.Run("every master bullet appears exactly once even when the ranking omits some", func(t *testing.T) {
		items := SeedRankedItems(highlights, 2, []int{0, 1})
		if len(items) != 5 {
			t.Fatalf("len(items) = %d, want 5", len(items))
		}
		seen := make(map[int]bool)
		for _, it := range items {
			if it.SourceIndex == nil {
				t.Fatal("item has nil SourceIndex")
			}
			if seen[*it.SourceIndex] {
				t.Fatalf("index %d appears twice", *it.SourceIndex)
			}
			seen[*it.SourceIndex] = true
		}
	})

	t.Run("an invalid index in the ranking is skipped rather than trusted", func(t *testing.T) {
		items := SeedRankedItems(highlights, 2, []int{99, -1, 0, 0, 1})
		if len(items) != 5 {
			t.Fatalf("len(items) = %d, want 5", len(items))
		}
		if items[0].SourceIndex == nil || *items[0].SourceIndex != 0 {
			t.Errorf("items[0].SourceIndex = %v, want 0 (bad indices skipped)", items[0].SourceIndex)
		}
	})
}
