package application

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/generation/infrastructure"
)

const (
	exportRendering = "rendering"
	exportExported  = "exported"
	exportBlocked   = "blocked"
	exportFailed    = "error"
)

const exportModel = "workspace-export"

type exportOutcome struct {
	doc        domain.RendercvMaster
	pdfPath    string
	pages      int
	blocked    bool
	candidates []domain.OverflowCandidate
}

func renderExport(ctx context.Context, deps renderDeps, master domain.RendercvMaster, sections []domain.Section, cfg domain.ShapeConfig, baseName string) (exportOutcome, error) {
	doc, err := domain.Assemble(master, sections)
	if err != nil {
		return exportOutcome{}, err
	}
	domain.ApplyFontSize(doc, cfg)

	pdfPath, err := deps.render(ctx, doc, baseName)
	if err != nil {
		return exportOutcome{}, err
	}
	pages, err := deps.countPages(pdfPath)
	if err != nil {

		slog.Warn("could not count exported PDF pages, accepting the render", "path", pdfPath, "err", err)
		return exportOutcome{doc: doc, pdfPath: pdfPath}, nil
	}
	if cfg.TargetPages <= 0 || pages <= cfg.TargetPages {
		return exportOutcome{doc: doc, pdfPath: pdfPath, pages: pages}, nil
	}

	domain.CompactDesign(doc)
	compactPath, err := deps.render(ctx, doc, baseName+"-compact")
	if err != nil {
		return exportOutcome{}, err
	}
	compactPages, err := deps.countPages(compactPath)
	if err != nil {
		slog.Warn("could not count compacted PDF pages, accepting the render", "path", compactPath, "err", err)
		return exportOutcome{doc: doc, pdfPath: compactPath}, nil
	}
	if compactPages <= cfg.TargetPages {
		return exportOutcome{doc: doc, pdfPath: compactPath, pages: compactPages}, nil
	}

	return exportOutcome{
		doc:        doc,
		pdfPath:    compactPath,
		pages:      compactPages,
		blocked:    true,
		candidates: domain.OverflowCandidates(sections, compactPages-cfg.TargetPages),
	}, nil
}

func (s *Service) exportRenderDeps() renderDeps {
	if s.exportRender.render != nil {
		return s.exportRender
	}
	return renderDeps{
		render: func(ctx context.Context, doc domain.RendercvMaster, name string) (string, error) {
			_, pdfPath, err := s.rendercv.Render(ctx, doc, name)
			return pdfPath, err
		},
		countPages: infrastructure.CountPages,
	}
}

func (s *Service) SetExportRenderer(
	render func(ctx context.Context, doc domain.RendercvMaster, name string) (string, error),
	countPages func(pdfPath string) (int, error),
) {
	s.exportRender = renderDeps{render: render, countPages: countPages}
}

func (s *Service) ExportGenerationRun(ctx context.Context, runID string) (dto.GenerationExportDto, error) {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return dto.GenerationExportDto{}, apperr.NotFound("generation run", runID)
	}
	if s.tx == nil {
		return dto.GenerationExportDto{}, apperr.Internal("generation workspace: no transaction runner configured")
	}

	var (
		run      sqlcgen.GenerationRun
		sections []domain.Section
	)

	err = s.tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		fresh, gErr := q.GetRunForUpdate(ctx, rid)
		if gErr != nil {
			return apperr.NotFound("generation run", runID)
		}
		if fresh.State == string(domain.RunRunning) {
			return apperr.Conflict("generation run is running")
		}
		secRows, sErr := q.ListSectionsByRun(ctx, rid)
		if sErr != nil {
			return sErr
		}
		itemRows, iErr := q.ListItemsByRun(ctx, rid)
		if iErr != nil {
			return iErr
		}
		run = fresh
		sections = sectionsFromRows(secRows, itemRows)
		if !hasAnySelection(sections) {
			return apperr.Conflict("nothing is selected: this run has no content to export")
		}
		status := exportRendering
		return q.SetRunExport(ctx, sqlcgen.SetRunExportParams{ID: rid, ExportStatus: &status})
	})
	if err != nil {
		return dto.GenerationExportDto{}, err
	}

	var master domain.RendercvMaster
	if err := json.Unmarshal(run.MasterSnapshot, &master); err != nil {
		s.markExportFailed(ctx, rid)
		return dto.GenerationExportDto{}, err
	}
	var cfg domain.ShapeConfig
	_ = json.Unmarshal(run.ShapeConfig, &cfg)

	outcome, err := renderExport(ctx, s.exportRenderDeps(), master, sections, cfg, exportBaseName(run))
	if err != nil {
		s.markExportFailed(ctx, rid)
		return dto.GenerationExportDto{}, err
	}

	if outcome.blocked {
		return s.persistBlockedExport(ctx, rid, outcome, cfg)
	}
	return s.persistExportedDocument(ctx, rid, run, outcome)
}

func (s *Service) GetGenerationExport(ctx context.Context, runID string) (dto.GenerationExportDto, error) {
	rid, err := dbutil.ParseUUID(runID)
	if err != nil {
		return dto.GenerationExportDto{}, apperr.NotFound("generation run", runID)
	}
	run, err := s.q.GetRun(ctx, rid)
	if err != nil {
		return dto.GenerationExportDto{}, apperr.NotFound("generation run", runID)
	}
	return exportDtoFromRun(run), nil
}

