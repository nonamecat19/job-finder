package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/generation/infrastructure"
)

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}

type VacancyAnalysisResult struct {
	RequiredSkills      []string `json:"requiredSkills"`
	NiceToHaveSkills    []string `json:"niceToHaveSkills"`
	ExperienceLevel     string   `json:"experienceLevel"`
	KeyResponsibilities []string `json:"keyResponsibilities"`
	IndustryKeywords    []string `json:"industryKeywords"`
	SeniorityKeywords   []string `json:"seniorityKeywords"`
}

type GenerationCompletedResult struct {
	Analysis           VacancyAnalysisResult `json:"analysis"`
	Summary            string                `json:"summary"`
	Experience         []map[string]any      `json:"experience"`
	Skills             []map[string]any      `json:"skills"`
	Projects           []map[string]any      `json:"projects"`
	SelectionEscalated bool                  `json:"selectionEscalated"`
	SummaryOption      string                `json:"summaryOption"`
}

type CoverLetterCompletedResult struct {
	Letter string `json:"letter"`
}

func parseIdempotencyKey(key string) (docType, profileID string, ok bool) {
	parts := strings.SplitN(key, ":", 5)
	if len(parts) != 5 || parts[0] != Kind {
		return "", "", false
	}
	return parts[2], parts[3], true
}

