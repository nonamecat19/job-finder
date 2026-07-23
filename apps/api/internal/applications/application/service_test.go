package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/applications/application"
	"github.com/job-finder/api/internal/applications/domain"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
)

// fakeRepo embeds the Repository port; the test overrides only what it needs.
type fakeRepo struct {
	domain.Repository
	rows    []sqlcgen.ListApplicationsRow
	listErr error
}

func (f *fakeRepo) ListApplications(ctx context.Context, status *string) ([]sqlcgen.ListApplicationsRow, error) {
	return f.rows, f.listErr
}

func TestListEmpty(t *testing.T) {
	svc := application.NewService(&fakeRepo{})
	out, err := svc.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
	// non-nil empty slice (JSON serializes as [], not null)
	if out == nil {
		t.Error("expected non-nil empty slice")
	}
	var _ []dto.ApplicationDto = out
}

func TestListError(t *testing.T) {
	svc := application.NewService(&fakeRepo{listErr: errors.New("db down")})
	if _, err := svc.List(context.Background(), nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --------------- outcome capture on status change ---------------

const testAppID = "11111111-1111-1111-1111-111111111111"

// fakeWriteRepo records the writes Update performs so the outcome-capture path
// can be asserted without a database.
type fakeWriteRepo struct {
	domain.Repository
	existing sqlcgen.Application

	outcomes    []sqlcgen.InsertApplicationOutcomeParams
	outcomeErr  error
	updateParam sqlcgen.UpdateApplicationParams
	jobStatuses []string
}

func (f *fakeWriteRepo) GetApplicationByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Application, error) {
	f.existing.ID = id
	return f.existing, nil
}

func (f *fakeWriteRepo) UpdateApplication(ctx context.Context, arg sqlcgen.UpdateApplicationParams) (sqlcgen.Application, error) {
	f.updateParam = arg
	out := f.existing
	if arg.Status != nil {
		out.Status = *arg.Status
	}
	if arg.AppliedAt.Valid {
		out.AppliedAt = arg.AppliedAt
	}
	out.Events = arg.Events
	return out, nil
}

func (f *fakeWriteRepo) InsertApplicationOutcome(ctx context.Context, arg sqlcgen.InsertApplicationOutcomeParams) (sqlcgen.ApplicationOutcome, error) {
	if f.outcomeErr != nil {
		return sqlcgen.ApplicationOutcome{}, f.outcomeErr
	}
	f.outcomes = append(f.outcomes, arg)
	return sqlcgen.ApplicationOutcome{ApplicationId: arg.ApplicationId, EventType: arg.EventType, OccurredAt: arg.OccurredAt}, nil
}

func (f *fakeWriteRepo) UpdateJobStatus(ctx context.Context, arg sqlcgen.UpdateJobStatusParams) (sqlcgen.Job, error) {
	f.jobStatuses = append(f.jobStatuses, arg.Status)
	return sqlcgen.Job{}, nil
}

func (f *fakeWriteRepo) GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	return sqlcgen.Job{}, errors.New("no job")
}

func newWriteRepo(status string) *fakeWriteRepo {
	return &fakeWriteRepo{existing: sqlcgen.Application{Status: status, Events: []byte(`[]`)}}
}

func statusPtr(s dto.ApplicationStatus) *dto.ApplicationStatus { return &s }

// TestUpdateRecordsOutcomeEvent covers the status→event mapping: submission and
// post-submission transitions record an outcome, pre-submission ones do not.
func TestUpdateRecordsOutcomeEvent(t *testing.T) {
	tests := []struct {
		name      string
		from      string
		to        dto.ApplicationStatus
		wantEvent dto.OutcomeEventType // "" = no event recorded
	}{
		{"applied records applied", "shortlisted", dto.StatusApplied, dto.OutcomeApplied},
		{"interview records screen", "applied", dto.StatusInterview, dto.OutcomeScreen},
		{"offer records offer", "interview", dto.StatusOffer, dto.OutcomeOffer},
		{"rejected records rejected", "applied", dto.StatusRejected, dto.OutcomeRejected},
		{"shortlisted records nothing", "found", dto.StatusShortlisted, ""},
		{"docs_generated records nothing", "shortlisted", dto.StatusDocsGenerated, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newWriteRepo(tt.from)
			svc := application.NewService(repo)
			if _, err := svc.Update(context.Background(), testAppID, application.UpdateInput{Status: statusPtr(tt.to)}); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if tt.wantEvent == "" {
				if len(repo.outcomes) != 0 {
					t.Fatalf("expected no outcome event, got %d", len(repo.outcomes))
				}
				return
			}
			if len(repo.outcomes) != 1 {
				t.Fatalf("expected 1 outcome event, got %d", len(repo.outcomes))
			}
			if got := repo.outcomes[0].EventType; got != string(tt.wantEvent) {
				t.Fatalf("eventType = %s, want %s", got, tt.wantEvent)
			}
			if !repo.outcomes[0].OccurredAt.Valid {
				t.Fatal("expected occurredAt to be set on the event")
			}
		})
	}
}

