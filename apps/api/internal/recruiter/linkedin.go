package recruiter

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/llm"
)

// linkedInPeoplePath is the public People section of a LinkedIn company
// page — the only part of LinkedIn this source ever reads. No login, no
// auth-wall defeat, no non-public data (research.md Decision 4).
const linkedInPeoplePath = "/people/"

// linkedInSource builds this job's LinkedIn resolution source. It is only
// ever placed into sources() when Service.linkedInEnabled is true
// (LINKEDIN_SCRAPE_ENABLED), so a disabled run never constructs, let alone
// invokes, this closure — zero LinkedIn requests are made (FR-004, SC-004).
// Read-only, public-page-only, paced through the same shared
// scraping.Service as every other fetch in this package. A blocked
// request or a markup change degrades this source to zero contacts with a
// logged warning, never an error that would fail the whole run (FR-015,
// spec edge case "LinkedIn markup / gating change").
func (s *Service) linkedInSource(job sqlcgen.Job) resolutionSource {
	return resolutionSource{
		name: SourceLinkedIn,
		run: func(ctx context.Context) ([]ResolvedContact, error) {
			if s.scraping == nil {
				return nil, nil
			}
			slug := companySlug(job.Company)
			if slug == "" {
				return nil, nil
			}
			pageURL := "https://www.linkedin.com/company/" + slug + linkedInPeoplePath

			html, err := s.scraping.FetchHTML(ctx, pageURL, nil)
			if err != nil {
				// Blocked / unreachable — degrade to zero, not a hard
				// failure of the whole Resolve run.
				return nil, fmt.Errorf("recruiter: linkedin fetch %s: %w", pageURL, err)
			}
			text := extractPageText(html)
			if text == "" {
				return nil, nil
			}

			return ExtractLinkedInContacts(ctx, s.llmc, s.postingModel, text)
		},
	}
}

// companySlug renders a best-effort LinkedIn company-page slug from the
// job's raw company name (lowercase, spaces collapsed to hyphens). This is
// a heuristic, not a lookup against LinkedIn's own slug — a wrong guess
// simply 404s and the source degrades to zero contacts, same as any other
// unreachable page.
func companySlug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	name = whitespaceRe.ReplaceAllString(name, "-")
	return name
}

// ExtractLinkedInContacts runs the local LLM over a LinkedIn People page's
// (already-flattened) text and returns every grounded contact, applying
// the same no-fabrication rules as the other two sources (groundContact):
// every field must occur verbatim in pageText, an ungrounded name is
// dropped, and a generic mailbox never counts as a personal channel.
// Confidence sits between the posting and company-page tiers: a LinkedIn
// People listing names a role at the company but, like the company-page
// source, doesn't confirm ownership of this specific requisition.
func ExtractLinkedInContacts(ctx context.Context, llmc llm.Provider, model string, pageText string) ([]ResolvedContact, error) {
	text := strings.TrimSpace(pageText)
	if text == "" {
		return nil, nil
	}

	truncated := text
	if len(truncated) > 4000 {
		truncated = truncated[:4000]
	}

	prompt := fmt.Sprintf(
		"Read this LinkedIn company People-section page and list every named human team member who could "+
			"plausibly own hiring for a role — a recruiter, talent acquisition specialist, HR/People team "+
			"member, or hiring manager.\n\n"+
			"Only report a person, title, email, phone, or LinkedIn URL if it is EXPLICITLY present in the text "+
			"below. Never guess or invent a name.\n\n"+
			"PAGE TEXT:\n%s\n\n"+
			"Return a single JSON object with a \"contacts\" array (empty if none).",
		truncated,
	)

	out, err := llm.CompleteStructured[extractedContactList](ctx, llmc, prompt, &llm.CompleteOptions{
		System: "You extract only what is explicitly written in the given text; you never fabricate names, titles, or contact details.",
		Model:  model,
	})
	if err != nil {
		return nil, fmt.Errorf("recruiter: linkedin extraction: %w", err)
	}

	var results []ResolvedContact
	for _, raw := range out.Contacts {
		contact, err := groundContact(raw, text, SourceLinkedIn, linkedInConfidence)
		if err != nil {
			return nil, err
		}
		if contact != nil {
			results = append(results, *contact)
		}
	}
	return results, nil
}

// linkedInConfidence sits between postingConfidence and
// companyPageConfidence.
func linkedInConfidence(_, email, phone string) float64 {
	c := 0.45
	if email != "" {
		c += 0.05
	}
	if phone != "" {
		c += 0.03
	}
	if c > 0.6 {
		c = 0.6
	}
	return c
}
