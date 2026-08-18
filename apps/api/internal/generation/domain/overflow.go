package domain

import (
	"sort"
	"strconv"
	"strings"
)

const candidatesPerPageOver = 5

type OverflowCandidate struct {
	ItemID    string
	SectionID string
	Label     string
	Rank      int
}

func OverflowCandidates(sections []Section, over int) []OverflowCandidate {
	if over <= 0 {
		return nil
	}

	type entry struct {
		candidate    OverflowCandidate
		sectionOrder int
		itemOrder    int
	}
	var entries []entry
	for _, sec := range sections {
		for _, it := range selectedInOrder(sec) {
			entries = append(entries, entry{
				candidate: OverflowCandidate{
					ItemID:    it.ID,
					SectionID: sec.ID,
					Label:     candidateLabel(sec, it),
					Rank:      it.Rank,
				},
				sectionOrder: sec.Position,
				itemOrder:    it.Position,
			})
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].candidate.Rank != entries[j].candidate.Rank {
			return entries[i].candidate.Rank > entries[j].candidate.Rank
		}
		if entries[i].sectionOrder != entries[j].sectionOrder {
			return entries[i].sectionOrder > entries[j].sectionOrder
		}
		return entries[i].itemOrder > entries[j].itemOrder
	})

	limit := over * candidatesPerPageOver
	if limit > len(entries) {
		limit = len(entries)
	}
	out := make([]OverflowCandidate, 0, limit)
	for _, e := range entries[:limit] {
		out = append(out, e.candidate)
	}
	return out
}

func candidateLabel(sec Section, it Item) string {
	switch sec.Kind {
	case SectionKindSummary:
		return "Summary"
	case SectionKindSkills:
		label, _, _ := strings.Cut(it.EffectiveText(), ":")
		return "Skills · " + strings.TrimSpace(label)
	case SectionKindProjects:
		name, _, _ := strings.Cut(it.EffectiveText(), " · ")
		return "Projects · " + strings.TrimSpace(name)
	default:
		entry := ""
		if sec.EntryKey != nil {
			entry = *sec.EntryKey
		}
		bullet := "bullet " + strconv.Itoa(it.Position+1)
		if entry == "" {
			return bullet
		}
		return entry + " · " + bullet
	}
}
