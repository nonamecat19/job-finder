package profile

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation"
)

// ExtProfile builds the browser-extension autofill projection (spec
// 014-autofill-extension section 3) from the default profile's rendercv
// master. It never takes an id parameter — the extension's access token is
// scoped to the token subject's own profile only (there is exactly one
// profile "owner" in this single-tenant deployment; see internal/extauth),
// so "get my profile" and "get the default profile" are the same operation.
func (s *Service) ExtProfile(ctx context.Context) (dto.ExtProfileDto, error) {
	_, master, err := generation.GetDefaultMasterID(ctx, s)
	if err != nil {
		return dto.ExtProfileDto{}, fmt.Errorf("no profile configured: %w", err)
	}
	return extProfileFromMaster(master), nil
}

func extProfileFromMaster(master generation.RendercvMaster) dto.ExtProfileDto {
	out := dto.ExtProfileDto{
		Skills:      []string{},
		WorkHistory: []dto.ExtWorkEntry{},
		Education:   []dto.ExtEducation{},
		Links:       []dto.ExtLink{},
	}
	cv, _ := master["cv"].(map[string]any)
	if cv == nil {
		return out
	}
	out.FullName = generation.StringField(cv, "name")
	out.Email = generation.StringField(cv, "email")
	out.Phone = generation.StringField(cv, "phone")
	out.Location = generation.StringField(cv, "location")
	out.Headline = generation.StringField(cv, "headline")

	for _, conn := range generation.AsSliceOfMaps(cv["custom_connections"]) {
		url := generation.StringField(conn, "url")
		if url == "" {
			continue
		}
		label := generation.StringField(conn, "placeholder")
		out.Links = append(out.Links, dto.ExtLink{URL: url, Label: label})
	}
	for _, net := range generation.AsSliceOfMaps(cv["social_networks"]) {
		url := generation.StringField(net, "url")
		if url == "" {
			continue
		}
		label := generation.StringField(net, "network")
		out.Links = append(out.Links, dto.ExtLink{URL: url, Label: label})
	}

	sections := generation.CvSections(master)
	if sections == nil {
		return out
	}

	if skillsRaw, ok := sections["skills"]; ok {
		for _, g := range generation.AsSliceOfMaps(skillsRaw) {
			// "details" is a single comma-joined string in the rendercv
			// schema (see resume.yaml), not a list.
			details := generation.StringField(g, "details")
			for _, tok := range strings.Split(details, ",") {
				tok = strings.TrimSpace(tok)
				if tok != "" {
					out.Skills = append(out.Skills, tok)
				}
			}
		}
	}

	if expRaw, ok := sections["experience"]; ok {
		for _, e := range generation.AsSliceOfMaps(expRaw) {
			endDate := generation.StringField(e, "end_date")
			entry := dto.ExtWorkEntry{
				Employer:    generation.StringField(e, "company"),
				Role:        generation.StringField(e, "position"),
				StartDate:   generation.StringField(e, "start_date"),
				Current:     endDate == "" || endDate == "present",
				Description: strings.Join(generation.StringSliceField(e, "highlights"), "\n"),
			}
			if !entry.Current {
				ed := endDate
				entry.EndDate = &ed
			}
			out.WorkHistory = append(out.WorkHistory, entry)
		}
	}

	if eduRaw, ok := sections["education"]; ok {
		for _, e := range generation.AsSliceOfMaps(eduRaw) {
			degree := generation.StringField(e, "degree")
			area := generation.StringField(e, "area")
			if area != "" {
				if degree != "" {
					degree = degree + ", " + area
				} else {
					degree = area
				}
			}
			out.Education = append(out.Education, dto.ExtEducation{
				Institution: generation.StringField(e, "institution"),
				Degree:      degree,
				StartDate:   generation.StringField(e, "start_date"),
				EndDate:     generation.StringField(e, "end_date"),
			})
		}
	}

	return out
}
