// Package extauth is the facade for the browser-extension authentication
// bounded context (specs/014-autofill-extension/spec.md section 2): the
// Repository outbound port, the Signer (mints/verifies the JWT access
// token), Claims, TokenPair, and sentinel errors live in domain/; the
// bootstrap-code issuance/exchange and refresh-token rotation
// orchestration (Service) lives in application/. This file re-exports the
// shape callers already depend on (compose.go, httpapi/ext_auth.go,
// httpapi/ext_profile.go) so relocating the package required no changes at
// call sites beyond the import path.
package extauth

import (
	"github.com/job-finder/api/internal/extauth/application"
	"github.com/job-finder/api/internal/extauth/domain"
)

type (
	Repository = domain.Repository
	Signer     = domain.Signer
	Claims     = domain.Claims
	TokenPair  = domain.TokenPair

	Service = application.Service
)

const (
	Subject          = domain.Subject
	ScopeProfileRead = domain.ScopeProfileRead

	AccessTokenTTL   = domain.AccessTokenTTL
	RefreshTokenTTL  = domain.RefreshTokenTTL
	BootstrapCodeTTL = domain.BootstrapCodeTTL
)

var (
	ErrInvalidToken        = domain.ErrInvalidToken
	ErrInvalidCode         = domain.ErrInvalidCode
	ErrInvalidRefreshToken = domain.ErrInvalidRefreshToken

	NewSigner  = domain.NewSigner
	NewService = application.NewService
)
