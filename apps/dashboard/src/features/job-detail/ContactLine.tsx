import { RefreshCw } from 'lucide-react';
import type { JobContactDto } from '@job-finder/shared';
import { Button, Spinner, Surface } from '../../components/ui';
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

export default function ContactLine({ jobId }: { jobId: string | undefined }) {
  const { data: contacts, isLoading } = useJobContacts(jobId);
  const refresh = useRefreshJobContacts(jobId);

  if (isLoading) {
    return (
      <Surface>
        <Spinner label="loading contact…" />
      </Surface>
    );
  }

  const headline = pickHeadline(contacts);

  return (
    <Surface>
      <div className="flex items-center justify-between gap-2">
        <h2 className="font-semibold">Contact</h2>
        <div className="flex items-center gap-3">
          {headline ? (
            <span className="text-sm text-fg" data-testid="contact-headline">
              {headlineLabel(headline)}
              {headline.email ? <span className="ml-2 text-faint">{headline.email}</span> : null}
            </span>
          ) : (
            <span className="text-sm text-faint" data-testid="contact-empty">
              No contact found — try Refresh
            </span>
          )}
          <Button variant="secondary" onClick={() => refresh.mutate()} disabled={refresh.isPending}>
            {refresh.isPending ? <Spinner /> : <RefreshCw className="h-4 w-4" aria-hidden="true" />}
            Refresh contacts
          </Button>
        </div>
      </div>
    </Surface>
  );
}
