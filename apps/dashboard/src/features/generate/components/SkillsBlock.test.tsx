import { describe, it, expect, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import type { GenerationItemDto, GenerationSectionDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../../test/test-utils';
import SkillsBlock from './SkillsBlock';

function item(overrides: Partial<GenerationItemDto> = {}): GenerationItemDto {
  return {
    id: 'item-1',
    origin: 'profile',
    kind: 'skill_group',
    text: 'Backend: Go, NestJS, Redis',
    rank: 0,
    position: 0,
    selected: true,
    edited: false,
    unavailable: false,
    skillEntries: [
      { text: 'Go', selected: true },
      { text: 'NestJS', selected: true },
      { text: 'Redis', selected: true },
    ],
    ...overrides,
  };
}

function section(items: GenerationItemDto[]): GenerationSectionDto {
  return {
    id: 'sec-skills',
    kind: 'skills',
    position: 3,
    targetCount: 6,
    state: 'ready',
    fallbackUsed: false,
    items,
  } as GenerationSectionDto;
}

describe('SkillsBlock per-skill toggles', () => {
  it('renders every skill in an included group as its own control', () => {
    renderWithProviders(
      <SkillsBlock section={section([item()])} onToggle={vi.fn()} onReorder={vi.fn()} onDropEntries={vi.fn()} />,
    );

    expect(screen.getAllByTestId('skill-entry').map((el) => el.textContent)).toEqual(['Go', 'NestJS', 'Redis']);
  });

  // The PATCH carries the whole drop set, not the one skill that changed, so
  // the write is idempotent and order-free.
  it('sends the full drop set when a skill is switched off', async () => {
    const user = userEvent.setup();
    const onDropEntries = vi.fn();
    renderWithProviders(
      <SkillsBlock
        section={section([
          item({
            skillEntries: [
              { text: 'Go', selected: true },
              { text: 'NestJS', selected: false },
              { text: 'Redis', selected: true },
            ],
          }),
        ])}
        onToggle={vi.fn()}
        onReorder={vi.fn()}
        onDropEntries={onDropEntries}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Redis' }));

    expect(onDropEntries).toHaveBeenCalledWith('item-1', ['NestJS', 'Redis']);
  });

  it('switches a dropped skill back on without disturbing the others', async () => {
    const user = userEvent.setup();
    const onDropEntries = vi.fn();
    renderWithProviders(
      <SkillsBlock
        section={section([
          item({
            skillEntries: [
              { text: 'Go', selected: true },
              { text: 'NestJS', selected: false },
              { text: 'Redis', selected: false },
            ],
          }),
        ])}
        onToggle={vi.fn()}
        onReorder={vi.fn()}
        onDropEntries={onDropEntries}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'NestJS' }));

    expect(onDropEntries).toHaveBeenCalledWith('item-1', ['Redis']);
  });

  // A switched-off group is not a place to fine-tune which skills it would
  // have shown: the row falls back to its plain "Label: a, b, c" line.
  it('offers no per-skill chips for an excluded group', () => {
    renderWithProviders(
      <SkillsBlock
        section={section([item({ selected: false })])}
        onToggle={vi.fn()}
        onReorder={vi.fn()}
        onDropEntries={vi.fn()}
      />,
    );

    expect(screen.queryByTestId('skill-entry')).not.toBeInTheDocument();
    expect(screen.getByText('Backend: Go, NestJS, Redis')).toBeInTheDocument();
  });
});
