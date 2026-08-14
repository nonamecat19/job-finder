package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/queue"
)

// defaultRunListLimit bounds `GET /v1/generations` when the caller sends no
// limit, matching the "recent runs" framing of the contract rather than
// returning the whole history.
const defaultRunListLimit = 20

// SetAsynqClient installs the queue client the workspace uses to enqueue a
// run's background processing (StartRun, below). A setter rather than a
// NewService parameter for the same reason SetSummaryModelProvider is: it
// keeps every existing call to NewService, and every test that constructs a
// Service without exercising this feature, unchanged.
func (s *Service) SetAsynqClient(c *asynq.Client) { s.asynqClient = c }

// StartGenerationRun is the synchronous half of `POST /v1/generations`
// (rest-api.md): it resolves the profile and vacancy, resolves ShapeConfig /
// grounding level / summary option once (resume-generation.md § 4.2), snapshots the
// master and its content hash, persists the run row and its master-order
// seeded sections/items (SeedFromMaster, T009 — no LLM call), then enqueues
// the background half (StartRun) on the existing `generate` queue.
//
// The row must exist before this returns: the client is handed a runId it
// immediately polls with GET /v1/generations/{runId}, so seeding happens here
// rather than in the worker.
func (s *Service) StartGenerationRun(ctx context.Context, req dto.StartGenerationRequestDto) (runID, activityID string, err error) {
	if strings.TrimSpace(req.ProfileID) == "" {
		return "", "", apperr.Validation("profileId is required")
	}
	hasJob := req.JobID != nil && *req.JobID != ""
	hasVacancy := req.Vacancy != nil && strings.TrimSpace(req.Vacancy.Text) != ""
	if !hasJob && !hasVacancy {
		return "", "", apperr.Validation("either jobId or vacancy.text is required")
	}

	profUUID, err := dbutil.ParseUUID(req.ProfileID)
	if err != nil {
		return "", "", apperr.NotFound("profile", req.ProfileID)
	}
	prof, err := s.profiles.Get(ctx, req.ProfileID)
	if err != nil {
		return "", "", apperr.NotFound("profile", req.ProfileID)
	}
	if prof.RendercvConfig == nil {
		return "", "", apperr.Validation("profile has no master content")
	}
	master, err := domain.MasterFromProfile(prof)
	if err != nil || master == nil {
		return "", "", apperr.Validation("profile has no master content")
	}

	var (
		jobUUID     pgtype.UUID
		vacancyText string
		company     *string
		title       *string
	)
	if hasJob {
		jid, err := dbutil.ParseUUID(*req.JobID)
		if err != nil {
			return "", "", apperr.NotFound("job", *req.JobID)
		}
		job, err := s.q.GetJobByID(ctx, jid)
		if err != nil {
			return "", "", apperr.NotFound("job", *req.JobID)
		}
		jobUUID = jid
		vacancyText = job.Description
		c, t := job.Company, job.Title
		company, title = &c, &t
	} else {
		vacancyText = req.Vacancy.Text
		if req.Vacancy.Company != "" {
			c := req.Vacancy.Company
			company = &c
		}
		if req.Vacancy.Title != "" {
			t := req.Vacancy.Title
			title = &t
		}
	}

	// Resolved once, at the top of the run: shapeConfig/summaryOption already
	// follow this discipline (service.go:120/:177); the workspace run
	// snapshots the result so a later settings change cannot alter a run the
	// user has already started reviewing (resume-generation.md § 4.2).
	cfg := s.shapeConfig(ctx)
	level := s.defaultLevel
	if req.GroundingLevel != nil {
		level = domain.ParseGroundingLevel(*req.GroundingLevel)
	}
	var summaryOptionID *string
	if req.SummaryOptionID != nil {
		opt, ok := domain.LookupSummaryOption(*req.SummaryOptionID)
		if !ok {
			return "", "", apperr.Validation("unknown summary model option: " + *req.SummaryOptionID)
		}
		summaryOptionID = &opt.ID
	}

	snapshot, err := json.Marshal(map[string]any(master))
	if err != nil {
		return "", "", err
	}
	hash, err := domain.ContentHash(master)
	if err != nil {
		return "", "", err
	}
	shapeJSON, err := json.Marshal(cfg)
	if err != nil {
		return "", "", err
	}

	run, err := s.q.CreateRun(ctx, sqlcgen.CreateRunParams{
		ProfileID:         profUUID,
		VacancyText:       vacancyText,
		MasterSnapshot:    snapshot,
		MasterContentHash: hash,
		ShapeConfig:       shapeJSON,
		GroundingLevel:    string(level),
		JobID:             jobUUID,
		VacancyCompany:    company,
		VacancyTitle:      title,
		SummaryOptionID:   summaryOptionID,
	})
	if err != nil {
		return "", "", err
	}

	seeded := domain.SeedFromMaster(master, cfg)
	if err := s.persistWorkspaceSections(ctx, run.ID, seeded); err != nil {
		errMsg := err.Error()
		_ = s.q.SetRunState(ctx, sqlcgen.SetRunStateParams{ID: run.ID, State: string(domain.RunFailed)})
		_ = errMsg
		return "", "", err
	}

	runIDStr := dbutil.UUIDString(run.ID)
	label := "generation workspace"
	if company != nil && title != nil {
		label = *company + " — " + *title
	}
	rec := activity.New(ctx, s.q, "generate_workspace", label, req.JobID, nil, "")
	var actID *string
	if rec != nil {
		idStr := dbutil.UUIDString(rec.ID())
		actID = &idStr
		rec.Step(ctx, "queued", map[string]any{"runId": runIDStr})
	}

	if s.asynqClient != nil {
		payload, err := json.Marshal(queue.GeneratePayload{
			JobID:           derefOrEmpty(req.JobID),
			Type:            string(dto.DocumentTypeResume),
			ProfileID:       &req.ProfileID,
			ActivityID:      actID,
			GenerationRunID: &runIDStr,
		})
		if err != nil {
			return "", "", err
		}
		genOpts := []asynq.Option{asynq.MaxRetry(0), asynq.Queue(queue.QueueGenerate)}
		if actID != nil {
			genOpts = append(genOpts, asynq.TaskID(*actID))
		}
		if _, err := s.asynqClient.EnqueueContext(ctx, asynq.NewTask(queue.TypeGenerate, payload), genOpts...); err != nil {
			return "", "", err
		}
	}

	if actID != nil {
		return runIDStr, *actID, nil
	}
	return runIDStr, "", nil
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// persistWorkspaceSections writes SeedFromMaster's output: one CreateSections
// batch call for every section, then one CreateItems call per section that
// has items (summary starts empty — it is filled by StartRun below). Sections
// are matched back to their domain.Section by position, since a batch INSERT
// ... SELECT unnest(...) RETURNING gives no order guarantee.
func (s *Service) persistWorkspaceSections(ctx context.Context, runID pgtype.UUID, secs []domain.Section) error {
	n := len(secs)
	kinds := make([]string, n)
	entryKeys := make([]string, n)
	entryLabels := make([]string, n)
	positions := make([]int32, n)
	targetCounts := make([]int32, n)
	for i, sec := range secs {
		kinds[i] = string(sec.Kind)
		if sec.EntryKey != nil {
			entryKeys[i] = *sec.EntryKey
		}
		if sec.EntryLabel != nil {
			entryLabels[i] = *sec.EntryLabel
		}
		positions[i] = int32(sec.Position)
		targetCounts[i] = int32(sec.TargetCount)
	}

	created, err := s.q.CreateSections(ctx, sqlcgen.CreateSectionsParams{
		RunID: runID, Kinds: kinds, EntryKeys: entryKeys, EntryLabels: entryLabels,
		Positions: positions, TargetCounts: targetCounts,
	})
	if err != nil {
		return err
	}
	byPosition := make(map[int32]sqlcgen.GenerationSection, len(created))
	for _, row := range created {
		byPosition[row.Position] = row
	}

	for _, sec := range secs {
		row, ok := byPosition[int32(sec.Position)]
		if !ok {
			continue
		}
		if len(sec.Items) > 0 {
			if err := s.persistWorkspaceItems(ctx, row.ID, sec.Items); err != nil {
				return err
			}
		}
		if sec.State != "" && sec.State != domain.SectionRunning {
			if err := s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{
				ID: row.ID, State: string(sec.State), FallbackUsed: sec.FallbackUsed, Error: sec.Error,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// persistWorkspaceItems is the same zipped-array batch insert pattern as
// persistWorkspaceSections, for one section's items.
func (s *Service) persistWorkspaceItems(ctx context.Context, sectionID pgtype.UUID, items []domain.Item) error {
	n := len(items)
	origins := make([]string, n)
	sourceIndexes := make([]int32, n)
	sourceTexts := make([]string, n)
	editedTexts := make([]string, n)
	ranks := make([]int32, n)
	positions := make([]int32, n)
	selected := make([]bool, n)
	for i, it := range items {
		origins[i] = string(it.Origin)
		if it.SourceIndex != nil {
			sourceIndexes[i] = int32(*it.SourceIndex)
		} else {
			sourceIndexes[i] = -1
		}
		sourceTexts[i] = it.SourceText
		if it.EditedText != nil {
			editedTexts[i] = *it.EditedText
		}
		ranks[i] = int32(it.Rank)
		positions[i] = int32(it.Position)
		selected[i] = it.Selected
	}
	_, err := s.q.CreateItems(ctx, sqlcgen.CreateItemsParams{
		SectionID: sectionID, Origins: origins, SourceIndexes: sourceIndexes, SourceTexts: sourceTexts,
		EditedTexts: editedTexts, Ranks: ranks, Positions: positions, SelectedFlags: selected,
	})
	return err
}

// rankedSectionResult is one experience section's outcome from the ranking
// stage (T043): either a verified ranking ready to replace the master-order
// seed, or a signal to leave the section exactly as StartGenerationRun's
// SeedFromMaster already left it — master order, top min(N, A) selected —
// and record that the FR-010 fallback fired. That seed IS the fallback
// (resume-generation.md § 2b: "ranking = [0,1,…,K-1]" is master order), so a fallback
// entry has no items to persist.
type rankedSectionResult struct {
	entryKey     string
	items        []domain.Item // nil when falling back to the existing master-order seed
	fallbackUsed bool
}

// rankExperienceSections runs the ranking stage once for every master
// experience entry in a single call (mirroring buildSelectPrompt/selectContent,
// which also rank/select every entry together), verifies each entry's
// ranking independently, retries the whole call once when any entry is
// invalid (the existing groundingAttempts idiom — service.go's retry ladders
// all cap at 2 attempts), and falls back to master order per entry rather
// than failing the run (FR-010).
//
// No I/O beyond the two LLM calls: it takes the master and analysis already
// resolved by the caller and returns a decision per entry, so it can be
// tested without a database (T038).
// It also returns the verified skill-group order from the same response
// (T060) — one call ranks both surfaces, so pulling the skills order out here
// costs nothing and keeps the two decisions on the same attempt. A nil order
// means no attempt verified, and the caller leaves the master-order skills
// seed in place, exactly as it does for a fallback experience entry.
func rankExperienceSections(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, cfg domain.ShapeConfig) ([]rankedSectionResult, []int) {
	sections := domain.CvSections(master)
	experience := domain.AsSliceOfMaps(sections["experience"])
	skillGroupCount := len(domain.AsSliceOfMaps(sections["skills"]))
	if len(experience) == 0 {
		return nil, nil
	}

	type entry struct {
		company    string
		highlights []string
	}
	entries := make([]entry, len(experience))
	for i, e := range experience {
		entries[i] = entry{
			company:    domain.StringField(e, "company"),
			highlights: domain.StringSliceField(e, "highlights"),
		}
	}

	// findRanking matches by containment, not just equality: the prompt asks
	// for the EXACT company name, but a model that echoes the whole "company
	// (position) | location" header line back (observed with the entry-line
	// rendering renderExperienceEntryLines produces, shared with the select
	// prompt) still names the right entry — a stricter match would turn a
	// cosmetic prompt-following slip into an unnecessary FR-010 fallback.
	findRanking := func(sel domain.RankedSelection, company string) []int {
		target := strings.ToLower(strings.TrimSpace(company))
		for _, re := range sel.Experience {
			if strings.Contains(strings.ToLower(strings.TrimSpace(re.Company)), target) {
				return re.Ranking
			}
		}
		return nil
	}

	type attempt struct {
		ranking []int
		valid   bool
	}
	evaluate := func(sel domain.RankedSelection, callErr error, e entry) attempt {
		if callErr != nil {
			return attempt{}
		}
		ranking := findRanking(sel, e.company)
		return attempt{ranking: ranking, valid: len(domain.VerifyRanking(len(e.highlights), cfg.ExperienceBulletsMin, ranking)) == 0}
	}

	// A skill-group order is verified on the same footing as an achievement
	// ranking (T059): every group index exactly once, or the attempt is
	// rejected and retried like any other. A master with no skill groups has
	// nothing to order, so it is trivially valid.
	skillOrderOf := func(sel domain.RankedSelection, callErr error) []int {
		if callErr != nil || skillGroupCount == 0 {
			return nil
		}
		order := sel.Skills.GroupOrder
		if len(domain.VerifySkillGroupOrder(skillGroupCount, order)) > 0 {
			return nil
		}
		return order
	}

	first, firstErr := rankContent(ctx, lc, model, master, analysis, cfg, nil)
	firstAttempts := make([]attempt, len(entries))
	var needRetry []string
	for i, e := range entries {
		firstAttempts[i] = evaluate(first, firstErr, e)
		if !firstAttempts[i].valid {
			needRetry = append(needRetry, e.company)
		}
	}
	skillOrder := skillOrderOf(first, firstErr)
	skillsRejected := skillGroupCount > 0 && skillOrder == nil

	var second domain.RankedSelection
	secondErr := fmt.Errorf("ranking retry not attempted")
	if len(needRetry) > 0 || skillsRejected {
		violationMsgs := make([]string, 0, len(needRetry)+1)
		for _, c := range needRetry {
			violationMsgs = append(violationMsgs, fmt.Sprintf("the ranking for company %q was rejected: it must name exactly K distinct in-range indices", c))
		}
		if skillsRejected {
			violationMsgs = append(violationMsgs, fmt.Sprintf("the skill groupOrder was rejected: it must name each of the %d skill group indices exactly once", skillGroupCount))
		}
		second, secondErr = rankContent(ctx, lc, model, master, analysis, cfg, violationMsgs)
		if skillsRejected {
			skillOrder = skillOrderOf(second, secondErr)
		}
	}

	results := make([]rankedSectionResult, len(entries))
	for i, e := range entries {
		if firstAttempts[i].valid {
			results[i] = rankedSectionResult{entryKey: e.company, items: domain.SeedRankedItems(e.highlights, cfg.ExperienceBulletsMin, firstAttempts[i].ranking)}
			continue
		}
		retry := evaluate(second, secondErr, e)
		if retry.valid {
			results[i] = rankedSectionResult{entryKey: e.company, items: domain.SeedRankedItems(e.highlights, cfg.ExperienceBulletsMin, retry.ranking)}
			continue
		}
		results[i] = rankedSectionResult{entryKey: e.company, fallbackUsed: true}
	}
	return results, skillOrder
}

// applyRankedSections persists rankExperienceSections' decisions: a section
// with a verified ranking has its master-order seed replaced (delete then
// recreate, the same pattern a section rerun uses); a fallback section is
// left untouched and only has fallback_used recorded, per data-model.md §4's
// state transition ("rejected twice -> ready (fallback_used = true)").
//
// oldBySection carries a section's pre-replacement items, keyed by section
// id, so preserveMatchedSelections (T077) can carry a matched item's
// selected/position/edited_text into its replacement. A plain run (not a
// rerun) has nothing to preserve and passes nil — every lookup on a nil map
// returns the zero value, so preserveMatchedSelections is a no-op identity
// in that case.
//
// A section whose replacement items fail to persist is marked `failed` with
// the error, rather than left silently emptied or silently stale (T081): the
// delete already committed, so the old items are gone either way, and
// `failed` is what tells the client the section needs the per-section retry
// rather than showing an inexplicably-empty block. The caller folds the
// returned anyFailed into the run's final state (`partial`, not `ready`).
// A DeleteSectionItems failure, by contrast, leaves the section's existing
// items completely untouched — nothing was lost, so the section keeps
// whatever state it already had and is not counted as a failure.
func (s *Service) applyRankedSections(ctx context.Context, runID pgtype.UUID, results []rankedSectionResult, oldBySection map[pgtype.UUID][]domain.Item) (anyFailed bool) {
	if len(results) == 0 {
		return false
	}
	sections, err := s.q.ListSectionsByRun(ctx, runID)
	if err != nil {
		return true
	}
	byEntryKey := make(map[string]sqlcgen.GenerationSection, len(sections))
	for _, sec := range sections {
		if sec.Kind == string(domain.SectionKindExperience) && sec.EntryKey != nil {
			byEntryKey[*sec.EntryKey] = sec
		}
	}

	for _, r := range results {
		sec, ok := byEntryKey[r.entryKey]
		if !ok {
			continue
		}
		if r.items != nil {
			items := preserveMatchedSelections(oldBySection[sec.ID], r.items)
			if err := s.q.DeleteSectionItems(ctx, sec.ID); err != nil {
				// Nothing lost: the section's prior items are exactly as
				// they were before this attempt. Leave its state alone.
				continue
			}
			if err := s.persistWorkspaceItems(ctx, sec.ID, items); err != nil {
				anyFailed = true
				msg := "ranking persistence failed: " + err.Error()
				_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: sec.ID, State: string(domain.SectionFailed), Error: &msg})
				continue
			}
		}
		if err := s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{
			ID: sec.ID, State: string(domain.SectionReady), FallbackUsed: r.fallbackUsed,
		}); err != nil {
			anyFailed = true
		}
	}
	return anyFailed
}

// preserveMatchedSelections is data-model.md §4's rerun rule: a profile item
// is matched to its replacement by SourceIndex, an AI item by
// domain.NormalizeText(SourceText); a matched item's Selected/Position and
// (for an AI item) EditedText survive into the replacement. An old item with
// no match in the new set is simply not carried forward — that is what
// "re-running replaces the AI's ordering for this section" means (FR-021). A
// new item with no match keeps the default state its seeder gave it.
//
// oldItems empty (nil, for a plain run with nothing to preserve, or a
// section whose old items don't exist yet) makes this the identity function
// on newItems.
func preserveMatchedSelections(oldItems, newItems []domain.Item) []domain.Item {
	if len(oldItems) == 0 {
		return newItems
	}
	byIndex := make(map[int]domain.Item, len(oldItems))
	byText := make(map[string]domain.Item, len(oldItems))
	for _, old := range oldItems {
		switch old.Origin {
		case domain.OriginProfile:
			if old.SourceIndex != nil {
				byIndex[*old.SourceIndex] = old
			}
		case domain.OriginAI:
			byText[domain.NormalizeText(old.SourceText)] = old
		}
	}

	out := make([]domain.Item, len(newItems))
	for i, it := range newItems {
		var (
			match domain.Item
			ok    bool
		)
		switch it.Origin {
		case domain.OriginProfile:
			if it.SourceIndex != nil {
				match, ok = byIndex[*it.SourceIndex]
			}
		case domain.OriginAI:
			match, ok = byText[domain.NormalizeText(it.SourceText)]
		}
		if ok {
			it.Selected = match.Selected
			it.Position = match.Position
			if it.Origin == domain.OriginAI {
				it.EditedText = match.EditedText
			}
		}
		out[i] = it
	}
	return out
}

// sqlcItemToDomain converts one persisted row into the domain.Item shape
// preserveMatchedSelections matches against — the same conversion itemToDto
// does, minus the DTO-only Edited/effective-text fields this caller doesn't
// need.
func sqlcItemToDomain(it sqlcgen.GenerationItem) domain.Item {
	var sourceIndex *int
	if it.SourceIndex != nil {
		v := int(*it.SourceIndex)
		sourceIndex = &v
	}
	return domain.Item{
		ID:          dbutil.UUIDString(it.ID),
		SectionID:   dbutil.UUIDString(it.SectionID),
		Origin:      domain.ItemOrigin(it.Origin),
		SourceIndex: sourceIndex,
		SourceText:  it.SourceText,
		EditedText:  it.EditedText,
		Rank:        int(it.Rank),
		Position:    int(it.Position),
		Selected:    it.Selected,
		Unavailable: it.Unavailable,
	}
}

// applyRankedSkills replaces the skills section's master-order seed with the
// ranking stage's group order (T060/T061). A nil or empty order leaves the
// seed exactly as StartGenerationRun wrote it — the FR-010 fallback shape,
// identical to what applyRankedSections does for an experience entry.
//
// Group *contents* never come from the model: they are read back from the
// master and ordered within each group by the deterministic domain.RankSkills,
// so the model's only influence on this section is which group comes first
// and — through cfg.SkillsMaxGroups — which are pre-selected.
// oldBySection is the same rerun-preservation input applyRankedSections
// takes (T077); nil for a plain run.
func (s *Service) applyRankedSkills(ctx context.Context, runID pgtype.UUID, master domain.RendercvMaster, analysis domain.VacancyAnalysis, cfg domain.ShapeConfig, groupOrder []int, oldBySection map[pgtype.UUID][]domain.Item) error {
	if len(groupOrder) == 0 {
		return nil
	}
	groups := withinGroupRankedSkills(master, analysis)
	if len(groups) == 0 {
		return nil
	}
	sections, err := s.q.ListSectionsByRun(ctx, runID)
	if err != nil {
		return err
	}
	for _, sec := range sections {
		if sec.Kind != string(domain.SectionKindSkills) {
			continue
		}
		items := preserveMatchedSelections(oldBySection[sec.ID], domain.SeedSkillItems(groups, groupOrder, cfg.SkillsMaxGroups))
		if err := s.q.DeleteSectionItems(ctx, sec.ID); err != nil {
			return err
		}
		return s.persistWorkspaceItems(ctx, sec.ID, items)
	}
	return nil
}

// backfillProjectsSection creates the projects section for a run that predates
// it, returning the created row (nil when the run already has one, or when the
// master has no projects to show). Items are left to applyRankedProjects, the
// same split StartRun uses.
func (s *Service) backfillProjectsSection(ctx context.Context, runID pgtype.UUID, master domain.RendercvMaster, cfg domain.ShapeConfig, sections []sqlcgen.GenerationSection) (*sqlcgen.GenerationSection, error) {
	if len(domain.AsSliceOfMaps(domain.CvSections(master)["projects"])) == 0 {
		return nil, nil
	}
	position := 0
	for _, sec := range sections {
		if sec.Kind == string(domain.SectionKindProjects) {
			return nil, nil
		}
		if int(sec.Position) >= position {
			position = int(sec.Position) + 1
		}
	}
	created, err := s.q.CreateSections(ctx, sqlcgen.CreateSectionsParams{
		RunID:        runID,
		Kinds:        []string{string(domain.SectionKindProjects)},
		EntryKeys:    []string{""},
		EntryLabels:  []string{""},
		Positions:    []int32{int32(position)},
		TargetCounts: []int32{int32(cfg.ProjectsMax)},
	})
	if err != nil || len(created) == 0 {
		return nil, err
	}
	if err := s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{
		ID: created[0].ID, State: string(domain.SectionReady),
	}); err != nil {
		return nil, err
	}
	return &created[0], nil
}

// applyRankedProjects replaces the projects section's master-order seed with
// the relevance order, the same way applyRankedSkills does for skill groups.
//
// No model call is involved: `domain.RankedProjectOrder` is arithmetic over
// the vacancy analysis and the profile's own evidence, so there is nothing to
// verify or fall back from. A run whose master has no projects has no projects
// section, and this is a no-op.
//
// oldBySection is the rerun-preservation input: a project the user had
// promoted or dropped keeps that choice when it still exists (T077).
func (s *Service) applyRankedProjects(ctx context.Context, runID pgtype.UUID, master domain.RendercvMaster, analysis domain.VacancyAnalysis, cfg domain.ShapeConfig, oldBySection map[pgtype.UUID][]domain.Item) error {
	projects := domain.AsSliceOfMaps(domain.CvSections(master)["projects"])
	if len(projects) == 0 {
		return nil
	}
	sections, err := s.q.ListSectionsByRun(ctx, runID)
	if err != nil {
		return err
	}
	order := domain.RankedProjectOrder(master, analysis)
	for _, sec := range sections {
		if sec.Kind != string(domain.SectionKindProjects) {
			continue
		}
		items := preserveMatchedSelections(oldBySection[sec.ID], domain.SeedProjectItems(projects, order, cfg.ProjectsMax))
		if err := s.q.DeleteSectionItems(ctx, sec.ID); err != nil {
			return err
		}
		return s.persistWorkspaceItems(ctx, sec.ID, items)
	}
	return nil
}

// withinGroupRankedSkills returns the master's skill groups with each group's
// entries ordered by vacancy relevance. RankSkills mutates in place and the
// run's master snapshot must stay byte-identical to what was persisted, so it
// runs on a JSON round-trip copy; SkillsMaxGroups is deliberately zeroed on
// that copy, because the between-group order is the ranking stage's GroupOrder
// now and RankSkills' own group reordering would fight it for the same slots.
func withinGroupRankedSkills(master domain.RendercvMaster, analysis domain.VacancyAnalysis) []map[string]any {
	raw, err := json.Marshal(map[string]any(master))
	if err != nil {
		return domain.AsSliceOfMaps(domain.CvSections(master)["skills"])
	}
	var clone domain.RendercvMaster
	if err := json.Unmarshal(raw, &clone); err != nil {
		return domain.AsSliceOfMaps(domain.CvSections(master)["skills"])
	}
	domain.RankSkills(clone, analysis, domain.ShapeConfig{SkillsEnabled: true})
	return domain.AsSliceOfMaps(domain.CvSections(clone)["skills"])
}

// StartRun is the background half of a workspace run, dispatched by
// worker.Handler on payload.GenerationRunID (T012). Ranking runs here, after
// analysis and before the summary highlights are read (T044): the experience
// sections start `ready` in master order from StartGenerationRun's seeding
// (FR-010's own fallback shape), and this stage upgrades them to a real
// ranking where one verifies, leaving the fallback in place — and recording
// it — where it does not.
func (s *Service) StartRun(ctx context.Context, runID string, rec *activity.Recorder) error {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return err
	}
	run, err := s.q.GetRun(ctx, rid)
	if err != nil {
		return err
	}

	var master domain.RendercvMaster
	if err := json.Unmarshal(run.MasterSnapshot, &master); err != nil {
		_ = s.q.SetRunState(ctx, sqlcgen.SetRunStateParams{ID: rid, State: string(domain.RunFailed)})
		return err
	}
	var cfg domain.ShapeConfig
	_ = json.Unmarshal(run.ShapeConfig, &cfg)

	if rec != nil {
		rec.Step(ctx, "analyzing vacancy", nil)
	}
	analysis, err := analyzeVacancy(ctx, s.llm.Analyze, s.genModel, run.VacancyText, nil)
	if err != nil {
		_ = s.q.SetRunState(ctx, sqlcgen.SetRunStateParams{ID: rid, State: string(domain.RunFailed)})
		return err
	}
	if analysisJSON, mErr := json.Marshal(analysis); mErr == nil {
		_ = s.q.SetRunAnalysis(ctx, sqlcgen.SetRunAnalysisParams{ID: rid, Analysis: analysisJSON})
	}

	if rec != nil {
		rec.Step(ctx, "ranking achievements", nil)
	}
	rankResults, skillOrder := rankExperienceSections(ctx, s.llm.Select, s.genModel, master, analysis, cfg)
	anyRankingFailed := false
	if err := s.applyRankedSkills(ctx, rid, master, analysis, cfg, skillOrder, nil); err != nil {
		anyRankingFailed = true
		if rec != nil {
			rec.Step(ctx, "skills ranking persistence failed", map[string]any{"error": err.Error()})
		}
	}
	if err := s.applyRankedProjects(ctx, rid, master, analysis, cfg, nil); err != nil {
		anyRankingFailed = true
		if rec != nil {
			rec.Step(ctx, "projects ranking persistence failed", map[string]any{"error": err.Error()})
		}
	}
	// A ranking persistence failure does not fail the whole run on its own
	// (T081): a DeleteSectionItems failure leaves the master-order seed
	// StartGenerationRun already wrote in place (FR-010's fallback shape),
	// and a persistWorkspaceItems failure after a successful delete marks
	// just that section `failed` — applyRankedSections's own doing — so the
	// run reaches `partial` with every other section intact rather than
	// discarding the whole thing.
	if s.applyRankedSections(ctx, rid, rankResults, nil) {
		anyRankingFailed = true
		if rec != nil {
			rec.Step(ctx, "ranking persistence failed for one or more sections", nil)
		}
	}

	// The suggestion stage (T053/T054) runs concurrently with the summary
	// stage below — a separate LLM call, through the same generation-select
	// provider, taking the analysis plus company names and skill-group
	// labels but never the master's bullet text (resume-generation.md § 4.3). It is
	// joined just before the run's final state is set, so it never delays
	// the summary and never fails the run on its own.
	if rec != nil {
		rec.Step(ctx, "suggesting additional content", nil)
	}
	var (
		suggestWG  sync.WaitGroup
		suggestSet domain.SuggestionSet
		suggestErr error
	)
	suggestWG.Add(1)
	go func() {
		defer suggestWG.Done()
		suggestSet, suggestErr = suggestContent(ctx, s.llm.Select, s.genModel, experienceCompanies(master), domain.SkillGroupLabels(master), analysis)
	}()

	// selectedProfileHighlights reads the run's currently-selected profile
	// items, so it picks up whatever rankExperienceSections/applyRankedSections
	// just wrote — real ranking where it verified, the master-order fallback
	// where it did not (T044, resume-generation.md § 0).
	highlights, err := s.selectedProfileHighlights(ctx, rid)
	if err != nil {
		highlights = nil
	}

	summaryOpt := domain.DefaultSummaryOption()
	if run.SummaryOptionID != nil {
		if opt, ok := domain.LookupSummaryOption(*run.SummaryOptionID); ok {
			summaryOpt = opt
		}
	}
	summaryLC := s.summaryProviderFor(summaryOpt)

	if rec != nil {
		rec.Step(ctx, "writing summary", map[string]any{"option": summaryOpt.ID})
	}
	minS, maxS := summarySelectRange(cfg)
	brief := domain.SummaryBrief{
		Analysis:         analysis,
		TotalYears:       domain.DeriveTotalExperienceYears(master),
		Highlights:       highlights,
		SkillGroupLabels: domain.SkillGroupLabels(master),
		SentenceMin:      minS,
		SentenceMax:      maxS,
	}

	summary, summaryErr := writeSummary(ctx, summaryLC, s.genModel, brief)
	if summaryErr == nil {
		if violations := domain.VerifySummaryGrounding(master, summary, brief); len(violations) > 0 {
			if retry, rErr := writeSummary(ctx, summaryLC, s.genModel, brief.WithViolations(violations)); rErr == nil {
				if reViolations := domain.VerifySummaryGrounding(master, retry, brief); len(reViolations) == 0 {
					summary = retry
				} else {
					summary = domain.StripSummaryViolations(retry, reViolations)
				}
			}
		}
	}

	summarySectionID, hasSummarySection := s.summarySectionID(ctx, rid)
	finalState := domain.RunReady
	if anyRankingFailed {
		finalState = domain.RunPartial
	}
	if summaryErr != nil {
		finalState = domain.RunPartial
		if hasSummarySection {
			msg := "summary generation failed: " + summaryErr.Error()
			_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: summarySectionID, State: string(domain.SectionFailed), Error: &msg})
		}
		if rec != nil {
			rec.Step(ctx, "summary generation failed", map[string]any{"error": summaryErr.Error()})
		}
	} else if hasSummarySection {
		if err := s.persistWorkspaceItems(ctx, summarySectionID, []domain.Item{{
			Origin: domain.OriginAI, Kind: domain.ItemKindSummary,
			SourceText: strings.TrimSpace(summary.Summary), Rank: 0, Position: 0, Selected: true,
		}}); err != nil {
			finalState = domain.RunPartial
			msg := "summary persistence failed: " + err.Error()
			_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: summarySectionID, State: string(domain.SectionFailed), Error: &msg})
		} else {
			_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: summarySectionID, State: string(domain.SectionReady)})
		}
	}

	// Join the suggestion stage (spawned above, alongside ranking). Its
	// content is unverified by definition (FR-016 — no grounding check runs
	// on a SuggestionSet) and off by default, so a failure here is logged
	// and otherwise ignored: it never downgrades finalState or fails the run.
	suggestWG.Wait()
	if suggestErr == nil {
		expItems, skillItems := buildSuggestionItems(suggestSet, master)
		if err := s.persistSuggestions(ctx, rid, expItems, skillItems, nil, nil); err != nil && rec != nil {
			rec.Step(ctx, "suggestion persistence failed", map[string]any{"error": err.Error()})
		}
	} else if rec != nil {
		rec.Step(ctx, "suggestion generation failed", map[string]any{"error": suggestErr.Error()})
	}

	_ = s.q.SetRunState(ctx, sqlcgen.SetRunStateParams{ID: rid, State: string(finalState)})
	if rec != nil {
		rec.Ok(ctx, runID, map[string]any{"state": string(finalState)})
	}
	return nil
}

