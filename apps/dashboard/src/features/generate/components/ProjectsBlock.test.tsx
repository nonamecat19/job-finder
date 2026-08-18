import { describe, it, expect, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import type { GenerationItemDto, GenerationSectionDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../../test/test-utils';
import ProjectsBlock from './ProjectsBlock';

function item(overrides: Partial<GenerationItemDto> = {}): GenerationItemDto {
  return {
    id: 'item-1',
    origin: 'profile',
    kind: 'project',
    text: 'Trading engine · 3 bullets',
    rank: 0,
    position: 0,
    selected: true,
    edited: false,
    unavailable: false,
    ...overrides,
  };
}

function section(items: GenerationItemDto[]): GenerationSectionDto {
  return {
    id: 'sec-1',
    kind: 'projects',
    position: 4,
    targetCount: 2,
    state: 'ready',
    fallbackUsed: false,
    enabled: true,
    items,
  } as GenerationSectionDto;
}

describe('ProjectsBlock', () => {

  it('separates included projects from the ranked-but-excluded ones', () => {
    renderWithProviders(
      <ProjectsBlock
        section={section([
          item({ id: 'a', text: 'Trading engine · 3 bullets', selected: true }),
          item({ id: 'b', text: 'Recipe blog · 2 bullets', selected: false, position: 1, rank: 1 }),
        ])}
        onToggle={vi.fn()}
        onReorder={vi.fn()}
        onToggleEnabled={vi.fn()}
      />,
    );

    expect(screen.getByText('Trading engine · 3 bullets')).toBeInTheDocument();
    expect(screen.getByText('Recipe blog · 2 bullets')).toBeInTheDocument();
    expect(screen.getByTestId('unselected-divider')).toBeInTheDocument();
  });

  it('promotes a project through onToggle', async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    renderWithProviders(
      <ProjectsBlock
        section={section([item({ id: 'b', selected: false })])}
        onToggle={onToggle}
        onReorder={vi.fn()}
        onToggleEnabled={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('checkbox'));

    expect(onToggle).toHaveBeenCalledWith('b', true);
  });

  it('renders an explicit empty state', () => {
    renderWithProviders(<ProjectsBlock section={section([])} onToggle={vi.fn()} onReorder={vi.fn()} onToggleEnabled={vi.fn()} />);

    expect(screen.getByText(/no projects in your profile/i)).toBeInTheDocument();
  });
});
