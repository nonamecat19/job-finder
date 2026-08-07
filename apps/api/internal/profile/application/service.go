package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/pgvector/pgvector-go"
	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/profile/domain"
)

type Service struct {
	q           domain.Repository
	llmc        llm.Provider
	embedModel  string
	rendercvBin string
}

func NewService(q domain.Repository, llmc llm.Provider, embedModel string, rendercvBin string) *Service {
	if embedModel == "" {
		embedModel = "nomic-embed-text"
	}
	return &Service{q: q, llmc: llmc, embedModel: embedModel, rendercvBin: rendercvBin}
}

func (s *Service) List(ctx context.Context) ([]dto.ProfileDto, error) {
	rows, err := s.q.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ProfileDto, len(rows))
	for i, r := range rows {
		out[i] = s.toDto(r)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (sqlcgen.Profile, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return sqlcgen.Profile{}, err
	}
	row, err := s.q.GetProfile(ctx, uid)
	if err != nil {
		return sqlcgen.Profile{}, fmt.Errorf("profile %s not found", id)
	}
	return row, nil
}

func (s *Service) GetDto(ctx context.Context, id string) (dto.ProfileDto, error) {
	row, err := s.Get(ctx, id)
	if err != nil {
		return dto.ProfileDto{}, err
	}
	return s.toDto(row), nil
}

func (s *Service) GetDefault(ctx context.Context) (sqlcgen.Profile, error) {
	row, err := s.q.GetDefaultProfile(ctx)
	if err != nil {
		return sqlcgen.Profile{}, fmt.Errorf("no profile exists yet — create one first")
	}
	return row, nil
}

func (s *Service) Create(ctx context.Context, name string, rendercvYaml string, extraNotes *string) (dto.ProfileDto, error) {
	if name == "" {
		return dto.ProfileDto{}, fmt.Errorf("name is required")
	}
	if rendercvYaml == "" {
		seed, err := yaml.Marshal(map[string]any{"cv": map[string]any{"name": name}})
		if err != nil {
			return dto.ProfileDto{}, err
		}
		rendercvYaml = string(seed)
	}
	master, err := generation.ParseRendercv(rendercvYaml)
	if err != nil {
		return dto.ProfileDto{}, err
	}
	configJSON, err := json.Marshal(master)
	if err != nil {
		return dto.ProfileDto{}, err
	}
	row, err := s.q.CreateProfile(ctx, sqlcgen.CreateProfileParams{
		Name:           name,
		RendercvYaml:   &rendercvYaml,
		RendercvConfig: configJSON,
		ExtraNotes:     extraNotes,
	})
	if err != nil {
		return dto.ProfileDto{}, err
	}
	id := dbutil.UUIDString(row.ID)
	if err := s.RefreshEmbedding(ctx, id); err != nil {
		_ = err
	}
	row, _ = s.Get(ctx, id)
	return s.toDto(row), nil
}

type UpdateInput struct {
	Name         *string
	RendercvYaml *string
	ExtraNotes   **string
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (dto.ProfileDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return dto.ProfileDto{}, err
	}
	if _, err := s.Get(ctx, id); err != nil {
		return dto.ProfileDto{}, err
	}

	params := sqlcgen.UpdateProfileParams{ID: uid}
	if in.Name != nil {
		params.Name = in.Name
	}
	if in.RendercvYaml != nil {
		master, err := generation.ParseRendercv(*in.RendercvYaml)
		if err != nil {
			return dto.ProfileDto{}, fmt.Errorf("invalid rendercv yaml: %w", err)
		}
		configJSON, err := json.Marshal(master)
		if err != nil {
			return dto.ProfileDto{}, err
		}
		params.RendercvYaml = in.RendercvYaml
		params.RendercvConfig = configJSON
	}
	if err := s.q.UpdateProfile(ctx, params); err != nil {
		return dto.ProfileDto{}, err
	}
	if in.ExtraNotes != nil {
		if err := s.q.UpdateProfileExtraNotes(ctx, sqlcgen.UpdateProfileExtraNotesParams{ID: uid, ExtraNotes: *in.ExtraNotes}); err != nil {
			return dto.ProfileDto{}, err
		}
	}
	if in.RendercvYaml != nil {
		_ = s.RefreshEmbedding(ctx, id)
	}
	row, _ := s.Get(ctx, id)
	return s.toDto(row), nil
}