// selectedProfileHighlights gathers the selected experience achievements
// currently persisted for the run, in section-then-item position order — the
// summary brief's input. By the time StartRun calls this, rankExperienceSections
// / applyRankedSections have already run, so this reads real ranking where it
// verified and the master-order fallback where it did not (T044,
// resume-generation.md § 0): the same data SelectedHighlights(TailoredSelection)
// used to supply, read over the run's items instead of a TailoredSelection.
func (s *Service) selectedProfileHighlights(ctx context.Context, runID pgtype.UUID) ([]string, error) {
	sections, err := s.q.ListSectionsByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	experienceSections := make(map[pgtype.UUID]bool, len(sections))
	for _, sec := range sections {
		if sec.Kind == string(domain.SectionKindExperience) {
			experienceSections[sec.ID] = true
		}
	}
	items, err := s.q.ListItemsByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, it := range items {
		if !experienceSections[it.SectionID] {
			continue
		}
		if it.Origin != string(domain.OriginProfile) || !it.Selected {
			continue
		}
		out = append(out, it.SourceText)
	}
	return out, nil
}

func (s *Service) summarySectionID(ctx context.Context, runID pgtype.UUID) (pgtype.UUID, bool) {
	sections, err := s.q.ListSectionsByRun(ctx, runID)
	if err != nil {
		return pgtype.UUID{}, false
	}
	for _, sec := range sections {
		if sec.Kind == string(domain.SectionKindSummary) {
			return sec.ID, true
		}
	}
	return pgtype.UUID{}, false
}

