package profile

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/job-finder/api/internal/generation"
)

// Entry pairs one resume bullet with its source label (position, company,
// and employment dates). It is the grounding unit the 009 fit-gap coach
// matches adjacent evidence against: every claim it shows the user must
// trace back to one of these, verbatim.
type Entry struct {
	// SourceLabel is the human-readable entry header, e.g.
	// "Senior Backend Engineer, CloudScale (2022–2024)".
	SourceLabel string
	// Bullet is the verbatim resume highlight text.
	Bullet string
}

// ProfileEntries returns the default profile's existing resume bullets,
// verbatim, each paired with its source label. It mirrors ProfileBullets
// (008-5's grounding source) but keeps the source label alongside each
// bullet instead of flattening to bare strings, since the fit-gap coach
// (009) needs the label to ground seniority and duration claims in its
// rephrase suggestions. A missing or unparseable config yields an empty
// slice (nil error), matching ProfileBullets' degrade-gracefully behavior.
func (s *Service) ProfileEntries(ctx context.Context) ([]Entry, error) {
	p, err := s.GetDefault(ctx)
	if err != nil {
		// No profile yet is not an error — there is simply nothing to cite.
		return nil, nil
	}
	if len(p.RendercvConfig) == 0 {
		return nil, nil
	}

	var master generation.RendercvMaster
	if err := json.Unmarshal(p.RendercvConfig, &master); err != nil {
		return nil, nil
	}

	sections := generation.CvSections(master)
	if sections == nil {
		return nil, nil
	}

	expRaw, ok := sections["experience"]
	if !ok {
		return nil, nil
	}

	var entries []Entry
	for _, e := range generation.AsSliceOfMaps(expRaw) {
		label := experienceLabel(e)
		for _, h := range generation.StringSliceField(e, "highlights") {
			if h = strings.TrimSpace(h); h != "" {
				entries = append(entries, Entry{SourceLabel: label, Bullet: h})
			}
		}
	}
	return entries, nil
}

// experienceLabel builds "{position}, {company} ({start}–{end})" for one
// experience entry, degrading gracefully when a field is absent. Accepts
// both RenderCV's own snake_case date keys and this repo's seed camelCase
// keys.
func experienceLabel(e map[string]any) string {
	position := generation.StringField(e, "position")
	company := generation.StringField(e, "company")

	var head string
	switch {
	case position != "" && company != "":
		head = position + ", " + company
	case company != "":
		head = company
	default:
		head = position
	}

	start := firstNonEmpty(generation.StringField(e, "start_date"), generation.StringField(e, "startDate"))
	end := firstNonEmpty(generation.StringField(e, "end_date"), generation.StringField(e, "endDate"))

	var dates string
	switch {
	case start != "" && end != "":
		dates = " (" + start + "–" + end + ")"
	case start != "":
		dates = " (" + start + "–Present)"
	}
	return head + dates
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
