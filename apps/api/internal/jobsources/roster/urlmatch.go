// Package roster manages the employer board roster shared across the five
// ATS board vendor sources (013): registration, candidate discovery, and
// staleness — kept separate from jobsources.Service since the roster is
// shared across a vendor's many employers rather than one row per vendor.
package roster

import "regexp"

// SupportedVendors lists the board vendors this feature reads at launch
// (spec FR-002), in the order surfaced to the user in error messages.
var SupportedVendors = []string{"greenhouse", "lever", "ashby", "workable", "smartrecruiters"}

var vendorHostPatterns = map[string]*regexp.Regexp{
	"greenhouse":      regexp.MustCompile(`(?i)^https?://(?:www\.)?(?:job-)?boards\.greenhouse\.io/([a-z0-9][a-z0-9-]*)`),
	"lever":           regexp.MustCompile(`(?i)^https?://(?:jobs|www)\.lever\.co/([a-z0-9][a-z0-9-]*)`),
	"ashby":           regexp.MustCompile(`(?i)^https?://jobs\.ashbyhq\.com/([a-z0-9][a-z0-9-]*)`),
	"workable":        regexp.MustCompile(`(?i)^https?://(?:apply\.)?workable\.com/(?:[a-z]+/)?([a-z0-9][a-z0-9-]*)`),
	"smartrecruiters": regexp.MustCompile(`(?i)^https?://(?:careers|jobs)\.smartrecruiters\.com/([a-z0-9][a-zA-Z0-9-]*)`),
}

// MatchVendor recognizes a URL as belonging to one of the supported board
// vendors and extracts the employer's vendor-scoped identifier, per FR-009
// (candidate discovery) and FR-011 (paste-a-URL registration) — the single
// place vendor recognition lives, so both call sites agree.
func MatchVendor(rawURL string) (vendor, employerIdentifier string, ok bool) {
	for _, v := range SupportedVendors {
		if m := vendorHostPatterns[v].FindStringSubmatch(rawURL); m != nil {
			return v, m[1], true
		}
	}
	return "", "", false
}
