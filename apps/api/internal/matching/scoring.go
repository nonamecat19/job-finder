package matching

import (
	"context"
	"fmt"
	"math"

	"github.com/pgvector/pgvector-go"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/domain"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/strutil"
)

func (s *Service) runEmbeddingPrefilter(ctx context.Context, prof domain.Profile, job domain.Job, rec *activity.Recorder) (float64, error) {
	profileID := prof.ID

	if rec != nil {
		rec.Step(ctx, "embedding", nil)
	}

	jobText := strutil.Truncate(fmt.Sprintf("%s at %s\n%s", job.Title, job.Company, job.Description), 8000)
	jobEmbedding, err := s.llmc.Embed(ctx, jobText)
	if err != nil {
		return 0, err
	}
	jobVec := pgvector.NewVector(jobEmbedding)

	jobID, _ := dbutil.ParseUUID(job.ID)
	if err := s.q.UpdateJobEmbedding(ctx, sqlcgen.UpdateJobEmbeddingParams{
		ID: jobID, Embedding: &jobVec,
	}); err != nil {
		return 0, err
	}

	if has, err := s.profiles.HasEmbedding(ctx, profileID); err == nil && !has {
		_ = s.profiles.RefreshEmbedding(ctx, profileID)
	}

	if rec != nil {
		rec.Step(ctx, "prefilter (similarity)", nil)
	}

	similarity, err := s.profiles.Similarity(ctx, profileID, jobEmbedding)
	if err != nil {
		return 0, nil
	}
	return similarity, nil
}

func (s *Service) runLLMAnalysis(ctx context.Context, prof domain.Profile, job domain.Job, rec *activity.Recorder) (score int, matched, missing []string, summary string, redFlags []string, err error) {
	master, err := generation.MasterFromConfig(prof.RendercvConfig)
	if err != nil {
		return 0, nil, nil, "", nil, err
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
		return 0, nil, nil, "", nil, err
	}

	score = int(math.Round(fit.Score))
	matched = fit.MatchedSkills
	missing = fit.MissingSkills
	summary = fit.Summary
	redFlags = fit.RedFlags
	return
}
