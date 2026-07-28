import { ChevronDown, ChevronUp, RefreshCw } from 'lucide-react';
import { useState } from 'react';
import type { JobContactDto } from '@job-finder/shared';
import { Button, Chip, LoadingRegion, Spinner, SkeletonCircle, SkeletonLine } from '../../components/ui';
import { useJobContacts, useRefreshJobContacts } from './hooks';

// pickHeadline returns the highest-confidence contact. useJobContacts
// already serves contacts pre-ordered best-first by the API (confidence
// desc, source-priority, name — FR-010/SC-010), so the first entry is the
// headline; this stays a pure function so it is trivially unit-testable
// and doesn't depend on request ordering being preserved by a future change.
export function pickHeadline(contacts: JobContactDto[] | undefined): JobContactDto | null {
  if (!contacts || contacts.length === 0) return null;
  return contacts[0];
}

function headlineLabel(contact: JobContactDto): string {
  return contact.title ? `${contact.name} — ${contact.title}` : contact.name;
}

const SOURCE_LABEL: Record<JobContactDto['source'], string> = {
  posting: 'posting',
  'company-page': 'company page',
  linkedin: 'linkedin',
};

function ContactRow({ contact }: { contact: JobContactDto }) {
  return (
    <li className="flex flex-col gap-1 py-2 text-sm sm:flex-row sm:items-center sm:justify-between">
      <div>
        <span className="font-medium text-foreground">{contact.name}</span>
        {contact.title ? <span className="ml-2 text-muted">{contact.title}</span> : null}
        <div className="mt-0.5 flex flex-wrap gap-x-3 text-xs text-faint">
          {contact.email ? <span>{contact.email}</span> : null}
          {contact.phone ? <span>{contact.phone}</span> : null}
          {contact.linkedInUrl ? (
            <a href={contact.linkedInUrl} target="_blank" rel="noreferrer" className="text-accent hover:underline">
              LinkedIn
            </a>
          ) : null}
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Chip>{SOURCE_LABEL[contact.source]}</Chip>
        <Chip tone="slate">{Math.round(contact.confidence * 100)}% confidence</Chip>
      </div>
    </li>
  );
}

export default function ContactLine({ jobId }: { jobId: string | undefined }) {
  const [expanded, setExpanded] = useState(false);
  const { data: contacts, isLoading } = useJobContacts(jobId);
  const refresh = useRefreshJobContacts(jobId);

  if (isLoading) {
    return (
      <LoadingRegion label="loading contact…" className="flex items-center gap-2">
        <SkeletonCircle size="sm" />
        <SkeletonLine width="w-1/3" />
      </LoadingRegion>
    );
  }

  const headline = pickHeadline(contacts);
  const hasContacts = !!contacts && contacts.length > 0;

  return (
    <>
      <div className="flex items-center justify-between gap-2">
        <button
          type="button"
          className="flex flex-1 items-center gap-2 text-left disabled:cursor-default"
          onClick={() => hasContacts && setExpanded((v) => !v)}
          disabled={!hasContacts}
        >
          {headline ? (
            <span className="text-sm text-foreground" data-testid="contact-headline">
              {headlineLabel(headline)}
              {headline.email ? <span className="ml-2 text-faint">{headline.email}</span> : null}
            </span>
          ) : (
            <span className="text-sm text-faint" data-testid="contact-empty">
              No contact found — try Refresh
            </span>
          )}
          {hasContacts ? (
            expanded ? (
              <ChevronUp className="ml-auto h-4 w-4 flex-shrink-0 text-faint" />
            ) : (
              <ChevronDown className="ml-auto h-4 w-4 flex-shrink-0 text-faint" />
            )
          ) : null}
        </button>
        <Button variant="secondary" onClick={() => refresh.mutate()} disabled={refresh.isPending}>
          {refresh.isPending ? <Spinner /> : <RefreshCw className="h-4 w-4" aria-hidden="true" />}
          Refresh contacts
        </Button>
      </div>

      {expanded && contacts ? (
        <ul className="mt-3 divide-y divide-border border-t border-border" data-testid="contact-list">
          {contacts.map((c) => (
            <ContactRow key={c.id} contact={c} />
          ))}
        </ul>
      ) : null}
    </>
  );
}
