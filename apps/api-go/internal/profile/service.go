// Package profile ports modules/profile/*: CRUD, PDF import -> LLM
// structured draft, and the profile-to-text serialization used by matching
// and generation for embeddings and prompts.
package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pgvector/pgvector-go"

	"github.com/job-finder/api-go/internal/db/sqlcgen"
	"github.com/job-finder/api-go/internal/dbutil"
	"github.com/job-finder/api-go/internal/dto"
	"github.com/job-finder/api-go/internal/llm"
)

type Service struct {
	q          *sqlcgen.Queries
	llmc       llm.Provider
	embedModel string
}

func NewService(q *sqlcgen.Queries, llmc llm.Provider, embedModel string) *Service {
	if embedModel == "" {
		embedModel = "nomic-embed-text"
	}
	return &Service{q: q, llmc: llmc, embedModel: embedModel}
}

func (s *Service) List(ctx context.Context) ([]dto.ProfileDto, error) {
	rows, err := s.q.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ProfileDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDto(r))
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
	return toDto(row), nil
}

// GetDefault returns the most-recently-updated profile — the single-user
// default for matching/generation, matching ProfileService.getDefault.
func (s *Service) GetDefault(ctx context.Context) (sqlcgen.Profile, error) {
	row, err := s.q.GetDefaultProfile(ctx)
	if err != nil {
		return sqlcgen.Profile{}, fmt.Errorf("no profile exists yet — create one first")
	}
	return row, nil
}

func (s *Service) Create(ctx context.Context, name string, document dto.JsonResume, extraNotes *string) (dto.ProfileDto, error) {
	if name == "" {
		return dto.ProfileDto{}, fmt.Errorf("name is required")
	}
	doc, err := json.Marshal(document)
	if err != nil {
		return dto.ProfileDto{}, err
	}
	row, err := s.q.CreateProfile(ctx, sqlcgen.CreateProfileParams{Name: name, Document: doc, ExtraNotes: extraNotes})
	if err != nil {
		return dto.ProfileDto{}, err
	}
	id := dbutil.UUIDString(row.ID)
	if err := s.RefreshEmbedding(ctx, id); err != nil {
		// best-effort, matches the TS .catch(warn) — profile still gets created
		_ = err
	}
	return s.GetDto(ctx, id)
}

type UpdateInput struct {
	Name       *string
	Document   *dto.JsonResume
	ExtraNotes **string // outer pointer = "field present in request"; inner = new value (nil clears)
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
	if in.Document != nil {
		doc, err := json.Marshal(*in.Document)
		if err != nil {
			return dto.ProfileDto{}, err
		}
		params.Document = doc
	}
	if err := s.q.UpdateProfile(ctx, params); err != nil {
		return dto.ProfileDto{}, err
	}
	if in.ExtraNotes != nil {
		if err := s.q.UpdateProfileExtraNotes(ctx, sqlcgen.UpdateProfileExtraNotesParams{ID: uid, ExtraNotes: *in.ExtraNotes}); err != nil {
			return dto.ProfileDto{}, err
		}
	}
	_ = s.RefreshEmbedding(ctx, id) // best-effort
	return s.GetDto(ctx, id)
}

func (s *Service) Remove(ctx context.Context, id string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	uid, _ := dbutil.ParseUUID(id)
	return s.q.DeleteProfile(ctx, uid)
}

// HasEmbedding reports whether the profile already has a stored embedding.
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

// Similarity computes 1 - cosine_distance(profile.embedding, jobEmbedding)
// via pgvector's <=> operator, matching matching.service.ts's cosineDistance usage.
func (s *Service) Similarity(ctx context.Context, id string, jobEmbedding []float32) (float64, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return 0, err
	}
	vec := pgvector.NewVector(jobEmbedding)
	return s.q.ProfileSimilarity(ctx, sqlcgen.ProfileSimilarityParams{ID: uid, Embedding: &vec})
}

// RefreshEmbedding serializes the profile to text and stores its embedding —
// matches ProfileService.refreshEmbedding.
func (s *Service) RefreshEmbedding(ctx context.Context, id string) error {
	row, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	var doc dto.JsonResume
	_ = dbutil.UnmarshalJSONB(row.Document, &doc)
	text := ProfileToText(doc, row.ExtraNotes)
	embedding, err := s.llmc.Embed(ctx, text)
	if err != nil {
		return err
	}
	vec := pgvector.NewVector(embedding)
	model := s.embedModel
	uid, _ := dbutil.ParseUUID(id)
	return s.q.UpdateProfileEmbedding(ctx, sqlcgen.UpdateProfileEmbeddingParams{ID: uid, Embedding: &vec, EmbedModel: &model})
}

// ProfileToText mirrors ProfileService.profileToText.
func ProfileToText(doc dto.JsonResume, extraNotes *string) string {
	var parts []string
	if doc.Basics != nil {
		if doc.Basics.Label != nil && *doc.Basics.Label != "" {
			parts = append(parts, *doc.Basics.Label)
		}
		if doc.Basics.Summary != nil && *doc.Basics.Summary != "" {
			parts = append(parts, *doc.Basics.Summary)
		}
	}
	for _, w := range doc.Work {
		fields := []string{}
		if w.Position != nil {
			fields = append(fields, *w.Position)
		}
		fields = append(fields, "at", w.Name, "—")
		if w.Summary != nil {
			fields = append(fields, *w.Summary)
		}
		fields = append(fields, w.Highlights...)
		parts = append(parts, joinNonEmpty(fields))
	}
	for _, sk := range doc.Skills {
		fields := append([]string{sk.Name}, sk.Keywords...)
		parts = append(parts, strings.Join(fields, " "))
	}
	for _, p := range doc.Projects {
		fields := []string{p.Name}
		if p.Description != nil {
			fields = append(fields, *p.Description)
		}
		fields = append(fields, p.Keywords...)
		parts = append(parts, joinNonEmpty(fields))
	}
	if extraNotes != nil && *extraNotes != "" {
		parts = append(parts, *extraNotes)
	}
	return strings.Join(parts, "\n")
}

func joinNonEmpty(fields []string) string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return strings.Join(out, " ")
}

func toDto(r sqlcgen.Profile) dto.ProfileDto {
	var doc dto.JsonResume
	_ = dbutil.UnmarshalJSONB(r.Document, &doc)
	return dto.ProfileDto{
		ID:         dbutil.UUIDString(r.ID),
		Name:       r.Name,
		Document:   doc,
		ExtraNotes: r.ExtraNotes,
		UpdatedAt:  dbutil.Timestamp(r.UpdatedAt),
	}
}
