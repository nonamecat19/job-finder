import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ActivityRunDto } from '@job-finder/shared';
import StatusPage from './StatusPage';

vi.mock('./hooks', () => ({
  useActivity: vi.fn(),
  useRetryActivity: vi.fn(),
  useCancelActivity: vi.fn(),
  useCancelAllActivity: vi.fn(),
  useQueueBacklog: vi.fn(),
}));

vi.mock('../../components/VirtualList', () => ({
  VirtualList: ({ items, getKey, renderItem }: any) => (
    <div>{items.map((item: any, i: number) => <div key={getKey(item, i)}>{renderItem(item, i)}</div>)}</div>
  ),
}));

import { useActivity, useRetryActivity, useCancelActivity, useCancelAllActivity, useQueueBacklog } from './hooks';

function run(overrides: Partial<ActivityRunDto>): ActivityRunDto {
  return {
    id: 'run-1',
    op: 'match',
    state: 'failed',
    label: 'Some Job',
    step: null,
    jobId: null,
    sourceKey: null,
    refId: null,
    error: null,
    meta: {},
    createdAt: new Date().toISOString(),
    startedAt: new Date().toISOString(),
    finishedAt: new Date().toISOString(),
    elapsedMs: 1000,
    heartbeatAt: null,
    timeoutMs: null,
    ...overrides,
  };
}

function renderStatusPage() {
  return render(
    <MemoryRouter>
      <StatusPage />
    </MemoryRouter>,
  );
}

describe('StatusPage new terminal states (019-ai-job-throughput)', () => {
  beforeEach(() => {
    vi.mocked(useRetryActivity).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(useCancelActivity).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(useCancelAllActivity).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(useQueueBacklog).mockReturnValue({ data: undefined, isLoading: false, error: null } as any);
  });

  it('renders timed_out and interrupted distinctly from failed/cancelled, each with its reason', () => {
    vi.mocked(useActivity).mockReturnValue({
      data: {
        active: [],
        recent: [
          run({ id: 'r-failed', op: 'match', state: 'failed', error: 'boom' }),
          run({ id: 'r-cancelled', op: 'match', state: 'cancelled', error: 'rate limited' }),
          run({
            id: 'r-timedout',
            op: 'generate',
            state: 'timed_out',
            error: 'timed out after 5m0s (limit 5m0s)',
          }),
          run({
            id: 'r-interrupted',
            op: 'enrich',
            state: 'interrupted',
            error: 'interrupted: no worker heartbeat for at least 2m0s',
          }),
        ],
      },
      isLoading: false,
      error: null,
    } as any);

    renderStatusPage();

    expect(screen.getByText('⏱ timed out')).toBeInTheDocument();
    expect(screen.getByText('⚠ interrupted')).toBeInTheDocument();
    expect(screen.getByText('✗ failed')).toBeInTheDocument();
    expect(screen.getByText('⊘ cancelled')).toBeInTheDocument();

    expect(screen.getByText('timed out after 5m0s (limit 5m0s)')).toBeInTheDocument();
    expect(screen.getByText('interrupted: no worker heartbeat for at least 2m0s')).toBeInTheDocument();
  });

  it('includes timed_out and interrupted runs in the retryable set (retry-all is offered)', () => {
    vi.mocked(useActivity).mockReturnValue({
      data: {
        active: [],
        recent: [
          run({ id: 'r-timedout', op: 'generate', state: 'timed_out', error: 'timed out' }),
          run({ id: 'r-interrupted', op: 'enrich', state: 'interrupted', error: 'interrupted' }),
        ],
      },
      isLoading: false,
      error: null,
    } as any);

    renderStatusPage();

    expect(screen.getByText('retry all')).toBeInTheDocument();
    expect(screen.getAllByText('retry').length).toBeGreaterThanOrEqual(2);
  });
});

describe('StatusPage backlog panel (019-ai-job-throughput US4)', () => {
  beforeEach(() => {
    vi.mocked(useRetryActivity).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(useCancelActivity).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(useCancelAllActivity).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(useActivity).mockReturnValue({ data: { active: [], recent: [] }, isLoading: false, error: null } as any);
  });

  it('renders loading skeleton while activity is loading', () => {
    vi.mocked(useActivity).mockReturnValue({ data: undefined, isLoading: true, error: null } as any);
    vi.mocked(useQueueBacklog).mockReturnValue({ data: undefined, isLoading: false, error: null } as any);

    renderStatusPage();
    expect(screen.getByText('loading activity…')).toBeInTheDocument();
  });

  it('renders error state when activity fails', () => {
    vi.mocked(useActivity).mockReturnValue({ data: undefined, isLoading: false, error: new Error('network down') } as any);
    vi.mocked(useQueueBacklog).mockReturnValue({ data: undefined, isLoading: false, error: null } as any);

    renderStatusPage();
    expect(screen.getByText('network down')).toBeInTheDocument();
  });

  it('renders empty state when no active or recent activity', () => {
    vi.mocked(useActivity).mockReturnValue({ data: { active: [], recent: [] }, isLoading: false, error: null } as any);
    vi.mocked(useQueueBacklog).mockReturnValue({ data: undefined, isLoading: false, error: null } as any);

    renderStatusPage();
    expect(screen.getByText('Nothing running.')).toBeInTheDocument();
    expect(screen.getByText('No activity yet.')).toBeInTheDocument();
  });

  it('renders per-queue pending/active, concurrency, provider class, and ETA', () => {
    vi.mocked(useQueueBacklog).mockReturnValue({
      data: {
        queues: [
          {
            queue: 'match',
            providerClass: 'hosted',
            concurrency: 3,
            pending: 684,
            active: 3,
            scheduled: 0,
            retry: 2,
            archived: 11,
            processedPerMinute: 18.4,
            etaSeconds: 2230,
          },
          {
            queue: 'ingest',
            providerClass: null,
            concurrency: 2,
            pending: 0,
            active: 0,
            scheduled: 4,
            retry: 0,
            archived: 0,
            processedPerMinute: 0,
            etaSeconds: null,
          },
        ],
      },
      isLoading: false,
      error: null,
    } as any);

    renderStatusPage();

    expect(screen.getByText('Match')).toBeInTheDocument();
    expect(screen.getByText('hosted')).toBeInTheDocument();
    expect(screen.getAllByText(/684/).length).toBeGreaterThan(0);
    expect(screen.getByText(/37m/)).toBeInTheDocument();
    expect(screen.getAllByText('—').length).toBeGreaterThan(0);
  });
});
