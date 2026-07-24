import { Printer, AlertTriangle, ChevronDown, ChevronRight, Star } from 'lucide-react';
import { useState } from 'react';
import type {
  InterviewPrepPack,
  InterviewQuestion,
  StoryMapping,
  CompanyNewsItem,
  QuestionCategory,
} from '@job-finder/shared';
import { SectionTitle } from '../../components/layout/PageHeader';
import { Button, Chip, LoadingRegion, ScoreBadge, SkeletonLine, Surface } from '../../components/ui';
import { useInterviewPrep } from './hooks';

const CATEGORY_LABEL: Record<QuestionCategory, string> = {
  technical: 'Technical',
  behavioral: 'Behavioral',
  experience: 'Experience',
  situational: 'Situational',
  company: 'Company',
  gap: 'Gap',
};

const CATEGORY_CHIP_TONE: Record<QuestionCategory, 'green' | 'red' | 'slate'> = {
  technical: 'green',
  behavioral: 'slate',
  experience: 'green',
  situational: 'slate',
  company: 'slate',
  gap: 'red',
};

export default function PrepPackPanel({ jobId }: { jobId: string | undefined }) {
  const { data, isLoading, isError } = useInterviewPrep(jobId);

  if (isLoading) {
    return (
      <Surface>
        <LoadingRegion label="Preparing interview pack…" className="space-y-2">
          <SkeletonLine width="w-1/2" />
          <SkeletonLine width="w-2/3" />
          <SkeletonLine width="w-1/3" />
        </LoadingRegion>
      </Surface>
    );
  }

  if (isError || !data) return null;

  return (
    <Surface className="print:border-none print:shadow-none">
      <div className="mb-3 flex items-center justify-between gap-2">
        <SectionTitle className="mb-0">Interview prep pack</SectionTitle>
        <div className="flex items-center gap-2">
          {data.metadata.totalQuestions > 0 ? (
            <span className="hidden text-xs text-muted sm:inline">
              {data.metadata.coveredQuestions}/{data.metadata.totalQuestions} covered
            </span>
          ) : null}
          <Button variant="secondary" onClick={() => window.print()} aria-label="Print prep pack">
            <Printer className="h-4 w-4" aria-hidden="true" />
            <span className="hidden sm:inline">Print / PDF</span>
          </Button>
        </div>
      </div>

      {data.metadata.uncoveredQuestions > 0 ? (
        <UncoveredGapsBanner count={data.metadata.uncoveredQuestions} />
      ) : null}

      {data.questions.length > 0 ? (
        <div className="space-y-4">
          {data.questions.map((q, i) => (
            <QuestionCard key={q.id} question={q} index={i + 1} />
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted">
          No interview questions derived for this job description yet.
        </p>
      )}

      {data.keywordGap.missingRequired.length > 0 || data.keywordGap.missingPreferred.length > 0 ? (
        <GapSummarySection keywordGap={data.keywordGap} />
      ) : null}

      {data.companyNews.length > 0 ? (
        <CompanyNewsSection items={data.companyNews} stale={data.metadata.staleNews} />
      ) : null}

      <div className="print:hidden mt-4 border-t border-border pt-3 text-center text-xs text-faint">
        Generated {new Date(data.generatedAt).toLocaleString()}
      </div>
    </Surface>
  );
}

function UncoveredGapsBanner({ count }: { count: number }) {
  return (
    <div
      className="mb-4 flex items-start gap-2 rounded-lg border border-warning/30 bg-warning-soft p-3 text-sm text-warning"
      role="alert"
    >
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <div>
        <span className="font-semibold">{count} uncovered {count === 1 ? 'question' : 'questions'}</span>
        <p className="mt-1">
          Write STAR stories in your profile to cover these areas before the interview.
        </p>
      </div>
    </div>
  );
}

function QuestionCard({ question, index }: { question: InterviewQuestion; index: number }) {
  const [expanded, setExpanded] = useState(true);
  const uncovered = question.mappedStories.length === 0;

  return (
    <div className="print:break-inside-avoid rounded-lg border border-border bg-overlay/30 p-3">
      <button
        type="button"
        className="flex w-full items-start gap-2 text-left"
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
      >
        <span className="mt-0.5 shrink-0 text-xs font-bold text-faint">{index}.</span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <Chip tone={CATEGORY_CHIP_TONE[question.category]}>{CATEGORY_LABEL[question.category]}</Chip>
            {uncovered ? (
              <Chip tone="red">uncovered</Chip>
            ) : (
              <Chip tone="green">{question.mappedStories.length} {question.mappedStories.length === 1 ? 'story' : 'stories'}</Chip>
            )}
          </div>
          <p className="mt-1 text-sm font-medium leading-6 text-fg">{question.text}</p>
        </div>
        <span className="mt-1 shrink-0 text-faint">
          {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </span>
      </button>

      {expanded ? (
        <div className="mt-2 space-y-2 pl-6">
          <p className="text-xs italic text-faint">
            from JD: {question.sourceExcerpt}
          </p>

          {uncovered ? (
            <UncoveredQuestionCallout questionText={question.text} />
          ) : (
            <ul className="space-y-2">
              {question.mappedStories.map((story) => (
                <StoryCard key={story.storyId} story={story} />
              ))}
            </ul>
          )}
        </div>
      ) : null}
    </div>
  );
}

function UncoveredQuestionCallout({ questionText }: { questionText: string }) {
  const topicWords = questionText
    .replace(/[?.!,]/g, '')
    .split(' ')
    .filter((w) => w.length > 3)
    .slice(0, 8)
    .join(' ');

  return (
    <div className="rounded-md border border-dashed border-danger/30 bg-danger-soft/50 p-3 text-sm" role="alert">
      <div className="flex items-start gap-2">
        <Star className="mt-0.5 h-4 w-4 shrink-0 text-danger" aria-hidden="true" />
        <div>
          <p className="font-medium text-danger">No matching story</p>
          <p className="mt-1 text-muted">
            Write a STAR story{topicWords ? ` about "${topicWords}…"` : ''} in your profile to cover this question.
          </p>
        </div>
      </div>
    </div>
  );
}

function StoryCard({ story }: { story: StoryMapping }) {
  return (
    <li className="rounded-md border border-border bg-elevated/60 p-3">
      <div className="flex items-start justify-between gap-2">
        <div>
          <p className="text-sm font-medium text-fg">{story.storyTitle}</p>
          <p className="mt-0.5 text-xs text-muted">
            Relevance <ScoreBadge score={Math.round(story.relevanceScore * 100)} />
          </p>
        </div>
      </div>
      <p className="mt-1.5 text-xs leading-5 text-muted">{story.excerpt}</p>
      {story.matchedSkills.length > 0 ? (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {story.matchedSkills.map((skill) => (
            <Chip key={skill} tone="green">{skill}</Chip>
          ))}
        </div>
      ) : null}
    </li>
  );
}

function GapSummarySection({ keywordGap }: { keywordGap: InterviewPrepPack['keywordGap'] }) {
  return (
    <div className="mt-4 rounded-lg border border-border bg-overlay/30 p-3">
      <h3 className="mb-2 text-sm font-semibold text-fg">Skill gap awareness</h3>
      <p className="mb-2 text-xs text-muted">
        {`${keywordGap.coveragePct}% keyword coverage — these gaps may come up in the interview.`}
      </p>
      {keywordGap.missingRequired.length > 0 ? (
        <div className="mb-2">
          <p className="mb-1 text-xs font-semibold text-danger">Required skills missing</p>
          <div className="flex flex-wrap gap-1">
            {keywordGap.missingRequired.map((s) => (
              <Chip key={s} tone="red">{s}</Chip>
            ))}
          </div>
        </div>
      ) : null}
      {keywordGap.missingPreferred.length > 0 ? (
        <div className="mb-2">
          <p className="mb-1 text-xs font-semibold text-muted">Preferred skills missing</p>
          <div className="flex flex-wrap gap-1">
            {keywordGap.missingPreferred.map((s) => (
              <Chip key={s} tone="slate">{s}</Chip>
            ))}
          </div>
        </div>
      ) : null}
      {keywordGap.gapAwarenessTips.length > 0 ? (
        <ul className="space-y-1">
          {keywordGap.gapAwarenessTips.map((tip, i) => (
            <li key={i} className="text-xs leading-5 text-muted">
              {tip}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function CompanyNewsSection({ items, stale }: { items: CompanyNewsItem[]; stale: boolean }) {
  return (
    <div className="mt-4">
      <h3 className="mb-2 text-sm font-semibold text-fg">Company news briefing</h3>
      {stale ? (
        <p className="mb-2 text-xs italic text-warning">News data may be stale — refresh company intel.</p>
      ) : null}
      <div className="space-y-2">
        {items.map((item, i) => (
          <div key={`${item.kind}-${i}`} className="rounded-md border border-border bg-elevated/60 p-3 text-sm">
            <div className="flex items-start justify-between gap-2">
              <div>
                <span className="font-medium text-fg">{item.label}</span>
                <span className="ml-1.5 text-muted">{item.value}</span>
              </div>
              <span className="shrink-0 text-xs text-faint">{item.source}</span>
            </div>
            <p className="mt-0.5 text-xs text-faint">
              as of {new Date(item.fetchedAt).toLocaleDateString()}
            </p>
          </div>
        ))}
      </div>
    </div>
  );
}
