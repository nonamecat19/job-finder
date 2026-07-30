package domain

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port. *sqlcgen.Queries satisfies
// it structurally.
type Repository interface {
	InsertBootstrapCode(ctx context.Context, arg sqlcgen.InsertBootstrapCodeParams) (sqlcgen.ExtBootstrapCode, error)
	ConsumeBootstrapCode(ctx context.Context, codeHash string) (sqlcgen.ExtBootstrapCode, error)
	InsertRefreshToken(ctx context.Context, arg sqlcgen.InsertRefreshTokenParams) (sqlcgen.ExtRefreshToken, error)
	GetActiveRefreshToken(ctx context.Context, tokenHash string) (sqlcgen.ExtRefreshToken, error)
	RevokeRefreshToken(ctx context.Context, arg sqlcgen.RevokeRefreshTokenParams) error
}
