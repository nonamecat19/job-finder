package application

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/platform/llm"
)

// The lookups salary estimation offers the model.
//
// Both are **reads**, and that is a rule with a mechanism behind it rather than
// a convention: apps/api/internal/toolfence_test.go resolves this package's
// transitive dependency closure and fails the build if it can reach a write
// path, an outbound HTTP package or the ingestion stack. This package is on its
// declared list.
//
// The service's two writes — UpdateJobSalary and UpsertSalaryCache — stay in
// Infer, outside the model call, exactly where they were. A conversion that let
// the model reach them would mean a job posting's text could decide what gets
// written to the database.

type lookupComparableBandsArgs struct {
	// Title is the job title to find comparable bands for.
	Title string `json:"title" jsonschema_description:"Job title to find comparable salary bands for, e.g. 'Senior Backend Engineer'"`
	// Location is the city or region.
	Location string `json:"location" jsonschema_description:"City or region for the role, e.g. 'Berlin'. Use 'unknown' if not stated."`
}

type getPostingDetailsArgs struct {
	// JobID is the posting to read.
	JobID string `json:"job_id" jsonschema_description:"The id of the job posting to read in full"`
}

// newSalaryToolset builds the exchange's tools. It is a method on the service
// because both lookups read through the service's repository — but note what
// that means for the fence's second documented limit: these handlers are
// closures over `s`, so the fence's import analysis cannot see what they can
// reach through it. That is exactly why the set is two lookups long, both
// obvious reads, and reviewed as such.
func (s *Service) newSalaryToolset() ([]llm.ToolDef, error) {
	comparable := llm.NewTool("lookup_comparable_bands",
		"Look up salary bands already recorded for comparable roles. Returns an empty result when nothing comparable is on file — that is normal and not an error.",
		func(ctx context.Context, args lookupComparableBandsArgs) (string, error) {
			bucket := makeBucket(args.Title, args.Location, "")
			rows, err := s.q.GetSalaryCacheByBucket(ctx, bucket)
			if err != nil {
				return "", fmt.Errorf("reading comparable bands: %w", err)
			}
			type entry struct {
				Source   string `json:"source"`
				Min      int    `json:"min"`
				Max      int    `json:"max"`
				Currency string `json:"currency"`
			}
			out := make([]entry, 0, len(rows))
			for _, r := range rows {
				out = append(out, entry{
					Source:   r.Source,
					Min:      intVal(r.SalaryMin),
					Max:      intVal(r.SalaryMax),
					Currency: r.Currency,
				})
			}
			payload, err := json.Marshal(map[string]any{"bucket": bucket, "bands": out})
			if err != nil {
				return "", err
			}
			return string(payload), nil
		})

	details := llm.NewTool("get_posting_details",
		"Read the full text of a job posting, including the part truncated from the initial summary.",
		func(ctx context.Context, args getPostingDetailsArgs) (string, error) {
			uid, err := dbutil.ParseUUID(args.JobID)
			if err != nil {
				return "", fmt.Errorf("job_id %q is not a valid id", args.JobID)
			}
			job, err := s.q.GetJobByID(ctx, uid)
			if err != nil {
				return "", fmt.Errorf("reading posting %s: %w", args.JobID, err)
			}
			payload, err := json.Marshal(map[string]any{
				"title":       job.Title,
				"company":     job.Company,
				"location":    ptrStr(job.Location),
				"remote":      job.Remote,
				"description": job.Description,
			})
			if err != nil {
				return "", err
			}
			return string(payload), nil
		})

	return []llm.ToolDef{comparable, details}, nil
}
