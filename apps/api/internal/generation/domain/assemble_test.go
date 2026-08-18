package domain

import (
	"reflect"
	"testing"
)

func assembleMaster() RendercvMaster {
	return RendercvMaster{
		"cv": map[string]any{
			"name": "Ada Lovelace",
			"sections": map[string]any{
				"summary": []any{"Master summary."},
				"experience": []any{
					map[string]any{
						"company": "Acme Inc.", "position": "Senior Engineer",
						"start_date": "2021-01", "end_date": "2024-06", "location": "Remote",
						"highlights": []any{"Shipped the thing", "Fixed the other thing", "Wrote the docs"},
					},
					map[string]any{
						"company": "Globex", "position": "Engineer",
						"start_date": "2018-03", "end_date": "2020-12",
						"highlights": []any{"Built the pipeline", "Cut costs 30%"},
					},
				},
				"education": []any{
					map[string]any{"institution": "State University", "degree": "BSc", "area": "Computer Science"},
				},
				"skills": []any{
					map[string]any{"label": "Languages", "details": "Go, TypeScript"},
					map[string]any{"label": "Spoken Languages", "details": "English"},
				},
			},
		},
	}
}

func idx(i int) *int { return &i }

func str(s string) *string { return &s }

func assembleSections() []Section {
	return []Section{
		{
			ID: "sec-summary", Kind: SectionKindSummary, Position: 0, Enabled: true,
			Items: []Item{{ID: "sum-1", Origin: OriginAI, Kind: ItemKindSummary, SourceText: "Written summary.", Position: 0, Selected: true}},
		},
		{
			ID: "sec-acme", Kind: SectionKindExperience, EntryKey: str("Acme Inc."), Position: 1, Enabled: true,
			Items: []Item{
				{ID: "a-0", Origin: OriginProfile, SourceIndex: idx(0), SourceText: "Shipped the thing", Rank: 0, Position: 1, Selected: true},
				{ID: "a-1", Origin: OriginProfile, SourceIndex: idx(1), SourceText: "Fixed the other thing", Rank: 1, Position: 2, Selected: false},
				{ID: "a-2", Origin: OriginProfile, SourceIndex: idx(2), SourceText: "Wrote the docs", Rank: 2, Position: 0, Selected: true},
			},
		},
		{
			ID: "sec-globex", Kind: SectionKindExperience, EntryKey: str("Globex"), Position: 2, Enabled: true,
			Items: []Item{
				{ID: "g-0", Origin: OriginProfile, SourceIndex: idx(0), SourceText: "Built the pipeline", Rank: 0, Position: 0, Selected: true, Unavailable: true},
				{ID: "g-1", Origin: OriginProfile, SourceIndex: idx(1), SourceText: "Cut costs 30%", Rank: 1, Position: 1, Selected: true},
			},
		},
		{
			ID: "sec-skills", Kind: SectionKindSkills, Position: 3, Enabled: true,
			Items: []Item{
				{ID: "s-0", Origin: OriginProfile, Kind: ItemKindSkillGroup, SourceIndex: idx(0), SourceText: "Languages: Go, TypeScript", Rank: 0, Position: 0, Selected: true},
				{ID: "s-1", Origin: OriginProfile, Kind: ItemKindSkillGroup, SourceIndex: idx(1), SourceText: "Spoken Languages: English", Rank: 1, Position: 1, Selected: false},
			},
		},
	}
}

func assembledHighlights(t *testing.T, doc RendercvMaster, company string) []string {
	t.Helper()
	for _, e := range AsSliceOfMaps(CvSections(doc)["experience"]) {
		if StringField(e, "company") == company {
			return StringSliceField(e, "highlights")
		}
	}
	t.Fatalf("no experience entry for %q in %v", company, doc)
	return nil
}

func TestAssembleEmitsSelectedItemsInPositionOrder(t *testing.T) {
	doc, err := Assemble(assembleMaster(), assembleSections())
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	got := assembledHighlights(t, doc, "Acme Inc.")
	want := []string{"Wrote the docs", "Shipped the thing"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Acme highlights = %v, want the selected two in position order %v", got, want)
	}
}

