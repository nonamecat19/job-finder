package application

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"

	"context"
)

// SetTxRunner installs the transactional port PatchGenerationItem and
// ReorderSection need (T025/T026). A setter for the same reason
// SetAsynqClient is: it keeps every existing NewService call, and every test
// that doesn't exercise item mutation, unchanged.
func (s *Service) SetTxRunner(tx domain.TxRunner) { s.tx = tx }

// PatchGenerationItem is `PATCH /v1/generations/{runId}/items/{itemId}`
// (rest-api.md): toggle `selected`, move `position`, or edit `text` — any
// subset. Takes a row-level `SELECT ... FOR UPDATE` on the run first (the
// discipline 020 specified), then rejects `text` on a profile-origin item
// with 403 (FR-009 at the API boundary), refuses a running run or an
// unavailable item with 409, and is idempotent: re-applying the same values
// is just another COALESCE update that lands on the same row.
func (s *Service) PatchGenerationItem(ctx context.Context, runID, itemID string, req dto.PatchGenerationItemRequestDto) (dto.GenerationItemDto, error) {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return dto.GenerationItemDto{}, apperr.NotFound("generation run", runID)
	}
	iid, err := dbutil.ParseUUID(itemID)
	if err != nil {
		return dto.GenerationItemDto{}, apperr.NotFound("generation item", itemID)
	}
	if s.tx == nil {
		return dto.GenerationItemDto{}, apperr.Internal("generation workspace: no transaction runner configured")
	}

	var (
		updated     sqlcgen.GenerationItem
		sectionKind string
	)
	err = s.tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		run, gErr := q.GetRunForUpdate(ctx, rid)
		if gErr != nil {
			return apperr.NotFound("generation run", runID)
		}
		if run.State == string(domain.RunRunning) {
			return apperr.Conflict("generation run is running")
		}

		item, iErr := q.GetItemForUpdate(ctx, iid)
		if iErr != nil {
			return apperr.NotFound("generation item", itemID)
		}

		sections, sErr := q.ListSectionsByRun(ctx, rid)
		if sErr != nil {
			return sErr
		}
		kind, ok := sectionKindOf(sections, item.SectionID)
		if !ok {
			// The item exists but not under this run.
			return apperr.NotFound("generation item", itemID)
		}
		sectionKind = kind

		if req.Text != nil && item.Origin == string(domain.OriginProfile) {
			return apperr.New(apperr.KindForbidden, "text cannot be set on a profile-origin item")
		}
		if item.Unavailable {
			return apperr.Conflict("item is unavailable")
		}

		if req.Text != nil {
			if uErr := q.UpdateItemText(ctx, sqlcgen.UpdateItemTextParams{ID: iid, EditedText: req.Text}); uErr != nil {
				return uErr
			}
		}
		if req.Selected != nil || req.Position != nil {
			var pos *int32
			if req.Position != nil {
				p := int32(*req.Position)
				pos = &p
			}
			if uErr := q.UpdateItemSelection(ctx, sqlcgen.UpdateItemSelectionParams{
				ID: iid, Selected: req.Selected, Position: pos,
			}); uErr != nil {
				return uErr
			}
		}

		fresh, fErr := q.GetItemForUpdate(ctx, iid)
		if fErr != nil {
			return fErr
		}
		updated = fresh
		return nil
	})
	if err != nil {
		return dto.GenerationItemDto{}, err
	}
	return itemToDto(sectionKind, updated), nil
}

// ReorderSection is
// `PATCH /v1/generations/{runId}/sections/{sectionId}/order` (rest-api.md):
// a whole-section reorder in one transaction, from the caller's item id
// order. Same row-level run lock and `running` guard as
// PatchGenerationItem.
func (s *Service) ReorderSection(ctx context.Context, runID, sectionID string, itemIDs []string) (dto.GenerationSectionDto, error) {
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

	ids := make([]pgtype.UUID, len(itemIDs))
	positions := make([]int32, len(itemIDs))
	for i, raw := range itemIDs {
		id, pErr := dbutil.ParseUUID(raw)
		if pErr != nil {
			return dto.GenerationSectionDto{}, apperr.Validation("invalid item id: " + raw)
		}
		ids[i] = id
		positions[i] = int32(i)
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
				section = sec
				found = true
				break
			}
		}
		if !found {
			return apperr.NotFound("generation section", sectionID)
		}

		if len(ids) > 0 {
			if rErr := q.ReorderSectionItems(ctx, sqlcgen.ReorderSectionItemsParams{
				SectionID: sid, ItemIds: ids, Positions: positions,
			}); rErr != nil {
				return rErr
			}
		}

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
	return dto.GenerationSectionDto{
		ID: dbutil.UUIDString(section.ID), Kind: section.Kind, EntryKey: section.EntryKey, EntryLabel: section.EntryLabel,
		Position: int(section.Position), TargetCount: int(section.TargetCount), State: section.State, Error: section.Error,
		FallbackUsed: section.FallbackUsed, Items: itemDtos,
	}, nil
}

// sectionKindOf finds sec.ID == sectionID among a run's sections and reports
// its kind — how PatchGenerationItem confirms an item belongs to the run in
// its URL (404 otherwise) without a dedicated GetSectionByID query.
func sectionKindOf(sections []sqlcgen.GenerationSection, sectionID pgtype.UUID) (string, bool) {
	for _, sec := range sections {
		if sec.ID == sectionID {
			return sec.Kind, true
		}
	}
	return "", false
}