func (s *Service) persistBlockedExport(ctx context.Context, rid pgtype.UUID, outcome exportOutcome, cfg domain.ShapeConfig) (dto.GenerationExportDto, error) {
	report := dto.OverflowReportDto{
		PagesRendered: outcome.pages,
		PagesTarget:   cfg.TargetPages,
		Candidates:    make([]dto.OverflowCandidateDto, 0, len(outcome.candidates)),
	}
	for _, c := range outcome.candidates {
		report.Candidates = append(report.Candidates, dto.OverflowCandidateDto{
			ItemID: c.ItemID, SectionID: c.SectionID, Label: c.Label, Rank: c.Rank,
		})
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return dto.GenerationExportDto{}, err
	}
	status := exportBlocked
	if err := s.q.SetRunExport(ctx, sqlcgen.SetRunExportParams{ID: rid, ExportStatus: &status, ExportReport: reportJSON}); err != nil {
		return dto.GenerationExportDto{}, err
	}
	return dto.GenerationExportDto{Status: exportBlocked, Report: &report}, nil
}

func (s *Service) persistExportedDocument(ctx context.Context, rid pgtype.UUID, run sqlcgen.GenerationRun, outcome exportOutcome) (dto.GenerationExportDto, error) {
	content, err := json.Marshal(outcome.doc)
	if err != nil {
		return dto.GenerationExportDto{}, err
	}
	pdfPath := outcome.pdfPath
	doc, err := s.q.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		JobId:           run.JobID,
		Type:            string(dto.DocumentTypeResume),
		Version:         s.nextExportVersion(ctx, run.JobID),
		Content:         content,
		PdfPath:         &pdfPath,
		Model:           exportModel,
		Company:         run.VacancyCompany,
		Title:           run.VacancyTitle,
		SummaryOptionId: run.SummaryOptionID,
	})
	if err != nil {
		return dto.GenerationExportDto{}, err
	}

	status := exportExported
	if err := s.q.SetRunExport(ctx, sqlcgen.SetRunExportParams{ID: rid, ExportStatus: &status, ExportDocumentID: doc.ID}); err != nil {
		return dto.GenerationExportDto{}, err
	}
	docID := dbutil.UUIDString(doc.ID)
	return dto.GenerationExportDto{Status: exportExported, DocumentID: &docID}, nil
}

func (s *Service) nextExportVersion(ctx context.Context, jobID pgtype.UUID) int32 {
	if !jobID.Valid {
		return 1
	}
	max, err := s.q.MaxDocumentVersion(ctx, sqlcgen.MaxDocumentVersionParams{JobId: jobID, Type: string(dto.DocumentTypeResume)})
	if err != nil {
		return 1
	}
	return max + 1
}

func (s *Service) markExportFailed(ctx context.Context, rid pgtype.UUID) {
	status := exportFailed
	_ = s.q.SetRunExport(ctx, sqlcgen.SetRunExportParams{ID: rid, ExportStatus: &status})
}

func exportDtoFromRun(run sqlcgen.GenerationRun) dto.GenerationExportDto {
	status := ""
	if run.ExportStatus != nil {
		status = *run.ExportStatus
	}
	var report *dto.OverflowReportDto
	if len(run.ExportReport) > 0 {
		var r dto.OverflowReportDto
		if json.Unmarshal(run.ExportReport, &r) == nil {
			report = &r
		}
	}
	return dto.GenerationExportDto{
		Status:     status,
		DocumentID: dbutil.UUIDStringPtr(run.ExportDocumentID),
		Report:     report,
	}
}

func sectionsFromRows(secRows []sqlcgen.GenerationSection, itemRows []sqlcgen.GenerationItem) []domain.Section {
	itemsBySection := make(map[pgtype.UUID][]domain.Item, len(secRows))
	for _, it := range itemRows {
		var sourceIndex *int
		if it.SourceIndex != nil {
			v := int(*it.SourceIndex)
			sourceIndex = &v
		}
		itemsBySection[it.SectionID] = append(itemsBySection[it.SectionID], domain.Item{
			ID:          dbutil.UUIDString(it.ID),
			SectionID:   dbutil.UUIDString(it.SectionID),
			Origin:      domain.ItemOrigin(it.Origin),
			SourceIndex: sourceIndex,
			SourceText:  it.SourceText,
			EditedText:  it.EditedText,
			Rank:        int(it.Rank),
			Position:    int(it.Position),
			Selected:    it.Selected,
			Unavailable: it.Unavailable,
		})
	}

	out := make([]domain.Section, 0, len(secRows))
	for _, sec := range secRows {
		out = append(out, domain.Section{
			ID:           dbutil.UUIDString(sec.ID),
			Kind:         domain.SectionKind(sec.Kind),
			EntryKey:     sec.EntryKey,
			EntryLabel:   sec.EntryLabel,
			Position:     int(sec.Position),
			TargetCount:  int(sec.TargetCount),
			State:        domain.SectionState(sec.State),
			Error:        sec.Error,
			FallbackUsed: sec.FallbackUsed,
			Enabled:      sec.Enabled,
			Items:        itemsBySection[sec.ID],
		})
	}
	return out
}

func hasAnySelection(sections []domain.Section) bool {
	for _, sec := range sections {
		for _, it := range sec.Items {
			if it.Selected && !it.Unavailable {
				return true
			}
		}
	}
	return false
}

func exportBaseName(run sqlcgen.GenerationRun) string {
	name := "vacancy-resume"
	if run.VacancyCompany != nil && *run.VacancyCompany != "" {
		name = *run.VacancyCompany
		if run.VacancyTitle != nil && *run.VacancyTitle != "" {
			name += "-" + *run.VacancyTitle
		}
	}
	return sanitize(name) + "-export-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
}
