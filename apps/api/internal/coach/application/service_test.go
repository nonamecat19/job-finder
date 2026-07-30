package application

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/coach/domain"
	"github.com/job-finder/api/internal/keyword"
)

// fakeRephraseModel is a deterministic test double for RephraseModel.
type fakeRephraseModel struct {
	// responses maps term -> rephrase output
	responses map[string]string
}

func (f *fakeRephraseModel) Rephrase(ctx context.Context, prompt string) (string, error) {
	// Extract the target term from the prompt
	for term, resp := range f.responses {
		if contains(prompt, term) {
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

	diffResult := &keyword.DiffResult{
		Matched:          []keyword.DiffTerm{{Term: "Go", Polarity: keyword.PolarityRequired}},
		MissingRequired:  []keyword.DiffTerm{},
		MissingPreferred: []keyword.DiffTerm{{Term: "Rust", Polarity: keyword.PolarityPreferred}},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   1,
			MatchedRequired:  1,
			MatchedPreferred: 0,
			CoveragePct:      100.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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

	// Missing term: Rust. Profile has only Java/Python — genuinely no adjacency.
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "Rust", Canonical: "Rust", Polarity: keyword.PolarityRequired, Stemmed: "rust"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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

	// Missing: Docker. Profile has Podman (adjacent per adjacency map).
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: keyword.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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
	if ev.Proximity != keyword.ProximityClose {
		t.Errorf("Evidence.Proximity: got %q, want %q", ev.Proximity, keyword.ProximityClose)
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

	// Missing: Postgres. Profile has 5 entries with MySQL, SQLite, Oracle (all adjacent) — should return max 3.
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "Postgres", Canonical: "Postgres", Polarity: keyword.PolarityRequired, Stemmed: "postgr"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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
			// Return a rephrase that invents a technology not in the source bullet
			"gRPC": "Built gRPC and REST APIs handling 10k req/s",
		},
	}
	svc := NewService(model)

	// Missing: gRPC. Profile has REST (adjacent), but the rephrase invents gRPC.
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "gRPC", Canonical: "gRPC", Polarity: keyword.PolarityRequired, Stemmed: "grpc"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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
	// The grounding check should reject the invented "gRPC" mention, so no evidence
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

	// Missing: Postgres. Profile has MySQL (close) and Oracle (distant).
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "Postgres", Canonical: "Postgres", Polarity: keyword.PolarityRequired, Stemmed: "postgr"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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

	// First evidence should be the closest proximity (MySQL = close)
	first := gap.AdjacentEvidence[0]
	if first.Proximity != keyword.ProximityClose {
		t.Errorf("First evidence proximity: got %q, want %q (closest first)", first.Proximity, keyword.ProximityClose)
	}
	if !contains(first.SourceBullet, "MySQL") {
		t.Errorf("First evidence should mention MySQL (close), got: %q", first.SourceBullet)
	}
}

// --- Adversarial: inflated seniority (009-4 framing rewriter) ---

func TestAssess_RejectsInflatedSeniority(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Docker": "As a Senior DevOps Engineer, managed CI/CD pipelines with Podman and BuildKit",
		},
	}
	svc := NewService(model)

	// Source label says "DevOps Engineer" (no seniority prefix), but the
	// rephrase claims "Senior" — must be rejected.
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: keyword.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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

	// Source label says "Junior DevOps Engineer" — rephrase claiming "Senior"
	// must be rejected.
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: keyword.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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

	// Source label says "Senior DevOps Engineer" — rephrase does not mention
	// seniority at all (only uses words from the source bullet), so no
	// inflation violation. Same-level or absent seniority is allowed.
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: keyword.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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

// --- Adversarial: invented duration (009-4 framing rewriter) ---

func TestAssess_RejectsInflatedDuration(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Docker": "With over 10 years of experience, managed CI/CD pipelines with Podman and BuildKit",
		},
	}
	svc := NewService(model)

	// Source label says "2022–2024" (2 years), but rephrase claims "10+ years".
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: keyword.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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

// --- Adversarial: borrowed technologies (009-4 framing rewriter) ---

func TestAssess_RejectsBorrowedTechnology(t *testing.T) {
	model := &fakeRephraseModel{
		responses: map[string]string{
			"Docker": "Managed Docker containers and Kubernetes clusters in production",
		},
	}
	svc := NewService(model)

	// Source bullet only mentions "Podman and BuildKit" — "Kubernetes" is
	// borrowed from elsewhere and must be rejected.
	diffResult := &keyword.DiffResult{
		Matched: []keyword.DiffTerm{},
		MissingRequired: []keyword.DiffTerm{
			{Term: "Docker", Canonical: "Docker", Polarity: keyword.PolarityRequired, Stemmed: "docker"},
		},
		MissingPreferred: []keyword.DiffTerm{},
		Metadata: keyword.DiffMetadata{
			TotalRequired:    1,
			TotalPreferred:   0,
			MatchedRequired:  0,
			MatchedPreferred: 0,
			CoveragePct:      0.0,
		},
	}

	profileEntries := []domain.ProfileEntry{
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

	result := svc.Assess(context.Background(), "job-1", nil, []domain.ProfileEntry{}, "")

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
