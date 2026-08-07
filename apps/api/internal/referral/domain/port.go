package domain

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	InsertContact(ctx context.Context, arg sqlcgen.InsertContactParams) (sqlcgen.Contact, error)
	GetContactByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Contact, error)
	ListContacts(ctx context.Context) ([]sqlcgen.Contact, error)
	ListContactsByCompany(ctx context.Context, company *string) ([]sqlcgen.Contact, error)
	FindContactsByGithubUsername(ctx context.Context, githubUsername *string) ([]sqlcgen.Contact, error)
	DeleteAllContacts(ctx context.Context) (int64, error)
	InsertContactConnection(ctx context.Context, arg sqlcgen.InsertContactConnectionParams) (sqlcgen.ContactConnection, error)
	ListConnections(ctx context.Context) ([]sqlcgen.ContactConnection, error)
	GetConnectionsFrom(ctx context.Context, fromContactID pgtype.UUID) ([]sqlcgen.ContactConnection, error)
	GetConnectionsTo(ctx context.Context, toContactID pgtype.UUID) ([]sqlcgen.ContactConnection, error)
	DeleteAllConnections(ctx context.Context) (int64, error)
	FindContactsAtCompany(ctx context.Context, company *string) ([]sqlcgen.Contact, error)
}

type JobRepository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
}
