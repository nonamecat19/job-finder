// Package outreach generates a short, draft-only post-apply outreach
// message addressed to a resolved contact (plan 007), grounding every
// specific claim it makes about the team or company in a stored
// company-intel signal (plan 004). Mirrors specs/012-outreach-generator.
//
// Two constitution-level hard constraints, enforced by construction rather
// than by convention:
//
//   - Grounding (Principle II): a specific claim about the team, its stack,
//     funding, size, or situation may only ever be a verbatim substring of a
//     stored CompanySignal value (see Fact/groundClaims). A claim that
//     cannot be traced this way is never presented — the draft either
//     retries or falls back to a fact-free generic opener (FR-004..FR-006,
//     FR-012).
//   - Draft-only (Principle I): this package has no send path whatsoever —
//     no SMTP client import, no auto-composed email-client link, no
//     delivery integration. GenerateDraft is a pure read-and-produce call;
//     nothing it returns is ever transmitted by the system (FR-002, FR-017).
package outreach

import "time"

// Tone is one of the defined outreach registers. Tone changes a draft's
// wording only — it never adds or removes a grounded claim (FR-010,
// assumptions.md "Tone changes wording only").
type Tone string

const (
	ToneWarm   Tone = "warm"
	ToneDirect Tone = "direct"
	ToneFormal Tone = "formal"
)

// DefaultTone is used whenever the caller selects none (FR-011).
const DefaultTone = ToneWarm

// AllTones returns the full set of offered tones, in display order.
func AllTones() []Tone {
	return []Tone{ToneWarm, ToneDirect, ToneFormal}
}

// isValidTone reports whether t is one of AllTones().
func isValidTone(t Tone) bool {
	for _, v := range AllTones() {
		if v == t {
			return true
		}
	}
	return false
}

// normalizeTone maps a raw (possibly empty or invalid) tone string to a
// valid Tone, defaulting rather than erroring (FR-011).
func normalizeTone(raw string) Tone {
	t := Tone(raw)
	if isValidTone(t) {
		return t
	}
	return DefaultTone
}

// Fact is one allowed grounding source: a single CompanySignal flattened to
// a (kind, value) pair. Facts are the *only* permitted source of a specific
// claim about the team or company (assumptions.md "Grounding source is
// plan 004 only").
type Fact struct {
	// Kind is one of the companyintel signal kinds (funding, layoffs,
	// glassdoor_rating, headcount, tech_stack).
	Kind string
	// Value is the human-readable signal value, exactly as stored — the
	// verbatim text a claim must be a substring of to count as grounded.
	Value string
}

// GroundingTrace maps one specific claim actually asserted in a draft to the
// company-intel signal that backs it (spec Key Entities: Grounding Trace,
// FR-014). The Story 3 verification view is built directly from this list.
type GroundingTrace struct {
	// Claim is the exact substring of Text this trace backs.
	Claim string
	// SignalKind is the Fact.Kind that grounds Claim.
	SignalKind string
	// SignalValue is the full Fact.Value Claim was verified against.
	SignalValue string
}

// OutreachDraft is one generated, never-sendable outreach message (spec Key
// Entities: Outreach Draft). Whether it is persisted at all is undecided by
// this package — GenerateDraft returns it fresh on every call and stores
// nothing.
type OutreachDraft struct {
	JobID string
	// ContactID/ContactName are empty when no resolved contact exists for
	// the job — the draft then uses a clearly-neutral salutation rather
	// than inventing a recipient (FR-007, edge case "no resolved contact").
	ContactID   string
	ContactName string
	Tone        Tone
	Text        string
	// GroundingTraces is always non-nil (possibly empty) so the wire shape
	// is a stable "[]", matching the JobContactDto convention.
	GroundingTraces []GroundingTrace
	GeneratedAt     time.Time
}