// TestUpdateAppliedAtMatchesEventTimestamp locks in the invariant the post-age
// signal depends on: "appliedAt" is the same instant as the `applied` event.
func TestUpdateAppliedAtMatchesEventTimestamp(t *testing.T) {
	repo := newWriteRepo("shortlisted")
	svc := application.NewService(repo)
	if _, err := svc.Update(context.Background(), testAppID, application.UpdateInput{Status: statusPtr(dto.StatusApplied)}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !repo.updateParam.AppliedAt.Valid {
		t.Fatal("expected appliedAt to be set on the applied transition")
	}
	if len(repo.outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(repo.outcomes))
	}
	if !repo.updateParam.AppliedAt.Time.Equal(repo.outcomes[0].OccurredAt.Time) {
		t.Fatalf("appliedAt %v != event occurredAt %v", repo.updateParam.AppliedAt.Time, repo.outcomes[0].OccurredAt.Time)
	}
}

// TestUpdateNoStatusChangeRecordsNothing: a no-op transition (same status) must
// not append to the log, and a notes-only edit must not either.
func TestUpdateNoStatusChangeRecordsNothing(t *testing.T) {
	notes := "just a note"
	for _, tt := range []struct {
		name string
		in   application.UpdateInput
	}{
		{"same status resubmitted", application.UpdateInput{Status: statusPtr(dto.StatusApplied)}},
		{"notes only", application.UpdateInput{Notes: func() **string { p := &notes; return &p }()}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newWriteRepo("applied")
			svc := application.NewService(repo)
			if _, err := svc.Update(context.Background(), testAppID, tt.in); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if len(repo.outcomes) != 0 {
				t.Fatalf("expected no outcome event, got %d", len(repo.outcomes))
			}
		})
	}
}

// TestUpdateDuplicateTerminalEventIsNotAnError: the partial unique index drops a
// duplicate terminal-once event and returns no row; Update must treat that as
// the specified idempotent no-op rather than failing the request.
func TestUpdateDuplicateTerminalEventIsNotAnError(t *testing.T) {
	repo := newWriteRepo("interview")
	repo.outcomeErr = pgx.ErrNoRows
	svc := application.NewService(repo)
	out, err := svc.Update(context.Background(), testAppID, application.UpdateInput{Status: statusPtr(dto.StatusOffer)})
	if err != nil {
		t.Fatalf("expected duplicate terminal event to be a no-op, got %v", err)
	}
	if out.Status != dto.StatusOffer {
		t.Fatalf("status = %s, want offer — the current-state write must still apply", out.Status)
	}
}

// TestUpdateOutcomeWriteErrorFails: a real insert failure must not be swallowed.
func TestUpdateOutcomeWriteErrorFails(t *testing.T) {
	repo := newWriteRepo("shortlisted")
	repo.outcomeErr = errors.New("db down")
	svc := application.NewService(repo)
	if _, err := svc.Update(context.Background(), testAppID, application.UpdateInput{Status: statusPtr(dto.StatusApplied)}); err == nil {
		t.Fatal("expected outcome insert failure to fail the update")
	}
}

// --------------- timeline read-back ---------------

type fakeTimelineRepo struct {
	domain.Repository
	rows []sqlcgen.ApplicationOutcome
	err  error
}

func (f *fakeTimelineRepo) ListApplicationOutcomes(ctx context.Context, id pgtype.UUID) ([]sqlcgen.ApplicationOutcome, error) {
	return f.rows, f.err
}

func TestTimeline(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	repo := &fakeTimelineRepo{rows: []sqlcgen.ApplicationOutcome{
		{EventType: "applied", OccurredAt: pgtype.Timestamp{Time: base, Valid: true}, RecordedAt: pgtype.Timestamp{Time: base, Valid: true}},
		{EventType: "screen", OccurredAt: pgtype.Timestamp{Time: base.AddDate(0, 0, 4), Valid: true}, RecordedAt: pgtype.Timestamp{Time: base, Valid: true}},
	}}
	out, err := application.NewService(repo).Timeline(context.Background(), testAppID)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].EventType != dto.OutcomeApplied || out[1].EventType != dto.OutcomeScreen {
		t.Fatalf("unexpected event order: %s, %s", out[0].EventType, out[1].EventType)
	}
	if out[0].OccurredAt == "" || out[0].RecordedAt == "" {
		t.Fatal("expected occurredAt and recordedAt to be rendered")
	}
}

func TestTimelineEmpty(t *testing.T) {
	out, err := application.NewService(&fakeTimelineRepo{}).Timeline(context.Background(), testAppID)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	// non-nil empty slice so JSON serializes as [], not null
	if out == nil || len(out) != 0 {
		t.Fatalf("expected non-nil empty slice, got %v", out)
	}
}

func TestTimelineInvalidID(t *testing.T) {
	if _, err := application.NewService(&fakeTimelineRepo{}).Timeline(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("expected error for malformed application id")
	}
}
