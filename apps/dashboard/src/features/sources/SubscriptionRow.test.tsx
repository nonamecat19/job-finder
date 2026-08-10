import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import type { SubscriptionDto } from '@job-finder/shared';
import { SubscriptionRow } from './SubscriptionRow';
import { mockSubscription } from '../../test/factories';

function renderRow(sub: SubscriptionDto) {
  return render(
    <MemoryRouter>
      <SubscriptionRow sub={sub} onRun={vi.fn()} onDelete={vi.fn()} running={false} />
    </MemoryRouter>,
  );
}

function manualSubscription(overrides: Partial<SubscriptionDto> = {}): SubscriptionDto {
  return mockSubscription({
    id: 'sub-manual',
    sourceKey: 'djinni',
    name: 'Manual',
    url: '',
    kind: 'manual',
    manualCount: 4,
    lastAddedAt: '2026-08-10T09:00:00Z',
    ...overrides,
  });
}

describe('SubscriptionRow for a manual subscription', () => {
  it('shows the count and the most recent addition', () => {
    renderRow(manualSubscription());

    expect(screen.getByText(/4 added by hand/)).toBeInTheDocument();
  });

  it('offers no run control, because a manual subscription has nothing to crawl', () => {
    renderRow(manualSubscription());

    expect(screen.queryByRole('button', { name: /run now/i })).not.toBeInTheDocument();
  });

  it('shows no URL, because a manual subscription has none', () => {
    renderRow(manualSubscription());

    expect(screen.queryByText(/djinni\.co\/jobs/)).not.toBeInTheDocument();
  });

  it('disables delete while its vacancies still reference it', () => {
    renderRow(manualSubscription({ manualCount: 4 }));

    expect(screen.getByRole('button', { name: /delete/i })).toBeDisabled();
  });

  it('allows delete once it holds nothing', () => {
    renderRow(manualSubscription({ manualCount: 0, lastAddedAt: undefined }));

    expect(screen.getByRole('button', { name: /delete/i })).toBeEnabled();
  });
});

describe('SubscriptionRow for a crawl subscription', () => {
  it('keeps its URL and its run control', () => {
    renderRow(mockSubscription({ sourceKey: 'dou', url: 'https://jobs.dou.ua/vacancies/?category=Golang' }));

    expect(screen.getByRole('button', { name: /run now/i })).toBeInTheDocument();
    expect(screen.getByText('https://jobs.dou.ua/vacancies/?category=Golang')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /delete/i })).toBeEnabled();
  });
});
