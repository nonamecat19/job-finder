package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/job-finder/api/internal/matching/domain"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/profile"
	"github.com/job-finder/api/internal/strutil"
)

var ErrNoProfileConfig = errors.New("no profile config")

// contentHash identifies the exact text an embedding was produced from, so a
// later match on unchanged content can skip the call (019 R5, E4-3).
func contentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// reusableJobEmbedding returns the stored vector only when the row was embedded
// by embedModel over text with this hash. Anything else — different model,
// absent provenance, changed content, missing vector — is treated as
// unembedded and re-embedded rather than compared across vector spaces
// (FR-020, contracts/embeddings.md E4-2).
func reusableJobEmbedding(job sqlcgen.Job, hash, embedModel string) ([]float32, bool) {
	if embedModel == "" || job.EmbedModel == nil || *job.EmbedModel != embedModel {
		return nil, false
	}
	if job.EmbeddingHash == nil || *job.EmbeddingHash != hash {
		return nil, false
	}
	if job.Embedding == nil {
		return nil, false
	}
	stored := job.Embedding.Slice()
	if len(stored) == 0 {
		return nil, false
	}
	return stored, true
}

type Service struct {
	q          domain.Repository
	profiles   *profile.Service
	llmc       llm.Provider
	threshold  float64
	matchModel string

	aiPub    *Publisher
	aiRoutes func(capability string) string
}

func NewService(q domain.Repository, profiles *profile.Service, llmc llm.Provider, threshold float64, matchModel string) *Service {
	return &Service{q: q, profiles: profiles, llmc: llmc, threshold: threshold, matchModel: matchModel}
}

// SetAIRouting wires MatchJob's LLM fit-analysis step to publish
// match.requested instead of calling llmc directly, once AI_CAPABILITY_ROUTING
// routes "match" to python (C8-3: reversible by configuration alone — a nil
// or "go"-returning routes leaves MatchJob's behaviour unchanged).
func (s *Service) SetAIRouting(pub *Publisher, routes func(capability string) string) {
	s.aiPub, s.aiRoutes = pub, routes
}

func (s *Service) routedToPython() bool {
	return s.aiPub != nil && s.aiRoutes != nil && s.aiRoutes(Kind) == "python"
}

func (s *Service) Store() activity.Store {
	return s.q
}

func (s *Service) fitModel() string {
	if s.matchModel != "" {
		return s.matchModel
	}
	return s.llmc.ModelName()
}

func (s *Service) MatchJob(ctx context.Context, jobID string, rec *activity.Recorder) (dto.MatchResultDto, error) {
	// Correlate this match's LLM call with the platform's own run history
	// (036 FR-009/FR-010). The task key rides along separately — the router
	// stamps it on every call it makes — so this is the only half the call
	// site has to supply. An absent recorder leaves the call uncorrelated,
	// which is the pre-036 behaviour and valid.
	var activityID *string
	if rec != nil {
		if runID := dbutil.UUIDString(rec.ID()); runID != "" {
			ctx = llm.WithTraceID(ctx, runID)
			activityID = &runID
		}
	}

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
	hash := contentHash(jobText)
	// The profile's own provenance is the current served value this job has to
	// match: a similarity is only meaningful when both sides came out of the
	// same model (contracts/embeddings.md E4-2, FR-020).
	var embedModel string
	if prof.EmbedModel != nil {
		embedModel = *prof.EmbedModel
	}
	jobEmbedding, reused := reusableJobEmbedding(job, hash, embedModel)
	if !reused {
		// The served model is read off the gateway's response rather than a
		// Go-side config mirror, so a model swap in gateway/config.yaml stays a
		// config-only change (E4-1).
		embedCtx, servedModel := llm.WithServedModelCapture(ctx)
		jobEmbedding, err = s.llmc.Embed(embedCtx, jobText)
		if err != nil {
			return dto.MatchResultDto{}, err
		}
		jobVec := pgvector.NewVector(jobEmbedding)
		if err := s.q.UpdateJobEmbeddingWithHash(ctx, sqlcgen.UpdateJobEmbeddingWithHashParams{
			ID:            uid,
			Embedding:     &jobVec,
			EmbeddingHash: &hash,
			EmbedModel:    servedModel,
		}); err != nil {
			return dto.MatchResultDto{}, err
		}
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

	if s.routedToPython() {
		snapshot := MatchSnapshot{
			ProfileText: profileText,
			Title:       job.Title,
			Company:     job.Company,
			Location:    location,
			Remote:      job.Remote,
			Description: description,
		}
		if err := s.aiPub.PublishRequested(ctx, jobID, activityID, snapshot); err != nil {
			return dto.MatchResultDto{}, err
		}
		// The fit score lands later, off match.completed (result.go) — this
		// leaves score unset, the same shape a below-threshold result has,
		// which is exactly right: no score yet is indistinguishable from no
		// score, and result.go's UpsertMatchResult fills it in once the
		// python run completes.
		res, err := s.saveResult(ctx, uid, similarity, nil, nil, nil, nil, nil, "")
		if err == nil && rec != nil {
			rec.Ok(ctx, "", map[string]any{"similarity": similarity, "delegated": true})
		}
		return res, err
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

	fit, err := llm.CompleteStructured[domain.FitResult](ctx, s.llmc, prompt, &llm.CompleteOptions{
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
