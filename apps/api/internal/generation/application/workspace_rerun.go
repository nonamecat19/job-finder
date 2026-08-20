package application

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/queue"
)

func (s *Service) RerunGenerationRun(ctx context.Context, runID string, req dto.RerunGenerationRequestDto) (string, string, error) {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return "", "", apperr.NotFound("generation run", runID)
	}
	if s.tx == nil {
		return "", "", apperr.Internal("generation workspace: no transaction runner configured")
	}

	var (
		run     sqlcgen.GenerationRun
		targets []pgtype.UUID
	)
	err = s.tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		r, gErr := q.GetRunForUpdate(ctx, rid)
		if gErr != nil {
			return apperr.NotFound("generation run", runID)
		}
		if r.State == string(domain.RunRunning) {
			return apperr.Conflict("generation run is running")
		}
		run = r

		sections, sErr := q.ListSectionsByRun(ctx, rid)
		if sErr != nil {
			return sErr
		}
		if len(req.Sections) == 0 {
			for _, sec := range sections {
				targets = append(targets, sec.ID)
			}
		} else {
			bySectionID := make(map[string]pgtype.UUID, len(sections))
			for _, sec := range sections {
				bySectionID[dbutil.UUIDString(sec.ID)] = sec.ID
			}
			for _, sid := range req.Sections {
				id, ok := bySectionID[sid]
				if !ok {
					return apperr.NotFound("generation section", sid)
				}
				targets = append(targets, id)
			}
		}

		if sErr := q.SetRunState(ctx, sqlcgen.SetRunStateParams{ID: rid, State: string(domain.RunRunning)}); sErr != nil {
			return sErr
		}
		for _, sid := range targets {
			if sErr := q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: sid, State: string(domain.SectionRunning)}); sErr != nil {
				return sErr
			}
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}

	sectionIDs := make([]string, len(targets))
	for i, id := range targets {
		sectionIDs[i] = dbutil.UUIDString(id)
	}

	runIDStr := dbutil.UUIDString(run.ID)
	label := "generation workspace rerun"
	if run.VacancyCompany != nil && run.VacancyTitle != nil {
		label = *run.VacancyCompany + " — " + *run.VacancyTitle + " (rerun)"
	}
	rec := activity.New(ctx, s.q, "generate_workspace_rerun", label, dbutil.UUIDStringPtr(run.JobID), nil, "")
	var actID *string
	if rec != nil {
		idStr := dbutil.UUIDString(rec.ID())
		actID = &idStr
		rec.Step(ctx, "queued", map[string]any{"runId": runIDStr, "sections": sectionIDs})
	}

	if s.enqueuer != nil {
		payload, mErr := json.Marshal(queue.GeneratePayload{
			JobID:           derefOrEmpty(dbutil.UUIDStringPtr(run.JobID)),
			Type:            string(dto.DocumentTypeResume),
			ProfileID:       strPtr(dbutil.UUIDString(run.ProfileID)),
			ActivityID:      actID,
			GenerationRunID: &runIDStr,
			IsRerun:         true,
			RerunSections:   sectionIDs,
		})
		if mErr != nil {
			return "", "", mErr
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

func strPtr(s string) *string { return &s }

func (s *Service) RerunRun(ctx context.Context, runID string, sectionIDs []string, rec *activity.Recorder) error {
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

	analysis, err := s.runAnalysisOrReanalyze(ctx, rid, run, master)
	if err != nil {
		_ = s.q.SetRunState(ctx, sqlcgen.SetRunStateParams{ID: rid, State: string(domain.RunFailed)})
		return err
	}

	sections, err := s.q.ListSectionsByRun(ctx, rid)
	if err != nil {
		_ = s.q.SetRunState(ctx, sqlcgen.SetRunStateParams{ID: rid, State: string(domain.RunFailed)})
		return err
	}
	target := make(map[pgtype.UUID]bool, len(sectionIDs))
	for _, sid := range sectionIDs {
		if id, pErr := dbutil.ParseUUID(sid); pErr == nil {
			target[id] = true
		}
	}

	created, err := s.backfillProjectsSection(ctx, rid, master, cfg, sections)
	if err != nil && rec != nil {
		rec.Step(ctx, "projects section backfill failed", map[string]any{"error": err.Error()})
	}
	if created != nil {
		sections = append(sections, *created)
		target[created.ID] = true
	}
	if created, err := s.backfillCertificationsSection(ctx, rid, master, cfg, sections); err != nil {
		if rec != nil {
			rec.Step(ctx, "certifications section backfill failed", map[string]any{"error": err.Error()})
		}
	} else if created != nil {
		sections = append(sections, *created)
		target[created.ID] = true
	}
	if created, err := s.backfillEducationSection(ctx, rid, master, cfg, sections); err != nil {
		if rec != nil {
			rec.Step(ctx, "education section backfill failed", map[string]any{"error": err.Error()})
		}
	} else if created != nil {
		sections = append(sections, *created)
		target[created.ID] = true
	}

	existingItems, err := s.q.ListItemsByRun(ctx, rid)
	if err != nil {
		existingItems = nil
	}
	oldBySection := make(map[pgtype.UUID][]domain.Item, len(sections))
	for _, it := range existingItems {
		if !target[it.SectionID] {
			continue
		}
		oldBySection[it.SectionID] = append(oldBySection[it.SectionID], sqlcItemToDomain(it))
	}

	anyFailed := s.reseedTargetedSections(ctx, master, cfg, sections, target, oldBySection)

	var summarySection *sqlcgen.GenerationSection
	skillsTargeted := false
	projectsTargeted := false
	for i := range sections {
		sec := sections[i]
		if target[sec.ID] && sec.Kind == string(domain.SectionKindSummary) {
			summarySection = &sections[i]
		}
		if target[sec.ID] && sec.Kind == string(domain.SectionKindSkills) {
			skillsTargeted = true
		}
		if target[sec.ID] && sec.Kind == string(domain.SectionKindProjects) {
			projectsTargeted = true
		}
	}

	if rec != nil {
		rec.Step(ctx, "ranking achievements", nil)
	}
	rankResults, skillOrder := rankExperienceSections(ctx, s.llm.Select, s.genModel, master, analysis, cfg)
	rankResults = filterRankedResultsToTarget(rankResults, sections, target)
	if s.applyRankedSections(ctx, rid, rankResults, oldBySection) {
		anyFailed = true
	}
	if skillsTargeted {
		if err := s.applyRankedSkills(ctx, rid, master, analysis, cfg, skillOrder, oldBySection); err != nil {
			anyFailed = true
			if rec != nil {
				rec.Step(ctx, "skills ranking persistence failed", map[string]any{"error": err.Error()})
			}
		}
	}
	if projectsTargeted {
		if err := s.applyRankedProjects(ctx, rid, master, analysis, cfg, oldBySection); err != nil {
			anyFailed = true
			if rec != nil {
				rec.Step(ctx, "projects ranking persistence failed", map[string]any{"error": err.Error()})
			}
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

	finalState := domain.RunReady
	if anyFailed {
		finalState = domain.RunPartial
	}

	if summarySection != nil {
		if rec != nil {
			rec.Step(ctx, "writing summary", nil)
		}
		highlights, hErr := s.selectedProfileHighlights(ctx, rid)
		if hErr != nil {
			highlights = nil
		}
		skillLines, sErr := s.selectedSkillLines(ctx, rid)
		if sErr != nil {
			skillLines = nil
		}
		summaryOpt := domain.DefaultSummaryOption()
		if run.SummaryOptionID != nil {
			if opt, ok := domain.LookupSummaryOption(*run.SummaryOptionID); ok {
				summaryOpt = opt
			}
		}
		summaryLC := s.summaryProviderFor(summaryOpt)
		minS, maxS := summarySelectRange(cfg)
		brief := domain.SummaryBrief{
			Analysis:         analysis,
			TotalYears:       domain.DeriveTotalExperienceYears(master),
			Highlights:       highlights,
			SkillGroupLabels: domain.SkillGroupLabels(master),
			SkillLines:       skillLines,
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
		if summaryErr != nil {
			finalState = domain.RunPartial
			msg := "summary generation failed: " + summaryErr.Error()
			_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: summarySection.ID, State: string(domain.SectionFailed), Error: &msg})
			if rec != nil {
				rec.Step(ctx, "summary generation failed", map[string]any{"error": summaryErr.Error()})
			}
		} else {
			newSummary := preserveMatchedSelections(oldBySection[summarySection.ID], []domain.Item{{
				Origin: domain.OriginAI, Kind: domain.ItemKindSummary,
				SourceText: strings.TrimSpace(summary.Summary), Rank: 0, Position: 0, Selected: true,
			}})
			if err := s.q.DeleteSectionItems(ctx, summarySection.ID); err != nil {
				finalState = domain.RunPartial
				msg := "summary persistence failed: " + err.Error()
				_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: summarySection.ID, State: string(domain.SectionFailed), Error: &msg})
			} else if err := s.persistWorkspaceItems(ctx, summarySection.ID, newSummary); err != nil {
				finalState = domain.RunPartial
				msg := "summary persistence failed: " + err.Error()
				_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: summarySection.ID, State: string(domain.SectionFailed), Error: &msg})
			} else {
				_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: summarySection.ID, State: string(domain.SectionReady)})
			}
		}
	}

	suggestWG.Wait()
	if suggestErr == nil {
		expItems, skillItems := buildSuggestionItems(suggestSet, master)
		if err := s.persistSuggestions(ctx, rid, expItems, skillItems, target, oldBySection); err != nil && rec != nil {
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

func (s *Service) runAnalysisOrReanalyze(ctx context.Context, rid pgtype.UUID, run sqlcgen.GenerationRun, master domain.RendercvMaster) (domain.VacancyAnalysis, error) {
	vacancy := runTarget(run)
	var analysis domain.VacancyAnalysis
	if len(run.Analysis) > 0 {
		if err := json.Unmarshal(run.Analysis, &analysis); err == nil {
			// The target is deliberately not persisted with the analysis, so a
			// restored one has to be re-attached from the run itself.
			analysis.TargetTitle, analysis.TargetCompany = vacancy.Title, vacancy.Company
			return domain.MergeTitleSkills(analysis, domain.TitleRequiredSkills(master, vacancy.Title)), nil
		}
	}
	analysis, err := analyzeVacancy(ctx, s.llm.Analyze, s.genModel, vacancy, run.VacancyText, nil)
	if err != nil {
		return domain.VacancyAnalysis{}, err
	}
	analysis = domain.MergeTitleSkills(analysis, domain.TitleRequiredSkills(master, vacancy.Title))
	if analysisJSON, mErr := json.Marshal(analysis); mErr == nil {
		_ = s.q.SetRunAnalysis(ctx, sqlcgen.SetRunAnalysisParams{ID: rid, Analysis: analysisJSON})
	}
	return analysis, nil
}

func (s *Service) reseedTargetedSections(ctx context.Context, master domain.RendercvMaster, cfg domain.ShapeConfig, sections []sqlcgen.GenerationSection, target map[pgtype.UUID]bool, oldBySection map[pgtype.UUID][]domain.Item) (anyFailed bool) {
	fresh := domain.SeedFromMaster(master, cfg)
	freshByKey := make(map[string]domain.Section, len(fresh))
	for _, fs := range fresh {
		freshByKey[sectionMatchKey(fs.Kind, fs.EntryKey)] = fs
	}

	for _, sec := range sections {
		if !target[sec.ID] {
			continue
		}
		kind := domain.SectionKind(sec.Kind)
		switch kind {
		case domain.SectionKindExperience, domain.SectionKindSkills, domain.SectionKindCertifications, domain.SectionKindEducation:
		default:
			continue
		}
		fs, ok := freshByKey[sectionMatchKey(kind, sec.EntryKey)]
		if !ok {
			continue
		}
		items := preserveMatchedSelections(oldBySection[sec.ID], fs.Items)
		if err := s.q.DeleteSectionItems(ctx, sec.ID); err != nil {
			anyFailed = true
			msg := "rerun reseed failed: " + err.Error()
			_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: sec.ID, State: string(domain.SectionFailed), Error: &msg})
			continue
		}
		if len(items) > 0 {
			if err := s.persistWorkspaceItems(ctx, sec.ID, items); err != nil {
				anyFailed = true
				msg := "rerun reseed failed: " + err.Error()
				_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: sec.ID, State: string(domain.SectionFailed), Error: &msg})
				continue
			}
		}
		_ = s.q.SetSectionState(ctx, sqlcgen.SetSectionStateParams{ID: sec.ID, State: string(domain.SectionReady)})
	}
	return anyFailed
}

func sectionMatchKey(kind domain.SectionKind, entryKey *string) string {
	key := ""
	if entryKey != nil {
		key = *entryKey
	}
	return string(kind) + "|" + key
}

func filterRankedResultsToTarget(results []rankedSectionResult, sections []sqlcgen.GenerationSection, target map[pgtype.UUID]bool) []rankedSectionResult {
	allowed := make(map[string]bool, len(sections))
	for _, sec := range sections {
		if sec.Kind == string(domain.SectionKindExperience) && sec.EntryKey != nil && target[sec.ID] {
			allowed[*sec.EntryKey] = true
		}
	}
	out := make([]rankedSectionResult, 0, len(results))
	for _, r := range results {
		if allowed[r.entryKey] {
			out = append(out, r)
		}
	}
	return out
}
