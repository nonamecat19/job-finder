package seed

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

func seedProfile(ctx context.Context, q *sqlcgen.Queries) error {
	existing, err := q.ListProfiles(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		slog.Info("seed: profile already exists, skipping")
		return nil
	}

	doc := map[string]any{
		"basics": map[string]any{
			"name":    "Alex Petrov",
			"label":   "Senior Full-Stack Engineer",
			"email":   "alex.petrov@example.com",
			"phone":   "+380-50-123-4567",
			"url":     "https://alexpetrov.dev",
			"summary": "Senior full-stack engineer with 8+ years of experience building scalable web applications. Strong in Go, TypeScript, and PostgreSQL. Passionate about clean architecture, developer tooling, and shipping production-quality code.",
			"location": map[string]any{
				"city":        "Kyiv",
				"countryCode": "UA",
				"region":      "Kyiv Oblast",
			},
			"profiles": []map[string]any{
				{"network": "LinkedIn", "url": "https://linkedin.com/in/alexpetrov"},
				{"network": "GitHub", "url": "https://github.com/alexpetrov"},
			},
		},
		"work": []map[string]any{
			{
				"name":      "CloudScale",
				"position":  "Senior Backend Engineer",
				"url":       "https://cloudscale.io",
				"startDate": "2022-03-01",
				"summary":   "Designed and maintained Go microservices handling 5M+ daily API requests. Led migration from monolith to services.",
				"highlights": []string{
					"Reduced API latency by 40% through query optimization and caching",
					"Built real-time event pipeline processing 200k events/min with Kafka",
					"Mentored 3 junior developers",
				},
			},
			{
				"name":      "DataFlow",
				"position":  "Full-Stack Developer",
				"url":       "https://dataflow.io",
				"startDate": "2019-06-01",
				"endDate":   "2022-02-28",
				"summary":   "Built real-time data visualization dashboards for enterprise clients using React, D3.js, and Node.js.",
				"highlights": []string{
					"Delivered 12 customer-facing dashboards on time",
					"Implemented WebSocket-based live updates reducing data staleness to <1s",
				},
			},
			{
				"name":      "StartupLab",
				"position":  "Junior Developer",
				"url":       "https://startuplab.ua",
				"startDate": "2017-01-15",
				"endDate":   "2019-05-31",
				"summary":   "Full-stack development with Python/Django and React. Built MVPs for 4 early-stage startups.",
			},
		},
		"education": []map[string]any{
			{
				"institution": "Kyiv Polytechnic Institute",
				"area":        "Computer Science",
				"studyType":   "Bachelor of Science",
				"startDate":   "2013-09-01",
				"endDate":     "2017-06-30",
			},
		},
		"skills": []map[string]any{
			{"name": "Go", "level": "Expert", "keywords": []string{"microservices", "goroutines", "chi", "pgx"}},
			{"name": "TypeScript", "level": "Expert", "keywords": []string{"React", "Next.js", "Node.js"}},
			{"name": "PostgreSQL", "level": "Expert", "keywords": []string{"query optimization", "pgvector", "migrations"}},
			{"name": "Docker", "level": "Advanced", "keywords": []string{"multi-stage builds", "compose"}},
			{"name": "Kubernetes", "level": "Intermediate", "keywords": []string{"Helm", "deployments"}},
			{"name": "Python", "level": "Advanced", "keywords": []string{"Django", "FastAPI", "data pipelines"}},
			{"name": "Redis", "level": "Advanced", "keywords": []string{"caching", "queues", "pub/sub"}},
			{"name": "Git", "level": "Expert", "keywords": []string{"CI/CD", "GitHub Actions"}},
		},
		"projects": []map[string]any{
			{
				"name":        "job-finder",
				"description": "AI-powered job search aggregator with semantic matching, automated resume generation, and application pipeline tracking.",
				"url":         "https://github.com/alexpetrov/job-finder",
				"keywords":    []string{"Go", "React", "PostgreSQL", "pgvector", "LLM"},
			},
			{
				"name":        "go-cache-bench",
				"description": "Benchmarking suite for Go caching libraries (ristretto, bigcache, freecache).",
				"url":         "https://github.com/alexpetrov/go-cache-bench",
				"keywords":    []string{"Go", "benchmarks", "caching"},
			},
		},
		"languages": []map[string]any{
			{"language": "English", "fluency": "Professional"},
			{"language": "Ukrainian", "fluency": "Native"},
			{"language": "Russian", "fluency": "Native"},
		},
	}

	docJSON, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	extraNotes := "Open to remote roles. Prefers startups and product companies. Available to start in 2 weeks."

	_, err = q.CreateProfile(ctx, sqlcgen.CreateProfileParams{
		Name:       "Alex Petrov",
		Document:   docJSON,
		ExtraNotes: &extraNotes,
	})
	if err != nil {
		return err
	}

	slog.Info("seed: created profile", "name", "Alex Petrov")
	return nil
}
