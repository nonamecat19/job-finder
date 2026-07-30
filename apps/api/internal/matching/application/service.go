package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/profile"
	"github.com/job-finder/api/internal/strutil"
)

var ErrNoProfileConfig = errors.New("no profile config")

type Service struct {
	q          Repository
	profiles   *profile.Service
	llmc       llm.Provider
	threshold  float64
	matchModel string
}

func NewService(q Repository, profiles *profile.Service, llmc llm.Provider, threshold float64, matchModel string) *Service {
	return &Service{q: q, profiles: profiles, llmc: llmc, threshold: threshold, matchModel: matchModel}
}

// fitModel returns the model used for fit scoring, falling back to the
// provider default when no task-specific model is configured.
func (s *Service) fitModel() string {
	if s.matchModel != "" {
		return s.matchModel
	}
	return s.llmc.ModelName()
}

// MatchJob runs stage 1 (embedding prefilter) + stage 2 (LLM analysis) for
// one job, mirroring MatchingService.matchJob.
func (s *Service) MatchJob(ctx context.Context, jobID string, rec *activity.Recorder) (dto.MatchResultDto, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return dto.MatchResultDto{}, err
	}
	job, err := s.q.GetJobByID(ctx, uid)
	if err != nil {
		return dto.MatchResultDto{}, fmt.Errorf("job %s not found", jobID)
	}

	prof, err := s.profiles.GetDefault(ctx)
	if err != nil {
		return dto.MatchResultDto{}, err
	}
	if prof.RendercvConfig == nil {
		summary := "no profile config"
		res, err := s.saveResult(ctx, uid, 0, nil, nil, nil, &summary, nil, "")
		if err != nil {
			return dto.MatchResultDto{}, err
		}
		return res, ErrNoProfileConfig
	}
	profileID := dbutil.UUIDString(prof.ID)

	if rec != nil {
		rec.Step(ctx, "embedding", nil)
	}

	jobText := strutil.Truncate(fmt.Sprintf("%s at %s\n%s", job.Title, job.Company, job.Description), 8000)
	jobEmbedding, err := s.llmc.Embed(ctx, jobText)
	if err != nil {
		return dto.MatchResultDto{}, err
	}
	jobVec := pgvector.NewVector(jobEmbedding)
	if err := s.q.UpdateJobEmbedding(ctx, sqlcgen.UpdateJobEmbeddingParams{ID: uid, Embedding: &jobVec}); err != nil {
		return dto.MatchResultDto{}, err
	}

	if has, err := s.profiles.HasEmbedding(ctx, profileID); err == nil && !has {
		_ = s.profiles.RefreshEmbedding(ctx, profileID)
	}

	if rec != nil {
		rec.Step(ctx, "prefilter (similarity)", nil)
	}

	similarity, err := s.profiles.Similarity(ctx, profileID, jobEmbedding)
	if err != nil {
		similarity = 0
	}

	if similarity < s.threshold {
		res, err := s.saveResult(ctx, uid, similarity, nil, nil, nil, nil, nil, "")
		if err == nil && rec != nil {
			rec.Ok(ctx, "", map[string]any{"similarity": similarity})
		}
		return res, err
	}

	master, err := generation.MasterFromProfile(prof)
	if err != nil {
		return dto.MatchResultDto{}, err
	}
	var extraNotes string
	if prof.ExtraNotes != nil {
		extraNotes = *prof.ExtraNotes
	}
	profileText := strutil.Truncate(generation.RendercvToText(master)+"\n"+extraNotes, 6000)
	description := strutil.Truncate(job.Description, 6000)
	location := "n/a"
	if job.Location != nil && *job.Location != "" {
		location = *job.Location
	}

	prompt := fmt.Sprintf(
		"Rate how well this candidate fits this job.\n\n"+
			"CANDIDATE PROFILE:\n%s\n\n"+
			"JOB POSTING:\nTitle: %s\nCompany: %s\n"+
			"Location: %s (remote: %v)\n"+
			"Description:\n%s\n\n"+
			"Scoring guide: 90-100 near-perfect fit; 70-89 strong fit, minor gaps; "+
			"50-69 partial fit, notable gaps; below 50 poor fit. "+
			"matchedSkills/missingSkills = concrete skills from the job description. "+
			"redFlags = concerns like seniority mismatch, hard requirements the candidate lacks, "+
			"suspicious posting. summary = 2-3 sentences.",
		profileText, job.Title, job.Company, location, job.Remote, description,
	)

	if rec != nil {
		rec.Step(ctx, "LLM fit analysis", nil)
	}

	fit, err := llm.CompleteStructured[FitResult](ctx, s.llmc, prompt, &llm.CompleteOptions{
		System: "You are a precise technical recruiter. Judge only from the given profile and job text.",
		Model:  s.matchModel,
	})
	if err != nil {
		return dto.MatchResultDto{}, err
	}

	score := int(math.Round(fit.Score))
	res, err := s.saveResult(ctx, uid, similarity, &score, fit.MatchedSkills, fit.MissingSkills, &fit.Summary, fit.RedFlags, s.fitModel())
	if err == nil && rec != nil {
		rec.Ok(ctx, "", map[string]any{"score": score, "similarity": similarity})
	}
	return res, err
}

