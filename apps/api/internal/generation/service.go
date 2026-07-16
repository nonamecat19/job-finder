package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/strutil"

	"gopkg.in/yaml.v3"
)

const groundingAttempts = 2

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type RendercvFromTextInput struct {
	Vacancy        string
	Company        string
	Title          string
	GroundingLevel *GroundingLevel
	Hints          *VacancyHints
}

type RendercvFromTextResult struct {
	YamlPath       string
	PdfPath        string
	GroundingLevel GroundingLevel
}

type coverLetterResult struct {
	Letter string `json:"letter"`
}

type Service struct {
	q            *sqlcgen.Queries
	profiles     ProfileStore
	htmlRenderer *HtmlPdfRenderer
	rendercv     *RenderCvRenderer
	llmc         llm.Provider
	masterPath   string
	defaultLevel GroundingLevel
}

func NewService(q *sqlcgen.Queries, profiles ProfileStore, htmlRenderer *HtmlPdfRenderer, rendercv *RenderCvRenderer, llmc llm.Provider, masterPath, defaultLevel string) *Service {
	if masterPath == "" {
		masterPath = "./resume/resume.yaml"
	}
	return &Service{
		q: q, profiles: profiles, htmlRenderer: htmlRenderer, rendercv: rendercv, llmc: llmc,
		masterPath: masterPath, defaultLevel: ParseGroundingLevel(defaultLevel),
	}
}

func sanitize(s string) string {
	out := sanitizeRe.ReplaceAllString(s, "_")
	out = strings.ToLower(out)
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

func (s *Service) masterFor(ctx context.Context, profileID *string) (RendercvMaster, error) {
	var prof sqlcgen.Profile
	var err error
	if profileID != nil && *profileID != "" {
		prof, err = s.profiles.Get(ctx, *profileID)
	} else {
		prof, err = s.profiles.GetDefault(ctx)
	}
	if err == nil && prof.RendercvConfig != nil {
		return MasterFromProfile(prof)
	}

	// Dev fallback to masterPath if no profile exists
	data, err := os.ReadFile(s.masterPath)
	if err != nil {
		return nil, fmt.Errorf("generation: read master resume from %s: %w", s.masterPath, err)
	}
	var master map[string]any
	if err := yaml.Unmarshal(data, &master); err != nil {
		return nil, fmt.Errorf("generation: parse master resume: %w", err)
	}
	return RendercvMaster(NormalizeYAMLMap(master).(map[string]any)), nil
}

func (s *Service) GenerateRendercvFromText(ctx context.Context, in RendercvFromTextInput) (*RendercvFromTextResult, error) {
	level := s.defaultLevel
	if in.GroundingLevel != nil {
		level = *in.GroundingLevel
	}
	master, err := s.masterFor(ctx, nil)
	if err != nil {
		return nil, err
	}

	merged, err := s.tailorRendercvResume(ctx, master, in.Vacancy, level, in.Hints, nil)
	if err != nil {
		return nil, err
	}

	company := in.Company
	if company == "" {
		company = "vacancy"
	}
	title := in.Title
	if title == "" {
		title = "resume"
	}
	stem := sanitize(company+"-"+title) + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10)

	yamlPath, pdfPath, err := s.rendercv.Render(ctx, merged, stem)
	if err != nil {
		return nil, err
	}
	return &RendercvFromTextResult{YamlPath: yamlPath, PdfPath: pdfPath, GroundingLevel: level}, nil
}

