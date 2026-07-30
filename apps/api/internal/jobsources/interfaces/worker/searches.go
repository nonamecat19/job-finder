package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

type UpdateSearchInput struct {
	Name    *string
	Query   *dto.SearchQuery
	Cron    *string
	Enabled *bool
}

func (s *Service) ListSearches(ctx context.Context) ([]dto.SavedSearchDto, error) {
	rows, err := s.q.ListSavedSearches(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SavedSearchDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, savedSearchDto(r))
	}
	return out, nil
}

func (s *Service) CreateSearch(ctx context.Context, name string, query dto.SearchQuery, cron string, enabled bool) (*dto.SavedSearchDto, error) {
	if name == "" || query.Keywords == "" {
		return nil, fmt.Errorf("name and query.keywords are required")
	}
	q, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	if cron == "" {
		cron = "0 */6 * * *"
	}
	row, err := s.q.CreateSavedSearch(ctx, sqlcgen.CreateSavedSearchParams{
		Name: name, Query: q, Cron: cron, Enabled: enabled,
	})
	if err != nil {
		return nil, err
	}
	dtoRow := savedSearchDto(row)
	return &dtoRow, nil
}

func (s *Service) UpdateSearch(ctx context.Context, id string, in UpdateSearchInput) (*dto.SavedSearchDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return nil, err
	}
	params := sqlcgen.UpdateSavedSearchParams{ID: uid}
	if in.Name != nil {
		params.Name = in.Name
	}
	if in.Query != nil {
		q, err := json.Marshal(in.Query)
		if err != nil {
			return nil, err
		}
		params.Query = q
	}
	if in.Cron != nil {
		params.Cron = in.Cron
	}
	if in.Enabled != nil {
		params.Enabled = in.Enabled
	}
	row, err := s.q.UpdateSavedSearch(ctx, params)
	if err != nil {
		return nil, err
	}
	dtoRow := savedSearchDto(row)
	return &dtoRow, nil
}

func (s *Service) DeleteSearch(ctx context.Context, id string) error {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.q.DeleteSavedSearch(ctx, uid)
}

func (s *Service) RecentRuns(ctx context.Context, limit int32) ([]dto.SourceRunDto, error) {
	rows, err := s.q.RecentRunsJoined(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SourceRunDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.SourceRunDto{
			ID:           dbutil.UUIDString(r.ID),
			SourceKey:    r.SourceKey,
			SearchID:     r.SearchID,
			StartedAt:    dbutil.Timestamp(r.StartedAt),
			FinishedAt:   dbutil.TimestampPtr(r.FinishedAt),
			OK:           r.Ok,
			Found:        int(r.Found),
			New:          int(r.New),
			Error:        r.Error,
			Verdict:      r.Verdict,
			BlockedCount: int(r.BlockedCount),
			BlockReason:  r.BlockReason,
		})
	}
	return out, nil
}

func savedSearchDto(r sqlcgen.SavedSearch) dto.SavedSearchDto {
	var q dto.SearchQuery
	_ = dbutil.UnmarshalJSONB(r.Query, &q)
	return dto.SavedSearchDto{
		ID: dbutil.UUIDString(r.ID), Name: r.Name, Query: q,
		Cron: r.Cron, Enabled: r.Enabled, LastRunAt: dbutil.TimestampPtr(r.LastRunAt),
	}
}