func TestAssembleOmitsUnselectedAndUnavailableItems(t *testing.T) {
	doc, err := Assemble(assembleMaster(), assembleSections())
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	for _, h := range assembledHighlights(t, doc, "Acme Inc.") {
		if h == "Fixed the other thing" {
			t.Errorf("unselected bullet %q reached the export", h)
		}
	}
	globex := assembledHighlights(t, doc, "Globex")
	if !reflect.DeepEqual(globex, []string{"Cut costs 30%"}) {
		t.Errorf("Globex highlights = %v, want the unavailable bullet dropped", globex)
	}

	var labels []string
	for _, g := range AsSliceOfMaps(CvSections(doc)["skills"]) {
		labels = append(labels, StringField(g, "label"))
	}
	if !reflect.DeepEqual(labels, []string{"Languages"}) {
		t.Errorf("skill groups = %v, want only the selected one", labels)
	}
}

func TestAssembleRemovesDisabledSectionsRegardlessOfKind(t *testing.T) {
	sections := assembleSections()
	for i := range sections {
		sections[i].Enabled = false
	}
	doc, err := Assemble(assembleMaster(), sections)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	cv := CvSections(doc)
	if _, ok := cv["summary"]; ok {
		t.Errorf("summary = %v, want the key removed", cv["summary"])
	}
	if _, ok := cv["skills"]; ok {
		t.Errorf("skills = %v, want the key removed", cv["skills"])
	}

	if _, ok := cv["experience"]; ok {
		t.Errorf("experience = %v, want the key removed once every entry is disabled", cv["experience"])
	}
}

func TestAssembleRemovesOneDisabledExperienceEntry(t *testing.T) {
	sections := assembleSections()
	for i := range sections {
		if sections[i].Kind == SectionKindExperience && sections[i].EntryKey != nil && *sections[i].EntryKey == "Acme Inc." {
			sections[i].Enabled = false
		}
	}
	doc, err := Assemble(assembleMaster(), sections)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	var companies []string
	for _, e := range AsSliceOfMaps(CvSections(doc)["experience"]) {
		companies = append(companies, StringField(e, "company"))
	}
	if !reflect.DeepEqual(companies, []string{"Globex"}) {
		t.Errorf("companies = %v, want only Globex left", companies)
	}
}

func TestAssemblePreservesMasterExperienceOrder(t *testing.T) {
	sections := assembleSections()

	reversed := []Section{sections[3], sections[2], sections[1], sections[0]}

	doc, err := Assemble(assembleMaster(), reversed)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	var companies []string
	for _, e := range AsSliceOfMaps(CvSections(doc)["experience"]) {
		companies = append(companies, StringField(e, "company"))
	}
	if !reflect.DeepEqual(companies, []string{"Acme Inc.", "Globex"}) {
		t.Errorf("experience order = %v, want master order", companies)
	}
}

func TestAssemblePreservesOutOfScopeFieldsVerbatim(t *testing.T) {
	master := assembleMaster()
	doc, err := Assemble(master, assembleSections())
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	masterSections := CvSections(master)
	docSections := CvSections(doc)

	if !reflect.DeepEqual(docSections["education"], masterSections["education"]) {
		t.Errorf("education = %v, want it verbatim from the master %v", docSections["education"], masterSections["education"])
	}
	if got := StringField(doc["cv"].(map[string]any), "name"); got != "Ada Lovelace" {
		t.Errorf("cv.name = %q, want it carried through", got)
	}
	for _, e := range AsSliceOfMaps(docSections["experience"]) {
		if StringField(e, "company") != "Acme Inc." {
			continue
		}
		if StringField(e, "position") != "Senior Engineer" || StringField(e, "start_date") != "2021-01" ||
			StringField(e, "end_date") != "2024-06" || StringField(e, "location") != "Remote" {
			t.Errorf("Acme entry lost an out-of-scope field: %v", e)
		}
	}

	if got := assembledHighlights(t, master, "Acme Inc."); len(got) != 3 {
		t.Errorf("master highlights = %v, want the snapshot left alone", got)
	}
}

