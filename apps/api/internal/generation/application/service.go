package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/generation/infrastructure"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/strutil"
)

const (
	groundingAttempts = 2

	selectionAttempts = 3

	shapeAttempts = 2
)

var coverLetterMaxTokens = 1024

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type coverLetterResult struct {
	Letter string `json:"letter"`
}

type ShapeProvider interface {
	Shape(ctx context.Context) domain.ShapeConfig
}

type SummaryModelProvider interface {
	SummaryOption(ctx context.Context) domain.SummaryOption
}

type GenerationRouters struct {
	Analyze llm.Provider
	Select  llm.Provider

	Premium llm.Provider

	Summary llm.Provider

	SummaryByOption map[string]llm.Provider
	Cover           llm.Provider
}

type Service struct {
	q            domain.Repository
	profiles     domain.ProfileStore
	htmlRenderer *infrastructure.HtmlPdfRenderer
	rendercv     *infrastructure.RenderCvRenderer
	llm          GenerationRouters
	genModel     string
	masterPath   string
	defaultLevel domain.GroundingLevel
	shape        ShapeProvider
	summaryModel SummaryModelProvider

	enqueuer queue.Enqueuer

	tx domain.TxRunner

	exportRender renderDeps
}

func (s *Service) SetSummaryModelProvider(p SummaryModelProvider) { s.summaryModel = p }

func (s *Service) summaryOption(ctx context.Context) domain.SummaryOption {

	if opt, ok := summaryOptionFromContext(ctx); ok {
		return opt
	}
	if s.summaryModel == nil {
		return domain.DefaultSummaryOption()
	}
	return s.summaryModel.SummaryOption(ctx)
}

type summaryOptionCtxKey struct{}

func WithSummaryOption(ctx context.Context, opt domain.SummaryOption) context.Context {
	return context.WithValue(ctx, summaryOptionCtxKey{}, opt)
}

func summaryOptionFromContext(ctx context.Context) (domain.SummaryOption, bool) {
	opt, ok := ctx.Value(summaryOptionCtxKey{}).(domain.SummaryOption)
	return opt, ok && opt.ID != ""
}

func (s *Service) summaryProviderFor(opt domain.SummaryOption) llm.Provider {
	if p, ok := s.llm.SummaryByOption[opt.ID]; ok && p != nil {
		return p
	}
	return s.llm.Summary
}

func NewService(q domain.Repository, profiles domain.ProfileStore, htmlRenderer *infrastructure.HtmlPdfRenderer, rendercv *infrastructure.RenderCvRenderer, routers GenerationRouters, genModel, masterPath, defaultLevel string, shape ShapeProvider) *Service {
	if masterPath == "" {
		masterPath = "./resume/resume.yaml"
	}
	return &Service{
		q: q, profiles: profiles, htmlRenderer: htmlRenderer, rendercv: rendercv, llm: routers, genModel: genModel,
		masterPath: masterPath, defaultLevel: domain.ParseGroundingLevel(defaultLevel), shape: shape,
	}
}

func (s *Service) shapeConfig(ctx context.Context) domain.ShapeConfig {
	if s.shape == nil {
		return domain.DefaultShapeConfig()
	}
	return s.shape.Shape(ctx)
}

func (s *Service) docModel(served string) string {
	if served != "" {
		return served
	}
	if s.genModel != "" {
		return s.genModel
	}
	return s.llm.Select.ModelName()
}