func (s *Service) masterFor(row sqlcgen.Profile) (generation.RendercvMaster, error) {
	if row.RendercvConfig == nil {
		return generation.RendercvMaster{"cv": map[string]any{"name": row.Name}}, nil
	}
	var master generation.RendercvMaster
	if err := json.Unmarshal(row.RendercvConfig, &master); err != nil {
		return nil, err
	}
	return master, nil
}

func (s *Service) GetResume(ctx context.Context, id string) (dto.Resume, error) {
	row, err := s.Get(ctx, id)
	if err != nil {
		return dto.Resume{}, err
	}
	master, err := s.masterFor(row)
	if err != nil {
		return dto.Resume{}, err
	}
	return generation.MasterToResume(master)
}

func (s *Service) UpdateResume(ctx context.Context, id string, resume dto.Resume) (dto.Resume, error) {
	if verr := generation.ValidateResume(resume); verr != nil {
		return dto.Resume{}, verr
	}
	row, err := s.Get(ctx, id)
	if err != nil {
		return dto.Resume{}, err
	}
	existing, err := s.masterFor(row)
	if err != nil {
		return dto.Resume{}, err
	}
	master, err := generation.ResumeToMaster(resume, existing)
	if err != nil {
		return dto.Resume{}, err
	}
	prepared, err := generation.PrepareMasterForMarshal(master)
	if err != nil {
		return dto.Resume{}, err
	}
	yamlBytes, err := yaml.Marshal(map[string]any(prepared))
	if err != nil {
		return dto.Resume{}, err
	}
	yamlText := string(yamlBytes)
	if _, err := s.Update(ctx, id, UpdateInput{RendercvYaml: &yamlText}); err != nil {
		return dto.Resume{}, err
	}
	return s.GetResume(ctx, id)
}

func (s *Service) HasResumeContent(ctx context.Context, id string) (bool, error) {
	resume, err := s.GetResume(ctx, id)
	if err != nil {
		return false, err
	}
	if resume.Headline != nil || resume.Location != nil || resume.Email != nil || resume.Phone != nil ||
		resume.Website != nil || len(resume.SocialNetworks) > 0 || len(resume.CustomConnections) > 0 {
		return true, nil
	}
	for _, sec := range resume.Sections {
		if len(sec.Entries) > 0 {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) SaveConfig(ctx context.Context, yamlText string) (dto.ProfileDto, error) {
	master, err := generation.ParseRendercv(yamlText)
	if err != nil {
		return dto.ProfileDto{}, err
	}

	tempDir, err := os.MkdirTemp("", "rendercv-smoke-*")
	if err != nil {
		return dto.ProfileDto{}, fmt.Errorf("failed to create temp dir for smoke test: %w", err)
	}
	defer os.RemoveAll(tempDir)

	smokeRenderer := generation.NewRenderCvRenderer(tempDir, s.rendercvBin)
	_, _, err = smokeRenderer.Render(ctx, master, "smoke_test")
	if err != nil {
		return dto.ProfileDto{}, fmt.Errorf("smoke test render failed: %w", err)
	}

	cv, _ := master["cv"].(map[string]any)
	name := "Default Profile"
	if cv != nil {
		if n, ok := cv["name"].(string); ok && strings.TrimSpace(n) != "" {
			name = n
		}
	}

	configJSON, err := json.Marshal(master)
	if err != nil {
		return dto.ProfileDto{}, fmt.Errorf("failed to marshal config: %w", err)
	}

	var profileRow sqlcgen.Profile
	defaultProf, err := s.GetDefault(ctx)
	if err == nil {
		params := sqlcgen.UpdateProfileParams{
			ID:             defaultProf.ID,
			Name:           &name,
			RendercvYaml:   &yamlText,
			RendercvConfig: configJSON,
		}
		if err := s.q.UpdateProfile(ctx, params); err != nil {
			return dto.ProfileDto{}, fmt.Errorf("failed to update profile: %w", err)
		}
		profileRow, err = s.Get(ctx, dbutil.UUIDString(defaultProf.ID))
		if err != nil {
			return dto.ProfileDto{}, err
		}
	} else {
		profileRow, err = s.q.CreateProfile(ctx, sqlcgen.CreateProfileParams{
			Name:           name,
			RendercvYaml:   &yamlText,
			RendercvConfig: configJSON,
		})
		if err != nil {
			return dto.ProfileDto{}, fmt.Errorf("failed to create profile: %w", err)
		}
	}

	id := dbutil.UUIDString(profileRow.ID)
	if err := s.RefreshEmbedding(ctx, id); err != nil {
		slog.Error("failed to refresh embedding after config upload", "error", err)
	}

	profileRow, err = s.Get(ctx, id)
	if err != nil {
		return dto.ProfileDto{}, err
	}

	return s.toDto(profileRow), nil
}

func (s *Service) Remove(ctx context.Context, id string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	uid, _ := dbutil.ParseUUID(id)
	return s.q.DeleteProfile(ctx, uid)
}

func (s *Service) HasEmbedding(ctx context.Context, id string) (bool, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return false, err
	}
	has, err := s.q.ProfileHasEmbedding(ctx, uid)
	if err != nil {
		return false, err
	}
	b, _ := has.(bool)
	return b, nil
}

func (s *Service) HasConfig(ctx context.Context, id string) (bool, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return false, err
	}
	has, err := s.q.ProfileHasConfig(ctx, uid)
	if err != nil {
		return false, err
	}
	b, _ := has.(bool)
	return b, nil
}

