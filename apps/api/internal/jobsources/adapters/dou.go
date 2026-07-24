package adapters

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/scraping"
)

var (
	douRemoteRe = regexp.MustCompile(`(?i)\bвіддалено\b|\bremote\b|\bдистанційно\b`)
	douSalaryRe = regexp.MustCompile(`(?i)(?:від|від\s+)?(\d[\d\s]*)\s*[-–]\s*(\d[\d\s]*)\s*(?:\*?\s*(?:долар|дол\.\s*?|USD|\$|грн|UAH|€|євро))`)
)

const (
	douMaxSubscriptionPages = 50
	douDetailDefaultDelay   = 200 * time.Millisecond
)

type DouAdapter struct {
	Scraping *scraping.Service
}

func (DouAdapter) Key() string          { return "dou" }
func (DouAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (d DouAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	return true, nil
}

type DouDetailPatch struct {
	DetailHTML string
	DetailText string
	IsRemote   bool
	PostedAt   *time.Time
}

func (d DouAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL != "" {
		return d.scrapeSubscription(ctx, query.SubscriptionURL)
	}
	return nil, fmt.Errorf("dou keyword search not implemented — use subscription URL instead")
}

func (d DouAdapter) Fetch(ctx context.Context, jobURL string) (dto.NormalizedJob, error) {
	htmlStr, err := d.Scraping.FetchHTML(ctx, jobURL, nil)
	if err != nil {
		return dto.NormalizedJob{}, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return dto.NormalizedJob{}, err
	}
	return parseDouDetail(doc, jobURL), nil
}

func (d DouAdapter) FetchDetail(ctx context.Context, jobURL string, config map[string]any) (DouDetailPatch, error) {
	logger := slog.Default().With("source", "dou", "url", jobURL)

	htmlStr, err := d.Scraping.FetchHTML(ctx, jobURL, nil)
	if err != nil {
		return DouDetailPatch{}, fmt.Errorf("dou detail fetch: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return DouDetailPatch{}, fmt.Errorf("parse detail HTML: %w", err)
	}

	bodySel := doc.Find(".l-vacancy .b-typo.vacancy-section")
	if bodySel.Length() == 0 {
		bodySel = doc.Find(".b-typo, article, .text")
	}
	if bodySel.Length() == 0 {
		bodySel = doc.Find("body")
	}

	bodyHTML, _ := bodySel.First().Html()
	bodyText := bodySel.First().Text()

	isRemote := douRemoteRe.MatchString(bodyText)

	var postedAt *time.Time
	dateText := doc.Find(".l-vacancy .date").First().Text()
	dateText = strings.TrimSpace(dateText)
	if dateText != "" {
		if parsed := parseRelativeDOUDate(dateText); parsed != nil {
			postedAt = parsed
		}
	}

	logger.Debug("parsed DOU detail",
		"bodyLen", len(bodyHTML),
		"isRemote", isRemote,
		"postedAt", postedAt,
	)

	return DouDetailPatch{
		DetailHTML: bodyHTML,
		DetailText: bodyText,
		IsRemote:   isRemote,
		PostedAt:   postedAt,
	}, nil
}

func (d DouAdapter) scrapeSubscription(ctx context.Context, subURL string) ([]dto.NormalizedJob, error) {
	logger := slog.Default().With("source", "dou", "subURL", subURL)

	logger.Info("start DOU subscription scrape")

	htmlStr, err := d.Scraping.FetchHTML(ctx, subURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dou subscription fetch: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("parse subscription page: %w", err)
	}

	csrfToken := extractDOUCSRFToken(doc)
	if csrfToken == "" {
		logger.Warn("no CSRF token found on DOU page — XHR pagination will not work")
	}

	xhrURL := extractDOUXHRURL(doc, subURL)
	logger.Debug("DOU XHR endpoint", "xhrURL", xhrURL, "hasCSRF", csrfToken != "")

	allJobs := parseDouCards(doc, logger)
	logger.Info("parsed initial DOU page", "count", len(allJobs))

	seen := make(map[string]bool)
	for _, j := range allJobs {
		seen[j.URL] = true
	}

	if xhrURL != "" && csrfToken != "" {
		var lastID string
		if len(allJobs) > 0 {
			lastID = extractXHRAfterID(allJobs[len(allJobs)-1].URL)
		}

		for page := 0; page < douMaxSubscriptionPages; page++ {
			select {
			case <-ctx.Done():
				logger.Info("subscription scrape cancelled", "reason", "context", "jobs", len(allJobs))
				return allJobs, ctx.Err()
			default:
			}

			fragmentJobs, newLastID, err := d.fetchDOUXHRPage(ctx, xhrURL, csrfToken, lastID, logger)
			if err != nil {
				logger.Warn("XHR page fetch failed", "page", page, "error", err)
				break
			}

			added := 0
			for _, j := range fragmentJobs {
				if !seen[j.URL] {
					seen[j.URL] = true
					allJobs = append(allJobs, j)
					added++
				}
			}

			logger.Info("DOU XHR page", "page", page+1, "fetched", len(fragmentJobs), "added", added, "total", len(allJobs))

			if added == 0 {
				logger.Debug("no new jobs on XHR page, stopping")
				break
			}

			if newLastID == lastID {
				logger.Debug("lastID did not change, stopping to avoid loop")
				break
			}
			lastID = newLastID

			time.Sleep(500 * time.Millisecond)
		}
	}

	logger.Info("DOU subscription scrape complete", "total", len(allJobs))
	return allJobs, nil
}

func (d DouAdapter) fetchDOUXHRPage(ctx context.Context, xhrURL, csrfToken, afterID string, logger *slog.Logger) ([]dto.NormalizedJob, string, error) {
	form := url.Values{}
	form.Set("csrfmiddlewaretoken", csrfToken)
	if afterID != "" {
		form.Set("after", afterID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, xhrURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", fmt.Errorf("create XHR request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", xhrURL)

	resp, err := d.Scraping.HTTPClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("XHR POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		logger.Warn("DOU XHR non-200", "status", resp.StatusCode, "body", string(bodyBytes))
		return nil, "", fmt.Errorf("XHR returned %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", fmt.Errorf("read XHR response: %w", err)
	}

	respStr := string(respBody)
	logger.Debug("DOU XHR response", "len", len(respStr), "preview", truncatePreview(respStr, 200))

	fragmentDoc, err := goquery.NewDocumentFromReader(strings.NewReader(respStr))
	if err != nil {
		return nil, "", fmt.Errorf("parse XHR HTML fragment: %w", err)
	}

	jobs := parseDouCards(fragmentDoc, logger)

	var newLastID string
	if len(jobs) > 0 {
		newLastID = extractXHRAfterID(jobs[len(jobs)-1].URL)
	}

	return jobs, newLastID, nil
}

func parseDouCards(doc *goquery.Document, logger *slog.Logger) []dto.NormalizedJob {
	var results []dto.NormalizedJob

	doc.Find("li.l-vacancy").Each(func(i int, s *goquery.Selection) {
		titleSel := s.Find("a.vt")
		title := strings.TrimSpace(titleSel.Text())
		href, _ := titleSel.Attr("href")
		href = strings.TrimSpace(href)

		if title == "" || href == "" {
			return
		}

		company := strings.TrimSpace(s.Find("a.company").Text())
		location := strings.TrimSpace(s.Find(".cities").Text())
		salary := strings.TrimSpace(s.Find("span.salary").Text())
		shortDesc := strings.TrimSpace(jobsources.SelectionText(s.Find(".sh-info").First()))
		dateText := strings.TrimSpace(s.Find(".date").Text())

		isRemote := douRemoteRe.MatchString(location)

		var externalID *string
		if parsed, err := url.Parse(href); err == nil {
			parts := strings.Split(parsed.Path, "/")
			for i := len(parts) - 1; i >= 0; i-- {
				if id, err := strconv.Atoi(parts[i]); err == nil && id > 0 {
					s := strconv.Itoa(id)
					externalID = &s
					break
				}
			}
		}

		postedAt := ""
		if t := parseRelativeDOUDate(dateText); t != nil {
			postedAt = t.Format(time.RFC3339)
		}

		job := dto.NormalizedJob{
			SourceKey:   "dou",
			ExternalID:  externalID,
			Title:       title,
			Company:     company,
			Location:    nilIfEmpty(location),
			Remote:      isRemote,
			SalaryRaw:   nilIfEmpty(salary),
			URL:         href,
			Description: shortDesc,
			PostedAt:    nilIfEmpty(postedAt),
			Raw:         map[string]string{"dateText": dateText},
		}

		results = append(results, job)
	})

	return results
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func extractDOUCSRFToken(doc *goquery.Document) string {
	var token string
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		if idx := strings.Index(text, `window.CSRF_TOKEN = "`); idx != -1 {
			start := idx + len(`window.CSRF_TOKEN = "`)
			end := strings.Index(text[start:], `"`)
			if end > 0 {
				token = text[start : start+end]
			}
		}
	})
	return token
}

func extractDOUXHRURL(doc *goquery.Document, fallbackURL string) string {
	var xhrURL string
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		if idx := strings.Index(text, `window.XHR_VS_URL = "`); idx != -1 {
			start := idx + len(`window.XHR_VS_URL = "`)
			end := strings.Index(text[start:], `"`)
			if end > 0 {
				xhrURL = text[start : start+end]
			}
		}
	})

	if xhrURL == "" {
		parsed, err := url.Parse(fallbackURL)
		if err == nil {
			params := parsed.Query()
			xhrParams := url.Values{}
			if cat := params.Get("category"); cat != "" {
				xhrParams.Set("category", cat)
			}
			if len(xhrParams) > 0 {
				xhrURL = "https://jobs.dou.ua/vacancies/xhr-load/?" + xhrParams.Encode()
			} else {
				xhrURL = "https://jobs.dou.ua/vacancies/xhr-load/"
			}
		}
	}

	return xhrURL
}

func extractXHRAfterID(canonicalURL string) string {
	if canonicalURL == "" {
		return ""
	}
	parsed, err := url.Parse(canonicalURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(parsed.Path, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if id, err := strconv.Atoi(parts[i]); err == nil && id > 0 {
			return strconv.Itoa(id)
		}
	}
	return ""
}

func parseDouDetail(doc *goquery.Document, jobURL string) dto.NormalizedJob {
	title := strings.TrimSpace(doc.Find("h1").First().Text())

	companySel := doc.Find(".b-compinfo .l-n a").Not(".all-v")
	company := strings.TrimSpace(companySel.First().Text())
	companyURL, _ := companySel.First().Attr("href")

	location := strings.TrimSpace(doc.Find(".sh-info span.place").First().Text())

	salary := strings.TrimSpace(doc.Find(".sh-info span.salary").First().Text())

	descSel := doc.Find(".l-vacancy .b-typo.vacancy-section")
	if descSel.Length() == 0 {
		descSel = doc.Find(".b-typo, article, .text")
	}
	if descSel.Length() == 0 {
		descSel = doc.Find("body")
	}
	description := strings.TrimSpace(jobsources.SelectionText(descSel.First()))

	isRemote := douRemoteRe.MatchString(description) || douRemoteRe.MatchString(location)

	return dto.NormalizedJob{
		SourceKey:   "dou",
		Title:       title,
		Company:     company,
		Location:    nilIfEmpty(location),
		Remote:      isRemote,
		SalaryRaw:   nilIfEmpty(salary),
		URL:         jobURL,
		Description: description,
		Raw:         map[string]string{"companyURL": companyURL},
	}
}

func parseDOUSalary(s string) (*int, *int) {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.ToLower(s)

	matches := douSalaryRe.FindStringSubmatch(s)
	if len(matches) < 3 {
		return nil, nil
	}

	minStr := strings.ReplaceAll(matches[1], " ", "")
	maxStr := strings.ReplaceAll(matches[2], " ", "")

	min, err := strconv.Atoi(minStr)
	if err != nil {
		return nil, nil
	}

	max, err := strconv.Atoi(maxStr)
	if err != nil {
		return nil, nil
	}

	return &min, &max
}

func parseRelativeDOUDate(dateText string) *time.Time {
	dateText = strings.TrimSpace(dateText)
	if dateText == "" {
		return nil
	}
	dateText = strings.ToLower(dateText)

	now := time.Now()

	months := map[string]time.Month{
		"січня": time.January, "лютого": time.February, "березня": time.March,
		"квітня": time.April, "травня": time.May, "червня": time.June,
		"липня": time.July, "серпня": time.August, "вересня": time.September,
		"жовтня": time.October, "листопада": time.November, "грудня": time.December,
	}

	for monthName, month := range months {
		if strings.Contains(dateText, monthName) {
			dayStr := strings.ReplaceAll(dateText, monthName, "")
			dayStr = strings.TrimSpace(dayStr)
			day, err := strconv.Atoi(dayStr)
			if err != nil {
				return nil
			}
			t := time.Date(now.Year(), month, day, 0, 0, 0, 0, time.UTC)
			if t.After(now) {
				t = t.AddDate(-1, 0, 0)
			}
			return &t
		}
	}

	return nil
}

func truncatePreview(s string, maxRunes int) string {
	runeCount := utf8.RuneCountInString(s)
	if runeCount <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes]) + "..."
}

func strutilTruncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	cut := string(runes[:maxRunes])
	idx := strings.LastIndex(cut, "</")
	if idx > 0 {
		cut = cut[:idx]
	}
	return cut + "..."
}