func sanitize(s string) string {
	out := sanitizeRe.ReplaceAllString(s, "_")
	out = strings.ToLower(out)
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

func (s *Service) selectWithCompleteness(ctx context.Context, master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, prevViolations []string, cfg domain.ShapeConfig, rec *activity.Recorder, prov *runProvenance) (domain.TailoredSelection, error) {
	var last domain.CompletenessReport
	for attempt := 0; attempt < selectionAttempts; attempt++ {
		provider, tier := s.llm.Select, "economy"
		if attempt == selectionAttempts-1 {
			provider, tier = s.llm.Premium, "premium"
		}
		payload, err := observe(ctx, prov, stageSelect, tier == "premium", func(ctx context.Context) (domain.TailoredSelection, error) {
			return selectContent(ctx, provider, s.genModel, master, analysis, level, prevViolations, cfg)
		})
		if err != nil {
			return domain.TailoredSelection{}, err
		}
		probe, err := domain.MergeTailored(master, payload, nil, level)
		if err != nil {
			return domain.TailoredSelection{}, err
		}
		domain.ApplySectionToggles(probe, cfg)
		report := domain.VerifyCompleteness(master, probe, analysis, cfg)
		if !report.Shortfall {
			if tier == "premium" && rec != nil {
				rec.Step(ctx, "selection escalated to premium model after repeated shortfalls", map[string]any{"escalated": true})
			}
			if report.StructuralFallback && rec != nil {
				rec.Step(ctx, "completeness: vacancy analysis listed no required skills, structural check used instead", map[string]any{"structuralFallback": true})
			}
			return payload, nil
		}
		last = report
		if rec != nil {
			rec.Step(ctx, fmt.Sprintf("completeness shortfall on %s model (attempt %d/%d)", tier, attempt+1, selectionAttempts), map[string]any{
				"requiredMissing":    last.RequiredMissing,
				"niceToHaveRetained": last.NiceToHaveRetained,
				"bulletShortfalls":   last.BulletShortfalls,
				"tier":               tier,
			})
		}
	}
	return domain.TailoredSelection{}, fmt.Errorf("selection incomplete after %d attempts: %s", selectionAttempts, last.Reason())
}

func (s *Service) summarize(ctx context.Context, master domain.RendercvMaster, payload domain.TailoredSelection, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig, rec *activity.Recorder, prov *runProvenance, summaryLC llm.Provider) (*domain.TailoredSummary, error) {
	if summaryLC == nil {
		summaryLC = s.llm.Summary
	}
	if rec != nil {
		rec.Step(ctx, "writing summary (premium model)", nil)
	}
	min, max := summarySelectRange(cfg)
	brief := domain.SummaryBrief{
		Analysis:         analysis,
		TotalYears:       domain.DeriveTotalExperienceYears(master),
		Highlights:       domain.SelectedHighlights(master, payload, level),
		SkillGroupLabels: domain.SkillGroupLabels(master),
		SkillLines:       domain.RankedSkillLines(master, analysis, cfg),
		SentenceMin:      min,
		SentenceMax:      max,
	}
	summary, err := observe(ctx, prov, stageSummary, false, func(ctx context.Context) (domain.TailoredSummary, error) {
		return writeSummary(ctx, summaryLC, s.genModel, brief)
	})
	if err != nil {
		return nil, fmt.Errorf("summary: %w", err)
	}

	violations := domain.VerifySummaryGrounding(master, summary, brief)
	if len(violations) == 0 {
		return &summary, nil
	}
	if rec != nil {
		rec.Step(ctx, "summary grounding violation, re-prompting", map[string]any{"violations": violations})
	}
	retry, err := observe(ctx, prov, stageSummary, false, func(ctx context.Context) (domain.TailoredSummary, error) {
		return writeSummary(ctx, summaryLC, s.genModel, brief.WithViolations(violations))
	})
	if err != nil {
		return nil, fmt.Errorf("summary re-prompt: %w", err)
	}
	reViolations := domain.VerifySummaryGrounding(master, retry, brief)
	if len(reViolations) == 0 {
		return &retry, nil
	}
	stripped := domain.StripSummaryViolations(retry, reViolations)
	if rec != nil {
		rec.Step(ctx, "summary grounding violation recurred, offending claims stripped", map[string]any{
			"violations": reViolations,
			"before":     retry.Summary,
			"after":      stripped.Summary,
		})
	}
	return &stripped, nil
}

func (s *Service) tailorRendercvResume(ctx context.Context, master domain.RendercvMaster, target domain.VacancyTarget, vacancy string, level domain.GroundingLevel, cfg domain.ShapeConfig, hints *domain.VacancyHints, rec *activity.Recorder, prov *runProvenance) (domain.RendercvMaster, domain.VacancyAnalysis, error) {

	ctx = withRunTrace(ctx, rec)

	summaryOpt := s.summaryOption(ctx)
	summaryLC := s.summaryProviderFor(summaryOpt)
	if prov != nil {
		prov.summaryOption = summaryOpt.ID
	}
	if rec != nil {
		rec.Step(ctx, "summary model: "+summaryOpt.Label, map[string]any{
			"option": summaryOpt.ID,
			"cost":   summaryOpt.Cost,
		})
	}

	if rec != nil {
		rec.Step(ctx, "analyzing vacancy", nil)
	}
	analysis, err := observe(ctx, prov, stageAnalyze, false, func(ctx context.Context) (domain.VacancyAnalysis, error) {
		return analyzeVacancy(ctx, s.llm.Analyze, s.genModel, target, vacancy, hints)
	})
	if err != nil {
		return nil, domain.VacancyAnalysis{}, fmt.Errorf("vacancy analysis: %w", err)
	}
	if thin := domain.VacancyIsThin(vacancy, analysis); thin && rec != nil {
		rec.Step(ctx, "vacancy text carries no requirements; tailoring from the job title alone", map[string]any{
			"vacancyWords": len(strings.Fields(vacancy)),
		})
	}
	analysis = domain.MergeTitleSkills(analysis, domain.TitleRequiredSkills(master, target.Title))

	var lastViolations []string
	for attempt := 0; attempt < groundingAttempts; attempt++ {
		if rec != nil {
			rec.Step(ctx, fmt.Sprintf("tailoring resume (LLM) (attempt %d/%d)", attempt+1, groundingAttempts), nil)
		}
		payload, err := s.selectWithCompleteness(ctx, master, analysis, level, lastViolations, cfg, rec, prov)
		if err != nil {
			return nil, domain.VacancyAnalysis{}, err
		}
		summary, err := s.summarize(ctx, master, payload, analysis, level, cfg, rec, prov, summaryLC)
		if err != nil {
			return nil, domain.VacancyAnalysis{}, err
		}
		merged, err := domain.MergeTailored(master, payload, summary, level)
		if err != nil {
			return nil, domain.VacancyAnalysis{}, err
		}

		domain.ApplySectionToggles(merged, cfg)

		domain.RankSkills(merged, analysis, cfg)

		domain.TrimSkillGroups(merged, analysis)

		domain.RankProjects(merged, analysis, cfg)
		domain.TrimProjectHighlights(merged, analysis)
		report := domain.ApplyHardLimits(master, merged, cfg)
		recordShortfalls(ctx, rec, report)

		skillsBefore := skillDetailEntries(merged)
		domain.DropUngroundedSkillTokens(master, merged)
		if rec != nil {
			rec.Step(ctx, "grounding: ungrounded skill tokens dropped", map[string]any{
				"tokens": droppedSkillEntries(skillsBefore, skillDetailEntries(merged)),
			})
		}
		if rec != nil {
			rec.Step(ctx, fmt.Sprintf("grounding check (attempt %d/%d)", attempt+1, groundingAttempts), nil)
		}
		lastViolations = domain.VerifyRendercvGrounding(master, merged, level, analysis)

		lastViolations = append(lastViolations, domain.VerifyHighlightProvenance(master, merged)...)
		if len(lastViolations) == 0 {
			fixed, err := s.fixStructureIntegrity(ctx, master, merged, analysis, level, cfg, rec)
			if rec != nil && prov != nil {

				rec.Step(ctx, "generation stages", prov.meta())
			}
			return fixed, analysis, err
		}
	}
	return nil, domain.VacancyAnalysis{}, fmt.Errorf("tailored rendercv resume failed grounding check: %s", strings.Join(lastViolations, "; "))
}

func shapeConfigMeta(cfg domain.ShapeConfig) map[string]any {
	return map[string]any{
		"summaryLines":          cfg.SummaryLines,
		"skillsEnabled":         cfg.SkillsEnabled,
		"skillsMaxGroups":       cfg.SkillsMaxGroups,
		"experienceBulletsMin":  cfg.ExperienceBulletsMin,
		"experienceBulletsMax":  cfg.ExperienceBulletsMax,
		"targetPages":           cfg.TargetPages,
		"projectsEnabled":       cfg.ProjectsEnabled,
		"projectsMin":           cfg.ProjectsMin,
		"projectsMax":           cfg.ProjectsMax,
		"projectBulletsMax":     cfg.ProjectBulletsMax,
		"certificationsEnabled": cfg.CertificationsEnabled,
		"certificationsMin":     cfg.CertificationsMin,
		"certificationsMax":     cfg.CertificationsMax,
	}
}

func recordShortfalls(ctx context.Context, rec *activity.Recorder, report domain.ShapeReport) {
	if rec == nil {
		return
	}
	for _, sf := range report.Shortfalls {
		rec.Step(ctx, "resume shape shortfall: "+sf.Path, map[string]any{
			"path":      sf.Path,
			"requested": sf.Requested,
			"available": sf.Available,
		})
	}
}

func skillDetailEntries(doc domain.RendercvMaster) []string {
	var out []string
	for _, g := range domain.AsSliceOfMaps(domain.CvSections(doc)["skills"]) {
		for _, entry := range strings.Split(domain.StringField(g, "details"), ",") {
			if e := strings.TrimSpace(entry); e != "" {
				out = append(out, e)
			}
		}
	}
	return out
}

func droppedSkillEntries(before, after []string) []string {
	remaining := make(map[string]int, len(after))
	for _, e := range after {
		remaining[e]++
	}
	var dropped []string
	for _, e := range before {
		if remaining[e] > 0 {
			remaining[e]--
			continue
		}
		dropped = append(dropped, e)
	}
	sort.Strings(dropped)
	return dropped
}

type renderDeps struct {
	render     func(ctx context.Context, merged domain.RendercvMaster, name string) (string, error)
	countPages func(pdfPath string) (int, error)
	expand     func(ctx context.Context, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSelection, error)

	condense func(doc domain.RendercvMaster, cfg domain.ShapeConfig) (domain.RendercvMaster, bool, error)
}

func (s *Service) defaultRenderDeps() renderDeps {
	return renderDeps{
		render: func(ctx context.Context, merged domain.RendercvMaster, name string) (string, error) {
			_, pdfPath, err := s.rendercv.Render(ctx, merged, name)
			return pdfPath, err
		},
		countPages: infrastructure.CountPages,
		expand: func(ctx context.Context, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSelection, error) {
			return expandContent(ctx, s.llm.Select, s.genModel, merged, analysis, level, cfg)
		},
		condense: func(doc domain.RendercvMaster, cfg domain.ShapeConfig) (domain.RendercvMaster, bool, error) {
			shorter, err := domain.DeepCloneYAML(doc)
			if err != nil {
				return nil, false, err
			}
			_, max := bulletsCondenseRange(cfg)
			return shorter, domain.TrimHighlights(shorter, max), nil
		},
	}
}

type renderOutcome struct {
	pdfPath  string
	pages    int
	conflict bool
}

func (s *Service) renderResume(ctx context.Context, master, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig, baseName string, rec *activity.Recorder) (string, error) {

	domain.ApplyFontSize(merged, cfg)
	outcome, err := s.renderToPageTarget(ctx, s.defaultRenderDeps(), master, merged, analysis, level, cfg, baseName, rec)
	if err != nil {
		return "", err
	}
	if rec != nil {
		if outcome.conflict {
			rec.Step(ctx, "resume shape: page target overrode section length targets", map[string]any{
				"conflict":    "page_target_overrides_section_lengths",
				"pageTarget":  cfg.TargetPages,
				"pagesBefore": outcome.pages,
			})
		}
		rec.Step(ctx, "resume page target", map[string]any{
			"pageTarget":    cfg.TargetPages,
			"pagesAchieved": outcome.pages,
			"targetMet":     outcome.pages == cfg.TargetPages,
		})
	}
	return outcome.pdfPath, nil
}

func (s *Service) renderToPageTarget(ctx context.Context, deps renderDeps, master, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig, baseName string, rec *activity.Recorder) (renderOutcome, error) {
	pdfPath, err := deps.render(ctx, merged, baseName)
	if err != nil {
		return renderOutcome{}, err
	}

	pages, err := deps.countPages(pdfPath)
	if err != nil {
		slog.Warn("could not count PDF pages, proceeding with rendered file", "path", pdfPath, "err", err)
		return renderOutcome{pdfPath: pdfPath}, nil
	}
	if pages == cfg.TargetPages {
		return renderOutcome{pdfPath: pdfPath, pages: pages}, nil
	}

	if pages < cfg.TargetPages {
		if rec != nil {
			rec.Step(ctx, fmt.Sprintf("resume is only %d page(s), asking LLM to expand content", pages), map[string]any{"pages": pages, "pageTarget": cfg.TargetPages})
		}
		expanded, err := deps.expand(ctx, merged, analysis, level, cfg)
		if err != nil {
			slog.Warn("LLM expand failed, returning short version", "err", err)
			return renderOutcome{pdfPath: pdfPath, pages: pages}, nil
		}
		reMerged, err := domain.MergeTailored(merged, expanded, nil, level)
		if err != nil {
			slog.Warn("merge after expand failed, returning short version", "err", err)
			return renderOutcome{pdfPath: pdfPath, pages: pages}, nil
		}
		domain.DropUngroundedSkillTokens(merged, reMerged)
		domain.ApplyHardLimits(master, reMerged, cfg)

		domain.StripUngroundedHighlights(merged, reMerged)
		expandedPath, err := deps.render(ctx, reMerged, baseName+"-expanded")
		if err != nil {
			return renderOutcome{}, err
		}
		expandedPages, err := deps.countPages(expandedPath)
		if err != nil {
			slog.Warn("could not count PDF pages after expand", "path", expandedPath, "err", err)
			return renderOutcome{pdfPath: expandedPath}, nil
		}
		return renderOutcome{pdfPath: expandedPath, pages: expandedPages}, nil
	}

	out := renderOutcome{pdfPath: pdfPath, pages: pages}
	for attempt := 0; attempt < shapeAttempts && out.pages > cfg.TargetPages; attempt++ {
		var candidate domain.RendercvMaster
		var suffix string

		if attempt == 0 {
			if rec != nil {
				rec.Step(ctx, fmt.Sprintf("resume overflows past %d page(s), applying compact design", cfg.TargetPages), map[string]any{"pages": out.pages, "pageTarget": cfg.TargetPages})
			}
			domain.CompactDesign(merged)
			candidate, suffix = merged, "-compact"
		} else {
			if rec != nil {
				rec.Step(ctx, "resume still overflows, dropping the least relevant bullets", map[string]any{"pages": out.pages, "pageTarget": cfg.TargetPages})
			}
			shorter, trimmed, err := deps.condense(merged, cfg)
			if err != nil {
				slog.Warn("condense failed, returning compact version", "err", err)
				break
			}
			if !trimmed {

				break
			}
			domain.CompactDesign(shorter)

			out.conflict = true
			candidate, suffix = shorter, "-condensed"
		}

		candidatePath, err := deps.render(ctx, candidate, baseName+suffix)
		if err != nil {
			return renderOutcome{}, err
		}
		candidatePages, err := deps.countPages(candidatePath)
		if err != nil {
			slog.Warn("could not count PDF pages after shape adjustment", "path", candidatePath, "err", err)
			return renderOutcome{pdfPath: candidatePath, conflict: out.conflict}, nil
		}
		out.pdfPath, out.pages = candidatePath, candidatePages
	}
	return out, nil
}

func (s *Service) fixStructureIntegrity(ctx context.Context, master, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig, rec *activity.Recorder) (domain.RendercvMaster, error) {

	structViolations := domain.VerifyStructureIntegrity(master, merged)
	if len(structViolations) > 0 {
		if rec != nil {
			rec.Step(ctx, "structure integrity: years assertion detected, re-prompting", map[string]any{"violations": len(structViolations)})
		}
		rePrompted, err := retailorForStructure(ctx, s.llm.Select, s.genModel, master, analysis, level, structViolations, cfg)
		if err != nil {
			return nil, err
		}
		reMerged, err := domain.MergeTailored(master, rePrompted, domain.CurrentSummary(merged), level)
		if err != nil {
			return nil, err
		}
		domain.ApplySectionToggles(reMerged, cfg)
		domain.RankSkills(reMerged, analysis, cfg)
		domain.TrimSkillGroups(reMerged, analysis)
		domain.RankProjects(reMerged, analysis, cfg)
		domain.TrimProjectHighlights(reMerged, analysis)
		domain.ApplyHardLimits(master, reMerged, cfg)
		reViolations := domain.VerifyStructureIntegrity(master, reMerged)
		if len(reViolations) == 0 {
			merged = reMerged
		} else {
			if rec != nil {
				rec.Step(ctx, "structure integrity: years assertion recurred, stripped", map[string]any{"violations": len(reViolations)})
			}
			merged = domain.StripStructureViolations(reMerged, reViolations)
		}
	}

	driftViolations := domain.VerifyHighlightGrounding(master, merged)
	if len(driftViolations) == 0 {
		return merged, nil
	}
	if rec != nil {
		rec.Step(ctx, "structure integrity: highlight drift detected, re-prompting", map[string]any{"violations": len(driftViolations)})
	}
	rePrompted, err := retailorForStructure(ctx, s.llm.Select, s.genModel, master, analysis, level, driftViolations, cfg)
	if err != nil {
		return nil, err
	}
	reMerged, err := domain.MergeTailored(master, rePrompted, domain.CurrentSummary(merged), level)
	if err != nil {
		return nil, err
	}
	domain.ApplySectionToggles(reMerged, cfg)
	domain.RankSkills(reMerged, analysis, cfg)
	domain.TrimSkillGroups(reMerged, analysis)
	domain.RankProjects(reMerged, analysis, cfg)
	domain.TrimProjectHighlights(reMerged, analysis)
	domain.ApplyHardLimits(master, reMerged, cfg)
	reDrift := domain.VerifyHighlightGrounding(master, reMerged)
	if len(reDrift) == 0 {
		return reMerged, nil
	}
	if rec != nil {
		rec.Step(ctx, "structure integrity: highlight drift recurred, replaced with master bullets", map[string]any{"violations": len(reDrift)})
	}
	return domain.StripUngroundedHighlights(master, reMerged), nil
}

func (s *Service) Generate(ctx context.Context, jobID, docType string, profileID *string, rec *activity.Recorder) (dto.GeneratedDocumentDto, error) {
	if rec != nil {
		rec.Step(ctx, "loading profile & match", nil)
	}

	jid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	job, err := s.q.GetJobByID(ctx, jid)
	if err != nil {
		return dto.GeneratedDocumentDto{}, fmt.Errorf("job %s not found", jobID)
	}

	var prof sqlcgen.Profile
	if profileID != nil {
		prof, err = s.profiles.Get(ctx, *profileID)
	} else {
		prof, err = s.profiles.GetDefault(ctx)
	}
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	if prof.RendercvConfig == nil {
		return dto.GeneratedDocumentDto{}, fmt.Errorf("precondition failed: profile has no RenderCV config — upload one first")
	}
	master, err := domain.MasterFromProfile(prof)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	profileText := domain.RendercvToText(master)

	maxVersion, err := s.q.MaxDocumentVersion(ctx, sqlcgen.MaxDocumentVersionParams{JobId: jid, Type: docType})
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	version := maxVersion + 1
	baseName := sanitize(job.Company + "-" + job.Title)

	var content []byte
	var pdfPath string
	genCtx, served := llm.WithServedModelCapture(ctx)

	genCtx = withRunTrace(genCtx, rec)
	prov := &runProvenance{}

	if docType == string(dto.DocumentTypeResume) {

		cfg := s.shapeConfig(ctx)
		if rec != nil {
			rec.Step(ctx, "resume shape config", shapeConfigMeta(cfg))
		}
		tailored, analysis, err := s.tailorRendercvResume(genCtx, master, domain.VacancyTarget{Title: job.Title, Company: job.Company}, job.Description, s.defaultLevel, cfg, nil, rec, prov)
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		content, err = json.Marshal(tailored)
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		if rec != nil {
			rec.Step(ctx, "rendering PDF", nil)
		}
		p, err := s.renderResume(ctx, master, tailored, analysis, s.defaultLevel, cfg, fmt.Sprintf("%s-resume-v%d", baseName, version), rec)
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		pdfPath = p
	} else {
		if rec != nil {
			rec.Step(ctx, "writing cover letter (LLM)", nil)
		}
		letter, err := s.writeCoverLetter(genCtx, profileText, prof.ExtraNotes, job.Company, job.Title, job.Description)
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		content, err = json.Marshal(map[string]string{"text": letter})
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		var namePtr *string
		if basics, _ := master["cv"].(map[string]any); basics != nil {
			if n, _ := basics["name"].(string); n != "" {
				namePtr = &n
			}
		}
		if rec != nil {
			rec.Step(ctx, "rendering PDF", nil)
		}
		p, err := s.htmlRenderer.RenderCoverLetter(ctx, letter, namePtr, job.Company, job.Title, fmt.Sprintf("%s-cover-v%d.pdf", baseName, version))
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		pdfPath = p
	}

	doc, err := s.q.InsertGeneratedDocument(ctx, withProvenance(sqlcgen.InsertGeneratedDocumentParams{
		JobId: jid, Type: docType, Version: version, Content: content, PdfPath: &pdfPath, Model: s.docModel(*served),
	}, prov))
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}

	if job.Status == "found" || job.Status == "shortlisted" {
		if _, err := s.q.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{ID: jid, Status: "docs_generated"}); err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		events, _ := json.Marshal([]dto.ApplicationEvent{{Status: "docs_generated", At: time.Now().UTC().Format(time.RFC3339)}})
		if err := s.q.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
			JobId: jid, Status: "docs_generated", Events: events,
		}); err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
	}

	if rec != nil {
		rec.Ok(ctx, dbutil.UUIDString(doc.ID), map[string]any{"version": version})
	}

	return toDocumentDto(doc), nil
}

