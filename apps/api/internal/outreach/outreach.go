// Package outreach is the facade for the Outreach bounded context (plan
// 012): the Tone/Fact/GroundingTrace/OutreachDraft model and the pure
// grounding/prompt-building rules live in domain/, the cross-context ports
// (ContactsProvider, IntelProvider) and the LLM-backed draft orchestration
// live in application/. This file re-exports the shape callers already
// depend on (compose.go, httpapi/outreach.go) so relocating the package
// required no changes at call sites beyond the import path.
package outreach

import (
	"github.com/job-finder/api/internal/outreach/application"
	"github.com/job-finder/api/internal/outreach/domain"
)

type (
	Tone           = domain.Tone
	Fact           = domain.Fact
	GroundingTrace = domain.GroundingTrace
	OutreachDraft  = domain.OutreachDraft

	ContactsProvider = application.ContactsProvider
	IntelProvider    = application.IntelProvider
	Service          = application.Service
)

const (
	ToneWarm   = domain.ToneWarm
	ToneDirect = domain.ToneDirect
	ToneFormal = domain.ToneFormal

	DefaultTone = domain.DefaultTone
)

var (
	ErrContactNotFound = domain.ErrContactNotFound
	ErrContactRequired = domain.ErrContactRequired

	AllTones = domain.AllTones

	NewService = application.NewService
)
