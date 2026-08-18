package application

import (
	"context"
	"testing"

	coachdomain "github.com/job-finder/api/internal/coach/domain"
	"github.com/job-finder/api/internal/keyword/domain"
)

type fakeRephraseModel struct {
	responses map[string]string
}

func (f *fakeRephraseModel) Rephrase(ctx context.Context, req RephraseRequest) (string, error) {
	haystack := req.Term + " " + req.Canonical + " " + req.SourceBullet + " " + req.SourceLabel
	for term, resp := range f.responses {
		if contains(haystack, term) {
			return resp, nil
		}
	}
	return "Managed various technologies in production environment", nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestAssess_NoMissingRequired(t *testing.T) {
	model := &fakeRephraseModel{responses: map[string]string{}}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched:          []domain.DiffTerm{{Term: "Go", Polarity: domain.PolarityRequired}},
		MissingRequired:  []domain.DiffTerm{},
		MissingPreferred: []domain.DiffTerm{{Term: "Rust", Polarity: domain.PolarityPreferred}},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   1,
			MatchedRequired:  1,
			MatchedPreferred: 0,
			CoveragePct:      100.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{SourceLabel: "Backend Engineer, Acme (2022-2024)", Bullet: "Built Go services"},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "backend")

	if result.TotalMustHaves != 1 {
		t.Errorf("TotalMustHaves: got %d, want 1", result.TotalMustHaves)
	}
	if result.FailedMustHaves != 0 {
		t.Errorf("FailedMustHaves: got %d, want 0", result.FailedMustHaves)
	}
	if len(result.Gaps) != 0 {
		t.Errorf("Gaps: got %d, want 0", len(result.Gaps))
	}
}

func TestAssess_ZeroAdjacency(t *testing.T) {
	model := &fakeRephraseModel{responses: map[string]string{}}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "Rust", Canonical: "Rust", Polarity: domain.PolarityRequired, Stemmed: "rust"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{SourceLabel: "Backend Engineer, Beta Inc (2021-2023)", Bullet: "Built Java microservices"},
		{SourceLabel: "Data Engineer, Gamma Corp (2019-2021)", Bullet: "Wrote Python ETL pipelines"},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "backend")

	if result.TotalMustHaves != 1 {
		t.Errorf("TotalMustHaves: got %d, want 1", result.TotalMustHaves)
	}
	if result.FailedMustHaves != 1 {
		t.Errorf("FailedMustHaves: got %d, want 1", result.FailedMustHaves)
	}
	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if gap.Term != "Rust" {
		t.Errorf("Gap.Term: got %q, want %q", gap.Term, "Rust")
	}
	if !gap.NoAdjacentEvidence {
		t.Errorf("Gap.NoAdjacentEvidence: got false, want true (honest empty result)")
	}
	if len(gap.AdjacentEvidence) != 0 {
		t.Errorf("Gap.AdjacentEvidence: got %d items, want 0", len(gap.AdjacentEvidence))
	}
}

func TestAssess_AdjacentEvidenceFound(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Docker": "Managed CI/CD pipelines with Podman and BuildKit for containerization",
		},
	}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: domain.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{
			SourceLabel: "DevOps Engineer, Acme Corp (2022–2024)",
			Bullet:      "Managed CI/CD pipelines with Podman and BuildKit",
		},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "devops")

	if result.FailedMustHaves != 1 {
		t.Errorf("FailedMustHaves: got %d, want 1", result.FailedMustHaves)
	}
	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if gap.Term != "Docker" {
		t.Errorf("Gap.Term: got %q, want %q", gap.Term, "Docker")
	}
	if gap.NoAdjacentEvidence {
		t.Errorf("Gap.NoAdjacentEvidence: got true, want false")
	}
	if len(gap.AdjacentEvidence) == 0 {
		t.Fatalf("Gap.AdjacentEvidence: got 0 items, want >= 1")
	}

	ev := gap.AdjacentEvidence[0]
	if ev.SourceEntry != "DevOps Engineer, Acme Corp (2022–2024)" {
		t.Errorf("Evidence.SourceEntry: got %q, want %q", ev.SourceEntry, "DevOps Engineer, Acme Corp (2022–2024)")
	}
	if ev.SourceBullet != "Managed CI/CD pipelines with Podman and BuildKit" {
		t.Errorf("Evidence.SourceBullet: got %q", ev.SourceBullet)
	}
	if ev.Proximity != domain.ProximityClose {
		t.Errorf("Evidence.Proximity: got %q, want %q", ev.Proximity, domain.ProximityClose)
	}
	if ev.Rephrase == "" {
		t.Errorf("Evidence.Rephrase: got empty, want non-empty")
	}
}

