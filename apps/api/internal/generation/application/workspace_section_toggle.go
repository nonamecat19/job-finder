package application

import (
	"context"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
)

// SetSectionEnabled is `PATCH /v1/generations/{runId}/sections/{sectionId}`:
// the per-run switch that excludes a whole section from export/preview
// (Assemble skips it) without touching its items, so re-enabling it restores
// exactly the selection it had. Same row-level run lock and `running` guard
// as PatchGenerationItem/ReorderSection.
func (s *Service) SetSectionEnabled(ctx context.Context, runID, sectionID string, enabled bool) (dto.GenerationSectionDto, error) {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return dto.GenerationSectionDto{}, apperr.NotFound("generation run", runID)
	}
	sid, err := dbutil.ParseUUID(sectionID)
	if err != nil {
		return dto.GenerationSectionDto{}, apperr.NotFound("generation section", sectionID)
	}
	if s.tx == nil {
		return dto.GenerationSectionDto{}, apperr.Internal("generation workspace: no transaction runner configured")
	}

	var (
		section  sqlcgen.GenerationSection
		secItems []sqlcgen.GenerationItem
	)
	err = s.tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		run, gErr := q.GetRunForUpdate(ctx, rid)
		if gErr != nil {
			return apperr.NotFound("generation run", runID)
		}
		if run.State == string(domain.RunRunning) {
			return apperr.Conflict("generation run is running")
		}

		sections, sErr := q.ListSectionsByRun(ctx, rid)
		if sErr != nil {
			return sErr
		}
		found := false
		for _, sec := range sections {
			if sec.ID == sid {
				found = true
				break
			}
		}
		if !found {
			return apperr.NotFound("generation section", sectionID)
		}

		updated, uErr := q.UpdateSectionEnabled(ctx, sqlcgen.UpdateSectionEnabledParams{ID: sid, Enabled: enabled})
		if uErr != nil {
			return uErr
		}
		section = updated

		items, lErr := q.ListItemsByRun(ctx, rid)
		if lErr != nil {
			return lErr
		}
		for _, it := range items {
			if it.SectionID == sid {
				secItems = append(secItems, it)
			}
		}
		return nil
	})
	if err != nil {
		return dto.GenerationSectionDto{}, err
	}

	itemDtos := make([]dto.GenerationItemDto, 0, len(secItems))
	for _, it := range secItems {
		itemDtos = append(itemDtos, itemToDto(section.Kind, it))
	}
	return sectionToDto(section, itemDtos), nil
}
