package interviewprep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/keyword"
)

const staleAfter = 30 * 24 * time.Hour

var ErrNoDiff = errors.New("interviewprep: no keyword diff for job")

type JobReader interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
}

type DiffReader interface {
	GetKeywordDiffByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.KeywordDiff, error)
}

type StoriesFunc func(ctx context.Context) ([]keyword.StarStory, error)

type CompanyNewsProvider interface {
	GetIntel(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error)
}

type Service struct {
	jobs    JobReader
	diffs   DiffReader
	stories StoriesFunc
	news    CompanyNewsProvider
}

func NewService(jobs JobReader, diffs DiffReader, stories StoriesFunc, news CompanyNewsProvider) *Service {
	return &Service{jobs: jobs, diffs: diffs, stories: stories, news: news}
}

func (s *Service) Get(ctx context.Context, jobID string) (dto.InterviewPrepPackDto, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return dto.InterviewPrepPackDto{}, ErrNoDiff
	}

	diffRow, err := s.diffs.GetKeywordDiffByJobID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dto.InterviewPrepPackDto{}, ErrNoDiff
		}
		return dto.InterviewPrepPackDto{}, fmt.Errorf("interviewprep: read keyword diff: %w", err)
	}

	matched, err := unmarshalTerms(diffRow.Matched)
	if err != nil {
		return dto.InterviewPrepPackDto{}, fmt.Errorf("interviewprep: decode matched: %w", err)
	}
	missingRequired, err := unmarshalTerms(diffRow.MissingRequired)
	if err != nil {
		return dto.InterviewPrepPackDto{}, fmt.Errorf("interviewprep: decode missingRequired: %w", err)
	}
	missingPreferred, err := unmarshalTerms(diffRow.MissingPreferred)
	if err != nil {
		return dto.InterviewPrepPackDto{}, fmt.Errorf("interviewprep: decode missingPreferred: %w", err)
	}

	job, err := s.jobs.GetJobByID(ctx, uid)
	if err != nil {
		return dto.InterviewPrepPackDto{}, fmt.Errorf("interviewprep: read job: %w", err)
	}

	classified := classifiedFromDiff(matched, missingRequired)
	questions := keyword.DeriveQuestions(classified, job.Description)

	var stories []keyword.StarStory
	if s.stories != nil {
		stories, err = s.stories(ctx)
		if err != nil {
			return dto.InterviewPrepPackDto{}, fmt.Errorf("interviewprep: load stories: %w", err)
		}
	}

	selection := keyword.SelectStories(questions, stories, nil)

	questionDtos := make([]dto.InterviewQuestionDto, 0, len(selection.MappedQuestions))
	covered := 0
	for _, mq := range selection.MappedQuestions {
		if mq.IsCovered {
			covered++
		}
		questionDtos = append(questionDtos, dto.InterviewQuestionDto{
			ID:            mq.Question.ID,
			Text:          mq.Question.Text,
			Category:      string(mq.Question.Category),
			Source:        string(mq.Question.Source),
			SourceExcerpt: mq.Question.SourceExcerpt,
			MappedStories: toStoryMappingDtos(mq.MappedStories),
		})
	}

	news, staleNews := s.companyNews(ctx, jobID)

	coveragePct := 0.0
	if diffRow.CoveragePct != nil {
		coveragePct = *diffRow.CoveragePct
	}

	return dto.InterviewPrepPackDto{
		JobID:       jobID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Questions:   questionDtos,
		CompanyNews: news,
		KeywordGap:  buildKeywordGap(missingRequired, missingPreferred, coveragePct),
		Metadata: dto.InterviewPrepMetadataDto{
			TotalQuestions:     len(questionDtos),
			CoveredQuestions:   covered,
			UncoveredQuestions: len(questionDtos) - covered,
			StaleNews:          staleNews,
		},
	}, nil
}