func TestAssess_MaxThreeEvidence(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Postgres": "Built MySQL applications with database management",
		},
	}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "Postgres", Canonical: "Postgres", Polarity: domain.PolarityRequired, Stemmed: "postgr"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{SourceLabel: "Backend 1", Bullet: "Built MySQL applications"},
		{SourceLabel: "Backend 2", Bullet: "Managed MySQL databases"},
		{SourceLabel: "Backend 3", Bullet: "Deployed SQLite solutions"},
		{SourceLabel: "Backend 4", Bullet: "Configured Oracle clusters"},
		{SourceLabel: "Backend 5", Bullet: "Optimized MySQL queries"},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "backend")

	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if len(gap.AdjacentEvidence) > 3 {
		t.Errorf("AdjacentEvidence count: got %d, want max 3", len(gap.AdjacentEvidence))
	}
	if len(gap.AdjacentEvidence) == 0 {
		t.Errorf("AdjacentEvidence count: got 0, want 1-3")
	}
}

func TestAssess_GroundingRejection(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"gRPC": "Built gRPC and REST APIs handling 10k req/s",
		},
	}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "gRPC", Canonical: "gRPC", Polarity: domain.PolarityRequired, Stemmed: "grpc"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{
			SourceLabel: "Backend Engineer, Beta Inc (2021–2023)",
			Bullet:      "Built REST and WebSocket APIs handling 10k req/s",
		},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "backend")

	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if !gap.NoAdjacentEvidence {
		t.Errorf("Gap.NoAdjacentEvidence: got false, want true (grounding rejected)")
	}
	if len(gap.AdjacentEvidence) != 0 {
		t.Errorf("Gap.AdjacentEvidence: got %d items, want 0 (all rephrases rejected)", len(gap.AdjacentEvidence))
	}
}

func TestAssess_ProximitySorting(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Postgres": "Managed MySQL databases handling 1M records with high reliability",
		},
	}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "Postgres", Canonical: "Postgres", Polarity: domain.PolarityRequired, Stemmed: "postgr"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{SourceLabel: "DBA, Gamma (2020–2022)", Bullet: "Administered Oracle databases with 99% uptime"},
		{SourceLabel: "Backend, Beta (2018–2020)", Bullet: "Managed MySQL databases handling 1M records"},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "backend")

	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if len(gap.AdjacentEvidence) == 0 {
		t.Fatalf("AdjacentEvidence: got 0, want >= 1")
	}

	first := gap.AdjacentEvidence[0]
	if first.Proximity != domain.ProximityClose {
		t.Errorf("First evidence proximity: got %q, want %q (closest first)", first.Proximity, domain.ProximityClose)
	}
	if !contains(first.SourceBullet, "MySQL") {
		t.Errorf("First evidence should mention MySQL (close), got: %q", first.SourceBullet)
	}
}

func TestAssess_RejectsInflatedSeniority(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Docker": "As a Senior DevOps Engineer, managed CI/CD pipelines with Podman and BuildKit",
		},
	}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: domain.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{
			SourceLabel: "DevOps Engineer, Acme Corp (2022–2024)",
			Bullet:      "Managed CI/CD pipelines with Podman and BuildKit",
		},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "devops")

	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if !gap.NoAdjacentEvidence {
		t.Errorf("Gap.NoAdjacentEvidence: got false, want true (seniority inflation rejected)")
	}
	if len(gap.AdjacentEvidence) != 0 {
		t.Errorf("Gap.AdjacentEvidence: got %d items, want 0 (all rephrases rejected)", len(gap.AdjacentEvidence))
	}
}

