package outreach

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/platform/llm"
)

// maxDraftChars is the hard length ceiling (FR-009): outreach is a brief
// opener a busy recruiter reads in one glance, not a cover letter.
const maxDraftChars = 500

// maxAttempts bounds the grounded-generation retry loop (grounding
// violation or over-length), mirroring coach.generateGroundedRephrase's
// retry-then-fallback shape. After maxAttempts a compliant deterministic
// generic opener is used instead — never a fabricated or over-length draft.
const maxAttempts = 3

// ErrContactNotFound is returned when a caller passes a contactID that does
// not match any resolved contact for the job.
var ErrContactNotFound = errors.New("outreach: contact not found for job")

// ErrContactRequired is returned when more than one contact is resolved for
// the job and the caller did not choose one (FR-008): the draft must
// address exactly one recipient, never silently merge or guess among them.
var ErrContactRequired = errors.New("outreach: multiple contacts resolved — choose one")

// ContactsProvider is the outbound port onto the resolved contacts a draft
// may address (plan 007). *recruiter.Service satisfies it structurally.
type ContactsProvider interface {
	ListContacts(ctx context.Context, jobID string) ([]dto.JobContactDto, error)
}

// IntelProvider is the outbound port onto the stored company-intel signals
// that are the sole grounding source (plan 004). *companyintel.Service
// satisfies it structurally.
type IntelProvider interface {
	GetIntel(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error)
}

// Service generates outreach drafts. It never persists anything and never
// transmits anything — see the package doc's Principle I note.
type Service struct {
	contacts ContactsProvider
	intel    IntelProvider
	llmc     llm.Provider
	model    string
}

func NewService(contacts ContactsProvider, intel IntelProvider, llmc llm.Provider, model string) *Service {
	return &Service{contacts: contacts, intel: intel, llmc: llmc, model: model}
}

// Tones returns the offered tone options for the HTTP layer (FR-010,
// FR-011): a defined set plus a marked default.
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

// GenerateDraft builds one outreach draft for jobID, addressed to
// contactID (or the job's sole resolved contact when contactID is empty and
// exactly one exists), written in tone (or DefaultTone when tone is empty
// or unrecognized). Every specific claim it contains traces to a stored
// company-intel signal (FR-004); when no signal exists, or grounded
// generation cannot be produced within maxAttempts, it falls back to an
// honest generic opener with zero specific claims (FR-005, FR-012).
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
		// No signals for this company at all: a specific claim is
		// impossible to ground, so skip the LLM entirely and use the
		// deterministic, guaranteed-honest opener (FR-012, SC-003).
		text = genericOpener(t, contactName, companyName)
		traces = []GroundingTrace{}
	} else if s.llmc != nil {
		text, traces = s.generateGrounded(ctx, t, contactName, companyName, facts)
	} else {
		text = genericOpener(t, contactName, companyName)
		traces = []GroundingTrace{}
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

// pickContact resolves which JobContactDto (if any) the draft addresses.
// Never invents a contact (FR-007): the only inputs are the actually
// resolved rows for the job.
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

// factsFrom flattens the non-nil CompanyIntelDto fields into the ordered
// Fact list a draft is permitted to draw specific claims from.
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

// ToDto flattens an OutreachDraft into its wire shape.
func (d *OutreachDraft) ToDto() dto.OutreachDraftDto {
	traces := make([]dto.GroundingTraceDto, 0, len(d.GroundingTraces))
	for _, tr := range d.GroundingTraces {
		traces = append(traces, dto.GroundingTraceDto{
			Claim:       tr.Claim,
			SignalKind:  tr.SignalKind,
			SignalValue: tr.SignalValue,
		})
	}
	out := dto.OutreachDraftDto{
		JobID:           d.JobID,
		Tone:            string(d.Tone),
		Text:            d.Text,
		GroundingTraces: traces,
		GeneratedAt:     d.GeneratedAt.Format(time.RFC3339),
	}
	if d.ContactID != "" {
		id := d.ContactID
		out.ContactID = &id
	}
	if d.ContactName != "" {
		name := d.ContactName
		out.ContactName = &name
	}
	return out
}
