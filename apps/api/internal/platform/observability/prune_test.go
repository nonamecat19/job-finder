package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeCollector is a stand-in for the collector's public API holding records
// with timestamps, so retention is proven by deletion rather than by reading
// configuration (036 C7-4).
type fakeCollector struct {
	records map[string]time.Time
	listErr int // when non-zero, the listing endpoint returns this status
	delErr  int // when non-zero, the delete endpoint returns this status
	deletes int
}

func newFakeCollector(records map[string]time.Time) *fakeCollector {
	return &fakeCollector{records: records}
}

func (f *fakeCollector) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/public/traces", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user == "" || pass == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if f.listErr != 0 {
				w.WriteHeader(f.listErr)
				_, _ = w.Write([]byte(`{"message":"boom"}`))
				return
			}
			raw := r.URL.Query().Get("toTimestamp")
			cutoff, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				t.Errorf("listing called with unparseable toTimestamp %q: %v", raw, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			type entry struct {
				ID string `json:"id"`
			}
			limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
			if err != nil || limit <= 0 {
				t.Errorf("listing called without a usable limit: %q", r.URL.Query().Get("limit"))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var out []entry
			for id, ts := range f.records {
				if len(out) == limit {
					break
				}
				if ts.Before(cutoff) {
					out = append(out, entry{ID: id})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": out})
		case http.MethodDelete:
			if f.delErr != 0 {
				w.WriteHeader(f.delErr)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
				return
			}
			var body struct {
				TraceIDs []string `json:"traceIds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.deletes++
			for _, id := range body.TraceIDs {
				delete(f.records, id)
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func newTestPruner(t *testing.T, f *fakeCollector, retentionDays int) *Pruner {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return New(Config{
		BaseURL:       srv.URL,
		PublicKey:     "pk-test",
		SecretKey:     "sk-test",
		RetentionDays: retentionDays,
	}, srv.Client())
}

// 036 SC-008 / C7-4: the window is enforced by deletion. A backdated record is
// gone after a run; a recent one is untouched.
func TestPruneDeletesOnlyRecordsOlderThanTheWindow(t *testing.T) {
	now := time.Now().UTC()
	f := newFakeCollector(map[string]time.Time{
		"ancient": now.Add(-90 * 24 * time.Hour),
		"stale":   now.Add(-31 * 24 * time.Hour),
		"fresh":   now.Add(-29 * 24 * time.Hour),
		"today":   now.Add(-time.Hour),
	})
	p := newTestPruner(t, f, 30)

	report, err := p.Prune(context.Background())
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if report.Skipped {
		t.Fatal("run reported as skipped despite a configured collector")
	}
	if report.Deleted != 2 {
		t.Errorf("deleted %d records, want 2 (the 90-day and 31-day ones)", report.Deleted)
	}
	for _, id := range []string{"ancient", "stale"} {
		if _, still := f.records[id]; still {
			t.Errorf("record %q survived the window; the retention guarantee is not enforced", id)
		}
	}
	for _, id := range []string{"fresh", "today"} {
		if _, still := f.records[id]; !still {
			t.Errorf("record %q was deleted while still inside the window", id)
		}
	}
}

// The window is configurable, and the configured value is the one that decides
// what goes — not the default.
func TestPruneHonoursTheConfiguredWindow(t *testing.T) {
	now := time.Now().UTC()
	f := newFakeCollector(map[string]time.Time{
		"eight-days": now.Add(-8 * 24 * time.Hour),
		"six-days":   now.Add(-6 * 24 * time.Hour),
	})
	p := newTestPruner(t, f, 7)

	report, err := p.Prune(context.Background())
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if report.Deleted != 1 {
		t.Fatalf("deleted %d records under a 7-day window, want 1", report.Deleted)
	}
	if _, still := f.records["eight-days"]; still {
		t.Error("record older than the configured 7-day window survived")
	}
	if _, still := f.records["six-days"]; !still {
		t.Error("record inside the configured 7-day window was deleted")
	}
}

// An unset window is 30 days, so a deployment that configures nothing still
// gets the guarantee the documentation states (FR-008).
func TestPruneDefaultsToThirtyDays(t *testing.T) {
	now := time.Now().UTC()
	f := newFakeCollector(map[string]time.Time{
		"thirty-one": now.Add(-31 * 24 * time.Hour),
		"twenty-two": now.Add(-22 * 24 * time.Hour),
	})
	p := newTestPruner(t, f, 0)

	report, err := p.Prune(context.Background())
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if report.Deleted != 1 {
		t.Fatalf("deleted %d with an unset window, want 1 under the 30-day default", report.Deleted)
	}
	if _, still := f.records["thirty-one"]; still {
		t.Error("31-day-old record survived the default window")
	}
}

// 036 FR-008a: a failure must be visible. A pruning job that returns nil on a
// broken collector leaves the documented window in place while data piles up.
func TestPruneSurfacesAListingFailure(t *testing.T) {
	f := newFakeCollector(map[string]time.Time{"old": time.Now().Add(-90 * 24 * time.Hour)})
	f.listErr = http.StatusInternalServerError
	p := newTestPruner(t, f, 30)

	_, err := p.Prune(context.Background())
	if err == nil {
		t.Fatal("Prune returned nil on a collector that failed the listing; the failure would be invisible")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not carry the collector's status; diagnosing it would need a packet capture", err)
	}
}

func TestPruneSurfacesADeleteFailure(t *testing.T) {
	f := newFakeCollector(map[string]time.Time{"old": time.Now().Add(-90 * 24 * time.Hour)})
	f.delErr = http.StatusForbidden
	p := newTestPruner(t, f, 30)

	report, err := p.Prune(context.Background())
	if err == nil {
		t.Fatal("Prune returned nil when the collector refused the delete")
	}
	if report.Deleted != 0 {
		t.Errorf("report claims %d deletions after a refused delete", report.Deleted)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not name the refusal", err)
	}
}

// An unconfigured collector is not an error state. Most deployments never turn
// collection on, and a scheduled tick must not log a failure every five minutes
// for a feature nobody enabled (036 C2-3).
func TestPruneSkipsWhenTheCollectorIsNotConfigured(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no url":         {PublicKey: "pk", SecretKey: "sk"},
		"no public key":  {BaseURL: "http://collector", SecretKey: "sk"},
		"no secret key":  {BaseURL: "http://collector", PublicKey: "pk"},
		"nothing at all": {},
	} {
		t.Run(name, func(t *testing.T) {
			report, err := New(cfg, nil).Prune(context.Background())
			if err != nil {
				t.Fatalf("Prune on an unconfigured collector returned %v, want no error", err)
			}
			if !report.Skipped {
				t.Error("report does not say the run was skipped")
			}
			if report.Deleted != 0 {
				t.Errorf("report claims %d deletions with no collector configured", report.Deleted)
			}
		})
	}
}

// A collector holding more than one page of expired records is drained across
// repeated batches within the run, not left half-pruned.
func TestPruneDrainsMoreThanOnePage(t *testing.T) {
	old := time.Now().UTC().Add(-90 * 24 * time.Hour)
	records := map[string]time.Time{}
	for i := 0; i < pageSize*2+5; i++ {
		records[string(rune('a'+i%26))+"-"+time.Duration(i).String()] = old
	}
	total := len(records)

	f := newFakeCollector(records)
	p := newTestPruner(t, f, 30)

	report, err := p.Prune(context.Background())
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if report.Deleted != total {
		t.Errorf("deleted %d of %d expired records", report.Deleted, total)
	}
	if len(f.records) != 0 {
		t.Errorf("%d expired records left behind", len(f.records))
	}
	if f.deletes < 2 {
		t.Errorf("collector saw %d delete calls; a multi-page prune should batch", f.deletes)
	}
}
