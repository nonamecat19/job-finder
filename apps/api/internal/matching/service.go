package matching

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/profile"
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

func (s *Service) fitModel() string {
	if s.matchModel != "" {
		return s.matchModel
	}
	return s.llmc.ModelName()
}

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

	similarity, err := s.runEmbeddingPrefilter(ctx, prof, job, rec)
	if err != nil {
		return dto.MatchResultDto{}, err
	}
	if similarity < s.threshold {
		return s.saveResult(ctx, uid, similarity, nil, nil, nil, nil, nil, "")
	}

	score, matched, missing, summary, redFlags, err := s.runLLMAnalysis(ctx, prof, job, rec)
	if err != nil {
		return dto.MatchResultDto{}, err
	}

	return s.saveResult(ctx, uid, similarity, &score, matched, missing, &summary, redFlags, s.fitModel())
}

func (s *Service) saveResult(
	ctx context.Context, jobID pgtype.UUID, similarity float64, score *int,
	matchedSkills, missingSkills []string, summary *string, redFlags []string, model string,
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
		JobId:         jobID, Similarity: similarity, Score: scoreI32,
		MatchedSkills: matchedJSON, MissingSkills: missingJSON,
		Summary: summary, RedFlags: redFlagsJSON, Model: model,
	})
	if err != nil {
		return dto.MatchResultDto{}, err
	}
	return toDto(row), nil
}
