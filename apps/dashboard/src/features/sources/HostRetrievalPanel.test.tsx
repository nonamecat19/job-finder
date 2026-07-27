import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { HostRetrievalPanel } from './SourcesPage';

vi.mock('./hooks', () => ({
  useSources: vi.fn(),
  useHostRetrievalStatus: vi.fn(),
  useClearRungPreference: vi.fn(),
  useClearCookies: vi.fn(),
  useOverrideCoolingOff: vi.fn(),
}));

import {
  useSources,
  useHostRetrievalStatus,
  useClearRungPreference,
  useClearCookies,
  useOverrideCoolingOff,
} from './hooks';

const mockedUseSources = vi.mocked(useSources);
const mockedUseHostRetrievalStatus = vi.mocked(useHostRetrievalStatus);

function setupMutationMocks() {
  vi.mocked(useClearRungPreference).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
  vi.mocked(useClearCookies).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
  vi.mocked(useOverrideCoolingOff).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
}

describe('HostRetrievalPanel pacing display', () => {
  it('renders the pacing line as a routine fact, with no budget text anywhere', async () => {
    const user = userEvent.setup();
    setupMutationMocks();
    mockedUseSources.mockReturnValue({ data: [{ key: 'djinni.co' }] } as any);
    mockedUseHostRetrievalStatus.mockReturnValue({
      data: {
        host: 'djinni.co',
        identityVersion: 'v3',
        currentRung: 'direct',
        lastBlockAt: null,
        lastBlockReason: null,
        coolingOffUntil: null,
        crawlDelaySeconds: 5,
        pacing: { requestsPerSecond: 0.2, intervalSeconds: 5, source: 'site-requested' },
      },
      isLoading: false,
    } as any);

    render(<HostRetrievalPanel />);
    await user.click(screen.getByText('djinni.co'));

    const paceLine = screen.getByText(/request every 5s \(site-requested\)/);
    expect(paceLine).toBeInTheDocument();
    expect(paceLine.className).not.toMatch(/text-warning|text-danger/);
    expect(paceLine.closest('span')?.querySelector('svg')).toBeNull();

    expect(screen.queryByText(/Budget/i)).not.toBeInTheDocument();
  });

  it('keeps block and cooling-off lines in their warning/danger styling', async () => {
    const user = userEvent.setup();
    setupMutationMocks();
    mockedUseSources.mockReturnValue({ data: [{ key: 'blocked.example' }] } as any);
    mockedUseHostRetrievalStatus.mockReturnValue({
      data: {
        host: 'blocked.example',
        identityVersion: 'v3',
        currentRung: 'direct',
        lastBlockAt: '2026-07-27T09:14:02Z',
        lastBlockReason: 'challenged via direct',
        coolingOffUntil: '2026-07-28T09:14:02Z',
        crawlDelaySeconds: null,
        pacing: { requestsPerSecond: 0.7, intervalSeconds: 1.4, source: 'default' },
      },
      isLoading: false,
    } as any);

    render(<HostRetrievalPanel />);
    await user.click(screen.getByText('blocked.example'));

    const blockedLine = screen.getByText(/Blocked at/).closest('span');
    expect(blockedLine?.className).toMatch(/text-warning/);

    const coolingLine = screen.getByText(/Cooling off until/).closest('div');
    expect(coolingLine?.className).toMatch(/text-danger/);
  });
});