func (s *Service) tailorRendercvResume(ctx context.Context, master RendercvMaster, vacancy string, level GroundingLevel, hints *VacancyHints, rec *activity.Recorder) (RendercvMaster, error) {
	if rec != nil {
		rec.Step(ctx, "analyzing vacancy", nil)
	}
	analysis, err := analyzeVacancy(ctx, s.llmc, vacancy, hints)
	if err != nil {
		return nil, fmt.Errorf("vacancy analysis: %w", err)
	}

	var lastViolations []string
	for attempt := 0; attempt < groundingAttempts; attempt++ {
		if rec != nil {
			rec.Step(ctx, fmt.Sprintf("tailoring resume (LLM) (attempt %d/%d)", attempt+1, groundingAttempts), nil)
		}
		payload, err := selectAndTailor(ctx, s.llmc, master, analysis, level, lastViolations)
		if err != nil {
			return nil, err
		}
		merged, err := mergeTailored(master, payload)
		if err != nil {
			return nil, err
		}
		if rec != nil {
			rec.Step(ctx, fmt.Sprintf("grounding check (attempt %d/%d)", attempt+1, groundingAttempts), nil)
		}
		lastViolations = verifyRendercvGrounding(master, merged, level)
		if len(lastViolations) == 0 {
			return merged, nil
		}
	}
	return nil, fmt.Errorf("tailored rendercv resume failed grounding check: %s", strings.Join(lastViolations, "; "))
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
	master, err := MasterFromProfile(prof)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	profileText := RendercvToText(master)

	maxVersion, err := s.q.MaxDocumentVersion(ctx, sqlcgen.MaxDocumentVersionParams{JobId: jid, Type: docType})
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	version := maxVersion + 1
	baseName := sanitize(job.Company + "-" + job.Title)

	var content []byte
	var pdfPath string

	if docType == string(dto.DocumentTypeResume) {
		tailored, err := s.tailorRendercvResume(ctx, master, job.Description, s.defaultLevel, nil, rec)
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
		_, p, err := s.rendercv.Render(ctx, tailored, fmt.Sprintf("%s-resume-v%d", baseName, version))
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		pdfPath = p
	} else {
		if rec != nil {
			rec.Step(ctx, "writing cover letter (LLM)", nil)
		}
		letter, err := s.writeCoverLetter(ctx, profileText, prof.ExtraNotes, job)
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
		JobId: jid, Type: docType, Version: version, Content: content, PdfPath: &pdfPath, Model: s.llmc.ModelName(),
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

func (s *Service) tailorResume(ctx context.Context, master RendercvMaster, profileText string, extraNotes *string, job sqlcgen.Job, matchedSkills []string) (dto.JsonResume, error) {
	prompt := "Create a tailored resume for this job application by selecting, reordering and rephrasing " +
		"content from the candidate's master profile.\n\n" +
		"STRICT RULES:\n" +
		"- Use ONLY employers, roles, projects, education, dates and facts present in the master profile.\n" +
		"- Never invent experience, employers, dates, degrees, metrics or technologies.\n" +
		"- You may drop irrelevant entries, reorder, and rephrase highlights to emphasize what the job asks for.\n" +
		"- Copy employer names, institution names, project names and all dates EXACTLY as written in the master profile.\n" +
		"- Keep basics (name, email, phone, url, location) exactly as in the master profile.\n\n" +
		"MASTER PROFILE:\n" + strutil.Truncate(profileText, 12000) + "\n\n"
	if extraNotes != nil && *extraNotes != "" {
		prompt += "EXTRA CANDIDATE NOTES:\n" + strutil.Truncate(*extraNotes, 2000) + "\n\n"
	}
	prompt += fmt.Sprintf("TARGET JOB:\nTitle: %s\nCompany: %s\n", job.Title, job.Company)
	if len(matchedSkills) > 0 {
		ms, _ := json.Marshal(matchedSkills)
		prompt += "Matched skills: " + string(ms) + "\n"
	}
	prompt += "Description:\n" + strutil.Truncate(job.Description, 6000)

	var lastViolations []string
	for attempt := 0; attempt < groundingAttempts; attempt++ {
		p := prompt
		if len(lastViolations) > 0 {
			p += "\n\nYour previous attempt violated grounding rules:\n- " + strings.Join(lastViolations, "\n- ") + "\nRegenerate without these violations."
		}
		tailored, err := llm.CompleteStructured[dto.JsonResume](ctx, s.llmc, p, &llm.CompleteOptions{
			System: "You are an expert resume writer who never fabricates information.",
		})
		if err != nil {
			return dto.JsonResume{}, err
		}
		lastViolations = verifyGroundingFromRendercv(master, tailored)
		if len(lastViolations) == 0 {
			return tailored, nil
		}
	}
	return dto.JsonResume{}, fmt.Errorf("tailored resume failed grounding check: %s", strings.Join(lastViolations, "; "))
}

func (s *Service) writeCoverLetter(ctx context.Context, profileText string, extraNotes *string, job sqlcgen.Job) (string, error) {
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
	prompt += fmt.Sprintf("JOB:\nTitle: %s\nCompany: %s\nDescription:\n%s", job.Title, job.Company, strutil.Truncate(job.Description, 4000))

	result, err := llm.CompleteStructured[coverLetterResult](ctx, s.llmc, prompt, &llm.CompleteOptions{
		System: "You write concise, concrete, honest cover letters.",
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

func (s *Service) UpdateDocument(ctx context.Context, id, text string) (dto.GeneratedDocumentDto, error) {
	doc, err := s.GetDocument(ctx, id)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	if doc.Type != string(dto.DocumentTypeCoverLetter) || text == "" {
		return dto.GeneratedDocumentDto{}, fmt.Errorf("only cover_letter text is editable")
	}
	job, err := s.q.GetJobByID(ctx, doc.JobId)
	if err != nil {
		return dto.GeneratedDocumentDto{}, fmt.Errorf("job %s not found", dbutil.UUIDString(doc.JobId))
	}
	prof, err := s.profiles.GetDefault(ctx)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	if prof.RendercvConfig == nil {
		return dto.GeneratedDocumentDto{}, fmt.Errorf("profile has no RenderCV config")
	}
	master, err := MasterFromProfile(prof)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}

	baseName := sanitize(job.Company + "-" + job.Title)
	var namePtr *string
	if basics, _ := master["cv"].(map[string]any); basics != nil {
		if n, _ := basics["name"].(string); n != "" {
			namePtr = &n
		}
	}
	pdfPath, err := s.htmlRenderer.RenderCoverLetter(ctx, text, namePtr, job.Company, job.Title, fmt.Sprintf("%s-cover-v%d.pdf", baseName, doc.Version))
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
		JobID:     dbutil.UUIDString(r.JobId),
		Type:      r.Type,
		Version:   int(r.Version),
		Content:   content,
		PdfPath:   r.PdfPath,
		Model:     r.Model,
		CreatedAt: dbutil.Timestamp(r.CreatedAt),
	}
}
