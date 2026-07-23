package application

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/extauth/domain"
)

// Service implements bootstrap-code issuance/exchange and refresh-token
// rotation.
type Service struct {
	repo   domain.Repository
	signer *domain.Signer
}

func NewService(repo domain.Repository, signer *domain.Signer) *Service {
	return &Service{repo: repo, signer: signer}
}

// IssueBootstrapCode generates a new one-time code (spec 2.1 step 2),
// stores only its hash, and returns the plaintext for the dashboard to
// display (QR/string). The plaintext is never persisted or logged.
func (s *Service) IssueBootstrapCode(ctx context.Context) (code string, expiresAt time.Time, err error) {
	plaintext, hash, err := domain.RandomToken(domain.BootstrapCodeBytes)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = time.Now().Add(domain.BootstrapCodeTTL)
	if _, err := s.repo.InsertBootstrapCode(ctx, sqlcgen.InsertBootstrapCodeParams{
		CodeHash:  hash,
		ExpiresAt: toPgTimestamp(expiresAt),
	}); err != nil {
		return "", time.Time{}, err
	}
	return plaintext, expiresAt, nil
}

// ExchangeBootstrapCode consumes a one-time bootstrap code and issues a
// fresh access + refresh token pair (spec 2.1 step 3-4). The code is
// atomically marked used at the DB level (UPDATE ... WHERE "usedAt" IS
// NULL), so concurrent/duplicate exchange attempts can't both succeed.
func (s *Service) ExchangeBootstrapCode(ctx context.Context, code string) (domain.TokenPair, error) {
	if code == "" {
		return domain.TokenPair{}, domain.ErrInvalidCode
	}
	if _, err := s.repo.ConsumeBootstrapCode(ctx, domain.HashToken(code)); err != nil {
		return domain.TokenPair{}, domain.ErrInvalidCode
	}
	pair, _, err := s.issuePair(ctx)
	return pair, err
}

// RefreshTokens rotates a refresh token: the presented token is revoked
// (linked to its replacement) and a new access + refresh pair is issued
// (spec 2.3). One-time-use rotation means a stolen-then-replayed old token
// fails GetActiveRefreshToken (already revoked), forcing re-authentication
// via the dashboard bootstrap flow rather than silently accepting a replay.
func (s *Service) RefreshTokens(ctx context.Context, refreshToken string) (domain.TokenPair, error) {
	if refreshToken == "" {
		return domain.TokenPair{}, domain.ErrInvalidRefreshToken
	}
	old, err := s.repo.GetActiveRefreshToken(ctx, domain.HashToken(refreshToken))
	if err != nil {
		return domain.TokenPair{}, domain.ErrInvalidRefreshToken
	}

	pair, newID, err := s.issuePair(ctx)
	if err != nil {
		return domain.TokenPair{}, err
	}
	if err := s.repo.RevokeRefreshToken(ctx, sqlcgen.RevokeRefreshTokenParams{
		ID:          old.ID,
		RotatedToId: newID,
	}); err != nil {
		return domain.TokenPair{}, err
	}
	return pair, nil
}

// issuePair mints an access token + a brand-new refresh token row and
// returns both in plaintext (the refresh token's plaintext is never stored
// — only its hash — so this is the one and only place it exists), plus the
// new row's id so callers can link a rotated-from token to it.
func (s *Service) issuePair(ctx context.Context) (domain.TokenPair, pgtype.UUID, error) {
	access, accessExp, err := s.signer.Issue(domain.Subject)
	if err != nil {
		return domain.TokenPair{}, pgtype.UUID{}, err
	}
	refreshPlain, refreshHash, err := domain.RandomToken(domain.RefreshTokenBytes)
	if err != nil {
		return domain.TokenPair{}, pgtype.UUID{}, err
	}
	refreshExp := time.Now().Add(domain.RefreshTokenTTL)
	row, err := s.repo.InsertRefreshToken(ctx, sqlcgen.InsertRefreshTokenParams{
		TokenHash: refreshHash,
		ExpiresAt: toPgTimestamp(refreshExp),
	})
	if err != nil {
		return domain.TokenPair{}, pgtype.UUID{}, err
	}
	return domain.TokenPair{
		AccessToken:           access,
		AccessTokenExpiresAt:  accessExp,
		RefreshToken:          refreshPlain,
		RefreshTokenExpiresAt: refreshExp,
		Scope:                 domain.ScopeProfileRead,
	}, row.ID, nil
}

func toPgTimestamp(t time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{Time: t.UTC(), Valid: true}
}
