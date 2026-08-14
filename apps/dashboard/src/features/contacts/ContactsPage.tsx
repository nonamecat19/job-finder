import { RefreshCw, Upload, UserRound } from 'lucide-react';
import { useRef } from 'react';
import type { ReferralContactDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, IconTile, ListRow, Tile } from '../../components/layout';
import { VirtualList } from '../../components/VirtualList';
import { Button, Chip, EmptyState, ErrorState, LoadingRegion, Spinner, SkeletonBlock } from '../../components/ui';
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
          <p className="mb-4 [font:var(--type-caption)] text-faint">
            Expected columns (any order, case-insensitive): name, email, company, role, linkedin_url,
            github_username.
          </p>

          {importCsv.isSuccess ? (
            <div className="mb-4 rounded-lg border border-border bg-surface-secondary p-3 text-sm text-muted" role="status">
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
            <VirtualList
              items={contacts}
              getKey={(c) => c.id}
              estimateSize={64}
              gap={4}
              renderItem={(c) => <ContactRow contact={c} />}
            />
          ) : null}
        </Tile>
      </DashboardGrid>
    </div>
  );
}

function ContactRow({ contact }: { contact: ReferralContactDto }) {
  const githubSync = useGithubSync();
  const meta = [contact.role, contact.company].filter(Boolean).join(' · ');

  return (
    <ListRow
      leading={<IconTile icon={UserRound} tint="violet" size="md" />}
      title={
        <span className="flex flex-wrap items-center gap-1.5">
          {contact.name}
          {contact.gitHubUsername ? <Chip tone="slate">@{contact.gitHubUsername}</Chip> : null}
        </span>
      }
      meta={meta || undefined}
      aside={
        contact.gitHubUsername ? (
          <div className="flex shrink-0 flex-col items-end gap-1">
            <Button
              variant="secondary"
              onClick={() => githubSync.mutate(contact.id)}
              disabled={githubSync.isPending}
            >
              {githubSync.isPending ? <Spinner /> : <RefreshCw className="h-4 w-4" aria-hidden="true" />}
              Sync GitHub
            </Button>
            {githubSync.isSuccess && githubSync.variables === contact.id ? (
              <span className="[font:var(--type-caption)] text-faint">
                {githubSync.data.connectionsMade} new connection{githubSync.data.connectionsMade === 1 ? '' : 's'} found
              </span>
            ) : null}
          </div>
        ) : undefined
      }
    />
  );
}
