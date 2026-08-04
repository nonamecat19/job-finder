package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/job-finder/api/internal/activity"
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
	// shapeAttempts bounds the page-fit adjust cycle so an unreachable page
	// target cannot spin: compact design, then condense, then accept whatever
	// was reached.
	shapeAttempts = 2
)

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

type Service struct {
	q            domain.Repository
	profiles     domain.ProfileStore
	htmlRenderer *infrastructure.HtmlPdfRenderer
	rendercv     *infrastructure.RenderCvRenderer
	llmc         llm.Provider
	genModel     string
	masterPath   string
	defaultLevel domain.GroundingLevel
	shape        ShapeProvider
}

func NewService(q domain.Repository, profiles domain.ProfileStore, htmlRenderer *infrastructure.HtmlPdfRenderer, rendercv *infrastructure.RenderCvRenderer, llmc llm.Provider, genModel, masterPath, defaultLevel string, shape ShapeProvider) *Service {
	if masterPath == "" {
		masterPath = "./resume/resume.yaml"
	}
	return &Service{
		q: q, profiles: profiles, htmlRenderer: htmlRenderer, rendercv: rendercv, llmc: llmc, genModel: genModel,
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
	return s.llmc.ModelName()
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

// GenerateAdHoc tailors a resume and writes a cover letter from pasted
// vacancy text with no backing Job row, persisting both as GeneratedDocument
// rows with jobId NULL so they show up in the ad-hoc history list.
func (s *Service) GenerateAdHoc(ctx context.Context, in AdHocInput) (resumeDoc, coverLetterDoc dto.GeneratedDocumentDto, err error) {
	level := s.defaultLevel
	if in.GroundingLevel != nil {
		level = *in.GroundingLevel
	}
	master, err := s.masterFor(ctx, nil)
	if err != nil {
		return dto.GeneratedDocumentDto{}, dto.GeneratedDocumentDto{}, err
	}
	var extraNotes *string
	if prof, profErr := s.profiles.GetDefault(ctx); profErr == nil {
		extraNotes = prof.ExtraNotes
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
	merged, analysis, err := s.tailorRendercvResume(resumeCtx, master, in.Vacancy, level, cfg, in.Hints, nil)
	if err != nil {
		return dto.GeneratedDocumentDto{}, dto.GeneratedDocumentDto{}, err
	}
	resumeContent, err := json.Marshal(merged)
	if err != nil {
		return dto.GeneratedDocumentDto{}, dto.GeneratedDocumentDto{}, err
	}
	resumePdfPath, err := s.renderResume(ctx, master, merged, analysis, level, cfg, baseName+"-resume-"+strconv.FormatInt(time.Now().UnixMilli(), 10), nil)
	if err != nil {
		return dto.GeneratedDocumentDto{}, dto.GeneratedDocumentDto{}, err
	}
	resumeRow, err := s.q.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		Type: string(dto.DocumentTypeResume), Version: 1, Content: resumeContent, PdfPath: &resumePdfPath, Model: s.docModel(*resumeServed),
		Company: &company, Title: &title, Vacancy: &in.Vacancy,
	})
	if err != nil {
		return dto.GeneratedDocumentDto{}, dto.GeneratedDocumentDto{}, err
	}

	profileText := domain.RendercvToText(master)
	coverCtx, coverServed := llm.WithServedModelCapture(ctx)
	letter, err := s.writeCoverLetter(coverCtx, profileText, extraNotes, company, title, in.Vacancy)
	if err != nil {
		return dto.GeneratedDocumentDto{}, dto.GeneratedDocumentDto{}, err
	}
	coverContent, err := json.Marshal(map[string]string{"text": letter})
	if err != nil {
		return dto.GeneratedDocumentDto{}, dto.GeneratedDocumentDto{}, err
	}
	var namePtr *string
	if basics, _ := master["cv"].(map[string]any); basics != nil {
		if n, _ := basics["name"].(string); n != "" {
			namePtr = &n
		}
	}
	coverPdfPath, err := s.htmlRenderer.RenderCoverLetter(ctx, letter, namePtr, company, title, fmt.Sprintf("%s-cover-%d.pdf", baseName, time.Now().UnixMilli()))
	if err != nil {
		return dto.GeneratedDocumentDto{}, dto.GeneratedDocumentDto{}, err
	}
	coverRow, err := s.q.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		Type: string(dto.DocumentTypeCoverLetter), Version: 1, Content: coverContent, PdfPath: &coverPdfPath, Model: s.docModel(*coverServed),
		Company: &company, Title: &title, Vacancy: &in.Vacancy,
	})
	if err != nil {
		return dto.GeneratedDocumentDto{}, dto.GeneratedDocumentDto{}, err
	}

	return toDocumentDto(resumeRow), toDocumentDto(coverRow), nil
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

