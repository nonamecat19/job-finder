package ghostjob

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// GhostSignals is the deterministic measurement bundle computed by SQL (plus
// simhash) before any model call, handed to the prompt verbatim so the model
// blends rather than invents (FR-019). Every unmeasurable signal is nil
// *plus* an entry in Notes — never 0, never a guess (FR-011).
type GhostSignals struct {
	// RepostCount is always measurable (>=1: the job's own appearance).
	RepostCount int
	// DaysOpen is nil when Job.postedAt is null (edge case: no posting date).
	DaysOpen *int
	// CrossBoardCount is nil when the description is empty/a teaser.
	CrossBoardCount *int
	// AlwaysHiringCount is nil when the company name is unparseable.
	AlwaysHiringCount *int
	// Notes carries per-signal provenance: "measured" or "unknown: <reason>".
	Notes map[string]string
}

// AllOptionalSignalsUnknown reports whether every signal beyond the job's
// own repost count of 1 is unmeasurable — the "every signal is unknown"
// edge case where the service declines to score entirely (no LLM call, no
// row written; keeps SC-003 true).
func (g GhostSignals) AllOptionalSignalsUnknown() bool {
	return g.RepostCount <= 1 && g.DaysOpen == nil && g.CrossBoardCount == nil && g.AlwaysHiringCount == nil
}

// HasUnknownSignal reports whether any of the three optional signals could
// not be measured, used to cap an overconfident model result (FR-011).
func (g GhostSignals) HasUnknownSignal() bool {
	return g.DaysOpen == nil || g.CrossBoardCount == nil || g.AlwaysHiringCount == nil
}

// MeasureSignals computes GhostSignals for one job from data already held —
// no scraping, no employer contact, no third-party enrichment (FR-016).
func MeasureSignals(ctx context.Context, repo Repository, job sqlcgen.Job) GhostSignals {
	notes := map[string]string{}

	repostCount := measureRepostCount(ctx, repo, job, notes)
	daysOpen := measureDaysOpen(job, notes)
	crossBoard := measureCrossBoard(ctx, repo, job, notes)
	alwaysHiring := measureAlwaysHiring(ctx, repo, job, notes)

	return GhostSignals{
		RepostCount:       repostCount,
		DaysOpen:          daysOpen,
		CrossBoardCount:   crossBoard,
		AlwaysHiringCount: alwaysHiring,
		Notes:             notes,
	}
}

// measureRepostCount is always measurable: the job's own appearance counts
// as 1, and CountRepostsByDedupeKey (backed by "seenCount") only ever grows
// from there. A query failure degrades to 1 rather than failing the whole
// measurement (FR-018): the job's own existence is proof enough of "seen
// at least once".
func measureRepostCount(ctx context.Context, repo Repository, job sqlcgen.Job, notes map[string]string) int {
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

func measureCrossBoard(ctx context.Context, repo Repository, job sqlcgen.Job, notes map[string]string) *int {
	desc := strings.TrimSpace(job.Description)
	if len(desc) < MinHashableDescriptionLen {
		notes["crossBoard"] = "unknown: description empty or a teaser"
		return nil
	}

	candidates, err := repo.ListJobsForCrossBoardCheck(ctx, job.ID)
	if err != nil {
		notes["crossBoard"] = "unknown: query failed"
		return nil
	}

	targetHash := Hash(desc)
	sources := map[string]struct{}{}
	for _, c := range candidates {
		if c.SourceKey == job.SourceKey {
			continue // same board isn't "cross-board"
		}
		candidateDesc := strings.TrimSpace(c.Description)
		if len(candidateDesc) < MinHashableDescriptionLen {
			continue
		}
		if HammingDistance(targetHash, Hash(candidateDesc)) <= CrossBoardSimilarityThreshold {
			sources[c.SourceKey] = struct{}{}
		}
	}
	n := len(sources)
	notes["crossBoard"] = "measured"
	return &n
}

func measureAlwaysHiring(ctx context.Context, repo Repository, job sqlcgen.Job, notes map[string]string) *int {
	company := strings.TrimSpace(job.Company)
	if !isUsableCompanyName(company) {
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

// isUsableCompanyName reports whether company is a real, groupable company
// name: not empty/whitespace-only, not punctuation-only, and not the
// ingestion placeholder "Unknown" (case-insensitive). A placeholder value
// must never cause every unnamed company's jobs to be grouped as one
// employer (spec edge case).
func isUsableCompanyName(company string) bool {
	if company == "" {
		return false
	}
	if strings.EqualFold(company, "unknown") {
		return false
	}
	for _, r := range company {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false // punctuation-only
}