func (s *Service) writeCoverLetter(ctx context.Context, profileText string, extraNotes *string, company, title, vacancyText string) (string, error) {
	prompt := "Write a short cover letter (maximum 150 words, exactly 3 paragraphs separated by blank lines) " +
		"for this application.\n\n" +
		"Structure: (1) hook referencing the company and role, (2) 2-3 concrete matching experiences " +
		"from the candidate's real background, (3) brief close.\n" +
		`STRICT RULES: mention only experience present in the profile below; no invented facts, ` +
		`no clichés like "I am writing to express". Plain text, no salutation placeholders like [Hiring Manager] — ` +
		`use "Hello," if needed.` + "\n\n" +
		"CANDIDATE PROFILE:\n" + strutil.Truncate(profileText, 8000) + "\n\n"
	if extraNotes != nil && *extraNotes != "" {
		prompt += "EXTRA NOTES:\n" + strutil.Truncate(*extraNotes, 1500) + "\n\n"
	}
	prompt += fmt.Sprintf("JOB:\nTitle: %s\nCompany: %s\nDescription:\n%s", title, company, strutil.Truncate(vacancyText, 4000))

	result, err := llm.CompleteStructured[coverLetterResult](ctx, s.llm.Cover, prompt, &llm.CompleteOptions{
		System:    "You write concise, concrete, honest cover letters.",
		Model:     s.genModel,
		MaxTokens: &coverLetterMaxTokens,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Letter), nil
}

func (s *Service) ListDocuments(ctx context.Context, jobID string) ([]dto.GeneratedDocumentDto, error) {
	jid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListDocumentsForJob(ctx, jid)
	if err != nil {
		return nil, err
	}
	out := make([]dto.GeneratedDocumentDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDocumentDto(r))
	}
	return out, nil
}

