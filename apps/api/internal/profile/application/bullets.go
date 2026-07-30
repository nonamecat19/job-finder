package application

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/job-finder/api/internal/generation"
)

// ProfileBullets returns the default profile's existing resume bullet lines,
// verbatim, drawn from the experience.highlights of the stored RenderCV master
// config. These are the grounding source for the keyword-diff rephrase
// suggester (008-5): it may only reframe experience already present here, never
// invent new claims.
//
// It satisfies the keyword.BulletsProvider port structurally. A missing or
// unparseable config yields an empty slice (nil error) so the diff endpoint
// degrades to "no suggestions" rather than failing.
func (s *Service) ProfileBullets(ctx context.Context) ([]string, error) {
	p, err := s.GetDefault(ctx)
	if err != nil {
		// No profile yet is not an error for the advisory suggester — it simply
		// means there is nothing to reframe.
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

	var bullets []string
	if expRaw, ok := sections["experience"]; ok {
		for _, e := range generation.AsSliceOfMaps(expRaw) {
			for _, h := range generation.StringSliceField(e, "highlights") {
				if h = strings.TrimSpace(h); h != "" {
					bullets = append(bullets, h)
				}
			}
		}
	}
	return bullets, nil
}
