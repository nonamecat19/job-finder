package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
)

type fakeSettings struct {
	current domain.SummaryOption
	err     error
}

func (f *fakeSettings) Get() domain.SummaryOption       { return f.current }
func (f *fakeSettings) Options() []domain.SummaryOption { return domain.SummaryOptions() }
func (f *fakeSettings) Update(_ context.Context, id string) (domain.SummaryOption, error) {
	if f.err != nil {
		return domain.SummaryOption{}, f.err
	}
	opt, ok := domain.LookupSummaryOption(id)
	if !ok {
		return domain.SummaryOption{}, errors.New("unknown option " + id)
	}
	f.current = opt
	return opt, nil
}
func (f *fakeSettings) Reset(context.Context) (domain.SummaryOption, error) {
	f.current = domain.DefaultSummaryOption()
	return f.current, nil
}

func mount(s SummaryModelProvider) chi.Router {
	r := chi.NewRouter()
	(&SummaryModelHandler{Settings: s}).Mount(r)
	return r
}

func decode(t *testing.T, rr *httptest.ResponseRecorder) dto.SummaryModelSettingDto {
	t.Helper()
	var body dto.SummaryModelSettingDto
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rr.Body.String())
	}
	return body
}

func TestGetReturnsTheMenuWithExactlyOneCurrentOption(t *testing.T) {
	f := &fakeSettings{current: domain.DefaultSummaryOption()}
	rr := httptest.NewRecorder()
	mount(f).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/settings/summary-model", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rr.Code)
	}
	body := decode(t, rr)
	if len(body.Options) != len(domain.SummaryOptions()) {
		t.Fatalf("returned %d options, want %d", len(body.Options), len(domain.SummaryOptions()))
	}

	current := 0
	for _, o := range body.Options {
		if o.Current {
			current++
			if o.ID != body.OptionID {
				t.Errorf("option %q is marked current but optionId says %q", o.ID, body.OptionID)
			}
		}
		if o.Label == "" || o.Description == "" || o.Cost == "" {
			t.Errorf("option %q reached the client with an empty rendered field", o.ID)
		}
	}
	if current != 1 {
		t.Errorf("%d options marked current, want exactly 1", current)
	}
}

func TestUpdateRoundTripsAValidChoice(t *testing.T) {
	f := &fakeSettings{current: domain.DefaultSummaryOption()}
	body, _ := json.Marshal(dto.UpdateSummaryModelRequestDto{OptionID: "premium"})
	req := httptest.NewRequest(http.MethodPut, "/settings/summary-model", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mount(f).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	if got := decode(t, rr).OptionID; got != "premium" {
		t.Fatalf("optionId = %q, want premium", got)
	}
	if f.current.ID != "premium" {
		t.Fatalf("service holds %q after update, want premium", f.current.ID)
	}
}

func TestUpdateRejectsAnUnknownOptionWith400(t *testing.T) {
	f := &fakeSettings{current: domain.DefaultSummaryOption()}
	body, _ := json.Marshal(dto.UpdateSummaryModelRequestDto{OptionID: "gpt-9-ultra"})
	req := httptest.NewRequest(http.MethodPut, "/settings/summary-model", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mount(f).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rr.Code)
	}
	if f.current.ID != domain.DefaultSummaryOption().ID {
		t.Fatalf("a rejected update changed the stored choice to %q", f.current.ID)
	}
}

func TestResetReturnsToTheDefault(t *testing.T) {
	premium, _ := domain.LookupSummaryOption("premium")
	f := &fakeSettings{current: premium}
	rr := httptest.NewRecorder()
	mount(f).ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/settings/summary-model", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rr.Code)
	}
	if got, want := decode(t, rr).OptionID, domain.DefaultSummaryOption().ID; got != want {
		t.Fatalf("optionId = %q, want %q", got, want)
	}
}
