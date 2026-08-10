package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
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
// grounding level / summary option once (research.md R3), snapshots the
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
	// user has already started reviewing (research.md R3).
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

// StartRun is the background half of a workspace run, dispatched by
// worker.Handler on payload.GenerationRunID (T012). This phase runs only the
// existing summary stage — no ranking or suggestion LLM calls yet (those are
// Phase 4/5) — so the experience and skills sections are already `ready` from
// StartGenerationRun's seeding; only the summary section is still `running`
// when this is called.
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

	_ = s.q.SetRunState(ctx, sqlcgen.SetRunStateParams{ID: rid, State: string(finalState)})
	if rec != nil {
		rec.Ok(ctx, runID, map[string]any{"state": string(finalState)})
	}
	return nil
}

// selectedProfileHighlights gathers the selected experience achievements
// already persisted by StartGenerationRun's seeding, in section-then-item
// position order — the summary brief's input, standing in for
// SelectedHighlights(TailoredSelection) until US2 wires real ranking (T044).
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
// its sections and their items, in position order (contracts/rest-api.md).
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
	return s.runToDto(ctx, run, sections, items), nil
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
		out = append(out, s.runToDto(ctx, run, sections, items))
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

func (s *Service) runToDto(ctx context.Context, run sqlcgen.GenerationRun, sections []sqlcgen.GenerationSection, items []sqlcgen.GenerationItem) dto.GenerationRunDto {
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

	exportStatus := ""
	if run.ExportStatus != nil {
		exportStatus = *run.ExportStatus
	}
	var report *dto.OverflowReportDto
	if len(run.ExportReport) > 0 {
		var r dto.OverflowReportDto
		if json.Unmarshal(run.ExportReport, &r) == nil {
			report = &r
		}
	}

	return dto.GenerationRunDto{
		ID:                 dbutil.UUIDString(run.ID),
		State:              run.State,
		Vacancy:            dto.GenerationVacancyDto{Company: run.VacancyCompany, Title: run.VacancyTitle},
		JobID:              dbutil.UUIDStringPtr(run.JobID),
		GroundingLevel:     run.GroundingLevel,
		SummaryOptionID:    run.SummaryOptionID,
		SummarySubstituted: false,
		MasterChanged:      s.masterChanged(ctx, run),
		ShapeConfig:        shapeConfigToDto(cfg),
		Export: dto.GenerationExportDto{
			Status:     exportStatus,
			DocumentID: dbutil.UUIDStringPtr(run.ExportDocumentID),
			Report:     report,
		},
		Sections:  sectionDtos,
		CreatedAt: timestamptzString(run.CreatedAt),
		UpdatedAt: timestamptzString(run.UpdatedAt),
	}
}

// masterChanged is FR-022: the run's snapshot hash compared against the
// profile's current master content hash. Best-effort — a profile that no
// longer resolves (deleted, config cleared) reports unchanged rather than
// failing the whole workspace response over a staleness check.
func (s *Service) masterChanged(ctx context.Context, run sqlcgen.GenerationRun) bool {
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
	return hash != run.MasterContentHash
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
