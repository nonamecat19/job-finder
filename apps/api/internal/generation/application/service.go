package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/generation/infrastructure"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/strutil"
)

const (
	groundingAttempts = 2
	// selectionAttempts bounds the completeness ladder: two tries on the
	// economy model, then one on the premium model. The last attempt is the
	// escalation, so raising this raises economy retries, not premium ones.
	selectionAttempts = 3
	// shapeAttempts bounds the page-fit adjust cycle so an unreachable page
	// target cannot spin: compact design, then condense, then accept whatever
	// was reached.
	shapeAttempts = 2
)

// coverLetterMaxTokens bounds the cover-letter LLM output (033 FR-012).
// A 150-word letter is well under 1024 tokens; this leaves headroom.
var coverLetterMaxTokens = 1024

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type AdHocInput struct {
	Vacancy        string
	Company        string
	Title          string
	GroundingLevel *domain.GroundingLevel
	Hints          *domain.VacancyHints
}

type coverLetterResult struct {
	Letter string `json:"letter"`
}

// ShapeProvider hands the generation pipeline the user-configured resume
// shape. It is a narrow port so this package never imports the settings
// package; *resumeshape.Service satisfies it structurally.
type ShapeProvider interface {
	Shape(ctx context.Context) domain.ShapeConfig
}

// SummaryModelProvider hands the pipeline the summary option the user picked
// (034). Declared here for the same reason ShapeProvider is: so this package
// never imports the settings package. *summarymodel.Service satisfies it
// structurally.
type SummaryModelProvider interface {
	SummaryOption(ctx context.Context) domain.SummaryOption
}

// GenerationRouters is one LLM provider per generation stage. Each is a router
// over the same gateway, differing only in the task key it asks for, so which
// model serves a stage stays a deployment decision (035 FR-002/FR-003). Any of
// them may be the same provider — with no gateway configured they all route to
// the local model, which is exactly the pre-split behaviour.
type GenerationRouters struct {
	Analyze llm.Provider
	Select  llm.Provider
	// Premium serves the selection stage only after the economy model has
	// returned incomplete output twice (035 FR-007).
	Premium llm.Provider
	// Summary serves the default (`standard`) option, and is the fallback for
	// any option SummaryByOption does not carry. Keeping it a plain field
	// rather than folding it into the map is what makes a caller that knows
	// nothing about 034 — the eval harness, every existing test — behave
	// exactly as it did before the feature existed.
	Summary llm.Provider
	// SummaryByOption routes the non-default summary options a user can pick
	// (034). One entry per hosted catalogue option; the self-hosted option
	// resolves to a router with no gateway, which is the pre-split behaviour.
	// A missing entry is not an error: an option is a routing preference, and a
	// preference that can fail a resume run is a liability.
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
}

// SetSummaryModelProvider installs the 034 port. It is a setter rather than a
// NewService parameter because NewService already takes nine arguments and is
// called from a dozen tests, every one of which would have to grow a nil for a
// feature it does not exercise.
func (s *Service) SetSummaryModelProvider(p SummaryModelProvider) { s.summaryModel = p }

// summaryOption resolves the user's choice once, at the top of a run. Every
// step downstream takes the resolved value, so a settings change mid-run cannot
// alter the document being generated — the in-flight run finishes with the
// option it started with, by construction. This is the discipline shapeConfig
// established and it matters more here: swapping the model writing the summary
// halfway through a run would produce a document nobody chose.
func (s *Service) summaryOption(ctx context.Context) domain.SummaryOption {
	// A per-run choice wins over the stored default. It rides on the context
	// for the same reason the run trace does (withRunTrace, just below): the
	// tailoring path is eight frames deep and threading one more argument
	// through every one of them, plus every test that calls them, would buy
	// nothing this does not.
	if opt, ok := summaryOptionFromContext(ctx); ok {
		return opt
	}
	if s.summaryModel == nil {
		return domain.DefaultSummaryOption()
	}
	return s.summaryModel.SummaryOption(ctx)
}

type summaryOptionCtxKey struct{}

// WithSummaryOption carries a single run's chosen summary option. Callers that
// accept a choice on the request — the tailoring handler — set it here; a
// caller that sets nothing gets the user's stored default, and a deployment
// with no settings service at all gets the catalogue default.
func WithSummaryOption(ctx context.Context, opt domain.SummaryOption) context.Context {
	return context.WithValue(ctx, summaryOptionCtxKey{}, opt)
}