func classifiedFromDiff(matched, missingRequired []keyword.DiffTerm) []keyword.ClassifiedTerm {
	var out []keyword.ClassifiedTerm
	for _, t := range matched {
		if t.Polarity != keyword.PolarityRequired {
			continue
		}
		out = append(out, classifiedTerm(t))
	}
	for _, t := range missingRequired {
		out = append(out, classifiedTerm(t))
	}
	return out
}

func classifiedTerm(t keyword.DiffTerm) keyword.ClassifiedTerm {
	return keyword.ClassifiedTerm{
		ExtractedTerm: keyword.ExtractedTerm{
			Term:      t.Term,
			Polarity:  t.Polarity,
			Canonical: t.Canonical,
			Stemmed:   t.Stemmed,
			Evidence:  t.Term,
		},
		Class: keyword.ClassMustHave,
	}
}

func toStoryMappingDtos(mappings []keyword.StoryMapping) []dto.StoryMappingDto {
	out := make([]dto.StoryMappingDto, 0, len(mappings))
	for _, m := range mappings {
		out = append(out, dto.StoryMappingDto{
			StoryID:        m.StoryID,
			StoryTitle:     m.StoryTitle,
			RelevanceScore: m.RelevanceScore,
			MatchedSkills:  m.MatchedSkills,
			Excerpt:        m.Excerpt,
		})
	}
	return out
}

func buildKeywordGap(missingRequired, missingPreferred []keyword.DiffTerm, coveragePct float64) dto.KeywordGapSummaryDto {
	required := termStrings(missingRequired)
	preferred := termStrings(missingPreferred)

	var tips []string
	if len(required) > 0 {
		tips = append(tips, fmt.Sprintf("Be ready to address gaps in %s — they may come up directly.", joinTerms(required)))
	}
	if len(preferred) > 0 {
		tips = append(tips, fmt.Sprintf("%s are preferred but not required — mention any exposure, even informal.", joinTerms(preferred)))
	}

	return dto.KeywordGapSummaryDto{
		MissingRequired:  required,
		MissingPreferred: preferred,
		CoveragePct:      coveragePct,
		GapAwarenessTips: tips,
	}
}

func termStrings(terms []keyword.DiffTerm) []string {
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		out = append(out, t.Canonical)
	}
	return out
}

func joinTerms(terms []string) string {
	switch len(terms) {
	case 0:
		return ""
	case 1:
		return terms[0]
	default:
		out := terms[0]
		for _, t := range terms[1 : len(terms)-1] {
			out += ", " + t
		}
		return out + " and " + terms[len(terms)-1]
	}
}

func (s *Service) companyNews(ctx context.Context, jobID string) ([]dto.CompanyNewsItemDto, bool) {
	if s.news == nil {
		return nil, false
	}
	intel, err := s.news.GetIntel(ctx, jobID)
	if err != nil || intel == nil {
		return nil, false
	}

	fetchedAt, staleErr := time.Parse(time.RFC3339, intel.FetchedAt)
	stale := staleErr == nil && time.Since(fetchedAt) > staleAfter

	var items []dto.CompanyNewsItemDto
	add := func(kind, label, value string) {
		if value == "" {
			return
		}
		items = append(items, dto.CompanyNewsItemDto{
			Kind:      kind,
			Label:     label,
			Value:     value,
			Source:    "company-intel",
			FetchedAt: intel.FetchedAt,
		})
	}
	if intel.Funding != nil {
		add("funding", "Funding", *intel.Funding)
	}
	if intel.Layoffs != nil {
		add("layoffs", "Layoffs", *intel.Layoffs)
	}
	if intel.GlassdoorRating != nil {
		add("glassdoor_rating", "Glassdoor rating", fmt.Sprintf("%.1f", *intel.GlassdoorRating))
	}
	if intel.Headcount != nil {
		add("headcount", "Headcount", *intel.Headcount)
	}
	if intel.TechStack != nil {
		add("tech_stack", "Tech stack", *intel.TechStack)
	}

	return items, stale
}

func unmarshalTerms(raw []byte) ([]keyword.DiffTerm, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var terms []keyword.DiffTerm
	if err := json.Unmarshal(raw, &terms); err != nil {
		return nil, err
	}
	return terms, nil
}