// GetGenerationWorkspace is `GET /v1/generations/{runId}`: the whole run,
// its sections and their items, in position order (resume-generation.md § 4.1).
func (s *Service) GetGenerationWorkspace(ctx context.Context, runID string) (dto.GenerationRunDto, error) {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return dto.GenerationRunDto{}, apperr.NotFound("generation run", runID)
	}
	run, err := s.q.GetRun(ctx, rid)
	if err != nil {
		return dto.GenerationRunDto{}, apperr.NotFound("generation run", runID)
	}
	sections, err := s.q.ListSectionsByRun(ctx, rid)
	if err != nil {
		return dto.GenerationRunDto{}, err
	}
	items, err := s.q.ListItemsByRun(ctx, rid)
	if err != nil {
		return dto.GenerationRunDto{}, err
	}
	changed := s.syncMasterStaleness(ctx, run, sections, items)
	return s.runToDto(run, sections, items, changed), nil
}

// ListGenerationRuns is `GET /v1/generations?jobId=&limit=`: recent runs for
// a profile, newest first. profileID empty resolves to the default profile,
// matching every other profile-scoped endpoint in this codebase.
func (s *Service) ListGenerationRuns(ctx context.Context, profileID string, jobID *string, limit int) ([]dto.GenerationRunDto, error) {
	var pid pgtype.UUID
	if profileID != "" {
		var err error
		pid, err = dbutil.ParseUUID(profileID)
		if err != nil {
			return nil, apperr.Validation("invalid profileId")
		}
	} else {
		prof, err := s.profiles.GetDefault(ctx)
		if err != nil {
			return nil, err
		}
		pid = prof.ID
	}

	var jobUUID pgtype.UUID
	if jobID != nil && *jobID != "" {
		var err error
		jobUUID, err = dbutil.ParseUUID(*jobID)
		if err != nil {
			return nil, apperr.NotFound("job", *jobID)
		}
	}
	if limit <= 0 {
		limit = defaultRunListLimit
	}

	runs, err := s.q.ListRunsByProfile(ctx, sqlcgen.ListRunsByProfileParams{ProfileID: pid, Limit: int32(limit), JobID: jobUUID})
	if err != nil {
		return nil, err
	}
	out := make([]dto.GenerationRunDto, 0, len(runs))
	for _, run := range runs {
		sections, err := s.q.ListSectionsByRun(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		items, err := s.q.ListItemsByRun(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		changed := s.syncMasterStaleness(ctx, run, sections, items)
		out = append(out, s.runToDto(run, sections, items, changed))
	}
	return out, nil
}

// DeleteGenerationRun is `DELETE /v1/generations/{runId}`. Cascades to
// sections and items via the migration's ON DELETE CASCADE.
func (s *Service) DeleteGenerationRun(ctx context.Context, runID string) error {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return apperr.NotFound("generation run", runID)
	}
	if _, err := s.q.GetRun(ctx, rid); err != nil {
		return apperr.NotFound("generation run", runID)
	}
	return s.q.DeleteRun(ctx, rid)
}

func (s *Service) runToDto(run sqlcgen.GenerationRun, sections []sqlcgen.GenerationSection, items []sqlcgen.GenerationItem, masterChanged bool) dto.GenerationRunDto {
	itemsBySection := make(map[pgtype.UUID][]sqlcgen.GenerationItem, len(sections))
	for _, it := range items {
		itemsBySection[it.SectionID] = append(itemsBySection[it.SectionID], it)
	}

	sectionDtos := make([]dto.GenerationSectionDto, 0, len(sections))
	for _, sec := range sections {
		secItems := itemsBySection[sec.ID]
		itemDtos := make([]dto.GenerationItemDto, 0, len(secItems))
		for _, it := range secItems {
			itemDtos = append(itemDtos, itemToDto(sec.Kind, it))
		}
		sectionDtos = append(sectionDtos, dto.GenerationSectionDto{
			ID: dbutil.UUIDString(sec.ID), Kind: sec.Kind, EntryKey: sec.EntryKey, EntryLabel: sec.EntryLabel,
			Position: int(sec.Position), TargetCount: int(sec.TargetCount), State: sec.State, Error: sec.Error,
			FallbackUsed: sec.FallbackUsed, Items: itemDtos,
		})
	}

	var cfg domain.ShapeConfig
	_ = json.Unmarshal(run.ShapeConfig, &cfg)

	return dto.GenerationRunDto{
		ID:                 dbutil.UUIDString(run.ID),
		State:              run.State,
		Vacancy:            dto.GenerationVacancyDto{Company: run.VacancyCompany, Title: run.VacancyTitle},
		JobID:              dbutil.UUIDStringPtr(run.JobID),
		GroundingLevel:     run.GroundingLevel,
		SummaryOptionID:    run.SummaryOptionID,
		SummarySubstituted: false,
		MasterChanged:      masterChanged,
		ShapeConfig:        shapeConfigToDto(cfg),
		Export:             exportDtoFromRun(run),
		Sections:           sectionDtos,
		CreatedAt:          timestamptzString(run.CreatedAt),
		UpdatedAt:          timestamptzString(run.UpdatedAt),
	}
}

// syncMasterStaleness is FR-022: the run's snapshot hash compared against the
// profile's current master content hash. Best-effort — a profile that no
// longer resolves (deleted, config cleared) reports unchanged rather than
// failing the whole workspace response over a staleness check.
//
// When the master has changed, it also marks unavailable (MarkItemsUnavailable)
// every profile-origin item whose source_index no longer resolves against the
// current master — a shrunk highlight list, a renamed or removed experience
// entry, a shrunk skills group list — and updates `items` in place so the
// caller's response reflects the mark without a second round trip. An item is
// never deleted for this: data-model.md §4 requires it stay visible, badged
// unavailable, not silently dropped.
func (s *Service) syncMasterStaleness(ctx context.Context, run sqlcgen.GenerationRun, sections []sqlcgen.GenerationSection, items []sqlcgen.GenerationItem) bool {
	prof, err := s.profiles.Get(ctx, dbutil.UUIDString(run.ProfileID))
	if err != nil || prof.RendercvConfig == nil {
		return false
	}
	master, err := domain.MasterFromProfile(prof)
	if err != nil || master == nil {
		return false
	}
	hash, err := domain.ContentHash(master)
	if err != nil {
		return false
	}
	if hash == run.MasterContentHash {
		return false
	}

	availByCompany := make(map[string]int)
	for _, e := range domain.AsSliceOfMaps(domain.CvSections(master)["experience"]) {
		availByCompany[domain.StringField(e, "company")] = len(domain.StringSliceField(e, "highlights"))
	}
	skillGroupCount := len(domain.AsSliceOfMaps(domain.CvSections(master)["skills"]))

	sectionByID := make(map[pgtype.UUID]sqlcgen.GenerationSection, len(sections))
	for _, sec := range sections {
		sectionByID[sec.ID] = sec
	}

	var toMark []pgtype.UUID
	for i, it := range items {
		if it.Origin != string(domain.OriginProfile) || it.Unavailable || it.SourceIndex == nil {
			continue
		}
		sec, ok := sectionByID[it.SectionID]
		if !ok {
			continue
		}
		var avail int
		switch domain.SectionKind(sec.Kind) {
		case domain.SectionKindExperience:
			if sec.EntryKey == nil {
				continue
			}
			avail = availByCompany[*sec.EntryKey]
		case domain.SectionKindSkills:
			avail = skillGroupCount
		default:
			continue
		}
		if int(*it.SourceIndex) >= avail {
			toMark = append(toMark, it.ID)
			items[i].Unavailable = true
		}
	}
	if len(toMark) > 0 {
		_ = s.q.MarkItemsUnavailable(ctx, toMark)
	}
	return true
}

func itemToDto(sectionKind string, it sqlcgen.GenerationItem) dto.GenerationItemDto {
	var sourceIndex *int
	if it.SourceIndex != nil {
		v := int(*it.SourceIndex)
		sourceIndex = &v
	}
	text := it.SourceText
	edited := false
	if it.EditedText != nil && *it.EditedText != "" {
		text = *it.EditedText
		edited = true
	}
	return dto.GenerationItemDto{
		ID: dbutil.UUIDString(it.ID), Origin: it.Origin, Kind: itemKindFor(sectionKind),
		Text: text, SourceIndex: sourceIndex, Rank: int(it.Rank), Position: int(it.Position),
		Selected: it.Selected, Edited: edited, Unavailable: it.Unavailable,
	}
}

func itemKindFor(sectionKind string) string {
	switch domain.SectionKind(sectionKind) {
	case domain.SectionKindSummary:
		return string(domain.ItemKindSummary)
	case domain.SectionKindSkills:
		return string(domain.ItemKindSkillGroup)
	default:
		return string(domain.ItemKindAchievement)
	}
}

// shapeConfigToDto mirrors resumeshape/interfaces/http.configToDto — kept as
// a separate copy here rather than an import because that package's
// converter is unexported and this feature module does not otherwise depend
// on resumeshape's HTTP layer.
func shapeConfigToDto(c domain.ShapeConfig) dto.ResumeShapeConfigDto {
	return dto.ResumeShapeConfigDto{
		SummaryLines:          c.SummaryLines,
		SkillsEnabled:         c.SkillsEnabled,
		SkillsMaxGroups:       c.SkillsMaxGroups,
		ExperienceBulletsMin:  c.ExperienceBulletsMin,
		ExperienceBulletsMax:  c.ExperienceBulletsMax,
		TargetPages:           c.TargetPages,
		ProjectsEnabled:       c.ProjectsEnabled,
		ProjectsMin:           c.ProjectsMin,
		ProjectsMax:           c.ProjectsMax,
		ProjectBulletsMax:     c.ProjectBulletsMax,
		CertificationsEnabled: c.CertificationsEnabled,
		CertificationsMin:     c.CertificationsMin,
		CertificationsMax:     c.CertificationsMax,
		FontSize:              c.FontSize,
	}
}

func timestamptzString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
