package application

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/profile/domain"
)

func (s *Service) ProfileEntries(ctx context.Context) ([]domain.Entry, error) {
	p, err := s.GetDefault(ctx)
	if err != nil {
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

	var entries []domain.Entry
	for _, e := range generation.AsSliceOfMaps(expRaw) {
		label := experienceLabel(e)
		for _, h := range generation.StringSliceField(e, "highlights") {
			if h = strings.TrimSpace(h); h != "" {
				entries = append(entries, domain.Entry{SourceLabel: label, Bullet: h})
			}
		}
	}
	return entries, nil
}

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