func summaryOptionFromContext(ctx context.Context) (domain.SummaryOption, bool) {
	opt, ok := ctx.Value(summaryOptionCtxKey{}).(domain.SummaryOption)
	return opt, ok && opt.ID != ""
}

// summaryProviderFor picks the provider serving a chosen option, falling back
// to the stage default when the option has no router wired — an option added to
// the catalogue before its deployment exists, or a gateway-less install where
// every router is the local model anyway.
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

// shapeConfig resolves the shape once at the top of a generation run. Every
// step downstream takes the resolved value, so a settings change mid-run
// cannot alter the document being generated — the in-flight run finishes with
// the config it started with, by construction.
func (s *Service) shapeConfig(ctx context.Context) domain.ShapeConfig {
	if s.shape == nil {
		return domain.DefaultShapeConfig()
	}
	return s.shape.Shape(ctx)
}

// docModel returns the model recorded on generated documents, falling back to
// the provider default when no task-specific generation model is configured.
// docModel resolves the model label stored on a generated document. served
// is what the provider actually used to fulfil the call (captured via
// llm.WithServedModelCapture) — preferred because with gateway routing,
// llmc.ModelName() only knows the task-routing key (e.g. "generation"), not
// which upstream model the LiteLLM fallback chain actually served.
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

func (s *Service) masterFor(ctx context.Context, profileID *string) (domain.RendercvMaster, error) {
	var prof sqlcgen.Profile
	var err error
	if profileID != nil && *profileID != "" {
		prof, err = s.profiles.Get(ctx, *profileID)
	} else {
		prof, err = s.profiles.GetDefault(ctx)
	}
	if err == nil && prof.RendercvConfig != nil {
		return domain.MasterFromProfile(prof)
	}

	// Dev fallback to masterPath if no profile exists
	data, err := os.ReadFile(s.masterPath)
	if err != nil {
		return nil, fmt.Errorf("generation: read master resume from %s: %w", s.masterPath, err)
	}
	master, err := domain.ParseRendercv(string(data))
	if err != nil {
		return nil, fmt.Errorf("generation: parse master resume: %w", err)
	}
	return master, nil
}

// GenerateAdHoc tailors a resume from pasted vacancy text with no backing Job
// row, persisting it as a GeneratedDocument with jobId NULL so it shows up in
// the ad-hoc history list. It no longer writes a cover letter: that was a paid
// call and a share of the wait on every run, wanted or not, and is now
// requested explicitly against the finished resume (035 FR-013).
func (s *Service) GenerateAdHoc(ctx context.Context, in AdHocInput) (dto.GeneratedDocumentDto, error) {
	level := s.defaultLevel
	if in.GroundingLevel != nil {
		level = *in.GroundingLevel
	}
	master, err := s.masterFor(ctx, nil)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}

	company := in.Company
	if company == "" {
		company = "vacancy"
	}
	title := in.Title
	if title == "" {
		title = "resume"
	}
	baseName := sanitize(company + "-" + title)

	// Resolved once, at the top of the run: every step downstream uses this
	// value, so a settings change mid-run cannot alter this document.
	cfg := s.shapeConfig(ctx)

	resumeCtx, resumeServed := llm.WithServedModelCapture(ctx)
	prov := &runProvenance{}
	merged, analysis, err := s.tailorRendercvResume(resumeCtx, master, in.Vacancy, level, cfg, in.Hints, nil, prov)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	resumeContent, err := json.Marshal(merged)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	resumePdfPath, err := s.renderResume(ctx, master, merged, analysis, level, cfg, baseName+"-resume-"+strconv.FormatInt(time.Now().UnixMilli(), 10), nil)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	resumeRow, err := s.q.InsertGeneratedDocument(ctx, withProvenance(sqlcgen.InsertGeneratedDocumentParams{
		Type: string(dto.DocumentTypeResume), Version: 1, Content: resumeContent, PdfPath: &resumePdfPath, Model: s.docModel(*resumeServed),
		Company: &company, Title: &title, Vacancy: &in.Vacancy,
	}, prov))
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	return toDocumentDto(resumeRow), nil
}