func (s *Service) ListDocumentStatuses(ctx context.Context, jobID string) ([]dto.DocumentStatusDto, error) {
	jid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListDocumentsForJob(ctx, jid)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DocumentStatusDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.DocumentStatusDto{
			ID:        dbutil.UUIDString(r.ID),
			Type:      r.Type,
			Version:   int(r.Version),
			CreatedAt: dbutil.Timestamp(r.CreatedAt),
		})
	}
	return out, nil
}

func (s *Service) GetDocument(ctx context.Context, id string) (sqlcgen.GeneratedDocument, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return sqlcgen.GeneratedDocument{}, err
	}
	row, err := s.q.GetDocumentByID(ctx, uid)
	if err != nil {
		return sqlcgen.GeneratedDocument{}, fmt.Errorf("document %s not found", id)
	}
	return row, nil
}

func (s *Service) GetDocumentDto(ctx context.Context, id string) (dto.GeneratedDocumentDto, error) {
	row, err := s.GetDocument(ctx, id)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	return toDocumentDto(row), nil
}

func (s *Service) GetDocumentPdfPath(ctx context.Context, id string) (*string, error) {
	row, err := s.GetDocument(ctx, id)
	if err != nil {
		return nil, err
	}
	return row.PdfPath, nil
}

func (s *Service) GetDocumentDownload(ctx context.Context, id string) (path *string, filename string, err error) {
	row, err := s.GetDocument(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if row.PdfPath == nil {
		return nil, "", nil
	}
	filename = filepath.Base(*row.PdfPath)
	if row.Type != string(dto.DocumentTypeResume) {
		return row.PdfPath, filename, nil
	}
	if named := s.resumeDownloadName(ctx); named != "" {
		filename = named
	}
	return row.PdfPath, filename, nil
}

func (s *Service) resumeDownloadName(ctx context.Context) string {
	prof, err := s.profiles.GetDefault(ctx)
	if err != nil || prof.RendercvConfig == nil {
		return ""
	}
	master, err := domain.MasterFromProfile(prof)
	if err != nil {
		return ""
	}
	cv, _ := master["cv"].(map[string]any)
	if cv == nil {
		return ""
	}
	name, _ := cv["name"].(string)
	slug := downloadNameSlug(name)
	if slug == "" {
		return ""
	}
	return "CV_" + slug + ".pdf"
}

func downloadNameSlug(name string) string {
	var b strings.Builder
	pendingSep := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingSep && b.Len() > 0 {
				b.WriteByte('_')
			}
			pendingSep = false
			b.WriteRune(r)
			continue
		}
		pendingSep = true
	}
	return b.String()
}