func (s *Service) tailorRendercvResume(ctx context.Context, master domain.RendercvMaster, vacancy string, level domain.GroundingLevel, cfg domain.ShapeConfig, hints *domain.VacancyHints, rec *activity.Recorder) (domain.RendercvMaster, domain.VacancyAnalysis, error) {
	if rec != nil {
		rec.Step(ctx, "analyzing vacancy", nil)
	}
	analysis, err := analyzeVacancy(ctx, s.llmc, s.genModel, vacancy, hints)
	if err != nil {
		return nil, domain.VacancyAnalysis{}, fmt.Errorf("vacancy analysis: %w", err)
	}

	var lastViolations []string
	for attempt := 0; attempt < groundingAttempts; attempt++ {
		if rec != nil {
			rec.Step(ctx, fmt.Sprintf("tailoring resume (LLM) (attempt %d/%d)", attempt+1, groundingAttempts), nil)
		}
		payload, err := selectAndTailor(ctx, s.llmc, s.genModel, master, analysis, level, lastViolations, cfg)
		if err != nil {
			return nil, domain.VacancyAnalysis{}, err
		}
		merged, err := domain.MergeTailored(master, payload)
		if err != nil {
			return nil, domain.VacancyAnalysis{}, err
		}
		// Toggles then hard limits, both on the merged document and before
		// verification, so grounding and structure checks see exactly what
		// will be rendered.
		domain.ApplySectionToggles(merged, cfg)
		report := domain.ApplyHardLimits(master, merged, cfg)
		recordShortfalls(ctx, rec, report)
		if rec != nil {
			rec.Step(ctx, fmt.Sprintf("grounding check (attempt %d/%d)", attempt+1, groundingAttempts), nil)
		}
		lastViolations = domain.VerifyRendercvGrounding(master, merged, level)
		if len(lastViolations) == 0 {
			fixed, err := s.fixStructureIntegrity(ctx, master, merged, analysis, level, cfg, rec)
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

// renderDeps are the render-loop's collaborators, injected so the page-target
// loop can be tested without a Typst toolchain or a live LLM. The production
// values come from defaultRenderDeps.
type renderDeps struct {
	render     func(ctx context.Context, merged domain.RendercvMaster, name string) (string, error)
	countPages func(pdfPath string) (int, error)
	expand     func(ctx context.Context, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSections, error)
	condense   func(ctx context.Context, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSections, error)
}

func (s *Service) defaultRenderDeps() renderDeps {
	return renderDeps{
		render: func(ctx context.Context, merged domain.RendercvMaster, name string) (string, error) {
			_, pdfPath, err := s.rendercv.Render(ctx, merged, name)
			return pdfPath, err
		},
		countPages: infrastructure.CountPages,
		expand: func(ctx context.Context, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSections, error) {
			return expandContent(ctx, s.llmc, s.genModel, merged, analysis, level, cfg)
		},
		condense: func(ctx context.Context, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSections, error) {
			return condenseContent(ctx, s.llmc, s.genModel, merged, analysis, level, cfg)
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
		reMerged, err := domain.MergeTailored(merged, expanded)
		if err != nil {
			slog.Warn("merge after expand failed, returning short version", "err", err)
			return renderOutcome{pdfPath: pdfPath, pages: pages}, nil
		}
		domain.DropUngroundedSkillTokens(merged, reMerged)
		domain.ApplyHardLimits(master, reMerged, cfg)
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
				rec.Step(ctx, "resume still overflows, asking LLM to condense content", map[string]any{"pages": out.pages, "pageTarget": cfg.TargetPages})
			}
			condensed, err := deps.condense(ctx, merged, analysis, level, cfg)
			if err != nil {
				slog.Warn("LLM condense failed, returning compact version", "err", err)
				break
			}
			reMerged, err := domain.MergeTailored(merged, condensed)
			if err != nil {
				slog.Warn("merge after condense failed, returning compact version", "err", err)
				break
			}
			domain.DropUngroundedSkillTokens(merged, reMerged)
			domain.ApplyHardLimits(master, reMerged, cfg)
			domain.CompactDesign(reMerged)
			// The condense prompt asked for shorter sections than configured.
			out.conflict = true
			candidate, suffix = reMerged, "-condensed"
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
// on an already-grounded merge. Block sequence, experience order and job
// drops are enforced deterministically by MergeTailored and never reach here;
// the only violation kind is a text-asserted years figure contradicting the
// master's derivable total. On first detection it runs a single targeted
// re-prompt feeding the violation back; if the claim recurs, it strips the
// offending clause and logs the intervention on the activity row.
func (s *Service) fixStructureIntegrity(ctx context.Context, master, merged domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig, rec *activity.Recorder) (domain.RendercvMaster, error) {
	structViolations := domain.VerifyStructureIntegrity(master, merged)
	if len(structViolations) == 0 {
		return merged, nil
	}
	if rec != nil {
		rec.Step(ctx, "structure integrity: years assertion detected, re-prompting", map[string]any{"violations": len(structViolations)})
	}
	rePrompted, err := retailorForStructure(ctx, s.llmc, s.genModel, master, analysis, level, structViolations, cfg)
	if err != nil {
		return nil, err
	}
	reMerged, err := domain.MergeTailored(master, rePrompted)
	if err != nil {
		return nil, err
	}
	domain.ApplySectionToggles(reMerged, cfg)
	domain.ApplyHardLimits(master, reMerged, cfg)
	reViolations := domain.VerifyStructureIntegrity(master, reMerged)
	if len(reViolations) == 0 {
		return reMerged, nil
	}
	if rec != nil {
		rec.Step(ctx, "structure integrity: years assertion recurred, stripped", map[string]any{"violations": len(reViolations)})
	}
	return domain.StripStructureViolations(reMerged, reViolations), nil
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

	if docType == string(dto.DocumentTypeResume) {
		// Resolved once per run and threaded as a value (see shapeConfig).
		cfg := s.shapeConfig(ctx)
		if rec != nil {
			rec.Step(ctx, "resume shape config", shapeConfigMeta(cfg))
		}
		tailored, analysis, err := s.tailorRendercvResume(genCtx, master, job.Description, s.defaultLevel, cfg, nil, rec)
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

	doc, err := s.q.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		JobId: jid, Type: docType, Version: version, Content: content, PdfPath: &pdfPath, Model: s.docModel(*served),
	})
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

	result, err := llm.CompleteStructured[coverLetterResult](ctx, s.llmc, prompt, &llm.CompleteOptions{
		System: "You write concise, concrete, honest cover letters.",
		Model:  s.genModel,
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
	}
}
