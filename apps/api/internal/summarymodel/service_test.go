package summarymodel

import (
	"context"
	"errors"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/generation/domain"
)

type fakeRepo struct {
	row     sqlcgen.SummaryModelSetting
	getErr  error
	updated []string
	updErr  error
}

func (f *fakeRepo) GetSummaryModelSetting(context.Context) (sqlcgen.SummaryModelSetting, error) {
	return f.row, f.getErr
}

func (f *fakeRepo) UpdateSummaryModelSetting(_ context.Context, optionID string) (sqlcgen.SummaryModelSetting, error) {
	if f.updErr != nil {
		return sqlcgen.SummaryModelSetting{}, f.updErr
	}
	f.updated = append(f.updated, optionID)
	f.row = sqlcgen.SummaryModelSetting{ID: "default", OptionId: optionID}
	return f.row, nil
}

func TestNewServiceLoadsTheStoredChoice(t *testing.T) {
	repo := &fakeRepo{row: sqlcgen.SummaryModelSetting{ID: "default", OptionId: "premium"}}
	svc, err := NewService(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := svc.Get().ID; got != "premium" {
		t.Fatalf("Get() = %q, want premium", got)
	}
}

func TestNewServiceFallsBackToTheDefaultWhenTheRowIsUnreadable(t *testing.T) {
	repo := &fakeRepo{getErr: errors.New("no such table")}
	svc, err := NewService(context.Background(), repo)
	if err != nil {
		t.Fatalf("NewService returned an error for an unreadable row: %v", err)
	}
	if got, want := svc.Get().ID, domain.DefaultSummaryOption().ID; got != want {
		t.Fatalf("Get() = %q, want the default %q", got, want)
	}
}

func TestNewServiceResolvesAStaleStoredOptionToTheDefault(t *testing.T) {
	repo := &fakeRepo{row: sqlcgen.SummaryModelSetting{ID: "default", OptionId: "retired-option"}}
	svc, _ := NewService(context.Background(), repo)
	if got, want := svc.Get().ID, domain.DefaultSummaryOption().ID; got != want {
		t.Fatalf("stale stored option resolved to %q, want the default %q", got, want)
	}
}

func TestUpdateStoresAndCaches(t *testing.T) {
	repo := &fakeRepo{row: sqlcgen.SummaryModelSetting{ID: "default", OptionId: "standard"}}
	svc, _ := NewService(context.Background(), repo)

	got, err := svc.Update(context.Background(), "fast")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "fast" {
		t.Fatalf("Update returned %q, want fast", got.ID)
	}
	if svc.Get().ID != "fast" {
		t.Fatalf("cache holds %q after Update, want fast", svc.Get().ID)
	}
	if len(repo.updated) != 1 || repo.updated[0] != "fast" {
		t.Fatalf("repo saw %v, want one write of fast", repo.updated)
	}
}

func TestUpdateRejectsAnUnknownOption(t *testing.T) {
	repo := &fakeRepo{row: sqlcgen.SummaryModelSetting{ID: "default", OptionId: "standard"}}
	svc, _ := NewService(context.Background(), repo)

	if _, err := svc.Update(context.Background(), "gpt-9-ultra"); err == nil {
		t.Fatal("Update accepted an option not in the catalogue")
	}
	if len(repo.updated) != 0 {
		t.Fatalf("a rejected update still wrote %v", repo.updated)
	}
	if svc.Get().ID != "standard" {
		t.Fatalf("a rejected update disturbed the cache: %q", svc.Get().ID)
	}
}

func TestResetReturnsToTheCatalogueDefault(t *testing.T) {
	repo := &fakeRepo{row: sqlcgen.SummaryModelSetting{ID: "default", OptionId: "premium"}}
	svc, _ := NewService(context.Background(), repo)

	got, err := svc.Reset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := domain.DefaultSummaryOption().ID; got.ID != want {
		t.Fatalf("Reset() = %q, want %q", got.ID, want)
	}
}

func TestServiceSatisfiesTheGenerationPort(t *testing.T) {
	var _ interface {
		SummaryOption(ctx context.Context) domain.SummaryOption
	} = (*Service)(nil)
}
