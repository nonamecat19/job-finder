package application

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"

	"context"
	"strings"
)

func (s *Service) SetTxRunner(tx domain.TxRunner) { s.tx = tx }

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

			return apperr.NotFound("generation item", itemID)
		}
		sectionKind = kind

		if req.Text != nil && item.Origin == string(domain.OriginProfile) {
			return apperr.New(apperr.KindForbidden, "text cannot be set on a profile-origin item")
		}
		if item.Unavailable {
			return apperr.Conflict("item is unavailable")
		}

		if req.DroppedEntries != nil {
			if !hasSkillEntries(kind, item.Origin) {
				return apperr.New(apperr.KindForbidden, "droppedEntries can only be set on a profile-origin skills item")
			}
			normalized, vErr := normalizeDroppedEntries(item, *req.DroppedEntries)
			if vErr != nil {
				return vErr
			}
			if uErr := q.UpdateItemDroppedEntries(ctx, sqlcgen.UpdateItemDroppedEntriesParams{
				ID: iid, DroppedEntries: normalized,
			}); uErr != nil {
				return uErr
			}
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

func normalizeDroppedEntries(item sqlcgen.GenerationItem, requested []string) ([]string, error) {
	want := make(map[string]bool, len(requested))
	for _, r := range requested {
		if t := strings.TrimSpace(r); t != "" {
			want[t] = true
		}
	}

	entries := sqlcItemToDomain(item).SkillEntries()
	known := make(map[string]bool, len(entries))
	out := make([]string, 0, len(want))
	for _, e := range entries {
		known[e.Text] = true
		if want[e.Text] {
			out = append(out, e.Text)
		}
	}
	for name := range want {
		if !known[name] {
			return nil, apperr.Validation("not a skill in this group: " + name)
		}
	}
	return out, nil
}

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
	return sectionToDto(section, itemDtos), nil
}

func sectionToDto(section sqlcgen.GenerationSection, items []dto.GenerationItemDto) dto.GenerationSectionDto {
	return dto.GenerationSectionDto{
		ID: dbutil.UUIDString(section.ID), Kind: section.Kind, EntryKey: section.EntryKey, EntryLabel: section.EntryLabel,
		Position: int(section.Position), TargetCount: int(section.TargetCount), State: section.State, Error: section.Error,
		FallbackUsed: section.FallbackUsed, Enabled: section.Enabled, Items: items,
	}
}

func sectionKindOf(sections []sqlcgen.GenerationSection, sectionID pgtype.UUID) (string, bool) {
	for _, sec := range sections {
		if sec.ID == sectionID {
			return sec.Kind, true
		}
	}
	return "", false
}