func NewResultHandler(tx TxRunner, profiles domain.ProfileStore, shape ShapeProvider, rendercv *infrastructure.RenderCvRenderer, htmlRenderer *infrastructure.HtmlPdfRenderer, model string) events.ResultHandler {
	return func(ctx context.Context, envelope events.Envelope, result events.Result) error {
		return tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
			disposition, err := events.Admit(ctx, q, Kind, envelope.IdempotencyKey, envelope.RunID)
			if err != nil {
				return fmt.Errorf("generation: admit result: %w", err)
			}
			if disposition != events.Accepted {
				return nil
			}

			if result.Status != events.ResultSucceeded {
				msg := ""
				if result.Failure != nil {
					msg = result.Failure.Message
				}
				slog.Error("generation: python generation failed", "job", envelope.WorkID, "trace_id", envelope.TraceID, "error", msg)
				return nil
			}

			docType, profileIDPart, ok := parseIdempotencyKey(envelope.IdempotencyKey)
			if !ok {
				return fmt.Errorf("generation: persist result: idempotency key %q does not match the generation encoding", envelope.IdempotencyKey)
			}

			jid, err := dbutil.ParseUUID(envelope.WorkID)
			if err != nil {
				return fmt.Errorf("generation: persist result: %w", err)
			}
			job, err := q.GetJobByID(ctx, jid)
			if err != nil {
				return fmt.Errorf("generation: persist result: job %s: %w", envelope.WorkID, err)
			}

			var profileRow sqlcgen.Profile
			if profileIDPart != "" {
				profileRow, err = profiles.Get(ctx, profileIDPart)
			} else {
				profileRow, err = profiles.GetDefault(ctx)
			}
			if err != nil {
				return fmt.Errorf("generation: persist result: profile: %w", err)
			}
			master, err := domain.MasterFromProfile(profileRow)
			if err != nil {
				return fmt.Errorf("generation: persist result: %w", err)
			}

			maxVersion, err := q.MaxDocumentVersion(ctx, sqlcgen.MaxDocumentVersionParams{JobId: jid, Type: docType})
			if err != nil {
				return fmt.Errorf("generation: persist result: %w", err)
			}
			version := maxVersion + 1
			baseName := sanitize(job.Company + "-" + job.Title)

			var content []byte
			var pdfPath string
			insertParams := sqlcgen.InsertGeneratedDocumentParams{
				JobId: jid, Type: docType, Version: version, Model: model,
			}

			if docType == string(dto.DocumentTypeCoverLetter) {
				var letter CoverLetterCompletedResult
				if err := json.Unmarshal(result.Result, &letter); err != nil {
					return fmt.Errorf("generation: unmarshal generate.completed cover-letter result: %w", err)
				}
				content, err = json.Marshal(map[string]string{"text": strings.TrimSpace(letter.Letter)})
				if err != nil {
					return fmt.Errorf("generation: marshal cover letter content: %w", err)
				}
				var namePtr *string
				if master != nil {
					if basics, _ := master["cv"].(map[string]any); basics != nil {
						if n, _ := basics["name"].(string); n != "" {
							namePtr = &n
						}
					}
				}
				pdfPath, err = htmlRenderer.RenderCoverLetter(ctx, letter.Letter, namePtr, job.Company, job.Title, fmt.Sprintf("%s-cover-v%d.pdf", baseName, version))
				if err != nil {
					return fmt.Errorf("generation: render cover letter: %w", err)
				}
			} else {
				if master == nil {
					return fmt.Errorf("generation: persist result: precondition failed: profile has no RenderCV config")
				}
				var assembled GenerationCompletedResult
				if err := json.Unmarshal(result.Result, &assembled); err != nil {
					return fmt.Errorf("generation: unmarshal generate.completed result: %w", err)
				}

				merged, err := domain.DeepCloneYAML(master)
				if err != nil {
					return fmt.Errorf("generation: clone master: %w", err)
				}
				sections := domain.CvSections(merged)
				if sections == nil {
					return fmt.Errorf("generation: persist result: master has no cv.sections")
				}
				sections["summary"] = []any{strings.TrimSpace(assembled.Summary)}
				sections["experience"] = toAnySlice(assembled.Experience)
				sections["skills"] = toAnySlice(assembled.Skills)
				sections["projects"] = toAnySlice(assembled.Projects)

				analysis := domain.VacancyAnalysis{
					RequiredSkills:      assembled.Analysis.RequiredSkills,
					NiceToHaveSkills:    assembled.Analysis.NiceToHaveSkills,
					ExperienceLevel:     assembled.Analysis.ExperienceLevel,
					KeyResponsibilities: assembled.Analysis.KeyResponsibilities,
					IndustryKeywords:    assembled.Analysis.IndustryKeywords,
					SeniorityKeywords:   assembled.Analysis.SeniorityKeywords,
				}
				cfg := domain.DefaultShapeConfig()
				if shape != nil {
					cfg = shape.Shape(ctx)
				}

				domain.ApplySectionToggles(merged, cfg)
				domain.RankSkills(merged, analysis, cfg)
				domain.TrimSkillGroups(merged, analysis)
				domain.RankProjects(merged, analysis, cfg)
				domain.TrimProjectHighlights(merged, analysis)
				domain.ApplyHardLimits(master, merged, cfg)
				domain.DropUngroundedSkillTokens(master, merged)
				domain.ApplyFontSize(merged, cfg)

				content, err = json.Marshal(merged)
				if err != nil {
					return fmt.Errorf("generation: marshal tailored document: %w", err)
				}
				_, pdfPath, err = rendercv.Render(ctx, merged, fmt.Sprintf("%s-resume-v%d", baseName, version))
				if err != nil {
					return fmt.Errorf("generation: render resume: %w", err)
				}
				insertParams.SelectionEscalated = assembled.SelectionEscalated
				if assembled.SummaryOption != "" {
					opt := assembled.SummaryOption
					insertParams.SummaryOptionId = &opt
				}
			}

			insertParams.Content = content
			insertParams.PdfPath = &pdfPath
			doc, err := q.InsertGeneratedDocument(ctx, insertParams)
			if err != nil {
				return fmt.Errorf("generation: insert document: %w", err)
			}

			if job.Status == "found" || job.Status == "shortlisted" {
				if _, err := q.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{ID: jid, Status: "docs_generated"}); err != nil {
					return fmt.Errorf("generation: update job status: %w", err)
				}
				evs, _ := json.Marshal([]dto.ApplicationEvent{{Status: "docs_generated", At: time.Now().UTC().Format(time.RFC3339)}})
				if err := q.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
					JobId: jid, Status: "docs_generated", Events: evs,
				}); err != nil {
					return fmt.Errorf("generation: upsert application status: %w", err)
				}
			}

			slog.Info("generation complete (python)", "jobId", envelope.WorkID, "type", docType, "documentId", dbutil.UUIDString(doc.ID), "version", version)
			return nil
		})
	}
}

func toAnySlice(in []map[string]any) []any {
	out := make([]any, 0, len(in))
	for _, m := range in {
		out = append(out, m)
	}
	return out
}
