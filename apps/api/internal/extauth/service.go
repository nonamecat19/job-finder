package extauth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Subject is the JWT `sub` claim for every extension access token. This is
// a single-tenant deployment — internal/profile has no user/account model,
// `GET /api/profiles` is unscoped, and there is no dashboard login/session
// system to derive a real user id from (checked: no auth middleware exists
// anywhere in apps/api today). A fixed subject is therefore the honest
// representation of "the one profile owner" rather than fabricating a user
// id the rest of the codebase doesn't have. GET /api/v1/ext/profile still
// checks claims.Subject == extauth.Subject before returning data (spec
// 3.4's "sub must match profile owner"), so the check is real — it just
// always resolves to the same owner today. If multi-user support is added
// later, this is the seam to swap for a real account id.
const Subject = "owner"

// ErrInvalidCode is returned when a bootstrap code is unknown, already
// used, or expired. Deliberately undifferentiated (see ErrInvalidToken).
var ErrInvalidCode = errors.New("extauth: invalid or expired bootstrap code")

// ErrInvalidRefreshToken is returned when a refresh token is unknown,
// revoked, or expired — including reuse of an already-rotated token.
var ErrInvalidRefreshToken = errors.New("extauth: invalid or expired refresh token")

// Repository is the outbound persistence port. *sqlcgen.Queries satisfies
// it structurally.
type Repository interface {
	InsertBootstrapCode(ctx context.Context, arg sqlcgen.InsertBootstrapCodeParams) (sqlcgen.ExtBootstrapCode, error)
	ConsumeBootstrapCode(ctx context.Context, codeHash string) (sqlcgen.ExtBootstrapCode, error)
	InsertRefreshToken(ctx context.Context, arg sqlcgen.InsertRefreshTokenParams) (sqlcgen.ExtRefreshToken, error)
	GetActiveRefreshToken(ctx context.Context, tokenHash string) (sqlcgen.ExtRefreshToken, error)
	RevokeRefreshToken(ctx context.Context, arg sqlcgen.RevokeRefreshTokenParams) error
}

// TokenPair is the response shape for both the exchange and refresh
// endpoints (spec 2.1/2.3).
type TokenPair struct {
	AccessToken           string
	AccessTokenExpiresAt  time.Time
	RefreshToken          string
	RefreshTokenExpiresAt time.Time
	Scope                 string
}

// Service implements bootstrap-code issuance/exchange and refresh-token
// rotation.
type Service struct {
	repo   Repository
	signer *Signer
}

func NewService(repo Repository, signer *Signer) *Service {
	return &Service{repo: repo, signer: signer}
}

// IssueBootstrapCode generates a new one-time code (spec 2.1 step 2),
// stores only its hash, and returns the plaintext for the dashboard to
// display (QR/string). The plaintext is never persisted or logged.
func (s *Service) IssueBootstrapCode(ctx context.Context) (code string, expiresAt time.Time, err error) {
	plaintext, hash, err := randomToken(bootstrapCodeBytes)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = time.Now().Add(BootstrapCodeTTL)
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
func (s *Service) ExchangeBootstrapCode(ctx context.Context, code string) (TokenPair, error) {
	if code == "" {
		return TokenPair{}, ErrInvalidCode
	}
	if _, err := s.repo.ConsumeBootstrapCode(ctx, hashToken(code)); err != nil {
		return TokenPair{}, ErrInvalidCode
	}
	pair, _, err := s.issuePair(ctx)
	return pair, err
}

// RefreshTokens rotates a refresh token: the presented token is revoked
// (linked to its replacement) and a new access + refresh pair is issued
// (spec 2.3). One-time-use rotation means a stolen-then-replayed old token
// fails GetActiveRefreshToken (already revoked), forcing re-authentication
// via the dashboard bootstrap flow rather than silently accepting a replay.
func (s *Service) RefreshTokens(ctx context.Context, refreshToken string) (TokenPair, error) {
	if refreshToken == "" {
		return TokenPair{}, ErrInvalidRefreshToken
	}
	old, err := s.repo.GetActiveRefreshToken(ctx, hashToken(refreshToken))
	if err != nil {
		return TokenPair{}, ErrInvalidRefreshToken
	}

	pair, newID, err := s.issuePair(ctx)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.repo.RevokeRefreshToken(ctx, sqlcgen.RevokeRefreshTokenParams{
		ID:          old.ID,
		RotatedToId: newID,
	}); err != nil {
		return TokenPair{}, err
	}
	return pair, nil
}

// issuePair mints an access token + a brand-new refresh token row and
// returns both in plaintext (the refresh token's plaintext is never stored
// — only its hash — so this is the one and only place it exists), plus the
// new row's id so callers can link a rotated-from token to it.
func (s *Service) issuePair(ctx context.Context) (TokenPair, pgtype.UUID, error) {
	access, accessExp, err := s.signer.Issue(Subject)
	if err != nil {
		return TokenPair{}, pgtype.UUID{}, err
	}
	refreshPlain, refreshHash, err := randomToken(refreshTokenBytes)
	if err != nil {
		return TokenPair{}, pgtype.UUID{}, err
	}
	refreshExp := time.Now().Add(RefreshTokenTTL)
	row, err := s.repo.InsertRefreshToken(ctx, sqlcgen.InsertRefreshTokenParams{
		TokenHash: refreshHash,
		ExpiresAt: toPgTimestamp(refreshExp),
	})
	if err != nil {
		return TokenPair{}, pgtype.UUID{}, err
	}
	return TokenPair{
		AccessToken:           access,
		AccessTokenExpiresAt:  accessExp,
		RefreshToken:          refreshPlain,
		RefreshTokenExpiresAt: refreshExp,
		Scope:                 ScopeProfileRead,
	}, row.ID, nil
}

func toPgTimestamp(t time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{Time: t.UTC(), Valid: true}
}
