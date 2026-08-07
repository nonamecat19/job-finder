package roster

import "regexp"

var SupportedVendors = []string{"greenhouse", "lever", "ashby", "workable", "smartrecruiters"}

var vendorHostPatterns = map[string]*regexp.Regexp{
	"greenhouse":      regexp.MustCompile(`(?i)^https?://(?:www\.)?(?:job-)?boards\.greenhouse\.io/([a-z0-9][a-z0-9-]*)`),
	"lever":           regexp.MustCompile(`(?i)^https?://(?:jobs|www)\.lever\.co/([a-z0-9][a-z0-9-]*)`),
	"ashby":           regexp.MustCompile(`(?i)^https?://jobs\.ashbyhq\.com/([a-z0-9][a-z0-9-]*)`),
	"workable":        regexp.MustCompile(`(?i)^https?://(?:apply\.)?workable\.com/(?:[a-z]+/)?([a-z0-9][a-z0-9-]*)`),
	"smartrecruiters": regexp.MustCompile(`(?i)^https?://(?:careers|jobs)\.smartrecruiters\.com/([a-z0-9][a-zA-Z0-9-]*)`),
}

func MatchVendor(rawURL string) (vendor, employerIdentifier string, ok bool) {
	for _, v := range SupportedVendors {
		if m := vendorHostPatterns[v].FindStringSubmatch(rawURL); m != nil {
			return v, m[1], true
		}
	}
	return "", "", false
}