func (s *Service) UpdateDocument(ctx context.Context, id, text string) (dto.GeneratedDocumentDto, error) {
	doc, err := s.GetDocument(ctx, id)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	if doc.Type != string(dto.DocumentTypeCoverLetter) || text == "" {
		return dto.GeneratedDocumentDto{}, fmt.Errorf("only cover_letter text is editable")
	}
	company, title := "", ""
	if doc.JobId.Valid {
		job, err := s.q.GetJobByID(ctx, doc.JobId)
		if err != nil {
			return dto.GeneratedDocumentDto{}, fmt.Errorf("job %s not found", dbutil.UUIDString(doc.JobId))
		}
		company, title = job.Company, job.Title
	} else {
		if doc.Company != nil {
			company = *doc.Company
		}
		if doc.Title != nil {
			title = *doc.Title
		}
	}
	prof, err := s.profiles.GetDefault(ctx)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	if prof.RendercvConfig == nil {
		return dto.GeneratedDocumentDto{}, fmt.Errorf("profile has no RenderCV config")
	}
	master, err := domain.MasterFromProfile(prof)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}

	baseName := sanitize(company + "-" + title)
	var namePtr *string
	if basics, _ := master["cv"].(map[string]any); basics != nil {
		if n, _ := basics["name"].(string); n != "" {
			namePtr = &n
		}
	}
	pdfPath, err := s.htmlRenderer.RenderCoverLetter(ctx, text, namePtr, company, title, fmt.Sprintf("%s-cover-v%d.pdf", baseName, doc.Version))
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	updated, err := s.q.UpdateDocumentContent(ctx, sqlcgen.UpdateDocumentContentParams{ID: doc.ID, Content: content, PdfPath: &pdfPath})
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	return toDocumentDto(updated), nil
}

