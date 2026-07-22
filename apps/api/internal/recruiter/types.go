// Package recruiter resolves the likely recruiter or hiring manager who
// owns a job requisition, from up to three sources:
//
//   - posting (always on): the job's own description text, parsed with the
//     local LLM. Requires no external fetch and no opt-in.
//   - company-page (needs Company.website from plan 004): the company's
//     own team/about page, fetched via internal/companyintel's reusable
//     fetch and parsed for named contacts.
//   - linkedin (opt-in, default off): the public LinkedIn company page's
//     People section. Scraping LinkedIn is a ToS gray area (plan.md
//     Constitution Check), so this source only runs when the operator sets
//     LINKEDIN_SCRAPE_ENABLED=true; when unset it is silently skipped —
//     never invoked, never a failed run.
//
// This package is strictly read-only (Constitution Principle I): it never
// sends a message, email, connection request, or application to a resolved
// contact. Outreach is a separate, out-of-scope feature. Every field a
// source emits must trace to text actually observed in that source
// (Constitution Principle II, FR-008) — a generic mailbox with no named
// person is never surfaced as a human (FR-007), and a contact with no name
// is dropped rather than stored with an invented placeholder.
package recruiter

// ResolvedContact is the in-process shape emitted by each resolution
// source before it is upserted into JobContact. Field shape mirrors the
// JobContact columns minus the DB-managed ones (id, fetchedAt) — see
// data-model.md. A ResolvedContact with an empty Name is never persisted.
type ResolvedContact struct {
	Name        string
	Title       *string
	LinkedInURL *string
	Email       *string
	Phone       *string
	// Source is one of "posting", "company-page", "linkedin".
	Source string
	// Confidence is producer-assigned, in [0,1]. Higher means the source
	// more directly names the person as owning the req (an explicit
	// "Contact:" line in the posting ranks above a name scraped from a
	// team page).
	Confidence float64
}

// Source name constants — the only three valid values for
// ResolvedContact.Source / JobContact.source.
const (
	SourcePosting     = "posting"
	SourceCompanyPage = "company-page"
	SourceLinkedIn    = "linkedin"
)
