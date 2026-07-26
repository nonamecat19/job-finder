package roster

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port for the employer roster.
// *sqlcgen.Queries satisfies it structurally.
type Repository interface {
	ListEmployerBoards(ctx context.Context) ([]sqlcgen.EmployerBoard, error)
	ListEmployerBoardsByVendor(ctx context.Context, vendor string) ([]sqlcgen.EmployerBoard, error)
	GetEmployerBoard(ctx context.Context, arg sqlcgen.GetEmployerBoardParams) (sqlcgen.EmployerBoard, error)
	InsertEmployerBoard(ctx context.Context, arg sqlcgen.InsertEmployerBoardParams) (sqlcgen.EmployerBoard, error)
	DeleteEmployerBoard(ctx context.Context, id pgtype.UUID) error
	RecordEmployerBoardRunOutcome(ctx context.Context, arg sqlcgen.RecordEmployerBoardRunOutcomeParams) error

	ListBoardCandidates(ctx context.Context) ([]sqlcgen.BoardCandidate, error)
	GetBoardCandidate(ctx context.Context, arg sqlcgen.GetBoardCandidateParams) (sqlcgen.BoardCandidate, error)
	GetBoardCandidateByID(ctx context.Context, id pgtype.UUID) (sqlcgen.BoardCandidate, error)
	InsertBoardCandidate(ctx context.Context, arg sqlcgen.InsertBoardCandidateParams) (sqlcgen.BoardCandidate, error)
	DecideBoardCandidate(ctx context.Context, arg sqlcgen.DecideBoardCandidateParams) error

	ListApplyURLsForDiscovery(ctx context.Context, limit int32) ([]string, error)
}
