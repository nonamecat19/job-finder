//go:build integration

package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/generation/domain"
)

// TestCertificationsShortfall_UnmeetableMinStillSucceedsAndRecordsActivity
// covers SC-007: a run configured with a certificationsMin the master cannot
// satisfy must not fail, must keep rendering the certifications that do
// exist (nothing invented to close the gap), and must surface the shortfall
// on the activity record backing the run — the same real-Postgres path
// production traffic uses (activity.Recorder -> ActivityRun.step/meta),
// exercised end to end via recordShortfalls exactly as
// tailorRendercvResume calls it.
func TestCertificationsShortfall_UnmeetableMinStillSucceedsAndRecordsActivity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testDB := dbtest.New(t)

	rec := activity.New(ctx, testDB.Queries, "generate.adhoc", "certifications shortfall test", nil, nil, "")
	if rec == nil {
		t.Fatal("activity.New returned nil recorder")
	}

	// Two certifications on the master, a minimum of four: unmeetable.
	merged := domain.RendercvMaster{
		"cv": map[string]any{
			"sections": map[string]any{
				"certifications": []any{
					map[string]any{"label": "AWS Certified Solutions Architect", "details": "AWS details"},
					map[string]any{"label": "Certified Kubernetes Administrator", "details": "CKA details"},
				},
			},
		},
	}
	cfg := domain.DefaultShapeConfig()
	cfg.CertificationsMin = 4

	report := domain.ApplyHardLimits(merged, merged, cfg)

	// The run itself does not fail: ApplyHardLimits returns a report, not an
	// error, and the caller (tailorRendercvResume) proceeds regardless.
	if len(report.Shortfalls) != 1 {
		t.Fatalf("shortfalls = %+v, want exactly one", report.Shortfalls)
	}
	sf := report.Shortfalls[0]
	if sf.Path != "cv.sections.certifications" || sf.Requested != 4 || sf.Available != 2 {
		t.Fatalf("shortfall = %+v, want cv.sections.certifications requested 4 / available 2", sf)
	}

	// The available certifications still render — nothing was stripped or
	// invented to chase the minimum.
	sections, _ := merged["cv"].(map[string]any)["sections"].(map[string]any)
	certs, _ := sections["certifications"].([]any)
	if len(certs) != 2 {
		t.Fatalf("certifications = %v, want the 2 available entries left untouched", certs)
	}

	// This is the production call path: tailorRendercvResume feeds the
	// report straight into recordShortfalls.
	recordShortfalls(ctx, rec, report)

	row, err := testDB.Queries.GetActivityRun(ctx, rec.ID())
	if err != nil {
		t.Fatalf("get activity run: %v", err)
	}
	if row.Step == nil || *row.Step != "resume shape shortfall: cv.sections.certifications" {
		t.Fatalf("activity step = %v, want the certifications shortfall label", row.Step)
	}
	var meta struct {
		Path      string `json:"path"`
		Requested int    `json:"requested"`
		Available int    `json:"available"`
	}
	if err := json.Unmarshal(row.Meta, &meta); err != nil {
		t.Fatalf("unmarshal activity meta: %v", err)
	}
	if meta.Path != "cv.sections.certifications" || meta.Requested != 4 || meta.Available != 2 {
		t.Fatalf("activity meta = %+v, want path cv.sections.certifications requested 4 available 2", meta)
	}
}
