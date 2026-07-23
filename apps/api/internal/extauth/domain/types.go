// Package domain holds the browser-extension authentication scheme's core
// model from specs/014-autofill-extension/spec.md section 2: a
// dashboard-issued, short-lived bootstrap code exchanged once for a token
// pair (short-lived JWT access token + rotating opaque refresh token), plus
// the Signer that mints/verifies the access token and the Repository
// outbound port.
package domain

import (
	"errors"
	"time"
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

// TokenPair is the response shape for both the exchange and refresh
// endpoints (spec 2.1/2.3).
type TokenPair struct {
	AccessToken           string
	AccessTokenExpiresAt  time.Time
	RefreshToken          string
	RefreshTokenExpiresAt time.Time
	Scope                 string
}
