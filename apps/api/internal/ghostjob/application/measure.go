package application

import (
	"context"
	"strings"
	"time"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/ghostjob/domain"
)

func MeasureSignals(ctx context.Context, repo domain.Repository, job sqlcgen.Job) domain.GhostSignals {
	notes := map[string]string{}

	repostCount := measureRepostCount(ctx, repo, job, notes)
	daysOpen := measureDaysOpen(job, notes)
	crossBoard := measureCrossBoard(ctx, repo, job, notes)
	alwaysHiring := measureAlwaysHiring(ctx, repo, job, notes)

	return domain.GhostSignals{
		RepostCount:       repostCount,
		DaysOpen:          daysOpen,
		CrossBoardCount:   crossBoard,
		AlwaysHiringCount: alwaysHiring,
		Notes:             notes,
	}
}

func measureRepostCount(ctx context.Context, repo domain.Repository, job sqlcgen.Job, notes map[string]string) int {
	n, err := repo.CountRepostsByDedupeKey(ctx, job.DedupeKey)
	if err != nil || n < 1 {
		notes["repost"] = "unknown: query failed, defaulted to 1"
		return 1
	}
	notes["repost"] = "measured"
	return int(n)
}

func measureDaysOpen(job sqlcgen.Job, notes map[string]string) *int {
	if !job.PostedAt.Valid {
		notes["daysOpen"] = "unknown: no postedAt"
		return nil
	}
	d := int(time.Since(job.PostedAt.Time).Hours() / 24)
	if d < 0 {
		d = 0
	}
	notes["daysOpen"] = "measured"
	return &d
}

func measureCrossBoard(ctx context.Context, repo domain.Repository, job sqlcgen.Job, notes map[string]string) *int {
	desc := strings.TrimSpace(job.Description)
	if len(desc) < domain.MinHashableDescriptionLen {
		notes["crossBoard"] = "unknown: description empty or a teaser"
		return nil
	}

	candidates, err := repo.ListJobsForCrossBoardCheck(ctx, job.ID)
	if err != nil {
		notes["crossBoard"] = "unknown: query failed"
		return nil
	}

	targetHash := domain.Hash(desc)
	sources := map[string]struct{}{}
	for _, c := range candidates {
		if c.SourceKey == job.SourceKey {
			continue
		}
		candidateDesc := strings.TrimSpace(c.Description)
		if len(candidateDesc) < domain.MinHashableDescriptionLen {
			continue
		}
		if domain.HammingDistance(targetHash, domain.Hash(candidateDesc)) <= domain.CrossBoardSimilarityThreshold {
			sources[c.SourceKey] = struct{}{}
		}
	}
	n := len(sources)
	notes["crossBoard"] = "measured"
	return &n
}

func measureAlwaysHiring(ctx context.Context, repo domain.Repository, job sqlcgen.Job, notes map[string]string) *int {
	company := strings.TrimSpace(job.Company)
	if !domain.IsUsableCompanyName(company) {
		notes["alwaysHiring"] = "unknown: company name unparseable"
		return nil
	}

	n, err := repo.CountAlwaysHiringByCompany(ctx, company)
	if err != nil {
		notes["alwaysHiring"] = "unknown: query failed"
		return nil
	}
	v := int(n)
	notes["alwaysHiring"] = "measured"
	return &v
}
