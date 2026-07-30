package domain

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type ExtBootstrapCode struct {
	ID        pgtype.UUID
	CodeHash  string
	ExpiresAt time.Time
	UsedAt    pgtype.Timestamp
}

type InsertBootstrapCodeParams struct {
	CodeHash  string
	ExpiresAt time.Time
}

type ExtRefreshToken struct {
	ID          pgtype.UUID
	TokenHash   string
	ExpiresAt   time.Time
	RevokedAt   pgtype.Timestamp
	RotatedToId pgtype.UUID
}

type InsertRefreshTokenParams struct {
	TokenHash string
	ExpiresAt time.Time
}

type RevokeRefreshTokenParams struct {
	ID          pgtype.UUID
	RotatedToId pgtype.UUID
}

// Repository is the outbound persistence port. The in-memory fakeRepo in
// service_test.go satisfies it structurally; a sqlc-backed implementation
// would implement each method against the real database.
type Repository interface {
	InsertBootstrapCode(ctx context.Context, arg InsertBootstrapCodeParams) (ExtBootstrapCode, error)
	ConsumeBootstrapCode(ctx context.Context, codeHash string) (ExtBootstrapCode, error)
	InsertRefreshToken(ctx context.Context, arg InsertRefreshTokenParams) (ExtRefreshToken, error)
	GetActiveRefreshToken(ctx context.Context, tokenHash string) (ExtRefreshToken, error)
	RevokeRefreshToken(ctx context.Context, arg RevokeRefreshTokenParams) error
}