func toDocumentDto(r sqlcgen.GeneratedDocument) dto.GeneratedDocumentDto {
	var content any
	_ = dbutil.UnmarshalJSONB(r.Content, &content)
	return dto.GeneratedDocumentDto{
		ID:        dbutil.UUIDString(r.ID),
		JobID:     dbutil.UUIDStringPtr(r.JobId),
		Type:      r.Type,
		Version:   int(r.Version),
		Content:   content,
		PdfPath:   r.PdfPath,
		Model:     r.Model,
		Company:   r.Company,
		Title:     r.Title,
		Vacancy:   r.Vacancy,
		CreatedAt: dbutil.Timestamp(r.CreatedAt),

		SummaryModel:       r.SummaryModel,
		SummaryOptionID:    r.SummaryOptionId,
		SummarySubstituted: r.SummarySubstituted,
		SelectionEscalated: r.SelectionEscalated,
		StageCostUsd:       numericToFloatPtr(r.StageCostUsd),
	}
}

func numericToFloatPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

func withRunTrace(ctx context.Context, rec *activity.Recorder) context.Context {
	if rec == nil {
		return ctx
	}
	id := dbutil.UUIDString(rec.ID())
	if id == "" {
		return ctx
	}
	return llm.WithTraceID(ctx, id)
}
