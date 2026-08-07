package seed

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

type docSeed struct {
	jobUUID string
	docType string
	version int32
	content map[string]any
	model   string
}

var documents = []docSeed{
	{
		jobUUID: uuidJob1, docType: "resume", version: 1, model: "seed-fixture",
		content: map[string]any{
			"name":    "Alex Petrov",
			"label":   "Senior Full-Stack Engineer",
			"summary": "8+ years building scalable web applications with Go, TypeScript, and PostgreSQL.",
			"skills":  []string{"Go", "TypeScript", "React", "PostgreSQL", "Docker", "Kubernetes"},
		},
	},
	{
		jobUUID: uuidJob1, docType: "cover_letter", version: 1, model: "seed-fixture",
		content: map[string]any{
			"greeting": "Dear Hiring Manager,",
			"body":     "I am excited to apply for the Senior Go Developer position at NovaTech. With 8 years of experience in Go microservices and a strong background in PostgreSQL optimization, I am confident I can contribute to your core platform.",
			"closing":  "Best regards, Alex Petrov",
		},
	},
	{
		jobUUID: uuidJob3, docType: "resume", version: 1, model: "seed-fixture",
		content: map[string]any{
			"name":    "Alex Petrov",
			"label":   "Backend Developer (Go)",
			"summary": "Backend-focused engineer with deep Go expertise, experience building SaaS infrastructure platforms.",
			"skills":  []string{"Go", "gRPC", "PostgreSQL", "Kubernetes", "REST APIs"},
		},
	},
	{
		jobUUID: uuidJob7, docType: "resume", version: 1, model: "seed-fixture",
		content: map[string]any{
			"name":    "Alex Petrov",
			"label":   "Senior React Developer",
			"summary": "Full-stack engineer with strong React/TypeScript skills, experience building real-time collaborative tools.",
			"skills":  []string{"React", "TypeScript", "Next.js", "Zustand", "WebSocket"},
		},
	},
	{
		jobUUID: uuidJob2, docType: "cover_letter", version: 1, model: "seed-fixture",
		content: map[string]any{
			"greeting": "Hi Team,",
			"body":     "I would love to join DataFlow as a React/Node.js Engineer. My experience building real-time dashboards with WebSocket and D3.js aligns perfectly with your needs.",
			"closing":  "Cheers, Alex",
		},
	},
}

func seedDocuments(ctx context.Context, pool *pgxpool.Pool, q *sqlcgen.Queries) error {
	jobID1, err := dbutil.ParseUUID(uuidJob1)
	if err != nil {
		return err
	}
	existing, _ := q.ListDocumentsForJob(ctx, jobID1)
	if len(existing) > 0 {
		slog.Info("seed: documents already exist, skipping")
		return nil
	}

	for _, d := range documents {
		contentJSON, err := json.Marshal(d.content)
		if err != nil {
			return err
		}
		jobID, err := dbutil.ParseUUID(d.jobUUID)
		if err != nil {
			return err
		}
		_, err = q.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
			JobId:   jobID,
			Type:    d.docType,
			Version: d.version,
			Content: contentJSON,
			Model:   d.model,
		})
		if err != nil {
			return err
		}
	}

	_ = pool
	slog.Info("seed: created documents", "count", len(documents))
	return nil
}
