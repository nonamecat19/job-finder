package jobsources

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var multiNewline = regexp.MustCompile(`\n{3,}`)

// HTMLToText strips HTML to readable plain text (descriptions from API
// sources arrive as HTML). Mirrors modules/job-sources/html.util.ts.
func HTMLToText(html string) string {
	if html == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	doc.Find("br").Each(func(_ int, sel *goquery.Selection) {
		sel.ReplaceWithHtml("\n")
	})
	doc.Find("p, li, div, h1, h2, h3, h4").Each(func(_ int, sel *goquery.Selection) {
		sel.AppendHtml("\n")
	})
	text := doc.Text()
	text = multiNewline.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
