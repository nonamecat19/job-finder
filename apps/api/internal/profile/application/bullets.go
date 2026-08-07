package application

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/job-finder/api/internal/generation"
)

func (s *Service) ProfileBullets(ctx context.Context) ([]string, error) {
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
