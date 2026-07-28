import { Copy } from 'lucide-react';
import type { KeywordDiffResponse, KeywordDiffTerm, KeywordRephraseSuggestion } from '@job-finder/shared';
import { Button, Chip, EmptyState, ScoreBadge } from '../../components/ui';
import { emitToast, toErrorMessage } from '../../lib/toastBus';
import { useJobKeywordDiff } from './hooks';

export default function KeywordDiffPanel({ jobId }: { jobId: string | undefined }) {
  const { data, isLoading, isError } = useJobKeywordDiff(jobId);

  if (isLoading) return null;

  if (isError || !data) {
    return (
      <EmptyState>
        No keyword analysis yet. Run the keyword diff from the job page to compare your
        resume against this job description.
      </EmptyState>
    );
  }

  const { matched, missingRequired, missingPreferred, metadata, suggestions } = data;

  // Index suggestions by canonical term so each missing-required row can show
  // its own rephrase inline. Canonical is unique per bucket.
  const suggestionByCanonical = new Map<string, KeywordRephraseSuggestion>();
  for (const s of suggestions) suggestionByCanonical.set(s.canonical, s);

  return (
    <>
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-xs text-muted">
          <span>
            {metadata.matchedRequired + metadata.matchedPreferred}/
            {metadata.totalRequired + metadata.totalPreferred} terms
          </span>
          <ScoreBadge score={Math.round(metadata.coveragePct)} />
        </div>
      </div>

      <div className="space-y-4">
        <TermGroup
          title="Matched"
          emptyLabel="No matched terms."
          terms={matched}
          tone="green"
        />

        <div>
          <GroupHeading title="Missing — required" count={missingRequired.length} />
          {missingRequired.length === 0 ? (
            <p className="text-sm text-muted">No missing required terms — strong ATS coverage.</p>
          ) : (
            <ul className="space-y-2">
              {missingRequired.map((term, i) => (
                <MissingRequiredRow
                  key={`mr-${term.canonical}-${i}`}
                  term={term}
                  suggestion={suggestionByCanonical.get(term.canonical)}
                />
              ))}
            </ul>
          )}
        </div>

        <TermGroup
          title="Missing — preferred"
          emptyLabel="No missing preferred terms."
          terms={missingPreferred}
          tone="slate"
        />
      </div>
    </>
  );
}

function GroupHeading({ title, count }: { title: string; count: number }) {
  return (
    <h3 className="mb-2 text-sm font-semibold text-foreground">
      {title} <span className="font-normal text-faint">({count})</span>
    </h3>
  );
}

function TermGroup({
  title,
  emptyLabel,
  terms,
  tone,
}: {
  title: string;
  emptyLabel: string;
  terms: KeywordDiffTerm[];
  tone: 'green' | 'red' | 'slate';
}) {
  return (
    <div>
      <GroupHeading title={title} count={terms.length} />
      {terms.length === 0 ? (
        <p className="text-sm text-muted">{emptyLabel}</p>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {terms.map((term, i) => (
            <Chip key={`${title}-${term.canonical}-${i}`} tone={tone}>
              {term.canonical}
            </Chip>
          ))}
        </div>
      )}
    </div>
  );
}

function MissingRequiredRow({
  term,
  suggestion,
}: {
  term: KeywordDiffTerm;
  suggestion: KeywordRephraseSuggestion | undefined;
}) {
  const rephrase = suggestion?.rephrase ?? null;

  return (
    <li className="rounded-md border border-border bg-surface-tertiary/40 p-3">
      <div className="flex items-center gap-2">
        <Chip tone="red">{term.canonical}</Chip>
      </div>

      {rephrase ? (
        <div className="mt-2 space-y-1">
          <div className="flex items-start justify-between gap-2">
            <p className="text-sm leading-6 text-foreground">{rephrase}</p>
            <CopyButton text={rephrase} />
          </div>
          {suggestion?.sourceBullet ? (
            <p className="text-xs text-faint">from: {suggestion.sourceBullet}</p>
          ) : null}
        </div>
      ) : (
        <p className="mt-2 text-xs italic text-faint">
          No honest rephrase available — add this experience only if you truly have it.
        </p>
      )}
    </li>
  );
}

function CopyButton({ text }: { text: string }) {
  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      emitToast({ title: 'Suggestion copied', variant: 'success' });
    } catch (err) {
      emitToast({ title: 'Copy failed', description: toErrorMessage(err), variant: 'error' });
    }
  };

  return (
    <Button variant="secondary" onClick={onCopy} aria-label="Copy suggestion">
      copy <Copy className="h-3.5 w-3.5" aria-hidden="true" />
    </Button>
  );
}
