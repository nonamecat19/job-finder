package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/extauth/domain"
)

// fakeRepo is an in-memory Repository for testing without a database,
// following the fakeXProvider pattern used across internal/httpapi tests.
type fakeRepo struct {
	bootstrapCodes map[string]domain.ExtBootstrapCode
	refreshTokens  map[string]domain.ExtRefreshToken
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		bootstrapCodes: map[string]domain.ExtBootstrapCode{},
		refreshTokens:  map[string]domain.ExtRefreshToken{},
	}
}

func newUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}

func (f *fakeRepo) InsertBootstrapCode(ctx context.Context, arg domain.InsertBootstrapCodeParams) (domain.ExtBootstrapCode, error) {
	row := domain.ExtBootstrapCode{ID: newUUID(), CodeHash: arg.CodeHash, ExpiresAt: arg.ExpiresAt}
	f.bootstrapCodes[arg.CodeHash] = row
	return row, nil
}

func (f *fakeRepo) ConsumeBootstrapCode(ctx context.Context, codeHash string) (domain.ExtBootstrapCode, error) {
	row, ok := f.bootstrapCodes[codeHash]
	if !ok {
		return domain.ExtBootstrapCode{}, errNotFound
	}
	if row.UsedAt.Valid {
		return domain.ExtBootstrapCode{}, errNotFound
	}
	if row.ExpiresAt.Before(time.Now()) {
		return domain.ExtBootstrapCode{}, errNotFound
	}
	row.UsedAt = pgtype.Timestamp{Time: time.Now(), Valid: true}
	f.bootstrapCodes[codeHash] = row
	return row, nil
}

func (f *fakeRepo) InsertRefreshToken(ctx context.Context, arg domain.InsertRefreshTokenParams) (domain.ExtRefreshToken, error) {
	row := domain.ExtRefreshToken{ID: newUUID(), TokenHash: arg.TokenHash, ExpiresAt: arg.ExpiresAt}
	f.refreshTokens[arg.TokenHash] = row
	return row, nil
}

func (f *fakeRepo) GetActiveRefreshToken(ctx context.Context, tokenHash string) (domain.ExtRefreshToken, error) {
	row, ok := f.refreshTokens[tokenHash]
	if !ok || row.RevokedAt.Valid || row.ExpiresAt.Before(time.Now()) {
		return domain.ExtRefreshToken{}, errNotFound
	}
	return row, nil
}

func (f *fakeRepo) RevokeRefreshToken(ctx context.Context, arg domain.RevokeRefreshTokenParams) error {
	for hash, row := range f.refreshTokens {
		if row.ID == arg.ID {
			row.RevokedAt = pgtype.Timestamp{Time: time.Now(), Valid: true}
			row.RotatedToId = arg.RotatedToId
			f.refreshTokens[hash] = row
			return nil
		}
	}
	return errNotFound
}

type notFoundErr struct{}

func (notFoundErr) Error() string { return "not found" }

var errNotFound = notFoundErr{}

func newTestService() (*Service, *fakeRepo) {
	repo := newFakeRepo()
	svc := NewService(repo, domain.NewSigner([]byte("test-secret-32-bytes-of-padding")))
	return svc, repo
}

func TestExchangeBootstrapCode_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	code, _, err := svc.IssueBootstrapCode(ctx)
	if err != nil {
		t.Fatalf("IssueBootstrapCode: %v", err)
	}

	pair, err := svc.ExchangeBootstrapCode(ctx, code)
	if err != nil {
		t.Fatalf("ExchangeBootstrapCode: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected non-empty tokens")
	}
	if pair.Scope != domain.ScopeProfileRead {
		t.Errorf("Scope = %q, want %q", pair.Scope, domain.ScopeProfileRead)
	}
	if len(repo.bootstrapCodes) != 1 {
		t.Fatalf("expected 1 stored bootstrap code, got %d", len(repo.bootstrapCodes))
	}
}

func TestExchangeBootstrapCode_ReuseFails(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	code, _, err := svc.IssueBootstrapCode(ctx)
	if err != nil {
		t.Fatalf("IssueBootstrapCode: %v", err)
	}
	if _, err := svc.ExchangeBootstrapCode(ctx, code); err != nil {
		t.Fatalf("first exchange: %v", err)
	}
	if _, err := svc.ExchangeBootstrapCode(ctx, code); err != domain.ErrInvalidCode {
		t.Errorf("second exchange (reuse) = %v, want domain.ErrInvalidCode", err)
	}
}

func TestExchangeBootstrapCode_UnknownCode(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.ExchangeBootstrapCode(context.Background(), "bogus"); err != domain.ErrInvalidCode {
		t.Errorf("got %v, want domain.ErrInvalidCode", err)
	}
}

func TestExchangeBootstrapCode_Empty(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.ExchangeBootstrapCode(context.Background(), ""); err != domain.ErrInvalidCode {
		t.Errorf("got %v, want domain.ErrInvalidCode", err)
	}
}

func TestRefreshTokens_RotatesAndInvalidatesOld(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	code, _, _ := svc.IssueBootstrapCode(ctx)
	pair, err := svc.ExchangeBootstrapCode(ctx, code)
	if err != nil {
		t.Fatalf("ExchangeBootstrapCode: %v", err)
	}

	newPair, err := svc.RefreshTokens(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshTokens: %v", err)
	}
	if newPair.RefreshToken == pair.RefreshToken {
		t.Error("expected a rotated (different) refresh token")
	}

	// The original refresh token must now be rejected (one-time-use).
	if _, err := svc.RefreshTokens(ctx, pair.RefreshToken); err != domain.ErrInvalidRefreshToken {
		t.Errorf("reuse of rotated-out token = %v, want domain.ErrInvalidRefreshToken", err)
	}

	// The new refresh token must still work.
	if _, err := svc.RefreshTokens(ctx, newPair.RefreshToken); err != nil {
		t.Errorf("refresh with the newest token failed: %v", err)
	}
}

func TestRefreshTokens_UnknownToken(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.RefreshTokens(context.Background(), "bogus"); err != domain.ErrInvalidRefreshToken {
		t.Errorf("got %v, want domain.ErrInvalidRefreshToken", err)
	}
}
