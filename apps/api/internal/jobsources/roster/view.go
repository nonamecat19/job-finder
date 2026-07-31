package roster

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

// View presents roster state as DTOs. It exists so callers outside this
// feature — the HTTP adapter in particular — never have to name the sqlcgen
// row types or reach for dbutil to convert them
// (027-http-handler-decomposition, FR-012). Service keeps its row-typed
// methods for in-feature callers such as the board adapters.
type View struct{ svc *Service }

func NewView(svc *Service) *View { return &View{svc: svc} }

func (v *View) List(ctx context.Context) ([]dto.EmployerBoardDto, error) {
	boards, err := v.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.EmployerBoardDto, 0, len(boards))
	for _, b := range boards {
		out = append(out, toEmployerBoardDto(b))
	}
	return out, nil
}

func (v *View) RegisterFromURL(ctx context.Context, rawURL, displayName string) (dto.EmployerBoardDto, error) {
	board, err := v.svc.RegisterFromURL(ctx, rawURL, displayName)
	if err != nil {
		return dto.EmployerBoardDto{}, err
	}
	return toEmployerBoardDto(board), nil
}

func (v *View) Remove(ctx context.Context, id string) error {
	return v.svc.Remove(ctx, id)
}

func (v *View) ListCandidates(ctx context.Context) ([]dto.BoardCandidateDto, error) {
	candidates, err := v.svc.ListCandidates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.BoardCandidateDto, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, toBoardCandidateDto(c))
	}
	return out, nil
}

func (v *View) Accept(ctx context.Context, candidateID string) (dto.EmployerBoardDto, error) {
	board, err := v.svc.Accept(ctx, candidateID)
	if err != nil {
		return dto.EmployerBoardDto{}, err
	}
	return toEmployerBoardDto(board), nil
}

func (v *View) Reject(ctx context.Context, candidateID string) error {
	return v.svc.Reject(ctx, candidateID)
}

func (v *View) Discover(ctx context.Context) (int, error) {
	return v.svc.Discover(ctx)
}

func toEmployerBoardDto(e sqlcgen.EmployerBoard) dto.EmployerBoardDto {
	return dto.EmployerBoardDto{
		ID:                 dbutil.UUIDString(e.ID),
		Vendor:             e.Vendor,
		EmployerIdentifier: e.EmployerIdentifier,
		DisplayName:        e.DisplayName,
		AddedVia:           e.AddedVia,
		Enabled:            e.Enabled,
		LastSuccessAt:      dbutil.TimestampPtr(e.LastSuccessAt),
		LastPostingCount:   int(e.LastPostingCount),
		Stale:              Stale(e),
	}
}

func toBoardCandidateDto(c sqlcgen.BoardCandidate) dto.BoardCandidateDto {
	return dto.BoardCandidateDto{
		ID:                 dbutil.UUIDString(c.ID),
		Vendor:             c.Vendor,
		EmployerIdentifier: c.EmployerIdentifier,
		DisplayName:        c.EmployerIdentifier,
		InferredFromJobID:  dbutil.UUIDStringPtr(c.InferredFromJobId),
		State:              c.State,
	}
}
