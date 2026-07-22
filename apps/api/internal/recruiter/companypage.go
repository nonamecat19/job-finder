package recruiter

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/llm"
)

// companyPagePaths mirrors companyintel's HeadcountScraper About-page
// pattern (research.md Decision 3: reuse plan 004's fetch, don't fork it)
// — tried in order until one page yields at least one grounded contact.
var companyPagePaths = []string{"/about", "/team", "/about-us", "/company"}

// extractedContactList is the LLM structured-output shape for the
// (possibly multi-person) company-page source.
type extractedContactList struct {
	Contacts []extractedContact `json:"contacts" jsonschema:"description=Every named human team member on this page who could plausibly own hiring for a role: a recruiter, talent acquisition, HR/People team member, or hiring manager. Empty list if none. Never invent a person not present in the text."`
}

// companyPageSource builds this job's company-page resolution source
// (US2). It looks up the Company row plan 004 populates (keyed by the
// job's normalized company name) for its website, then tries a small set
// of common About/Team page paths via the shared scraping.Service — the
// same fetch abstraction companyintel's HeadcountScraper uses — and
// LLM-parses each page's text for named contacts. A missing website, an
// unreachable page, or a page with no People/Team section all degrade to
// zero contacts, never an error (spec edge case "Company page has no
// People/Team section").
func (s *Service) companyPageSource(job sqlcgen.Job) resolutionSource {
	return resolutionSource{
		name: SourceCompanyPage,
		run: func(ctx context.Context) ([]ResolvedContact, error) {
			if s.scraping == nil {
				return nil, nil
			}
			website, err := s.companyWebsite(ctx, job)
			if err != nil {
				return nil, err
			}
			if website == "" {
				return nil, nil // no known website yet — deliberate skip, not a failure
			}

			for _, path := range companyPagePaths {
				pageURL, err := joinURL(website, path)
				if err != nil {
					continue
				}
				html, err := s.scraping.FetchHTML(ctx, pageURL, nil)
				if err != nil {
					continue // unreachable path — try the next one
				}
				text := extractPageText(html)
				if text == "" {
					continue
				}
				contacts, err := ExtractCompanyPageContacts(ctx, s.llmc, s.postingModel, text)
				if err != nil {
					return nil, err
				}
				if len(contacts) > 0 {
					return contacts, nil
				}
			}
			return nil, nil
		},
	}
}

// companyWebsite resolves the job's company to a website via plan 004's
// Company row, mirroring companyintel's own normalizeCompanyName join key.
// Returns "" (not an error) when the company has never been probed by
// plan 004, or has no website recorded yet.
func (s *Service) companyWebsite(ctx context.Context, job sqlcgen.Job) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(job.Company))
	if normalized == "" {
		return "", nil
	}
	company, err := s.q.GetCompanyByNormalizedName(ctx, normalized)
	if err != nil {
		return "", nil //nolint:nilerr // "company never probed" is not a source failure
	}
	if company.Website == nil {
		return "", nil
	}
	return *company.Website, nil
}

func joinURL(website, path string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(website))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("companypage: invalid website %q", website)
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

var whitespaceRe = regexp.MustCompile(`\s+`)

// extractPageText collapses an HTML page to plain, whitespace-normalized
// text for the LLM prompt and for grounding checks.
func extractPageText(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(whitespaceRe.ReplaceAllString(doc.Text(), " "))
}

// ExtractCompanyPageContacts runs the local LLM over a company page's
// (already-flattened) text and returns every grounded contact found,
// applying the same no-fabrication rules as posting.go's
// ExtractPostingContact: every field must occur verbatim in pageText, a
// contact without a grounded name is dropped, and a generic mailbox never
// counts as a personal channel. Confidence is lower than the posting
// source's ceiling — a team-page listing is a weaker signal than an
// explicit "Contact:" line naming who owns this specific req.
func ExtractCompanyPageContacts(ctx context.Context, llmc llm.Provider, model string, pageText string) ([]ResolvedContact, error) {
	text := strings.TrimSpace(pageText)
	if text == "" {
		return nil, nil
	}

	truncated := text
	if len(truncated) > 4000 {
		truncated = truncated[:4000]
	}

	prompt := fmt.Sprintf(
		"Read this company About/Team page and list every named human team member who could plausibly own "+
			"hiring for a role — a recruiter, talent acquisition specialist, HR/People team member, or hiring "+
			"manager.\n\n"+
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
		return nil, fmt.Errorf("recruiter: company-page extraction: %w", err)
	}

	var results []ResolvedContact
	for _, raw := range out.Contacts {
		contact, err := groundContact(raw, text, SourceCompanyPage, companyPageConfidence)
		if err != nil {
			return nil, err
		}
		if contact != nil {
			results = append(results, *contact)
		}
	}
	return results, nil
}

// companyPageConfidence caps well below the posting source's ceiling: a
// name on a general team page is a weaker "owns this req" signal than an
// explicit contact line in the posting itself.
func companyPageConfidence(_, email, phone string) float64 {
	c := 0.35
	if email != "" {
		c += 0.05
	}
	if phone != "" {
		c += 0.03
	}
	if c > 0.5 {
		c = 0.5
	}
	return c
}
