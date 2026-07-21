import { ChevronDown, ChevronUp, ExternalLink, RefreshCw } from 'lucide-react';
import { useState } from 'react';
import { Button, Chip, Spinner, Surface } from '../../components/ui';
import { useCompanyIntel, useRefreshCompanyIntel } from './hooks';

const STALE_MS = 30 * 24 * 60 * 60 * 1000;

function isStale(fetchedAt: string): boolean {
  return Date.now() - new Date(fetchedAt).getTime() > STALE_MS;
}

function oneLineSummary(intel: { companyName: string; funding: string | null; glassdoorRating: number | null }) {
  const parts = [intel.companyName];
  if (intel.funding) parts.push(`funding ${intel.funding}`);
  if (intel.glassdoorRating != null) parts.push(`Glassdoor ${intel.glassdoorRating}`);
  return parts.join(' — ');
}

interface SignalRowProps {
  label: string;
  value: string | number | null;
  stale: boolean;
}

function SignalRow({ label, value, stale }: SignalRowProps) {
  return (
    <div className="flex items-center justify-between gap-2 py-1 text-sm">
      <span className="text-faint">{label}</span>
      <span className="flex items-center gap-1.5 text-fg">
        {value != null ? String(value) : <span className="text-faint">No data yet</span>}
        {stale ? <Chip tone="slate">possibly stale</Chip> : null}
      </span>
    </div>
  );
}

export default function CompanyIntelCard({ jobId }: { jobId: string }) {
  const [expanded, setExpanded] = useState(false);
  const { data: intel, isLoading, isError, refetch } = useCompanyIntel(jobId);
  const refresh = useRefreshCompanyIntel(jobId);

  const handleRefresh = () => {
    refresh.mutate();
  };

  if (isLoading) {
    return (
      <Surface>
        <Spinner label="loading company intel…" />
      </Surface>
    );
  }

  if (isError) {
    return (
      <Surface>
        <div className="flex items-center justify-between gap-2">
          <span className="text-sm text-danger">Failed to load company intel</span>
          <Button variant="secondary" onClick={() => refetch()}>
            retry
          </Button>
        </div>
      </Surface>
    );
  }

  if (!intel) {
    return (
      <Surface>
        <div className="flex flex-col items-center gap-3 py-2 text-center">
          <p className="text-sm text-muted">No company intel yet. Click Refresh to fetch.</p>
          <Button onClick={handleRefresh} disabled={refresh.isPending}>
            {refresh.isPending ? <Spinner /> : <RefreshCw className="h-4 w-4" />}
            Refresh
          </Button>
        </div>
      </Surface>
    );
  }

  const stale = isStale(intel.fetchedAt);

  return (
    <Surface>
      <div className="flex items-center justify-between gap-2">
        <button
          type="button"
          className="flex flex-1 items-center gap-2 text-left"
          onClick={() => setExpanded((v) => !v)}
        >
          <h2 className="font-semibold">Company Intel</h2>
          <span className="text-sm text-muted">{oneLineSummary(intel)}</span>
          {expanded ? (
            <ChevronUp className="ml-auto h-4 w-4 flex-shrink-0 text-faint" />
          ) : (
            <ChevronDown className="ml-auto h-4 w-4 flex-shrink-0 text-faint" />
          )}
        </button>
        <Button variant="secondary" onClick={handleRefresh} disabled={refresh.isPending}>
          {refresh.isPending ? <Spinner /> : <RefreshCw className="h-4 w-4" />}
          Refresh
        </Button>
      </div>

      {expanded ? (
        <div className="mt-3 border-t border-border pt-3">
          {intel.website ? (
            <a
              href={intel.website}
              target="_blank"
              rel="noreferrer"
              className="mb-2 inline-flex items-center gap-1 text-sm text-accent hover:underline"
            >
              {intel.website}
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
          ) : null}
          <div className="divide-y divide-border">
            <SignalRow label="Funding" value={intel.funding} stale={stale} />
            <SignalRow label="Layoffs" value={intel.layoffs} stale={stale} />
            <SignalRow label="Glassdoor rating" value={intel.glassdoorRating} stale={stale} />
            <SignalRow label="Headcount" value={intel.headcount} stale={stale} />
            <SignalRow label="Tech stack" value={intel.techStack} stale={stale} />
          </div>
          {stale ? (
            <p className="mt-2 text-xs text-faint">Data fetched {new Date(intel.fetchedAt).toLocaleDateString()} — may be stale</p>
          ) : null}
        </div>
      ) : null}
    </Surface>
  );
}