// GenerateCoverLetterFor writes a cover letter for an already-generated
// resume, reusing that document's vacancy and company so the letter is aimed
// at the same posting the resume was tailored to (035 FR-013).
func (s *Service) GenerateCoverLetterFor(ctx context.Context, resumeID string) (dto.GeneratedDocumentDto, error) {
	rid, err := dbutil.ParseUUID(resumeID)
	if err != nil {
		return dto.GeneratedDocumentDto{}, apperr.NotFound("document", resumeID)
	}
	resume, err := s.q.GetDocumentByID(ctx, rid)
	if err != nil {
		return dto.GeneratedDocumentDto{}, apperr.NotFound("document", resumeID)
	}
	if resume.Type != string(dto.DocumentTypeResume) {
		return dto.GeneratedDocumentDto{}, apperr.Conflict("cover letters are generated from a resume, not a " + resume.Type)
	}

	master, err := s.masterFor(ctx, nil)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	var extraNotes *string
	if prof, profErr := s.profiles.GetDefault(ctx); profErr == nil {
		extraNotes = prof.ExtraNotes
	}

	company, title, vacancy := "vacancy", "resume", ""
	if resume.Company != nil {
		company = *resume.Company
	}
	if resume.Title != nil {
		title = *resume.Title
	}
	if resume.Vacancy != nil {
		vacancy = *resume.Vacancy
	}
	baseName := sanitize(company + "-" + title)

	profileText := domain.RendercvToText(master)
	coverCtx, coverServed := llm.WithServedModelCapture(ctx)
	letter, err := s.writeCoverLetter(coverCtx, profileText, extraNotes, company, title, vacancy)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	coverContent, err := json.Marshal(map[string]string{"text": letter})
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	var namePtr *string
	if basics, _ := master["cv"].(map[string]any); basics != nil {
		if n, _ := basics["name"].(string); n != "" {
			namePtr = &n
		}
	}
	coverPdfPath, err := s.htmlRenderer.RenderCoverLetter(ctx, letter, namePtr, company, title, fmt.Sprintf("%s-cover-%d.pdf", baseName, time.Now().UnixMilli()))
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	coverRow, err := s.q.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		JobId: resume.JobId, Type: string(dto.DocumentTypeCoverLetter), Version: nextCoverVersion(ctx, s, resume),
		Content: coverContent, PdfPath: &coverPdfPath, Model: s.docModel(*coverServed),
		Company: &company, Title: &title, Vacancy: &vacancy,
	})
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	return toDocumentDto(coverRow), nil
}

// nextCoverVersion keeps the existing (jobId, type, version) uniqueness rule:
// a second request for the same job supersedes rather than collides. Ad-hoc
// documents have a NULL jobId and are not covered by the constraint, so they
// stay at version 1.
func nextCoverVersion(ctx context.Context, s *Service, resume sqlcgen.GeneratedDocument) int32 {
	if !resume.JobId.Valid {
		return 1
	}
	existing, err := s.q.ListDocumentsForJob(ctx, resume.JobId)
	if err != nil {
		return 1
	}
	next := int32(1)
	for _, d := range existing {
		if d.Type == string(dto.DocumentTypeCoverLetter) && d.Version >= next {
			next = d.Version + 1
		}
	}
	return next
}

// ListAdHocDocuments returns all documents generated from pasted vacancy
// text (jobId NULL), newest first.
func (s *Service) ListAdHocDocuments(ctx context.Context) ([]dto.GeneratedDocumentDto, error) {
	rows, err := s.q.ListAdHocDocuments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.GeneratedDocumentDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDocumentDto(r))
	}
	return out, nil
}

// selectWithCompleteness runs the economy selection stage and gates its output
// on completeness. A cheap model's characteristic failure is not malformed
// output — it is well-formed output with content quietly missing, which
// nothing downstream would notice. One retry on the economy model, then the
// stage escalates to the premium one rather than rendering a hollowed-out
// resume (035 FR-006/FR-007).
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

// summarize runs the one premium call in a generation run. Its brief carries
// the selected achievements and the derived years figure, not the master —
// that is what keeps the expensive stage small (035 FR-004).
// summaryLC is the provider serving this run's chosen option (034), resolved
// once at the top of the run and passed down rather than re-read here.
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
		SentenceMin:      min,
		SentenceMax:      max,
	}
	summary, err := observe(ctx, prov, stageSummary, false, func(ctx context.Context) (domain.TailoredSummary, error) {
		return writeSummary(ctx, summaryLC, s.genModel, brief)
	})
	if err != nil {
		return nil, fmt.Errorf("summary: %w", err)
	}

	// The summary is the one part of the document that is written rather than
	// selected, so it is verified on its own terms: one re-prompt, then strip
	// the offending claim rather than fail the run or ship it (035 FR-008/FR-009).
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

