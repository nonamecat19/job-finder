package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

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

const defaultRunListLimit = 20

func (s *Service) SetEnqueuer(e queue.Enqueuer) { s.enqueuer = e }

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

	if s.enqueuer != nil {
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
		if err := s.enqueuer.EnqueueContext(ctx, queue.TypeGenerate, payload); err != nil {
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
	created, err := s.q.CreateItems(ctx, sqlcgen.CreateItemsParams{
		SectionID: sectionID, Origins: origins, SourceIndexes: sourceIndexes, SourceTexts: sourceTexts,
		EditedTexts: editedTexts, Ranks: ranks, Positions: positions, SelectedFlags: selected,
	})
	if err != nil {
		return err
	}
	return s.persistDroppedEntries(ctx, created, items)
}

func (s *Service) persistDroppedEntries(ctx context.Context, created []sqlcgen.GenerationItem, items []domain.Item) error {
	bySource := make(map[int][]string)
	for _, it := range items {
		if len(it.DroppedEntries) == 0 || it.Origin != domain.OriginProfile || it.SourceIndex == nil {
			continue
		}
		bySource[*it.SourceIndex] = it.DroppedEntries
	}
	if len(bySource) == 0 {
		return nil
	}
	for _, row := range created {
		if row.Origin != string(domain.OriginProfile) || row.SourceIndex == nil {
			continue
		}
		dropped, ok := bySource[int(*row.SourceIndex)]
		if !ok {
			continue
		}
		if err := s.q.UpdateItemDroppedEntries(ctx, sqlcgen.UpdateItemDroppedEntriesParams{
			ID: row.ID, DroppedEntries: dropped,
		}); err != nil {
			return err
		}
	}
	return nil
}

type rankedSectionResult struct {
	entryKey     string
	items        []domain.Item
	fallbackUsed bool
}

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
			} else {

				it.DroppedEntries = match.DroppedEntries
			}
		}
		out[i] = it
	}
	return out
}

func sqlcItemToDomain(it sqlcgen.GenerationItem) domain.Item {
	var sourceIndex *int
	if it.SourceIndex != nil {
		v := int(*it.SourceIndex)
		sourceIndex = &v
	}
	return domain.Item{
		ID:             dbutil.UUIDString(it.ID),
		SectionID:      dbutil.UUIDString(it.SectionID),
		Origin:         domain.ItemOrigin(it.Origin),
		SourceIndex:    sourceIndex,
		SourceText:     it.SourceText,
		EditedText:     it.EditedText,
		Rank:           int(it.Rank),
		Position:       int(it.Position),
		Selected:       it.Selected,
		Unavailable:    it.Unavailable,
		DroppedEntries: it.DroppedEntries,
	}
}

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

func (s *Service) backfillProjectsSection(ctx context.Context, runID pgtype.UUID, master domain.RendercvMaster, cfg domain.ShapeConfig, sections []sqlcgen.GenerationSection) (*sqlcgen.GenerationSection, error) {
	if !cfg.ProjectsEnabled || len(domain.AsSliceOfMaps(domain.CvSections(master)["projects"])) == 0 {
		return nil, nil
	}
	return s.backfillSection(ctx, runID, domain.SectionKindProjects, cfg.ProjectsMax, sections)
}

func (s *Service) backfillCertificationsSection(ctx context.Context, runID pgtype.UUID, master domain.RendercvMaster, cfg domain.ShapeConfig, sections []sqlcgen.GenerationSection) (*sqlcgen.GenerationSection, error) {
	if !cfg.CertificationsEnabled || len(domain.AsSliceOfMaps(domain.CvSections(master)["certifications"])) == 0 {
		return nil, nil
	}
	return s.backfillSection(ctx, runID, domain.SectionKindCertifications, 0, sections)
}

func (s *Service) backfillEducationSection(ctx context.Context, runID pgtype.UUID, master domain.RendercvMaster, cfg domain.ShapeConfig, sections []sqlcgen.GenerationSection) (*sqlcgen.GenerationSection, error) {
	if !cfg.EducationEnabled || len(domain.AsSliceOfMaps(domain.CvSections(master)["education"])) == 0 {
		return nil, nil
	}
	return s.backfillSection(ctx, runID, domain.SectionKindEducation, 0, sections)
}

func (s *Service) backfillSection(ctx context.Context, runID pgtype.UUID, kind domain.SectionKind, targetCount int, sections []sqlcgen.GenerationSection) (*sqlcgen.GenerationSection, error) {
	position := 0
	for _, sec := range sections {
		if sec.Kind == string(kind) {
			return nil, nil
		}
		if int(sec.Position) >= position {
			position = int(sec.Position) + 1
		}
	}
	created, err := s.q.CreateSections(ctx, sqlcgen.CreateSectionsParams{
		RunID:        runID,
		Kinds:        []string{string(kind)},
		EntryKeys:    []string{""},
		EntryLabels:  []string{""},
		Positions:    []int32{int32(position)},
		TargetCounts: []int32{int32(targetCount)},
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

	if s.applyRankedSections(ctx, rid, rankResults, nil) {
		anyRankingFailed = true
		if rec != nil {
			rec.Step(ctx, "ranking persistence failed for one or more sections", nil)
		}
	}

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
		sectionDtos = append(sectionDtos, sectionToDto(sec, itemDtos))
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
		SkillEntries: skillEntriesToDto(sectionKind, it),
	}
}

func skillEntriesToDto(sectionKind string, it sqlcgen.GenerationItem) []dto.GenerationSkillEntryDto {
	if !hasSkillEntries(sectionKind, it.Origin) {
		return nil
	}
	entries := sqlcItemToDomain(it).SkillEntries()
	out := make([]dto.GenerationSkillEntryDto, 0, len(entries))
	for _, e := range entries {
		out = append(out, dto.GenerationSkillEntryDto{Text: e.Text, Selected: e.Selected})
	}
	return out
}

func hasSkillEntries(sectionKind, origin string) bool {
	return domain.SectionKind(sectionKind) == domain.SectionKindSkills && domain.ItemOrigin(origin) == domain.OriginProfile
}

func itemKindFor(sectionKind string) string {
	switch domain.SectionKind(sectionKind) {
	case domain.SectionKindSummary:
		return string(domain.ItemKindSummary)
	case domain.SectionKindSkills:
		return string(domain.ItemKindSkillGroup)
	case domain.SectionKindProjects:
		return string(domain.ItemKindProject)
	case domain.SectionKindCertifications:
		return string(domain.ItemKindCertification)
	case domain.SectionKindEducation:
		return string(domain.ItemKindEducation)
	default:
		return string(domain.ItemKindAchievement)
	}
}

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
		EducationEnabled:      c.EducationEnabled,
		ExperienceEnabled:     c.ExperienceEnabled,
		SummaryEnabled:        c.SummaryEnabled,
		FontSize:              c.FontSize,
	}
}

func timestamptzString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
