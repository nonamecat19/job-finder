package domain

import (
	"sort"
	"strconv"
	"strings"
)

// candidatesPerPageOver is how many drop candidates a report names per page
// of overflow. The user acts on these, so the list has to be short enough to
// read and long enough to actually recover a page.
const candidatesPerPageOver = 5

// OverflowCandidate is one named item the user could deselect to fit the page
// budget. It is a suggestion and nothing else: FR-019 requires the overflow to
// be reported with candidates, never resolved.
type OverflowCandidate struct {
	ItemID    string
	SectionID string
	Label     string
	Rank      int
}

// OverflowCandidates returns the lowest-ranked selected items, worst-ranked
// first — the answer FR-019 asks for, which `rank` already holds, so this is a
// sort rather than a heuristic. `over` is how many pages the render came out
// over target; a non-positive value means there is nothing to report.
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

// candidateLabel names an item the way the workspace shows it, so the user can
// find the thing being suggested: "Acme Inc. · bullet 8".
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
