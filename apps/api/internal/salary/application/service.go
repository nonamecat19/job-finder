// Package application holds the salary use-case: infer a job's salary band
// from its own salaryRaw text, an ingested-cache/levels.fyi bucket lookup, or
// an LLM estimate, in that priority order. Mirrors the salary inference flow.
package application

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/salary/domain"
)

type Service struct {
	q           domain.Repository
	llmc        llm.Provider
	levelsFyi   *LevelsFyiLoader
	salaryModel string
}

func NewService(q domain.Repository, llmc llm.Provider, levelsFyi *LevelsFyiLoader, salaryModel string) *Service {
	return &Service{q: q, llmc: llmc, levelsFyi: levelsFyi, salaryModel: salaryModel}
}

func (s *Service) Infer(ctx context.Context, jobID string) error {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return err
	}
	job, err := s.q.GetJobByID(ctx, uid)
	if err != nil {
		return fmt.Errorf("salary: job %s not found: %w", jobID, err)
	}

	bucket := makeBucket(job.Title, ptrStr(job.Location), "")

	if job.SalaryRaw != nil && *job.SalaryRaw != "" {
		if band, ok := domain.ParseSalaryRaw(*job.SalaryRaw); ok {
			slog.Info("salary: parsed from salaryRaw", "job", jobID, "min", band.Min, "max", band.Max, "currency", band.Currency)
			if err := s.persistJobSalary(ctx, uid, band); err != nil {
				return err
			}
			_ = s.upsertCache(ctx, bucket, band)
			return nil
		}
	}

	cacheBand, cacheOk := s.lookupCache(ctx, bucket, domain.SourceIngestedCache)
	levelsBand, levelsOk := s.lookupCache(ctx, bucket, domain.SourceLevelsFyi)

	if cacheOk && levelsOk {
		blended := blend(cacheBand, levelsBand)
		slog.Info("salary: blended cache + levels.fyi", "job", jobID, "min", blended.Min, "max", blended.Max, "currency", blended.Currency, "confidence", blended.Confidence)
		return s.persistJobSalary(ctx, uid, blended)
	}

	if cacheOk {
		slog.Info("salary: cache hit", "job", jobID, "min", cacheBand.Min, "max", cacheBand.Max, "currency", cacheBand.Currency)
		return s.persistJobSalary(ctx, uid, cacheBand)
	}

	if levelsOk {
		slog.Info("salary: levels.fyi hit", "job", jobID, "min", levelsBand.Min, "max", levelsBand.Max, "currency", levelsBand.Currency)
		return s.persistJobSalary(ctx, uid, levelsBand)
	}

	band, err := s.llmInfer(ctx, job)
	if err != nil {
		return fmt.Errorf("salary: LLM inference failed: %w", err)
	}
	slog.Info("salary: LLM inferred", "job", jobID, "min", band.Min, "max", band.Max, "currency", band.Currency, "confidence", band.Confidence)
	return s.persistJobSalary(ctx, uid, band)
}

func (s *Service) lookupCache(ctx context.Context, bucket string, source domain.SalarySource) (domain.SalaryBand, bool) {
	rows, err := s.q.GetSalaryCacheByBucket(ctx, bucket)
	if err != nil || len(rows) == 0 {
		return domain.SalaryBand{}, false
	}
	for _, r := range rows {
		if r.Source == string(source) {
			return domain.SalaryBand{
				Min:        intVal(r.SalaryMin),
				Max:        intVal(r.SalaryMax),
				Currency:   r.Currency,
				Confidence: 0.7,
				Source:     source,
			}, true
		}
	}
	return domain.SalaryBand{}, false
}

func (s *Service) llmInfer(ctx context.Context, job sqlcgen.Job) (domain.SalaryBand, error) {
	location := "n/a"
	if job.Location != nil && *job.Location != "" {
		location = *job.Location
	}
	description := job.Description
	if len(description) > 4000 {
		description = description[:4000]
	}

	prompt := fmt.Sprintf(
		"Estimate the annual salary range for this job in the local currency.\n\n"+
			"JOB:\nTitle: %s\nCompany: %s\nLocation: %s (remote: %v)\nDescription:\n%s\n\n"+
			"Return a single SalaryBand JSON object. Use the most likely currency for the location. "+
			"Set confidence based on how certain you are (0.3-0.9). "+
			"If you cannot make a reasonable estimate, set min=0, max=0, confidence=0.",
		job.Title, job.Company, location, job.Remote, description,
	)

	model := s.salaryModel
	if model == "" {
		model = s.llmc.ModelName()
	}

	band, err := llm.CompleteStructured[domain.SalaryBand](ctx, s.llmc, prompt, &llm.CompleteOptions{
		System: "You are a compensation analyst. Estimate realistic salary ranges based on job title, company, and location.",
		Model:  model,
	})
	if err != nil {
		return domain.SalaryBand{}, err
	}

	band.Source = domain.SourceLLM
	if band.Confidence == 0 {
		band.Confidence = 0.3
	}
	return band, nil
}

func (s *Service) persistJobSalary(ctx context.Context, uid pgtype.UUID, band domain.SalaryBand) error {
	return s.q.UpdateJobSalary(ctx, sqlcgen.UpdateJobSalaryParams{
		ID:               uid,
		SalaryMin:        int32Ptr(int32(band.Min)),
		SalaryMax:        int32Ptr(int32(band.Max)),
		SalaryCurrency:   strPtr(band.Currency),
		SalaryConfidence: float64Ptr(band.Confidence),
		SalarySource:     strPtr(string(band.Source)),
	})
}

func (s *Service) upsertCache(ctx context.Context, bucket string, band domain.SalaryBand) error {
	return s.q.UpsertSalaryCache(ctx, sqlcgen.UpsertSalaryCacheParams{
		Bucket:     bucket,
		SalaryMin:  int32Ptr(int32(band.Min)),
		SalaryMax:  int32Ptr(int32(band.Max)),
		Currency:   band.Currency,
		Source:     string(band.Source),
		SampleSize: 1,
	})
}

func blend(a, b domain.SalaryBand) domain.SalaryBand {
	wA := a.Confidence / (a.Confidence + b.Confidence)
	wB := b.Confidence / (a.Confidence + b.Confidence)

	min := int(math.Round(float64(a.Min)*wA + float64(b.Min)*wB))
	max := int(math.Round(float64(a.Max)*wA + float64(b.Max)*wB))
	confidence := math.Min(a.Confidence+b.Confidence, 1.0)

	currency := a.Currency
	if currency == "" {
		currency = b.Currency
	}

	return domain.SalaryBand{
		Min:        min,
		Max:        max,
		Currency:   currency,
		Confidence: confidence,
		Source:     domain.SourceBlended,
	}
}

func makeBucket(title, location, companySize string) string {
	titleSlug := slugify(title)
	geoSlug := slugify(location)
	sizeSlug := slugify(companySize)
	if sizeSlug == "" {
		sizeSlug = "unknown"
	}
	return titleSlug + "|" + geoSlug + "|" + sizeSlug
}

func slugify(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ",", "")
	return s
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intVal(v *int32) int {
	if v == nil {
		return 0
	}
	return int(*v)
}

func int32Ptr(v int32) *int32 {
	return &v
}

func strPtr(v string) *string {
	return &v
}

func float64Ptr(v float64) *float64 {
	return &v
}
