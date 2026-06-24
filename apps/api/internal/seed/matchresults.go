package seed

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

type matchSeed struct {
	jobUUID       string
	similarity    float64
	score         int32
	matchedSkills []string
	missingSkills []string
	summary       string
	redFlags      []string
	model         string
}

var matchResults = []matchSeed{
	{
		jobUUID: uuidJob1, similarity: 0.92, score: 90,
		matchedSkills: []string{"Go", "PostgreSQL", "microservices", "REST APIs"},
		missingSkills: []string{"gRPC", "Kafka"},
		summary:       "Excellent match. Strong Go and PostgreSQL experience aligns well with role requirements.",
		redFlags:      []string{},
		model:         "seed-fixture",
	},
	{
		jobUUID: uuidJob2, similarity: 0.78, score: 75,
		matchedSkills: []string{"React", "TypeScript", "Node.js", "WebSocket"},
		missingSkills: []string{"D3.js", "Recharts"},
		summary:       "Good match. Frontend skills are strong; data visualization experience is lighter.",
		redFlags:      []string{},
		model:         "seed-fixture",
	},
	{
		jobUUID: uuidJob3, similarity: 0.88, score: 87,
		matchedSkills: []string{"Go", "PostgreSQL", "REST APIs", "Kubernetes"},
		missingSkills: []string{"gRPC"},
		summary:       "Strong match. Go expertise and infrastructure experience are a great fit.",
		redFlags:      []string{},
		model:         "seed-fixture",
	},
	{
		jobUUID: uuidJob4, similarity: 0.74, score: 72,
		matchedSkills: []string{"React", "Go", "PostgreSQL", "Next.js"},
		missingSkills: []string{"Tailwind CSS"},
		summary:       "Good match. Full-stack skills are solid; CSS framework experience is limited.",
		redFlags:      []string{},
		model:         "seed-fixture",
	},
	{
		jobUUID: uuidJob7, similarity: 0.81, score: 80,
		matchedSkills: []string{"React", "TypeScript", "Next.js"},
		missingSkills: []string{"Zustand", "Jotai"},
		summary:       "Good match. React fundamentals are strong; specific state management libraries differ.",
		redFlags:      []string{},
		model:         "seed-fixture",
	},
	{
		jobUUID: uuidJob10, similarity: 0.55, score: 50,
		matchedSkills: []string{"Docker", "PostgreSQL"},
		missingSkills: []string{"Kubernetes", "Terraform", "Istio"},
		summary:       "Partial match. Core infrastructure knowledge exists but Kubernetes depth is limited.",
		redFlags:      []string{"Kubernetes experience is at intermediate level"},
		model:         "seed-fixture",
	},
	{
		jobUUID: uuidJob11, similarity: 0.79, score: 78,
		matchedSkills: []string{"Go", "microservices", "Kubernetes"},
		missingSkills: []string{"gRPC", "Kafka"},
		summary:       "Good match. Go and microservices experience is relevant; event streaming is weaker.",
		redFlags:      []string{},
		model:         "seed-fixture",
	},
	{
		jobUUID: uuidJob15, similarity: 0.42, score: 40,
		matchedSkills: []string{"Python"},
		missingSkills: []string{"PyTorch", "transformers", "MLOps"},
		summary:       "Weak match. Python skills exist but ML/deep learning experience is insufficient.",
		redFlags:      []string{"No production ML experience", "No deep learning framework experience"},
		model:         "seed-fixture",
	},
	{
		jobUUID: uuidJob18, similarity: 0.48, score: 45,
		matchedSkills: []string{"Go", "API documentation"},
		missingSkills: []string{"OpenAPI/Swagger", "technical writing portfolio"},
		summary:       "Below average match. Technical knowledge is present but dedicated writing experience is limited.",
		redFlags:      []string{"No formal technical writing experience"},
		model:         "seed-fixture",
	},
	{
		jobUUID: uuidJob19, similarity: 0.30, score: 25,
		matchedSkills: []string{},
		missingSkills: []string{"Swift", "SwiftUI", "Combine", "Core Data"},
		summary:       "Poor match. iOS development experience is minimal; role requires deep Swift/iOS expertise.",
		redFlags:      []string{"No iOS development experience", "No Swift experience"},
		model:         "seed-fixture",
	},
}

func seedMatchResults(ctx context.Context, pool *pgxpool.Pool, q *sqlcgen.Queries) error {
	// Idempotency: check if match results already exist for first seed job.
	jobID1, err := dbutil.ParseUUID(uuidJob1)
	if err != nil {
		return err
	}
	existing, _ := q.GetMatchResultByJobID(ctx, jobID1)
	if existing.ID.Valid {
		slog.Info("seed: match results already exist, skipping")
		return nil
	}

	for _, m := range matchResults {
		jobID, err := dbutil.ParseUUID(m.jobUUID)
		if err != nil {
			return err
		}
		matchedJSON, _ := json.Marshal(m.matchedSkills)
		missingJSON, _ := json.Marshal(m.missingSkills)
		redFlagsJSON, _ := json.Marshal(m.redFlags)
		score := int32(m.score)

		_, err = q.UpsertMatchResult(ctx, sqlcgen.UpsertMatchResultParams{
			JobId:         jobID,
			Similarity:    m.similarity,
			Score:         &score,
			MatchedSkills: matchedJSON,
			MissingSkills: missingJSON,
			Summary:       &m.summary,
			RedFlags:      redFlagsJSON,
			Model:         m.model,
		})
		if err != nil {
			return err
		}
	}

	_ = pool
	slog.Info("seed: created match results", "count", len(matchResults))
	return nil
}
