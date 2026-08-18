import { describe, it, expect, vi } from 'vitest';
import type { GenerationSectionDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../../test/test-utils';
import WorkEntryBlock from './WorkEntryBlock';

function section(overrides: Partial<GenerationSectionDto> = {}): GenerationSectionDto {
  return {
    id: 'sec-1',
    kind: 'experience',
    entryKey: 'Acme Inc.',
    entryLabel: 'Senior Engineer · 2021–2024',
    position: 1,
    targetCount: 8,
    state: 'ready',
    fallbackUsed: false,
    enabled: true,
    items: [],
    ...overrides,
  };
}

describe('WorkEntryBlock', () => {
  // T035: a work entry with zero master bullets must render the explicit
  // empty state — never a fabricated bullet standing in for one.
  it('renders the explicit empty state for an entry with zero bullets', () => {
    renderWithProviders(<WorkEntryBlock section={section()} onToggle={vi.fn()} onReorder={vi.fn()} onToggleEnabled={vi.fn()} />);

    expect(screen.getByText(/no bullets in your profile for this role/i)).toBeInTheDocument();
    expect(screen.queryByTestId('item-row')).not.toBeInTheDocument();
  });

  it('renders every profile item carrying the profile badge', () => {
    const sec = section({
      items: [
        {
          id: 'item-1',
          origin: 'profile',
          kind: 'achievement',
          text: 'Shipped the thing',
          sourceIndex: 0,
          rank: 0,
          position: 0,
          selected: true,
          edited: false,
          unavailable: false,
        },
        {
          id: 'item-2',
          origin: 'profile',
          kind: 'achievement',
          text: 'Fixed the other thing',
          sourceIndex: 1,
          rank: 1,
          position: 1,
          selected: true,
          edited: false,
          unavailable: false,
        },
      ],
    });

    renderWithProviders(<WorkEntryBlock section={sec} onToggle={vi.fn()} onReorder={vi.fn()} onToggleEnabled={vi.fn()} />);

    const rows = screen.getAllByTestId('item-row');
    expect(rows).toHaveLength(2);
    for (const row of rows) {
      expect(row.querySelector('[data-badge="origin-profile"]')).not.toBeNull();
    }
  });
});
