package domain

import (
	"sort"
	"testing"
)

func skillsMaster(groups ...[2]string) RendercvMaster {
	raw := make([]any, 0, len(groups))
	for _, g := range groups {
		raw = append(raw, map[string]any{"label": g[0], "details": g[1]})
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{"skills": raw}}}
}

func skillsSectionItems(t *testing.T, sections []Section) []Item {
	t.Helper()
	for _, s := range sections {
		if s.Kind == SectionKindSkills {
			return s.Items
		}
	}
	t.Fatal("no skills section was seeded")
	return nil
}

func masterGroups(m RendercvMaster) []map[string]any {
	return AsSliceOfMaps(CvSections(m)["skills"])
}

func assertPermutation(t *testing.T, groups []map[string]any, items []Item) {
	t.Helper()
	if len(items) != len(groups) {
		t.Fatalf("items = %d, want %d (one per master group)", len(items), len(groups))
	}
	want := make([]string, 0, len(groups))
	for _, g := range groups {
		want = append(want, skillGroupText(g))
	}
	got := make([]string, 0, len(items))
	for _, it := range items {
		got = append(got, it.SourceText)
	}
	sort.Strings(want)
	sort.Strings(got)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("skill items are not a permutation of the master's groups:\n got %v\nwant %v", got, want)
		}
	}
	seenIndex := map[int]bool{}
	for _, it := range items {
		if it.SourceIndex == nil {
			t.Fatalf("item %q has no source index", it.SourceText)
		}
		if seenIndex[*it.SourceIndex] {
			t.Fatalf("source index %d used twice", *it.SourceIndex)
		}
		seenIndex[*it.SourceIndex] = true
		if it.Origin != OriginProfile {
			t.Errorf("item %q origin = %q, want profile", it.SourceText, it.Origin)
		}
	}
}

func TestSeedFromMaster_SkillsAreAPermutationOfTheMastersGroups(t *testing.T) {
	m := skillsMaster(
		[2]string{"Languages", "Go, TypeScript"},
		[2]string{"Cloud", "AWS, GCP"},
		[2]string{"Data", "Postgres, Redis"},
	)

	items := skillsSectionItems(t, SeedFromMaster(m, DefaultShapeConfig()))

	assertPermutation(t, masterGroups(m), items)
	for i, it := range items {
		if !it.Selected {
			t.Errorf("item %d selected = false, want true (no cap configured)", i)
		}
	}
}

func TestSeedSkillItems_RankedOrderIsStillAPermutation(t *testing.T) {
	m := skillsMaster(
		[2]string{"Languages", "Go, TypeScript"},
		[2]string{"Cloud", "AWS, GCP"},
		[2]string{"Data", "Postgres, Redis"},
	)
	groups := masterGroups(m)

	items := SeedSkillItems(groups, []int{2, 0, 1}, 0)

	assertPermutation(t, groups, items)
	want := []string{"Data: Postgres, Redis", "Languages: Go, TypeScript", "Cloud: AWS, GCP"}
	for i, w := range want {
		if items[i].SourceText != w {
			t.Errorf("item %d = %q, want %q", i, items[i].SourceText, w)
		}
	}
}

func TestSeedSkillItems_PartialOrderKeepsOmittedGroupsInMasterOrder(t *testing.T) {
	m := skillsMaster(
		[2]string{"Languages", "Go"},
		[2]string{"Cloud", "AWS"},
		[2]string{"Data", "Postgres"},
	)
	groups := masterGroups(m)

	items := SeedSkillItems(groups, []int{2, 9, -1, 2}, 0)

	assertPermutation(t, groups, items)
	want := []string{"Data: Postgres", "Languages: Go", "Cloud: AWS"}
	for i, w := range want {
		if items[i].SourceText != w {
			t.Errorf("item %d = %q, want %q", i, items[i].SourceText, w)
		}
	}
}

