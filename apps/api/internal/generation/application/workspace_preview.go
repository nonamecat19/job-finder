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

// PreviewDocument is `GET /v1/generations/{runId}/preview-document`
// (046-real-resume-preview): the run's current selection assembled into
// RenderCV YAML, exactly as renderExport assembles it before calling
// ApplyFontSize and render — but stopping there. No Typst/PDF compilation
// happens here; the dashboard's in-browser WASM pipeline does that from the
// returned YAML.
//
// Unlike ExportGenerationRun, this is a pure read: no row lock, no export
// status transition, and — deliberately — no hasAnySelection refusal. A user
// mid-edit with nothing selected yet should still see *something* render
// (an empty document), not a 409; that refusal exists for export because
// shipping a caller a document with no content is a mistake worth blocking,
// but a live preview reflecting "you haven't picked anything yet" is
// correct, not an error (spec Edge Case: empty resume / no sections
// selected).
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

// previewDocumentFromMaster is PreviewDocument's DB-free core: the same
// domain.Assemble + domain.PrepareMasterForMarshal + yaml.Marshal sequence
// renderExport runs before ApplyFontSize and render. Split out so a unit test
// can assert byte-for-byte parity with the export path's assembly without a
// database (quickstart.md Scenario 1).
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

// sectionsHash is a content hash of the run's current section-selection
// state, so the client can skip a redundant WASM re-render when consecutive
// edits resolve to the same effective document (research.md Decision 3).
func sectionsHash(sections []domain.Section) string {
	// json.Marshal of []domain.Section is deterministic field order (struct,
	// not a map) and slice order is already the caller's `position` order, so
	// two calls over the same selection state always hash identically.
	b, err := json.Marshal(sections)
	if err != nil {
		// Unreachable for this concrete, marshalable type; a hash that can't
		// be computed must not be silently treated as "unchanged".
		return ""
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