func (s *Service) tailorRendercvResume(ctx context.Context, master domain.RendercvMaster, vacancy string, level domain.GroundingLevel, cfg domain.ShapeConfig, hints *domain.VacancyHints, rec *activity.Recorder, prov *runProvenance) (domain.RendercvMaster, domain.VacancyAnalysis, error) {
	// Stamp the run id once, here, so every LLM call below inherits it and the
	// whole tailoring pass lands in one observability trace (036 FR-009).
	// Retries, re-prompts and escalations are emitted from helpers several
	// frames down; stamping the context is what makes them correlate without
	// each of those call sites having to remember to.
	ctx = withRunTrace(ctx, rec)

	// 034: resolve the summary option once, here, at the top of the run. Every
	// call below takes the resolved provider, so a settings change while this
	// run is in flight cannot swap the model writing the summary halfway
	// through and produce a document nobody chose.
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
		return analyzeVacancy(ctx, s.llm.Analyze, s.genModel, vacancy, hints)
	})
	if err != nil {
		return nil, domain.VacancyAnalysis{}, fmt.Errorf("vacancy analysis: %w", err)
	}

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
		// Toggles then hard limits, both on the merged document and before
		// verification, so grounding and structure checks see exactly what
		// will be rendered.
		domain.ApplySectionToggles(merged, cfg)
		// Skills survive the merge exactly as the master wrote them; their
		// order is decided here, from the analysis, before the group cap in
		// ApplyHardLimits picks which ones make the page.
		domain.RankSkills(merged, analysis, cfg)
		report := domain.ApplyHardLimits(master, merged, cfg)
		recordShortfalls(ctx, rec, report)
		// 033 FR-001: drop ungrounded skill tokens on the primary pass, not
		// only after expand/condense. Capture the removed tokens for logging.
		// FR-010: DropUngroundedSkillTokens mutates in place and reports
		// nothing, so the before/after diff is the only record of what it took
		// out.
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
		if len(lastViolations) == 0 {
			fixed, err := s.fixStructureIntegrity(ctx, master, merged, analysis, level, cfg, rec)
			if rec != nil && prov != nil {
				// FR-017: what each stage cost and who served it, on the run
				// itself, so the pipeline's economics are observed rather than
				// estimated after the fact.
				rec.Step(ctx, "generation stages", prov.meta())
			}
			return fixed, analysis, err
		}
	}
	return nil, domain.VacancyAnalysis{}, fmt.Errorf("tailored rendercv resume failed grounding check: %s", strings.Join(lastViolations, "; "))
}

// shapeConfigMeta is the resolved config as activity metadata, so a past
// document can be explained by the settings the run actually used (FR-006).
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

// recordShortfalls reports each configured minimum the master could not meet.
// The document keeps what actually exists — nothing is invented to close the
// gap (FR-017) — so the run says so instead.
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

// skillDetailEntries flattens every comma-separated skill entry across the
// document's skills groups, trimmed and in document order. It reads the same
// shape domain.DropUngroundedSkillTokens writes, so a before/after pair of
// these is exactly what that function removed.
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

// droppedSkillEntries returns the entries present in before but not after, as a
// multiset difference so a token repeated across two groups and removed from
// only one is still reported once. Sorted, because the result is logged and an
// activity trail that reorders itself run to run is not comparable (033 FR-010).
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

