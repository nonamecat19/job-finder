package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/ghostjob/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

const Kind = "ghost"

const confidenceCapWhenUnknown = 0.6

var ErrDeclinedToScore = errors.New("ghostjob: insufficient signal to score this job")

type Service struct {
	q          domain.Repository
	llmc       llm.Provider
	ghostModel string
}

func NewService(q domain.Repository, llmc llm.Provider, ghostModel string) *Service {
	return &Service{q: q, llmc: llmc, ghostModel: ghostModel}
}

func (s *Service) modelName() string {
	if s.ghostModel != "" {
		return s.ghostModel
	}
	return s.llmc.ModelName()
}

func (s *Service) ScoreJob(ctx context.Context, jobID string) (dto.JobSignalDto, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return dto.JobSignalDto{}, err
	}
	job, err := s.q.GetJobByID(ctx, uid)
	if err != nil {
		return dto.JobSignalDto{}, fmt.Errorf("ghostjob: job %s not found: %w", jobID, err)
	}

	signals := MeasureSignals(ctx, s.q, job)
	if signals.AllOptionalSignalsUnknown() {
		return dto.JobSignalDto{}, ErrDeclinedToScore
	}

	prompt := buildPrompt(job, signals)
	result, err := llm.CompleteStructured[domain.GhostJobResult](ctx, s.llmc, prompt, &llm.CompleteOptions{
		System: "You are a skeptical but fair job-market analyst. You judge only from the " +
			"measured numbers given to you. You never assert anything about the employer, " +
			"the role, or hiring intent that the numbers do not support.",
		Model: s.ghostModel,
	})
	if err != nil {
		return dto.JobSignalDto{}, fmt.Errorf("ghostjob: scoring failed: %w", err)
	}

	confidence := result.Confidence
	if signals.HasUnknownSignal() && confidence > confidenceCapWhenUnknown {
		confidence = confidenceCapWhenUnknown
	}

	breakdown := dto.GhostSignalBreakdownDto{
		RepostCount:       signals.RepostCount,
		DaysOpen:          signals.DaysOpen,
		CrossBoardCount:   signals.CrossBoardCount,
		AlwaysHiringCount: signals.AlwaysHiringCount,
		Confidence:        confidence,
		Explanation:       result.Explanation,
		TopSignals:        result.TopSignals,
		Notes:             signals.Notes,
	}
	raw, err := dbutil.MarshalJSONB(breakdown)
	if err != nil {
		return dto.JobSignalDto{}, fmt.Errorf("ghostjob: marshal signals: %w", err)
	}

	score := int32(math.Round(result.Score))
	model := s.modelName()
	row, err := s.q.UpsertJobSignal(ctx, sqlcgen.UpsertJobSignalParams{
		JobId:   uid,
		Kind:    Kind,
		Score:   score,
		Signals: raw,
		Model:   &model,
	})
	if err != nil {
		return dto.JobSignalDto{}, fmt.Errorf("ghostjob: upsert: %w", err)
	}
	return toDto(row), nil
}

func buildPrompt(job sqlcgen.Job, s domain.GhostSignals) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Rate how likely this job posting is a \"ghost job\" — a posting the "+
		"employer has no real intent to fill. Score 0-100, where higher means more "+
		"suspicious. Base your score ONLY on the measured signals below; do not invent "+
		"facts about the employer, the role, or their hiring intent.\n\n")
	fmt.Fprintf(&b, "JOB:\nTitle: %s\nCompany: %s\n\n", job.Title, job.Company)
	fmt.Fprintf(&b, "MEASURED SIGNALS:\n")
	fmt.Fprintf(&b, "- Repost count (times this exact posting has reappeared across "+
		"ingestion runs): %d\n", s.RepostCount)
	if s.DaysOpen != nil {
		fmt.Fprintf(&b, "- Days open: %d (a posting older than 45 days with no user "+
			"progression contributes to suspicion, but age alone is never sufficient for "+
			"a high score)\n", *s.DaysOpen)
	} else {
		fmt.Fprintf(&b, "- Days open: unknown (no posting date available) — treat this as "+
			"missing information, not as zero or infinite, and lower your confidence\n")
	}
	if s.CrossBoardCount != nil {
		fmt.Fprintf(&b, "- Cross-board duplicates (same description on other sources, last "+
			"60 days): %d. IMPORTANT: a legitimate recruiter or agency often cross-posts "+
			"one JD to several boards — this signal alone, even if nonzero, MUST NOT push "+
			"the score into the 80-100 band. It only matters when it compounds with repost "+
			"count or the always-hiring count.\n", *s.CrossBoardCount)
	} else {
		fmt.Fprintf(&b, "- Cross-board duplicates: unknown (description too short/empty) — "+
			"lower your confidence\n")
	}
	if s.AlwaysHiringCount != nil {
		fmt.Fprintf(&b, "- Always-hiring count (other postings from this company in the "+
			"last 90 days that never progressed past initial discovery, including this "+
			"one): %d. IMPORTANT: a value of 1 means this is the ONLY posting seen from "+
			"this company — that is the normal case, not a red flag, and MUST NOT be "+
			"treated as evidence of ghosting.\n", *s.AlwaysHiringCount)
	} else {
		fmt.Fprintf(&b, "- Always-hiring count: unknown (company name unparseable) — lower "+
			"your confidence\n")
	}
	fmt.Fprintf(&b, "\nReport your confidence (0-1) in the score, lowering it for every "+
		"signal marked unknown above. Explain, in plain English, which of the measured "+
		"signals drove your score — reference only the numbers given.")
	return b.String()
}

func toDto(r sqlcgen.JobSignal) dto.JobSignalDto {
	var breakdown dto.GhostSignalBreakdownDto
	_ = dbutil.UnmarshalJSONB(r.Signals, &breakdown)
	var model string
	if r.Model != nil {
		model = *r.Model
	}
	return dto.JobSignalDto{
		ID:        dbutil.UUIDString(r.ID),
		JobID:     dbutil.UUIDString(r.JobId),
		Kind:      r.Kind,
		Score:     int(r.Score),
		Model:     model,
		CreatedAt: dbutil.Timestamp(r.CreatedAt),
		Signals:   breakdown,
	}
}
