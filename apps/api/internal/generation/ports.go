package generation

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port for the generation use-case.
// *sqlcgen.Queries satisfies it structurally. It embeds activity.Store because
// the handler resumes an activity record from the same value (activity.FromID).
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
}
