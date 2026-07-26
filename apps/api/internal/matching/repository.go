package matching

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/domain"
)

// sqlcRepository adapts *sqlcgen.Queries to the Repository port, mapping
// sqlcgen database types to domain types at the port boundary.
type sqlcRepository struct {
	*sqlcgen.Queries
}

func NewSqlcRepository(q *sqlcgen.Queries) Repository {
	return sqlcRepository{Queries: q}
}

func (r sqlcRepository) GetJobByID(ctx context.Context, id pgtype.UUID) (domain.Job, error) {
	row, err := r.Queries.GetJobByID(ctx, id)
	if err != nil {
		return domain.Job{}, err
	}
	return sqlcJobToDomain(row), nil
}

func sqlcJobToDomain(j sqlcgen.Job) domain.Job {
	var postedAt *time.Time
	if j.PostedAt.Valid {
		t := j.PostedAt.Time
		postedAt = &t
	}
	return domain.Job{
		ID:              dbutil.UUIDString(j.ID),
		DedupeKey:       j.DedupeKey,
		SourceKey:       j.SourceKey,
		SubscriptionID:  dbutil.UUIDStringPtr(j.SubscriptionId),
		ExternalID:      j.ExternalId,
		Title:           j.Title,
		Company:         j.Company,
		Location:        j.Location,
		Remote:          j.Remote,
		SalaryRaw:       j.SalaryRaw,
		URL:             j.Url,
		Description:     j.Description,
		PostedAt:        postedAt,
		IngestedAt:      j.IngestedAt.Time,
		Status:          domain.JobStatus(j.Status),
		Raw:             j.Raw,
		SalaryMin:        int32PtrToIntPtr(j.SalaryMin),
		SalaryMax:        int32PtrToIntPtr(j.SalaryMax),
		SalaryCurrency:   j.SalaryCurrency,
		SalaryConfidence: j.SalaryConfidence,
		SalarySource:     j.SalarySource,
	}
}

func int32PtrToIntPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}
