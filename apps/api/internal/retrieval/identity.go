package retrieval

import (
	"fmt"
	"strings"
)

type BrowserIdentity struct {
	Version      string
	UserAgent    string
	Platform     string
	Headers      [][2]string
	TLSProfileID string
}

func NewBrowserIdentity(version string) (*BrowserIdentity, error) {
	switch version {
	case "chrome126":
		id := chrome126Identity
		return &id, nil
	default:
		return nil, fmt.Errorf("retrieval: unknown BrowserIdentity version %q", version)
	}
}

var chrome126Identity = BrowserIdentity{
	Version:   "chrome126",
	UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	Platform:  "Linux x86_64",
	Headers: [][2]string{
		{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		{"Accept-Language", "en-US,en;q=0.9"},
		{"Accept-Encoding", "gzip, deflate, br"},
		{"Sec-Ch-Ua", `"Not/A)Brand";v="99", "Google Chrome";v="126", "Chromium";v="126"`},
		{"Sec-Ch-Ua-Mobile", "?0"},
		{"Sec-Ch-Ua-Platform", `"Linux"`},
		{"Sec-Fetch-Site", "none"},
		{"Sec-Fetch-Mode", "navigate"},
		{"Sec-Fetch-Dest", "document"},
		{"User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"},
		{"Upgrade-Insecure-Requests", "1"},
	},
	TLSProfileID: "chrome_124",
}

var Chrome126UserAgent = chrome126Identity.UserAgent

func (b *BrowserIdentity) Validate() error {
	if b.UserAgent == "" {
		return fmt.Errorf("retrieval: BrowserIdentity.UserAgent must not be empty")
	}
	if b.TLSProfileID == "" {
		return fmt.Errorf("retrieval: BrowserIdentity.TLSProfileID must not be empty")
	}
	uaVersion := extractBrowserVersion(b.UserAgent)
	tlsVersion := extractProfileVersion(b.TLSProfileID)
	if uaVersion != "" && tlsVersion != "" && uaVersion != tlsVersion {
		return fmt.Errorf("retrieval: BrowserIdentity UA version %q does not match TLS profile version %q", uaVersion, tlsVersion)
	}
	return nil
}

func extractBrowserVersion(ua string) string {
	idx := strings.LastIndex(ua, "Chrome/")
	if idx == -1 {
		return ""
	}
	rest := ua[idx+len("Chrome/"):]
	dot := strings.IndexByte(rest, '.')
	space := strings.IndexByte(rest, ' ')
	end := len(rest)
	if dot != -1 && dot < end {
		end = dot
	}
	if space != -1 && space < end {
		end = space
	}
	if end == 0 {
		return ""
	}
	return rest[:end]
}

func extractProfileVersion(tlsProfileID string) string {
	lower := strings.ToLower(tlsProfileID)
	parts := strings.SplitN(lower, "_", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}
