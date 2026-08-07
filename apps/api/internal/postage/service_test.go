package postage_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/postage"
)

type fakeRepo struct {
	postage.Repository
	rows []sqlcgen.PostAgeResponseRateRow
	err  error
}

func (f *fakeRepo) PostAgeResponseRate(ctx context.Context) ([]sqlcgen.PostAgeResponseRateRow, error) {
	return f.rows, f.err
}

func TestComputeObserved(t *testing.T) {
	repo := &fakeRepo{rows: []sqlcgen.PostAgeResponseRateRow{
		{Bucket: "fresh", N: 10, Responses: 4},
		{Bucket: "recent", N: 10, Responses: 2},
		{Bucket: "aging", N: 10, Responses: 1},
		{Bucket: "stale", N: 10, Responses: 0},
	}}
	svc := postage.NewService(repo)
	out, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if out.TotalApps != 40 {
		t.Fatalf("totalApps = %d, want 40", out.TotalApps)
	}
	if out.GlobalState != dto.PostAgeStateObserved {
		t.Fatalf("globalState = %s, want observed", out.GlobalState)
	}
	if len(out.Buckets) != 4 {
		t.Fatalf("buckets = %d, want 4", len(out.Buckets))
	}
	for _, b := range out.Buckets {
		if b.State != dto.PostAgeStateObserved {
			t.Fatalf("bucket %s state = %s, want observed", b.Bucket, b.State)
		}
		if b.Rate == nil {
			t.Fatalf("bucket %s rate is nil, want a computed rate", b.Bucket)
		}
	}
	if out.Buckets[0].N != 10 || out.Buckets[0].Responses != 4 || *out.Buckets[0].Rate != 0.4 {
		t.Fatalf("fresh bucket: n=%d responses=%d rate=%v", out.Buckets[0].N, out.Buckets[0].Responses, *out.Buckets[0].Rate)
	}
	if *out.Buckets[3].Rate != 0.0 {
		t.Fatalf("stale rate = %v, want 0.0", *out.Buckets[3].Rate)
	}
}

func TestComputePriorGlobalColdStart(t *testing.T) {
	repo := &fakeRepo{rows: []sqlcgen.PostAgeResponseRateRow{
		{Bucket: "fresh", N: 5, Responses: 2},
		{Bucket: "recent", N: 5, Responses: 1},
	}}
	svc := postage.NewService(repo)
	out, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if out.GlobalState != dto.PostAgeStatePrior {
		t.Fatalf("globalState = %s, want prior", out.GlobalState)
	}
	if out.TotalApps != 10 {
		t.Fatalf("totalApps = %d, want 10", out.TotalApps)
	}
	if out.PriorRate != postage.DocumentedPriorRate {
		t.Fatalf("priorRate = %f, want %f", out.PriorRate, postage.DocumentedPriorRate)
	}
	if out.ThresholdMsg == nil {
		t.Fatal("expected thresholdMsg when in prior state")
	}
	for _, b := range out.Buckets {
		if b.State != dto.PostAgeStatePrior {
			t.Fatalf("bucket %s state = %s, want prior", b.Bucket, b.State)
		}
		if b.Rate != nil {
			t.Fatalf("bucket %s rate should be nil in prior state", b.Bucket)
		}
	}
}

func TestComputeInsufficientPerBucket(t *testing.T) {
	repo := &fakeRepo{rows: []sqlcgen.PostAgeResponseRateRow{
		{Bucket: "fresh", N: 30, Responses: 12},
		{Bucket: "stale", N: 3, Responses: 1},
		{Bucket: "unknown", N: 7, Responses: 2},
	}}
	svc := postage.NewService(repo)
	out, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if out.GlobalState != dto.PostAgeStateObserved {
		t.Fatalf("globalState = %s, want observed", out.GlobalState)
	}
	if out.Buckets[0].State != dto.PostAgeStateObserved {
		t.Fatalf("fresh state = %s, want observed", out.Buckets[0].State)
	}
	if out.Buckets[0].Rate == nil {
		t.Fatal("fresh rate should not be nil")
	}
	if out.Buckets[1].State != dto.PostAgeStateInsufficient {
		t.Fatalf("stale state = %s, want insufficient", out.Buckets[1].State)
	}
	if out.Buckets[1].Rate != nil {
		t.Fatal("stale rate should be nil in insufficient state")
	}
	if out.Buckets[2].State != dto.PostAgeStateInsufficient {
		t.Fatalf("unknown state = %s, want insufficient", out.Buckets[2].State)
	}
}

func TestComputeEmpty(t *testing.T) {
	repo := &fakeRepo{rows: []sqlcgen.PostAgeResponseRateRow{}}
	svc := postage.NewService(repo)
	out, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if out.TotalApps != 0 {
		t.Fatalf("totalApps = %d, want 0", out.TotalApps)
	}
	if out.GlobalState != dto.PostAgeStatePrior {
		t.Fatalf("globalState = %s, want prior", out.GlobalState)
	}
	if len(out.Buckets) != 0 {
		t.Fatalf("buckets = %d, want 0", len(out.Buckets))
	}
	if out.ThresholdMsg == nil {
		t.Fatal("expected thresholdMsg when empty")
	}
}

func TestComputeUnknownBucket(t *testing.T) {
	repo := &fakeRepo{rows: []sqlcgen.PostAgeResponseRateRow{
		{Bucket: "fresh", N: 15, Responses: 5},
		{Bucket: "unknown", N: 15, Responses: 3},
	}}
	svc := postage.NewService(repo)
	out, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if len(out.Buckets) != 2 {
		t.Fatalf("buckets = %d, want 2", len(out.Buckets))
	}
	if out.Buckets[1].Bucket != "unknown" {
		t.Fatalf("last bucket = %s, want unknown", out.Buckets[1].Bucket)
	}
	if out.Buckets[1].State != dto.PostAgeStateObserved {
		t.Fatalf("unknown state = %s, want observed", out.Buckets[1].State)
	}
}

func TestComputeRatePrecision(t *testing.T) {
	repo := &fakeRepo{rows: []sqlcgen.PostAgeResponseRateRow{
		{Bucket: "fresh", N: 7, Responses: 3},
		{Bucket: "recent", N: 7, Responses: 0},
		{Bucket: "aging", N: 7, Responses: 7},
		{Bucket: "stale", N: 7, Responses: 0},
		{Bucket: "unknown", N: 7, Responses: 0},
	}}
	svc := postage.NewService(repo)
	out, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	for _, b := range out.Buckets {
		if b.State != dto.PostAgeStateInsufficient {
			t.Fatalf("bucket %s state = %s, want insufficient (n=%d < 8)", b.Bucket, b.State, b.N)
		}
	}
}