func (s *Service) saveResult(
	ctx context.Context,
	jobID pgtype.UUID,
	similarity float64,
	score *int,
	matchedSkills, missingSkills []string,
	summary *string,
	redFlags []string,
	model string,
) (dto.MatchResultDto, error) {
	if model == "" {
		model = s.llmc.ModelName()
	}
	var scoreI32 *int32
	if score != nil {
		v := int32(*score)
		scoreI32 = &v
	}
	matchedJSON, err := jsonOrNull(matchedSkills)
	if err != nil {
		return dto.MatchResultDto{}, err
	}
	missingJSON, err := jsonOrNull(missingSkills)
	if err != nil {
		return dto.MatchResultDto{}, err
	}
	redFlagsJSON, err := jsonOrNull(redFlags)
	if err != nil {
		return dto.MatchResultDto{}, err
	}

	row, err := s.q.UpsertMatchResult(ctx, sqlcgen.UpsertMatchResultParams{
		JobId:         jobID,
		Similarity:    similarity,
		Score:         scoreI32,
		MatchedSkills: matchedJSON,
		MissingSkills: missingJSON,
		Summary:       summary,
		RedFlags:      redFlagsJSON,
		Model:         model,
	})
	if err != nil {
		return dto.MatchResultDto{}, err
	}
	return toDto(row), nil
}

// jsonOrNull encodes a possibly-nil string slice to JSON, or to the jsonb
// literal "null" when nil — matching the TS `matchedSkills: data.matchedSkills
// ?? undefined` (column stays NULL when the LLM didn't produce a value, e.g.
// the prefiltered-out path).
func jsonOrNull(v []string) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}

func toDto(r sqlcgen.MatchResult) dto.MatchResultDto {
	var score *int
	if r.Score != nil {
		v := int(*r.Score)
		score = &v
	}
	var matched, missing, redFlags *[]string
	var m, mi, rf []string
	if dbutil.UnmarshalJSONB(r.MatchedSkills, &m) == nil && r.MatchedSkills != nil && string(r.MatchedSkills) != "null" {
		matched = &m
	}
	if dbutil.UnmarshalJSONB(r.MissingSkills, &mi) == nil && r.MissingSkills != nil && string(r.MissingSkills) != "null" {
		missing = &mi
	}
	if dbutil.UnmarshalJSONB(r.RedFlags, &rf) == nil && r.RedFlags != nil && string(r.RedFlags) != "null" {
		redFlags = &rf
	}
	return dto.MatchResultDto{
		ID:            dbutil.UUIDString(r.ID),
		JobID:         dbutil.UUIDString(r.JobId),
		Similarity:    r.Similarity,
		Score:         score,
		MatchedSkills: matched,
		MissingSkills: missing,
		Summary:       r.Summary,
		RedFlags:      redFlags,
		Model:         r.Model,
		CreatedAt:     dbutil.Timestamp(r.CreatedAt),
	}
}
