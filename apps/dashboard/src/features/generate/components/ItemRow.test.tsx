import { describe, it, expect, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import type { GenerationItemDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../../test/test-utils';
import ItemRow from './ItemRow';

function aiAchievement(overrides: Partial<GenerationItemDto> = {}): GenerationItemDto {
  return {
    id: 'item-1',
    origin: 'ai',
    kind: 'achievement',
    text: 'Introduced gRPC contracts between payment services.',
    rank: 0,
    position: 0,
    selected: true,
    edited: false,
    unavailable: false,
    ...overrides,
  };
}

describe('ItemRow rewrite affordance', () => {
  it('shows no rewrite control for a profile-origin item', () => {
    renderWithProviders(
      <ItemRow
        item={aiAchievement({ origin: 'profile' })}
        onToggle={vi.fn()}
        onRewrite={vi.fn().mockResolvedValue([])}
      />,
    );
    expect(screen.queryByLabelText(/rewrite this bullet/i)).not.toBeInTheDocument();
  });

  it('shows no rewrite control for an unselected AI item', () => {
    renderWithProviders(
      <ItemRow
        item={aiAchievement({ selected: false })}
        onToggle={vi.fn()}
        onRewrite={vi.fn().mockResolvedValue([])}
      />,
    );
    expect(screen.queryByLabelText(/rewrite this bullet/i)).not.toBeInTheDocument();
  });

  it('fetches and lists grounded variants for a selected AI achievement, and "use" applies one', async () => {
    const user = userEvent.setup();
    const onRewrite = vi.fn().mockResolvedValue(['Rolled out gRPC contracts across the payment services.']);
    const onEditText = vi.fn();

    renderWithProviders(
      <ItemRow item={aiAchievement()} onToggle={vi.fn()} onEditText={onEditText} onRewrite={onRewrite} />,
    );

    await user.click(screen.getByLabelText(/rewrite this bullet/i));

    expect(onRewrite).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('Rolled out gRPC contracts across the payment services.')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'use' }));
    expect(onEditText).toHaveBeenCalledWith('Rolled out gRPC contracts across the payment services.');
  });

  it('reports when no grounded variant survives', async () => {
    const user = userEvent.setup();
    const onRewrite = vi.fn().mockResolvedValue([]);

    renderWithProviders(<ItemRow item={aiAchievement()} onToggle={vi.fn()} onRewrite={onRewrite} />);

    await user.click(screen.getByLabelText(/rewrite this bullet/i));

    expect(await screen.findByText(/no alternative phrasing available/i)).toBeInTheDocument();
  });
});
