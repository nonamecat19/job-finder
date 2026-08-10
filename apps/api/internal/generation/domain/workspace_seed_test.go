package domain

import (
	"fmt"
	"testing"
)

func TestSeedFromMaster_EveryMasterBulletAppearsExactlyOnce(t *testing.T) {
	m := masterWithBullets(map[string]int{"Acme": 12, "Globex": 3}, "Acme", "Globex")
	cfg := DefaultShapeConfig()

	sections := SeedFromMaster(m, cfg)

	seen := map[string]int{}
	for _, s := range sections {
		if s.Kind != SectionKindExperience {
			continue
		}
		for _, it := range s.Items {
			seen[it.SourceText]++
		}
	}
	for _, company := range []string{"Acme", "Globex"} {
		n := 12
		if company == "Globex" {
			n = 3
		}
		for i := 0; i < n; i++ {
			text := highlightText(company, i)
			if got := seen[text]; got != 1 {
				t.Errorf("bullet %q appears %d times, want 1", text, got)
			}
		}
	}
}

func TestSeedFromMaster_EntryOrderEqualsMasterOrder(t *testing.T) {
	m := masterWithBullets(map[string]int{"Acme": 2, "Globex": 2, "Initech": 2}, "Acme", "Globex", "Initech")
	cfg := DefaultShapeConfig()

	sections := SeedFromMaster(m, cfg)

	var gotOrder []string
	for _, s := range sections {
		if s.Kind == SectionKindExperience {
			gotOrder = append(gotOrder, *s.EntryKey)
		}
	}
	want := []string{"Acme", "Globex", "Initech"}
	if len(gotOrder) != len(want) {
		t.Fatalf("entry order = %v, want %v", gotOrder, want)
	}
	for i, c := range want {
		if gotOrder[i] != c {
			t.Errorf("entry order[%d] = %q, want %q", i, gotOrder[i], c)
		}
	}
}

func TestSeedFromMaster_ZeroBulletEntrySeedsEmptySectionNotSkipped(t *testing.T) {
	m := masterWithBullets(map[string]int{"Acme": 0, "Globex": 2}, "Acme", "Globex")
	cfg := DefaultShapeConfig()

	sections := SeedFromMaster(m, cfg)

	var found bool
	for _, s := range sections {
		if s.Kind == SectionKindExperience && s.EntryKey != nil && *s.EntryKey == "Acme" {
			found = true
			if len(s.Items) != 0 {
				t.Errorf("Acme items = %d, want 0", len(s.Items))
			}
		}
	}
	if !found {
		t.Fatal("Acme section was skipped entirely, want an empty section")
	}
}

func TestSeedFromMaster_TopMinNASelected(t *testing.T) {
	m := masterWithBullets(map[string]int{"Acme": 12}, "Acme")
	cfg := DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 4

	sections := SeedFromMaster(m, cfg)

	for _, s := range sections {
		if s.Kind != SectionKindExperience {
			continue
		}
		selected := 0
		for i, it := range s.Items {
			if it.Selected != (i < 4) {
				t.Errorf("item %d selected = %v, want %v", i, it.Selected, i < 4)
			}
			if it.Selected {
				selected++
			}
		}
		if selected != 4 {
			t.Errorf("selected = %d, want 4", selected)
		}
	}
}

func TestSeedFromMaster_SelectsAllWhenFewerThanMin(t *testing.T) {
	m := masterWithBullets(map[string]int{"Acme": 2}, "Acme")
	cfg := DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 8

	sections := SeedFromMaster(m, cfg)

	for _, s := range sections {
		if s.Kind != SectionKindExperience {
			continue
		}
		if len(s.Items) != 2 {
			t.Fatalf("items = %d, want 2", len(s.Items))
		}
		for i, it := range s.Items {
			if !it.Selected {
				t.Errorf("item %d selected = false, want true (fewer bullets than min)", i)
			}
		}
	}
}

func TestSeedFromMaster_SummaryAndSkillsSections(t *testing.T) {
	m := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"skills": []any{
			map[string]any{"label": "Languages", "details": "Go, TypeScript"},
			map[string]any{"label": "Cloud", "details": "AWS, GCP"},
		},
	}}}
	cfg := DefaultShapeConfig()

	sections := SeedFromMaster(m, cfg)

	var summaryCount, skillsCount int
	for _, s := range sections {
		switch s.Kind {
		case SectionKindSummary:
			summaryCount++
			if len(s.Items) != 0 {
				t.Errorf("summary section items = %d, want 0 (filled by the summary stage)", len(s.Items))
			}
		case SectionKindSkills:
			skillsCount++
			if len(s.Items) != 2 {
				t.Fatalf("skills items = %d, want 2", len(s.Items))
			}
			if s.Items[0].SourceText != "Languages: Go, TypeScript" {
				t.Errorf("skills item 0 text = %q", s.Items[0].SourceText)
			}
			if !s.Items[0].Selected || !s.Items[1].Selected {
				t.Error("skill group items should be selected by default in phase 2")
			}
		}
	}
	if summaryCount != 1 {
		t.Errorf("summary sections = %d, want 1", summaryCount)
	}
	if skillsCount != 1 {
		t.Errorf("skills sections = %d, want 1", skillsCount)
	}
}

func highlightText(company string, i int) string {
	return fmt.Sprintf("%s bullet %d", company, i)
}
