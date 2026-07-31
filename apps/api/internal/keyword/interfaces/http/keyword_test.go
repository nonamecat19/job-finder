package http_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/keyword/application"
	keywordhttp "github.com/job-finder/api/internal/keyword/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

type fakeKeywordProvider struct {
	resp dto.KeywordDiffDto
	err  error
}

func (f *fakeKeywordProvider) KeywordDiff(ctx context.Context, jobID string) (dto.KeywordDiffDto, error) {
	if f.err != nil {
		return dto.KeywordDiffDto{}, f.err
	}
	f.resp.JobID = jobID
	return f.resp, nil
}

func strptr(s string) *string { return &s }

func sampleDiff() dto.KeywordDiffDto {
	return dto.KeywordDiffDto{
		Matched: []dto.KeywordDiffTermDto{
			{Term: "kubernetes", Canonical: "Kubernetes", Polarity: "required", Normalized: "kubernet", MatchType: "exact"},
		},
		MissingRequired: []dto.KeywordDiffTermDto{
			{Term: "docker", Canonical: "Docker", Polarity: "required", Normalized: "docker"},
		},
		MissingPreferred: []dto.KeywordDiffTermDto{
			{Term: "grpc", Canonical: "gRPC", Polarity: "preferred", Normalized: "grpc"},
		},
		Metadata: dto.KeywordDiffMetadataDto{
			TotalRequired: 2, TotalPreferred: 1, MatchedRequired: 1, MatchedPreferred: 0, CoveragePct: 33.3,
		},
		Suggestions: []dto.KeywordRephraseSuggestionDto{
			{Term: "docker", Canonical: "Docker", Rephrase: strptr("Containerized services with Docker-adjacent tooling"), SourceBullet: "Built container images"},
			{Term: "kafka", Canonical: "Kafka", Rephrase: nil, Reason: "no-honest-rephrase-available"},
		},
	}
}

func TestKeywordDiffGet(t *testing.T) {
	h := &keywordhttp.KeywordHandler{Diff: &fakeKeywordProvider{resp: sampleDiff()}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job-1/keyword-diff", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var out dto.KeywordDiffDto
	testutil.ParseJSON(w, &out)

	if out.JobID != "job-1" {
		t.Errorf("expected jobId echoed, got %q", out.JobID)
	}
	if len(out.Matched) != 1 || out.Matched[0].Canonical != "Kubernetes" {
		t.Errorf("unexpected matched: %+v", out.Matched)
	}
	if len(out.MissingRequired) != 1 || out.MissingRequired[0].Canonical != "Docker" {
		t.Errorf("unexpected missingRequired: %+v", out.MissingRequired)
	}
	if len(out.MissingPreferred) != 1 || out.MissingPreferred[0].Canonical != "gRPC" {
		t.Errorf("unexpected missingPreferred: %+v", out.MissingPreferred)
	}
	if out.Metadata.TotalRequired != 2 || out.Metadata.MatchedRequired != 1 {
		t.Errorf("unexpected metadata: %+v", out.Metadata)
	}
	if len(out.Suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(out.Suggestions))
	}
	if out.Suggestions[0].Rephrase == nil || *out.Suggestions[0].Rephrase == "" {
		t.Errorf("expected a rephrase for docker, got nil/empty")
	}
	if out.Suggestions[1].Rephrase != nil || out.Suggestions[1].Reason != "no-honest-rephrase-available" {
		t.Errorf("expected no-honest-rephrase for kafka, got %+v", out.Suggestions[1])
	}
}

func TestKeywordDiffNotFound(t *testing.T) {
	h := &keywordhttp.KeywordHandler{Diff: &fakeKeywordProvider{err: application.ErrDiffNotFound}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/missing/keyword-diff", nil, map[string]string{"id": "missing"})
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
