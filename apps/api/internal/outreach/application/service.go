package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/outreach/domain"
)

type (
	Tone           = domain.Tone
	Fact           = domain.Fact
	GroundingTrace = domain.GroundingTrace
	OutreachDraft  = domain.OutreachDraft
)

const (
	ToneWarm   = domain.ToneWarm
	ToneDirect = domain.ToneDirect
	ToneFormal = domain.ToneFormal

	DefaultTone = domain.DefaultTone

	MaxDraftChars = domain.MaxDraftChars
)

var (
	AllTones           = domain.AllTones
	ErrContactNotFound = domain.ErrContactNotFound
	ErrContactRequired = domain.ErrContactRequired
)

const maxDraftChars = MaxDraftChars

const maxAttempts = 3

func normalizeTone(raw string) Tone { return domain.NormalizeTone(raw) }

func genericOpener(tone Tone, contactName, companyName string) string {
	return domain.GenericOpener(tone, contactName, companyName)
}

func enforceLength(text string, traces []GroundingTrace, max int) (string, []GroundingTrace) {
	return domain.EnforceLength(text, traces, max)
}

type ContactsProvider interface {
	ListContacts(ctx context.Context, jobID string) ([]dto.JobContactDto, error)
}

type IntelProvider interface {
	GetIntel(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error)
}

type Service struct {
	contacts ContactsProvider
	intel    IntelProvider
	drafter  Drafter
}

func NewService(contacts ContactsProvider, intel IntelProvider, drafter Drafter) *Service {
	return &Service{contacts: contacts, intel: intel, drafter: drafter}
}

func (s *Service) Tones() []dto.OutreachToneOptionDto {
	labels := map[Tone]string{
		ToneWarm:   "Warm",
		ToneDirect: "Direct",
		ToneFormal: "Formal",
	}
	out := make([]dto.OutreachToneOptionDto, 0, len(AllTones()))
	for _, t := range AllTones() {
		out = append(out, dto.OutreachToneOptionDto{
			Value:   string(t),
			Label:   labels[t],
			Default: t == DefaultTone,
		})
	}
	return out
}

func (s *Service) GenerateDraft(ctx context.Context, jobID, contactID, rawTone string) (dto.OutreachDraftDto, error) {
	t := normalizeTone(rawTone)

	contacts, err := s.contacts.ListContacts(ctx, jobID)
	if err != nil {
		return dto.OutreachDraftDto{}, fmt.Errorf("outreach: list contacts: %w", err)
	}
	contact, err := pickContact(contacts, contactID)
	if err != nil {
		return dto.OutreachDraftDto{}, err
	}

	intel, err := s.intel.GetIntel(ctx, jobID)
	if err != nil {
		return dto.OutreachDraftDto{}, fmt.Errorf("outreach: get company intel: %w", err)
	}

	contactName := ""
	if contact != nil {
		contactName = contact.Name
	}
	companyName := ""
	if intel != nil {
		companyName = intel.CompanyName
	}

	facts := factsFrom(intel)

	var text string
	var traces []GroundingTrace
	if len(facts) == 0 {
		text = genericOpener(t, contactName, companyName)
		traces = []GroundingTrace{}
	} else {
		text, traces = s.generateGrounded(ctx, t, contactName, companyName, facts)
	}

	text, traces = enforceLength(text, traces, maxDraftChars)

	draft := OutreachDraft{
		JobID:           jobID,
		Tone:            t,
		Text:            text,
		GroundingTraces: traces,
		GeneratedAt:     time.Now().UTC(),
	}
	if contact != nil {
		draft.ContactID = contact.ID
		draft.ContactName = contact.Name
	}

	return draft.ToDto(), nil
}

func pickContact(contacts []dto.JobContactDto, contactID string) (*dto.JobContactDto, error) {
	if contactID != "" {
		for i := range contacts {
			if contacts[i].ID == contactID {
				c := contacts[i]
				return &c, nil
			}
		}
		return nil, ErrContactNotFound
	}
	switch len(contacts) {
	case 0:
		return nil, nil
	case 1:
		c := contacts[0]
		return &c, nil
	default:
		return nil, ErrContactRequired
	}
}

func factsFrom(intel *dto.CompanyIntelDto) []Fact {
	if intel == nil {
		return nil
	}
	var out []Fact
	if intel.Funding != nil && strings.TrimSpace(*intel.Funding) != "" {
		out = append(out, Fact{Kind: "funding", Value: strings.TrimSpace(*intel.Funding)})
	}
	if intel.Layoffs != nil && strings.TrimSpace(*intel.Layoffs) != "" {
		out = append(out, Fact{Kind: "layoffs", Value: strings.TrimSpace(*intel.Layoffs)})
	}
	if intel.GlassdoorRating != nil {
		out = append(out, Fact{Kind: "glassdoor_rating", Value: fmt.Sprintf("%.1f Glassdoor rating", *intel.GlassdoorRating)})
	}
	if intel.Headcount != nil && strings.TrimSpace(*intel.Headcount) != "" {
		out = append(out, Fact{Kind: "headcount", Value: strings.TrimSpace(*intel.Headcount)})
	}
	if intel.TechStack != nil && strings.TrimSpace(*intel.TechStack) != "" {
		out = append(out, Fact{Kind: "tech_stack", Value: strings.TrimSpace(*intel.TechStack)})
	}
	return out
}