func TestAssess_RejectsInflatedSeniority_JuniorToSenior(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Docker": "As a Senior DevOps Engineer, managed CI/CD pipelines with Podman and BuildKit",
		},
	}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: domain.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{
			SourceLabel: "Junior DevOps Engineer, Acme Corp (2022–2024)",
			Bullet:      "Managed CI/CD pipelines with Podman and BuildKit",
		},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "devops")

	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if !gap.NoAdjacentEvidence {
		t.Errorf("Gap.NoAdjacentEvidence: got false, want true (junior→senior inflation rejected)")
	}
	if len(gap.AdjacentEvidence) != 0 {
		t.Errorf("Gap.AdjacentEvidence: got %d items, want 0", len(gap.AdjacentEvidence))
	}
}

func TestAssess_AllowsSameSeniority(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Docker": "Managed CI/CD pipelines with Podman and BuildKit for containerization",
		},
	}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: domain.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{
			SourceLabel: "Senior DevOps Engineer, Acme Corp (2022–2024)",
			Bullet:      "Managed CI/CD pipelines with Podman and BuildKit",
		},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "devops")

	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if gap.NoAdjacentEvidence {
		t.Errorf("Gap.NoAdjacentEvidence: got true, want false (same seniority should be allowed)")
	}
	if len(gap.AdjacentEvidence) == 0 {
		t.Errorf("Gap.AdjacentEvidence: got 0 items, want >= 1")
	}
}

func TestAssess_RejectsInflatedDuration(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Docker": "With over 10 years of experience, managed CI/CD pipelines with Podman and BuildKit",
		},
	}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: domain.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{
			SourceLabel: "DevOps Engineer, Acme Corp (2022–2024)",
			Bullet:      "Managed CI/CD pipelines with Podman and BuildKit",
		},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "devops")

	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if !gap.NoAdjacentEvidence {
		t.Errorf("Gap.NoAdjacentEvidence: got false, want true (duration inflation rejected)")
	}
	if len(gap.AdjacentEvidence) != 0 {
		t.Errorf("Gap.AdjacentEvidence: got %d items, want 0", len(gap.AdjacentEvidence))
	}
}

func TestAssess_RejectsBorrowedTechnology(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Docker": "Managed Docker containers and Kubernetes clusters in production",
		},
	}
	svc := NewService(model)

	diffResult := &domain.DiffResult{
		Matched: []domain.DiffTerm{},
		MissingRequired: []domain.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: domain.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []domain.DiffTerm{},
		Metadata: domain.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []coachdomain.ProfileEntry{
		{
			SourceLabel: "DevOps Engineer, Acme Corp (2022–2024)",
			Bullet:      "Managed CI/CD pipelines with Podman and BuildKit",
		},
	}

	result := svc.Assess(context.Background(), "job-1", diffResult, profileEntries, "devops")

	if len(result.Gaps) != 1 {
		t.Fatalf("Gaps: got %d, want 1", len(result.Gaps))
	}

	gap := result.Gaps[0]
	if !gap.NoAdjacentEvidence {
		t.Errorf("Gap.NoAdjacentEvidence: got false, want true (borrowed tech rejected)")
	}
	if len(gap.AdjacentEvidence) != 0 {
		t.Errorf("Gap.AdjacentEvidence: got %d items, want 0", len(gap.AdjacentEvidence))
	}
}

func TestAssess_NilDiffResult(t *testing.T) {
	model := &fakeRephraseModel{responses: map[string]string{}}
	svc := NewService(model)

	result := svc.Assess(context.Background(), "job-1", nil, []coachdomain.ProfileEntry{}, "")

	if result.TotalMustHaves != 0 {
		t.Errorf("TotalMustHaves: got %d, want 0", result.TotalMustHaves)
	}
	if result.FailedMustHaves != 0 {
		t.Errorf("FailedMustHaves: got %d, want 0", result.FailedMustHaves)
	}
	if len(result.Gaps) != 0 {
		t.Errorf("Gaps: got %d, want 0", len(result.Gaps))
	}
}