// renderDeps are the render-loop's collaborators, injected so the page-target
// loop can be tested without a Typst toolchain or a live LLM. The production
// values come from defaultRenderDeps.
type renderDeps struct {
	render     func(ctx context.Context, merged domain.RendercvMaster, name string) (string, error)
	countPages func(pdfPath string) (int, error)
	expand     func(ctx context.Context, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSelection, error)
	// condense is not an LLM call: page fitting is a layout problem, and the
	// selection stage already ranked these bullets. It returns the shortened
	// document and whether there was anything left to shorten.
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

// renderOutcome is what the page-target loop reached: the PDF it settled on,
// the page count it achieved (0 when the count could not be taken) and
// whether the page target forced the section lengths down on the way (FR-016).
type renderOutcome struct {
	pdfPath  string
	pages    int
	conflict bool
}

// renderResume renders the merged master to PDF and drives it toward the
// configured page target: too short and it asks the LLM to expand, too long
// and it applies the compact design then condenses. Every failure along the
// way degrades gracefully — the best result reached is returned rather than
// erroring, so an unreachable target never fails a generation (FR-021).
func (s *Service) renderResume(ctx context.Context, master, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig, baseName string, rec *activity.Recorder) (string, error) {
	// Set once, on merged, before any render in this run: every reMerged
	// variant below is deep-cloned from merged (via MergeTailored), so this
	// carries through expand/condense/compact without being reapplied.
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
		reMerged, err := domain.MergeTailored(merged, expanded, nil, level) // summary carried by merged
		if err != nil {
			slog.Warn("merge after expand failed, returning short version", "err", err)
			return renderOutcome{pdfPath: pdfPath, pages: pages}, nil
		}
		domain.DropUngroundedSkillTokens(merged, reMerged)
		domain.ApplyHardLimits(master, reMerged, cfg)
		// 033 FR-009: apply the same highlight-drift cleanup as the primary pass.
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

	// Over target. Two bounded adjust attempts, matching the groundingAttempts
	// idiom: compact the design first (no LLM call), then condense the content.
	// Whatever the last attempt reached is what we return — an unreachable
	// target is reported, never an error.
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
				// Already at the condense floor: another render would produce
				// the same document, so the compact version is the answer.
				break
			}
			domain.CompactDesign(shorter)
			// Section lengths went below what the config asked for, to reach
			// the page target (FR-016).
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

// fixStructureIntegrity runs the structural-integrity verifier (feature 028)
// and the highlight-drift verifier (feature 033) on an already-grounded merge.
// Block sequence, experience order and job drops are enforced deterministically
// by MergeTailored and never reach here; the two violation kinds are:
//   - a text-asserted years figure contradicting the master's derivable total (028)
//   - an experience highlight that drifted too far from the master's bullet (033)
//
// On first detection each runs a single targeted re-prompt feeding the violation
// back; if the claim recurs, it strips the offending content and logs the
// intervention on the activity row.
func (s *Service) fixStructureIntegrity(ctx context.Context, master, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig, rec *activity.Recorder) (domain.RendercvMaster, error) {
	// 1. Years-assertion check (028)
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

	// 2. Highlight-drift check (033 FR-002, FR-003)
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
	// The cover-letter branch below is a single LLM call that never enters
	// tailorRendercvResume, so it would be the one call of this run with no
	// trace on it (036 C4-10). Stamping genCtx covers both branches: the resume
	// path re-stamps the same run id, which is idempotent.
	genCtx = withRunTrace(genCtx, rec)
	prov := &runProvenance{}

	if docType == string(dto.DocumentTypeResume) {
		// Resolved once per run and threaded as a value (see shapeConfig).
		cfg := s.shapeConfig(ctx)
		if rec != nil {
			rec.Step(ctx, "resume shape config", shapeConfigMeta(cfg))
		}
		tailored, analysis, err := s.tailorRendercvResume(genCtx, master, job.Description, s.defaultLevel, cfg, nil, rec, prov)
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

// GetDocumentPdfPath returns a document's rendered PDF path, or nil if it
// hasn't been rendered yet.
func (s *Service) GetDocumentPdfPath(ctx context.Context, id string) (*string, error) {
	row, err := s.GetDocument(ctx, id)
	if err != nil {
		return nil, err
	}
	return row.PdfPath, nil
}

// GetDocumentDownload returns a document's rendered PDF path along with the
// name it should be offered under. On-disk names encode company, title,
// version and a timestamp because the render loop needs them unique across
// candidate renders; the download name is the one a human files away, so a
// résumé is served as CV_Name_Surname.pdf from the profile's own name.
//
// The name is cosmetic: any failure to resolve it falls back to the on-disk
// base name rather than failing the download.
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

// resumeDownloadName builds "CV_Name_Surname.pdf" from the default profile's
// cv.name. It returns "" when the profile, its config or the name is missing —
// every one of those is a fall-back-to-disk-name case, not an error.
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

// downloadNameSlug turns a person's name into a filename-safe fragment,
// preserving case (unlike sanitize, which is for on-disk base names): runs of
// anything that isn't a letter or digit collapse to a single underscore, and
// leading/trailing underscores are trimmed. "Ada  Lovelace-King" becomes
// "Ada_Lovelace_King".
//
// Letters are judged by Unicode class, not ASCII: a Cyrillic or accented name
// must keep its own spelling here. The header writer is what deals with
// non-ASCII bytes (see downloadDisposition).
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

// numericToFloatPtr renders a nullable numeric column as a float pointer, so a
// run with no measured cost stays absent from the payload rather than
// reporting zero — an unmeasured run and a free one are different facts.
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

// withRunTrace stamps the activity-run id onto the context as the observability
// correlation id (036 FR-010). Reusing the run id rather than minting a new one
// is what lets an operator move between a trace and the platform's own run
// history in either direction with no lookup table.
//
// A nil or invalid recorder leaves the context untouched, so an uncorrelated
// run behaves exactly as it did before 036.
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
