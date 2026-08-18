import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import BlockContextMenu from './BlockContextMenu';

describe('BlockContextMenu', () => {
  const actions = (onSelect: () => void) => [
    { key: 'remove', label: 'Remove from resume', icon: null, danger: true, onSelect },
  ];

  it('runs the chosen action and closes', async () => {
    const onSelect = vi.fn();
    const onClose = vi.fn();
    render(<BlockContextMenu x={10} y={10} actions={actions(onSelect)} onClose={onClose} />);

    await userEvent.click(screen.getByRole('menuitem', { name: 'Remove from resume' }));

    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('closes on Escape and on a click outside it', async () => {
    const onClose = vi.fn();
    render(<BlockContextMenu x={10} y={10} actions={actions(vi.fn())} onClose={onClose} />);

    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledTimes(1);

    await userEvent.click(document.body);
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
