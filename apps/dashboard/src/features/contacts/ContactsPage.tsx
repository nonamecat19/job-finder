import { RefreshCw, Upload } from 'lucide-react';
import { useRef } from 'react';
import type { ReferralContactDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, Tile } from '../../components/layout';
import { VirtualList } from '../../components/VirtualList';
import { Button, Chip, EmptyState, ErrorState, LoadingRegion, Spinner, SkeletonBlock, Surface } from '../../components/ui';
import { useContacts, useGithubSync, useImportContactsCSV } from './hooks';

export default function ContactsPage() {
  const { data: contacts, isLoading, error } = useContacts();
  const importCsv = useImportContactsCSV();
  const fileRef = useRef<HTMLInputElement>(null);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    importCsv.mutate(file);
    e.target.value = '';
  };

  return (
    <div>
      <PageHeader
        title="Contacts"
        description="Import your professional network from a CSV export, then run GitHub cross-reference to discover warm connections into a company."
        actions={
          <>
            <input
              ref={fileRef}
              type="file"
              accept=".csv,text/csv"
              className="hidden"
              onChange={handleFileChange}
            />
            <Button onClick={() => fileRef.current?.click()} disabled={importCsv.isPending}>
              {importCsv.isPending ? <Spinner /> : <Upload className="h-4 w-4" aria-hidden="true" />}
              Import CSV
            </Button>
          </>
        }
      />

      <DashboardGrid>
        <Tile span="full" title="Contacts">

      <p className="mb-4 text-xs text-faint">
        Expected columns (any order, case-insensitive): name, email, company, role, linkedin_url, github_username.
      </p>

      {importCsv.isSuccess ? (
        <div className="mb-4 rounded-md border border-border bg-surface-secondary/60 p-3 text-sm text-muted" role="status">
          Imported {importCsv.data.imported} of {importCsv.data.total} contacts
          {importCsv.data.skipped > 0 ? ` (${importCsv.data.skipped} skipped)` : ''}.
        </div>
      ) : null}
      {importCsv.error ? <ErrorState error={importCsv.error} /> : null}

      {error ? <ErrorState error={error} /> : null}
      {isLoading ? (
        <LoadingRegion label="loading contacts…" className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <SkeletonBlock key={i} className="h-10 w-full" />
          ))}
        </LoadingRegion>
      ) : null}

      {!isLoading && contacts && contacts.length === 0 ? (
        <EmptyState>No contacts yet. Import a CSV to get started.</EmptyState>
      ) : null}

      {contacts && contacts.length > 0 ? (
        <Surface className="p-0">
          <VirtualList
            items={contacts}
            getKey={(c) => c.id}
            estimateSize={48}
            gap={0}
            className="px-4"
            renderItem={(c) => <ContactRow contact={c} />}
          />
        </Surface>
      ) : null}
        </Tile>
      </DashboardGrid>
    </div>
  );
}

function ContactRow({ contact }: { contact: ReferralContactDto }) {
  const githubSync = useGithubSync();

  return (
    <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border py-2 text-sm last:border-0">
      <div>
        <span className="font-medium text-foreground">{contact.name}</span>
        {contact.role ? <span className="ml-1.5 text-muted">{contact.role}</span> : null}
        {contact.company ? <span className="ml-1.5 text-faint">· {contact.company}</span> : null}
        {contact.gitHubUsername ? (
          <span className="ml-1.5">
            <Chip tone="slate">@{contact.gitHubUsername}</Chip>
          </span>
        ) : null}
      </div>
      {contact.gitHubUsername ? (
        <Button
          variant="secondary"
          onClick={() => githubSync.mutate(contact.id)}
          disabled={githubSync.isPending}
        >
          {githubSync.isPending ? <Spinner /> : <RefreshCw className="h-4 w-4" aria-hidden="true" />}
          Sync GitHub
        </Button>
      ) : null}
      {githubSync.isSuccess && githubSync.variables === contact.id ? (
        <span className="w-full text-xs text-faint">
          {githubSync.data.connectionsMade} new connection{githubSync.data.connectionsMade === 1 ? '' : 's'} found
        </span>
      ) : null}
    </div>
  );
}
