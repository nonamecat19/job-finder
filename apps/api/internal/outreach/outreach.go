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
