package application

import (
	"context"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
)

func (s *Service) RewriteGenerationItem(ctx context.Context, runID, itemID string) (dto.GenerationRewriteResponseDto, error) {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return dto.GenerationRewriteResponseDto{}, apperr.NotFound("generation run", runID)
	}
	iid, err := dbutil.ParseUUID(itemID)
	if err != nil {
		return dto.GenerationRewriteResponseDto{}, apperr.NotFound("generation item", itemID)
	}

	item, err := s.q.GetItemForUpdate(ctx, iid)
	if err != nil {
		return dto.GenerationRewriteResponseDto{}, apperr.NotFound("generation item", itemID)
	}
	sections, err := s.q.ListSectionsByRun(ctx, rid)
	if err != nil {
		return dto.GenerationRewriteResponseDto{}, err
	}
	kind, ok := sectionKindOf(sections, item.SectionID)
	if !ok {

		return dto.GenerationRewriteResponseDto{}, apperr.NotFound("generation item", itemID)
	}

	if item.Origin != string(domain.OriginAI) || kind != string(domain.SectionKindExperience) {
		return dto.GenerationRewriteResponseDto{}, apperr.New(apperr.KindForbidden, "rewrite is only available for AI-suggested experience bullets")
	}
	if item.Unavailable {
		return dto.GenerationRewriteResponseDto{}, apperr.Conflict("item is unavailable")
	}

	source := sqlcItemToDomain(item).EffectiveText()
	result, err := rewriteBullet(ctx, s.llm.Select, s.genModel, source)
	if err != nil {
		return dto.GenerationRewriteResponseDto{}, err
	}
	grounded := domain.FilterGroundedRewordings(source, result.Variants)
	return dto.GenerationRewriteResponseDto{Variants: grounded}, nil
}
