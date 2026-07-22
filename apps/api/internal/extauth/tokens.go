// Package extauth implements the browser-extension authentication scheme
// from specs/014-autofill-extension/spec.md section 2: a dashboard-issued,
// short-lived bootstrap code is exchanged once for a token pair (short-lived
// JWT access token + rotating opaque refresh token). The extension never
// receives or stores a long-lived secret, and the access token is scoped to
// `profile:read` only — it cannot be used to mutate any account data.
package extauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ScopeProfileRead is the only scope ever granted to an extension access
// token (spec 2.2). Handlers MUST check this before returning profile data.
const ScopeProfileRead = "profile:read"

// AccessTokenTTL / RefreshTokenTTL / BootstrapCodeTTL match spec 2.2 exactly.
const (
	AccessTokenTTL     = time.Hour
	RefreshTokenTTL    = 30 * 24 * time.Hour
	BootstrapCodeTTL   = 5 * time.Minute
	refreshTokenBytes  = 32
	bootstrapCodeBytes = 24
)

// Claims is the JWT payload for the extension access token. Scope is a
// custom claim (spec 2.2): "The access token is scoped to profile:read via
// a custom claim". Subject identifies the profile owner (see Subject below).
type Claims struct {
	Scope string `json:"scope"`
	jwt.RegisteredClaims
}

// ErrInvalidToken is returned for any access-token verification failure —
// bad signature, expired, wrong scope, malformed. Deliberately
// undifferentiated so handlers can't be coaxed into leaking which check
// failed.
var ErrInvalidToken = errors.New("extauth: invalid or expired access token")

// Signer issues and verifies extension access tokens with HMAC-SHA256.
type Signer struct {
	secret []byte
}

// NewSigner builds a Signer from a secret byte slice. Callers should use a
// cryptographically random secret of at least 32 bytes (config.ExtJWTSecret,
// hex-decoded, or an ephemeral fallback — see cmd/server).
func NewSigner(secret []byte) *Signer {
	return &Signer{secret: secret}
}

// Issue mints a new access token for subject (the profile owner), scoped to
// profile:read, valid for AccessTokenTTL.
func (s *Signer) Issue(subject string) (token string, expiresAt time.Time, err error) {
	expiresAt = time.Now().Add(AccessTokenTTL)
	claims := Claims{
		Scope: ScopeProfileRead,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// Verify parses and validates token, enforcing both signature/expiry and the
// profile:read scope. It returns ErrInvalidToken for every failure mode.
func (s *Signer) Verify(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Scope != ScopeProfileRead {
		return nil, ErrInvalidToken
	}
	if claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// randomToken returns a URL-safe base64 string decoded from n cryptographically
// random bytes, plus the SHA-256 hex hash used as the at-rest storage key.
// Only the hash is ever persisted — see migration 00017_ext_auth.sql.
func randomToken(n int) (plaintext, hash string, err error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	hash = hashToken(plaintext)
	return plaintext, hash, nil
}

// hashToken is the storage-side hash for bootstrap codes and refresh
// tokens: sha256 hex, so a leaked DB row cannot be replayed as the
// plaintext credential.
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
