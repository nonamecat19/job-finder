package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
)

func (s *Service) PreviewDocument(ctx context.Context, runID string) (dto.PreviewDocumentDto, error) {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return dto.PreviewDocumentDto{}, apperr.NotFound("generation run", runID)
	}

	run, err := s.q.GetRun(ctx, rid)
	if err != nil {
		return dto.PreviewDocumentDto{}, apperr.NotFound("generation run", runID)
	}

	secRows, err := s.q.ListSectionsByRun(ctx, rid)
	if err != nil {
		return dto.PreviewDocumentDto{}, err
	}
	itemRows, err := s.q.ListItemsByRun(ctx, rid)
	if err != nil {
		return dto.PreviewDocumentDto{}, err
	}
	sections := sectionsFromRows(secRows, itemRows)

	var master domain.RendercvMaster
	if err := json.Unmarshal(run.MasterSnapshot, &master); err != nil {
		return dto.PreviewDocumentDto{}, apperr.Internal("preview-document: decode master snapshot: " + err.Error())
	}
	if master == nil {
		return dto.PreviewDocumentDto{}, apperr.Precondition("profile has no RenderCV config — upload one first")
	}

	return previewDocumentFromMaster(master, sections)
}

func previewDocumentFromMaster(master domain.RendercvMaster, sections []domain.Section) (dto.PreviewDocumentDto, error) {
	doc, err := domain.Assemble(master, sections)
	if err != nil {
		return dto.PreviewDocumentDto{}, err
	}

	ordered, err := domain.PrepareMasterForMarshal(doc)
	if err != nil {
		return dto.PreviewDocumentDto{}, err
	}
	yamlBytes, err := yaml.Marshal(map[string]any(ordered))
	if err != nil {
		return dto.PreviewDocumentDto{}, err
	}

	return dto.PreviewDocumentDto{
		Yaml:         string(yamlBytes),
		SectionsHash: sectionsHash(sections),
	}, nil
}

func sectionsHash(sections []domain.Section) string {

	b, err := json.Marshal(sections)
	if err != nil {

		return ""
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
