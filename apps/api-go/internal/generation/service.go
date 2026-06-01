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

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api-go/internal/db/sqlcgen"
	"github.com/job-finder/api-go/internal/dbutil"
	"github.com/job-finder/api-go/internal/dto"
	"github.com/job-finder/api-go/internal/llm"
	"github.com/job-finder/api-go/internal/profile"
	"github.com/job-finder/api-go/internal/strutil"
)

const groundingAttempts = 2 // matches GROUNDING_ATTEMPTS in generation.service.ts

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// coverLetterResult mirrors the zod `coverLetterSchema = z.object({ letter: z.string() })`.
type coverLetterResult struct {
	Letter string `json:"letter"`
}

// Service ports GenerationService: tailorResume + writeCoverLetter with
// grounding verify+retry, versioned GeneratedDocument persistence, and the
// ad-hoc RenderCV tailoring path.
type Service struct {
	q            *sqlcgen.Queries
	profiles     *profile.Service
	htmlRenderer *HtmlPdfRenderer
	rendercv     *RenderCvRenderer
	llmc         llm.Provider
	masterPath   string
	defaultLevel GroundingLevel
}

func NewService(q *sqlcgen.Queries, profiles *profile.Service, htmlRenderer *HtmlPdfRenderer, rendercv *RenderCvRenderer, llmc llm.Provider, masterPath, defaultLevel string) *Service {
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

// ---------------------------------------------------------------------------
// Ad-hoc RenderCV path (documents.controller.ts POST /documents/tailor)
// ---------------------------------------------------------------------------

type RendercvFromTextInput struct {
	Vacancy        string
	Company        string
	Title          string
	GroundingLevel *GroundingLevel
}

type RendercvFromTextResult struct {
	YamlPath       string
	PdfPath        string
	GroundingLevel GroundingLevel
}

func (s *Service) GenerateRendercvFromText(ctx context.Context, in RendercvFromTextInput) (*RendercvFromTextResult, error) {
	level := s.defaultLevel
	if in.GroundingLevel != nil {
		level = *in.GroundingLevel
	}
	data, err := os.ReadFile(s.masterPath)
	if err != nil {
		return nil, fmt.Errorf("generation: read master resume: %w", err)
	}
	var master map[string]any
	if err := yaml.Unmarshal(data, &master); err != nil {
		return nil, fmt.Errorf("generation: parse master resume: %w", err)
	}
	master = normalizeYAMLMap(master).(map[string]any)

	merged, err := s.tailorRendercvResume(ctx, RendercvMaster(master), in.Vacancy, level)
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

func (s *Service) tailorRendercvResume(ctx context.Context, master RendercvMaster, vacancy string, level GroundingLevel) (RendercvMaster, error) {
	var lastViolations []string
	for attempt := 0; attempt < groundingAttempts; attempt++ {
		prompt := buildTailorPrompt(master, vacancy, level)
		if len(lastViolations) > 0 {
			prompt += "\n\nYour previous attempt violated grounding rules:\n- " + strings.Join(lastViolations, "\n- ") +
				"\nRegenerate without these violations."
		}
		payload, err := llm.CompleteStructured[TailoredSections](ctx, s.llmc, prompt, &llm.CompleteOptions{
			System: "You are an expert resume writer who never fabricates information.",
		})
		if err != nil {
			return nil, err
		}
		merged, err := mergeTailored(master, payload)
		if err != nil {
			return nil, err
		}
		lastViolations = verifyRendercvGrounding(master, merged, level)
		if len(lastViolations) == 0 {
			return merged, nil
		}
	}
	return nil, fmt.Errorf("tailored rendercv resume failed grounding check: %s", strings.Join(lastViolations, "; "))
}

// ---------------------------------------------------------------------------
// Job-backed generation (generation.processor.ts / jobs.controller.ts generate)
// ---------------------------------------------------------------------------

func (s *Service) Generate(ctx context.Context, jobID, docType string, profileID *string) (dto.GeneratedDocumentDto, error) {
	jid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	job, err := s.q.GetJobByID(ctx, jid)
	if err != nil {
		return dto.GeneratedDocumentDto{}, fmt.Errorf("job %s not found", jobID)
	}
	matchResult, _ := s.q.GetMatchResultByJobID(ctx, jid) // ignore not-found; matchedSkills stays nil

	var prof sqlcgen.Profile
	if profileID != nil {
		prof, err = s.profiles.Get(ctx, *profileID)
	} else {
		prof, err = s.profiles.GetDefault(ctx)
	}
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	var master dto.JsonResume
	_ = dbutil.UnmarshalJSONB(prof.Document, &master)

	maxVersion, err := s.q.MaxDocumentVersion(ctx, sqlcgen.MaxDocumentVersionParams{JobId: jid, Type: docType})
	if err != nil {
		return dto.GeneratedDocumentDto{}, err
	}
	version := maxVersion + 1
	baseName := sanitize(job.Company + "-" + job.Title)

	var content []byte
	var pdfPath string

	if docType == string(dto.DocumentTypeResume) {
		var matchedSkills []string
		_ = dbutil.UnmarshalJSONB(matchResult.MatchedSkills, &matchedSkills)
		tailored, err := s.tailorResume(ctx, master, prof.ExtraNotes, job, matchedSkills)
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		content, err = json.Marshal(tailored)
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		p, err := s.htmlRenderer.RenderResume(ctx, tailored, fmt.Sprintf("%s-resume-v%d.pdf", baseName, version))
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		pdfPath = p
	} else {
		letter, err := s.writeCoverLetter(ctx, master, prof.ExtraNotes, job)
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		content, err = json.Marshal(map[string]string{"text": letter})
		if err != nil {
			return dto.GeneratedDocumentDto{}, err
		}
		p, err := s.htmlRenderer.RenderCoverLetter(ctx, letter, basicsName(master.Basics), job.Company, job.Title, fmt.Sprintf("%s-cover-v%d.pdf", baseName, version))
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

	return toDocumentDto(doc), nil
}

func (s *Service) tailorResume(ctx context.Context, master dto.JsonResume, extraNotes *string, job sqlcgen.Job, matchedSkills []string) (dto.JsonResume, error) {
	masterJSON, _ := json.Marshal(master)
	prompt := "Create a tailored resume for this job application by selecting, reordering and rephrasing " +
		"content from the candidate's master profile.\n\n" +
		"STRICT RULES:\n" +
		"- Use ONLY employers, roles, projects, education, dates and facts present in the master profile.\n" +
		"- Never invent experience, employers, dates, degrees, metrics or technologies.\n" +
		"- You may drop irrelevant entries, reorder, and rephrase highlights to emphasize what the job asks for.\n" +
		"- Copy employer names, institution names, project names and all dates EXACTLY as written in the master profile.\n" +
		"- Keep basics (name, email, phone, url, location) exactly as in the master profile.\n\n" +
		"MASTER PROFILE (JSON Resume):\n" + strutil.Truncate(string(masterJSON), 12000) + "\n\n"
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
		lastViolations = verifyGrounding(master, tailored)
		if len(lastViolations) == 0 {
			return tailored, nil
		}
	}
	return dto.JsonResume{}, fmt.Errorf("tailored resume failed grounding check: %s", strings.Join(lastViolations, "; "))
}

func (s *Service) writeCoverLetter(ctx context.Context, master dto.JsonResume, extraNotes *string, job sqlcgen.Job) (string, error) {
	masterJSON, _ := json.Marshal(master)
	prompt := "Write a short cover letter (maximum 150 words, exactly 3 paragraphs separated by blank lines) " +
		"for this application.\n\n" +
		"Structure: (1) hook referencing the company and role, (2) 2-3 concrete matching experiences " +
		"from the candidate's real background, (3) brief close.\n" +
		`STRICT RULES: mention only experience present in the profile below; no invented facts, ` +
		`no clichés like "I am writing to express". Plain text, no salutation placeholders like [Hiring Manager] — ` +
		`use "Hello," if needed.` + "\n\n" +
		"CANDIDATE PROFILE:\n" + strutil.Truncate(string(masterJSON), 8000) + "\n\n"
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

// ---------------------------------------------------------------------------
// Document retrieval / edit (documents.controller.ts)
// ---------------------------------------------------------------------------

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

// UpdateDocument edits cover-letter text, then re-renders its PDF in place
// (same version), matching GenerationService.updateDocument.
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
	var master dto.JsonResume
	_ = dbutil.UnmarshalJSONB(prof.Document, &master)

	baseName := sanitize(job.Company + "-" + job.Title)
	pdfPath, err := s.htmlRenderer.RenderCoverLetter(ctx, text, basicsName(master.Basics), job.Company, job.Title, fmt.Sprintf("%s-cover-v%d.pdf", baseName, doc.Version))
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

func basicsName(b *dto.ResumeBasics) *string {
	if b == nil {
		return nil
	}
	return b.Name
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
