package generation

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/scraping"
	"github.com/job-finder/api/internal/platform/storage"
)

//go:embed templates/*.html
var templatesFS embed.FS

// mmToInches converts a CSS millimeter margin to the inches PrintToPDF expects.
func mmToInches(mm float64) float64 { return mm / 25.4 }

// HtmlPdfRenderer renders JSON-Resume data through an html/template, then
// prints the resulting page to PDF via a shared headless Chromium instance
// (chromedp), mirroring pdf-renderer.ts (which used Handlebars + Playwright's
// page.pdf()).
type HtmlPdfRenderer struct {
	scraping  *scraping.Service
	outDir    string
	resumeTpl *template.Template
	letterTpl *template.Template
	// Store, when set, receives every rendered PDF so all resume files are
	// persisted to object storage (MinIO) in addition to the local outDir.
	Store storage.Blobstore
}

func NewHtmlPdfRenderer(scrapingSvc *scraping.Service, outDir string) (*HtmlPdfRenderer, error) {
	if outDir == "" {
		outDir = "/data/documents"
	}
	resumeTpl, err := template.ParseFS(templatesFS, "templates/resume.html")
	if err != nil {
		return nil, fmt.Errorf("generation: parse resume template: %w", err)
	}
	letterTpl, err := template.ParseFS(templatesFS, "templates/cover-letter.html")
	if err != nil {
		return nil, fmt.Errorf("generation: parse cover-letter template: %w", err)
	}
	return &HtmlPdfRenderer{scraping: scrapingSvc, outDir: outDir, resumeTpl: resumeTpl, letterTpl: letterTpl}, nil
}

// resumeView flattens dto.JsonResume's pointer fields into plain values so
// {{if .Field}} in the template behaves like Handlebars' {{#if}} (falsy for
// nil/empty), matching resume.hbs's conditionals field-for-field.
type resumeView struct {
	Name      string
	Label     string
	Email     string
	Phone     string
	URL       string
	Location  string
	Summary   string
	Skills    []skillView
	Work      []workView
	Projects  []projectView
	Education []eduView
	Languages []langView
}

type skillView struct {
	Name     string
	Keywords []string
}
type workView struct {
	Position, Name, StartDate, EndDate, Summary string
	Highlights                                  []string
}
type projectView struct {
	Name, Description, URL string
	Highlights             []string
}
type eduView struct {
	Institution, StudyType, Area, StartDate, EndDate string
}
type langView struct {
	Language, Fluency string
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toResumeView(r dto.JsonResume) resumeView {
	v := resumeView{}
	if r.Basics != nil {
		v.Name = deref(r.Basics.Name)
		v.Label = deref(r.Basics.Label)
		v.Email = deref(r.Basics.Email)
		v.Phone = deref(r.Basics.Phone)
		v.URL = deref(r.Basics.URL)
		v.Summary = deref(r.Basics.Summary)
		if r.Basics.Location != nil {
			city := deref(r.Basics.Location.City)
			cc := deref(r.Basics.Location.CountryCode)
			if city != "" && cc != "" {
				v.Location = city + ", " + cc
			} else {
				v.Location = city
			}
		}
	}
	for _, s := range r.Skills {
		v.Skills = append(v.Skills, skillView{Name: s.Name, Keywords: s.Keywords})
	}
	for _, w := range r.Work {
		v.Work = append(v.Work, workView{
			Position: deref(w.Position), Name: w.Name, StartDate: deref(w.StartDate),
			EndDate: deref(w.EndDate), Summary: deref(w.Summary), Highlights: w.Highlights,
		})
	}
	for _, p := range r.Projects {
		v.Projects = append(v.Projects, projectView{
			Name: p.Name, Description: deref(p.Description), URL: deref(p.URL), Highlights: p.Highlights,
		})
	}
	for _, e := range r.Education {
		v.Education = append(v.Education, eduView{
			Institution: e.Institution, StudyType: deref(e.StudyType), Area: deref(e.Area),
			StartDate: deref(e.StartDate), EndDate: deref(e.EndDate),
		})
	}
	for _, l := range r.Languages {
		v.Languages = append(v.Languages, langView{Language: deref(l.Language), Fluency: deref(l.Fluency)})
	}
	return v
}

func (r *HtmlPdfRenderer) RenderResume(ctx context.Context, resume dto.JsonResume, outName string) (string, error) {
	var buf bytes.Buffer
	if err := r.resumeTpl.Execute(&buf, toResumeView(resume)); err != nil {
		return "", err
	}
	return r.htmlToPDF(ctx, buf.String(), outName)
}

type coverLetterView struct {
	Name, Company, Title string
	Paragraphs           []string
}

func (r *HtmlPdfRenderer) RenderCoverLetter(ctx context.Context, text string, name *string, company, title string, outName string) (string, error) {
	paragraphs := splitParagraphs(text)
	var buf bytes.Buffer
	if err := r.letterTpl.Execute(&buf, coverLetterView{Name: deref(name), Company: company, Title: title, Paragraphs: paragraphs}); err != nil {
		return "", err
	}
	return r.htmlToPDF(ctx, buf.String(), outName)
}

func (r *HtmlPdfRenderer) htmlToPDF(ctx context.Context, html string, outName string) (string, error) {
	outDir, err := ensureOutDir(r.outDir)
	if err != nil {
		return "", err
	}
	outPath := filepath.Join(outDir, outName)

	browserCtx, err := r.scraping.BrowserContext(ctx)
	if err != nil {
		return "", err
	}
	tabCtx, cancel := chromedp.NewContext(browserCtx)
	defer cancel()

	var pdfBuf []byte
	err = chromedp.Run(tabCtx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(tree.Frame.ID, html).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// TS used page.pdf({ format: 'A4', ... }); CDP's PrintToPDF has no
			// named-format shortcut and defaults to US Letter (8.5x11in), so
			// the paper size must be set explicitly to match (A4 = 210x297mm).
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(mmToInches(210)).
				WithPaperHeight(mmToInches(297)).
				WithMarginTop(mmToInches(14)).
				WithMarginBottom(mmToInches(14)).
				WithMarginLeft(mmToInches(14)).
				WithMarginRight(mmToInches(14)).
				Do(ctx)
			if err != nil {
				return err
			}
			pdfBuf = buf
			return nil
		}),
	)
	if err != nil {
		return "", fmt.Errorf("generation: render pdf: %w", err)
	}
	if err := os.WriteFile(outPath, pdfBuf, 0o644); err != nil {
		return "", err
	}
	if r.Store != nil {
		if err := r.Store.Upload(ctx, filepath.Base(outPath), outPath, "application/pdf"); err != nil {
			return "", err
		}
	}
	return outPath, nil
}

var paragraphSplitRe = regexp.MustCompile(`\n{2,}|\n`)

// splitParagraphs mirrors `text.split(/\n{2,}|\n/).map(trim).filter(Boolean)`.
func splitParagraphs(text string) []string {
	var out []string
	for _, p := range paragraphSplitRe.Split(text, -1) {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