func (s *Service) Similarity(ctx context.Context, id string, jobEmbedding []float32) (float64, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return 0, err
	}
	vec := pgvector.NewVector(jobEmbedding)
	return s.q.ProfileSimilarity(ctx, sqlcgen.ProfileSimilarityParams{ID: uid, Embedding: &vec})
}

func (s *Service) RefreshEmbedding(ctx context.Context, id string) error {
	row, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if row.RendercvConfig == nil {
		return fmt.Errorf("profile %s has no rendercv config", id)
	}
	var master generation.RendercvMaster
	if err := json.Unmarshal(row.RendercvConfig, &master); err != nil {
		return err
	}
	text := generation.RendercvToText(master)
	embedding, err := s.llmc.Embed(ctx, text)
	if err != nil {
		return err
	}
	vec := pgvector.NewVector(embedding)
	model := s.embedModel
	uid, _ := dbutil.ParseUUID(id)
	return s.q.UpdateProfileEmbedding(ctx, sqlcgen.UpdateProfileEmbeddingParams{ID: uid, Embedding: &vec, EmbedModel: &model})
}

func (s *Service) toDto(p sqlcgen.Profile) dto.ProfileDto {
	out := dto.ProfileDto{
		ID:         dbutil.UUIDString(p.ID),
		Name:       p.Name,
		ExtraNotes: p.ExtraNotes,
		UpdatedAt:  dbutil.Timestamp(p.UpdatedAt),
	}
	if p.RendercvConfig != nil {
		var master generation.RendercvMaster
		if err := json.Unmarshal(p.RendercvConfig, &master); err == nil {
			out.HasConfig = true

			summary := &dto.RendercvSummary{}

			cv, _ := master["cv"].(map[string]any)
			if cv != nil {
				if n, ok := cv["name"].(string); ok {
					summary.Name = n
				}
				if h, ok := cv["headline"].(string); ok {
					summary.Headline = h
				}
			}

			sections := generation.CvSections(master)
			if sections != nil {
				if skillsRaw, ok := sections["skills"]; ok {
					for _, g := range generation.AsSliceOfMaps(skillsRaw) {
						if label := generation.StringField(g, "label"); label != "" {
							summary.SkillGroups = append(summary.SkillGroups, label)
						}
					}
				}
				if expRaw, ok := sections["experience"]; ok {
					for _, e := range generation.AsSliceOfMaps(expRaw) {
						company := generation.StringField(e, "company")
						highlights := generation.StringSliceField(e, "highlights")
						summary.Experience = append(summary.Experience, dto.RendercvSummaryExperience{
							Company:        company,
							HighlightCount: len(highlights),
						})
					}
				}
			}

			out.RendercvConfig = summary
			out.RendercvFull = map[string]any(master)
		}
	}
	return out
}
