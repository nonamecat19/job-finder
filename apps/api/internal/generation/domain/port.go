package domain

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	activity.Store

	GetDocumentByID(ctx context.Context, id pgtype.UUID) (sqlcgen.GeneratedDocument, error)
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	InsertGeneratedDocument(ctx context.Context, arg sqlcgen.InsertGeneratedDocumentParams) (sqlcgen.GeneratedDocument, error)
	ListAdHocDocuments(ctx context.Context) ([]sqlcgen.GeneratedDocument, error)
	ListDocumentsForJob(ctx context.Context, jobid pgtype.UUID) ([]sqlcgen.GeneratedDocument, error)
	MaxDocumentVersion(ctx context.Context, arg sqlcgen.MaxDocumentVersionParams) (int32, error)
	UpdateDocumentContent(ctx context.Context, arg sqlcgen.UpdateDocumentContentParams) (sqlcgen.GeneratedDocument, error)
	UpdateJobStatus(ctx context.Context, arg sqlcgen.UpdateJobStatusParams) (sqlcgen.Job, error)
	UpsertApplicationStatus(ctx context.Context, arg sqlcgen.UpsertApplicationStatusParams) error

	// 042: the resume generation workspace's own tables. Grouped separately
	// from the document methods above only by comment, not by interface — one
	// Repository keeps `s.q` a single dependency, matching every other method
	// on this port.
	CreateRun(ctx context.Context, arg sqlcgen.CreateRunParams) (sqlcgen.GenerationRun, error)
	GetRun(ctx context.Context, id pgtype.UUID) (sqlcgen.GenerationRun, error)
	GetRunForUpdate(ctx context.Context, id pgtype.UUID) (sqlcgen.GenerationRun, error)
	ListRunsByProfile(ctx context.Context, arg sqlcgen.ListRunsByProfileParams) ([]sqlcgen.GenerationRun, error)
	SetRunState(ctx context.Context, arg sqlcgen.SetRunStateParams) error
	SetRunAnalysis(ctx context.Context, arg sqlcgen.SetRunAnalysisParams) error
	SetRunExport(ctx context.Context, arg sqlcgen.SetRunExportParams) error
	DeleteRun(ctx context.Context, id pgtype.UUID) error
	CreateSections(ctx context.Context, arg sqlcgen.CreateSectionsParams) ([]sqlcgen.GenerationSection, error)
	ListSectionsByRun(ctx context.Context, runID pgtype.UUID) ([]sqlcgen.GenerationSection, error)
	SetSectionState(ctx context.Context, arg sqlcgen.SetSectionStateParams) error
	DeleteSectionItems(ctx context.Context, sectionID pgtype.UUID) error
	CreateItems(ctx context.Context, arg sqlcgen.CreateItemsParams) ([]sqlcgen.GenerationItem, error)
	ListItemsByRun(ctx context.Context, runID pgtype.UUID) ([]sqlcgen.GenerationItem, error)
	GetItemForUpdate(ctx context.Context, id pgtype.UUID) (sqlcgen.GenerationItem, error)
	UpdateItemSelection(ctx context.Context, arg sqlcgen.UpdateItemSelectionParams) error
	UpdateItemText(ctx context.Context, arg sqlcgen.UpdateItemTextParams) error
	ReorderSectionItems(ctx context.Context, arg sqlcgen.ReorderSectionItemsParams) error
	MarkItemsUnavailable(ctx context.Context, itemIds []pgtype.UUID) error
}

// TxRunner runs a function inside one database transaction, giving it a
// *sqlcgen.Queries bound to that transaction. Item/section mutations
// (PatchGenerationItem, ReorderSection) need this rather than the plain
// Repository above: rest-api.md requires "every write takes a row-level
// SELECT ... FOR UPDATE on the run first", and a row lock only serializes
// concurrent requests when the lock and the write it guards share one
// transaction. *db.DB satisfies this directly (internal/db/db.go), the same
// shape 020/manualadd's domain.TxRunner already established.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}