func TestSeedSkillItems_MaxGroupsDeselectsRatherThanRemoves(t *testing.T) {
	m := skillsMaster(
		[2]string{"Languages", "Go"},
		[2]string{"Cloud", "AWS"},
		[2]string{"Data", "Postgres"},
		[2]string{"Tooling", "Git"},
	)
	groups := masterGroups(m)

	items := SeedSkillItems(groups, []int{3, 1, 0, 2}, 2)

	assertPermutation(t, groups, items)
	wantSelected := []bool{true, true, false, false}
	for i, w := range wantSelected {
		if items[i].Selected != w {
			t.Errorf("item %d (%q) selected = %v, want %v", i, items[i].SourceText, items[i].Selected, w)
		}
	}
}

func TestSeedFromMaster_MaxGroupsDeselectsRatherThanRemoves(t *testing.T) {
	m := skillsMaster(
		[2]string{"Languages", "Go"},
		[2]string{"Cloud", "AWS"},
		[2]string{"Data", "Postgres"},
	)
	cfg := DefaultShapeConfig()
	cfg.SkillsMaxGroups = 1

	items := skillsSectionItems(t, SeedFromMaster(m, cfg))

	assertPermutation(t, masterGroups(m), items)
	selected := 0
	for _, it := range items {
		if it.Selected {
			selected++
		}
	}
	if selected != 1 {
		t.Errorf("selected = %d, want 1 (the cap deselects, it does not remove)", selected)
	}
}

func TestSeedSkillItems_PinnedGroupStaysWhereAuthoredAndSelected(t *testing.T) {
	m := skillsMaster(
		[2]string{"Languages", "Go"},
		[2]string{"Spoken Languages", "English, Ukrainian"},
		[2]string{"Cloud", "AWS"},
		[2]string{"Data", "Postgres"},
	)
	groups := masterGroups(m)

	items := SeedSkillItems(groups, []int{3, 2, 0, 1}, 2)

	assertPermutation(t, groups, items)
	if items[1].SourceText != "Spoken Languages: English, Ukrainian" {
		t.Errorf("pinned group moved to another position: item 1 = %q", items[1].SourceText)
	}
	if !items[1].Selected {
		t.Error("pinned group was deselected by the cap, want it kept regardless")
	}

	wantSelected := []bool{true, true, false, false}
	for i, w := range wantSelected {
		if items[i].Selected != w {
			t.Errorf("item %d (%q) selected = %v, want %v", i, items[i].SourceText, items[i].Selected, w)
		}
	}
}

func TestVerifySkillGroupOrder(t *testing.T) {
	cases := []struct {
		name   string
		count  int
		order  []int
		kinds  []RankingViolationKind
		wantOK bool
	}{
		{name: "complete order", count: 3, order: []int{2, 0, 1}, wantOK: true},
		{name: "out of range", count: 3, order: []int{0, 1, 3}, kinds: []RankingViolationKind{RankingOutOfRange}},
		{name: "duplicate", count: 3, order: []int{0, 1, 1}, kinds: []RankingViolationKind{RankingDuplicate}},
		{name: "duplicate leaving a group unnamed", count: 4, order: []int{0, 1, 1}, kinds: []RankingViolationKind{RankingDuplicate, RankingShort}},
		{name: "omits a group", count: 3, order: []int{0, 1}, kinds: []RankingViolationKind{RankingShort}},
		{name: "empty master", count: 0, order: nil, wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := VerifySkillGroupOrder(tc.count, tc.order)
			if tc.wantOK {
				if len(got) != 0 {
					t.Fatalf("violations = %v, want none", got)
				}
				return
			}
			if len(got) != len(tc.kinds) {
				t.Fatalf("violations = %v, want %d", got, len(tc.kinds))
			}
			for i, k := range tc.kinds {
				if got[i].Kind != k {
					t.Errorf("violation %d kind = %q, want %q", i, got[i].Kind, k)
				}
			}
		})
	}
}