func TestAssembleWritesTheSelectedSummaryOnly(t *testing.T) {
	doc, err := Assemble(assembleMaster(), assembleSections())
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	summary, _ := CvSections(doc)["summary"].([]any)
	if len(summary) != 1 || summary[0] != "Written summary." {
		t.Errorf("summary = %v, want the one selected summary item", summary)
	}

	master := assembleMaster()
	delete(CvSections(master), "summary")
	doc, err = Assemble(master, assembleSections())
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if _, ok := CvSections(doc)["summary"]; ok {
		t.Error("Assemble created a summary section the master did not have")
	}
}

func TestOverflowCandidatesAreWorstRankedSelectedItemsFirst(t *testing.T) {
	got := OverflowCandidates(assembleSections(), 1)
	if len(got) == 0 {
		t.Fatal("no candidates for a one-page overflow")
	}

	if got[0].ItemID != "a-2" {
		t.Errorf("first candidate = %+v, want item a-2 (rank 2)", got[0])
	}
	if got[0].Label != "Acme Inc. · bullet 1" {
		t.Errorf("label = %q, want it to name the entry and the bullet", got[0].Label)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Rank < got[i].Rank {
			t.Errorf("candidates not worst-first at %d: %+v then %+v", i, got[i-1], got[i])
		}
	}
	for _, c := range got {
		if c.ItemID == "a-1" || c.ItemID == "s-1" || c.ItemID == "g-0" {
			t.Errorf("candidate %+v is not a selected, available item", c)
		}
	}

	if OverflowCandidates(assembleSections(), 0) != nil {
		t.Error("a document that fits reported drop candidates")
	}
}

func assembledSkillDetails(doc RendercvMaster, label string) string {
	for _, g := range AsSliceOfMaps(CvSections(doc)["skills"]) {
		if StringField(g, "label") == label {
			return StringField(g, "details")
		}
	}
	return ""
}

func TestAssembleDropsIndividualSkillEntries(t *testing.T) {
	sections := assembleSections()
	sections[3].Items[0].DroppedEntries = []string{"TypeScript"}

	doc, err := Assemble(assembleMaster(), sections)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if got := assembledSkillDetails(doc, "Languages"); got != "Go" {
		t.Errorf("Languages details = %q, want only the kept entry %q", got, "Go")
	}
}

func TestAssembleDropsAnEmptiedSkillGroupEntirely(t *testing.T) {
	sections := assembleSections()
	sections[3].Items[0].DroppedEntries = []string{"Go", "TypeScript"}

	doc, err := Assemble(assembleMaster(), sections)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if groups := AsSliceOfMaps(CvSections(doc)["skills"]); len(groups) != 0 {
		t.Errorf("skills = %v, want the emptied group gone", groups)
	}
}

func TestAssembleDoesNotMutateTheMasterSkillGroup(t *testing.T) {
	master := assembleMaster()
	sections := assembleSections()
	sections[3].Items[0].DroppedEntries = []string{"TypeScript"}

	if _, err := Assemble(master, sections); err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if got := assembledSkillDetails(master, "Languages"); got != "Go, TypeScript" {
		t.Errorf("master Languages details = %q, want it unchanged", got)
	}
}

func TestSkillEntriesReportsPerEntrySelection(t *testing.T) {
	it := Item{
		Origin: OriginProfile, Kind: ItemKindSkillGroup,
		SourceText:     "Languages: Go, TypeScript, SQL",
		DroppedEntries: []string{"TypeScript"},
	}

	got := it.SkillEntries()
	want := []SkillEntry{{Text: "Go", Selected: true}, {Text: "TypeScript", Selected: false}, {Text: "SQL", Selected: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SkillEntries() = %+v, want %+v", got, want)
	}
}
