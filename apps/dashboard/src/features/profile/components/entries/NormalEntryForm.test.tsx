import { describe, it, expect, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import type { Entry } from '@job-finder/shared';
import { render, screen } from '../../../../test/test-utils';
import { NormalEntryForm } from './NormalEntryForm';

describe('NormalEntryForm project density', () => {

  it('does not show the density picker outside the projects section', () => {
    render(<NormalEntryForm sectionName="talks" entry={{ name: 'A talk' }} onChange={vi.fn()} />);

    expect(screen.queryByLabelText('Project density')).not.toBeInTheDocument();
  });

  it('shows an unset level as auto', () => {
    render(<NormalEntryForm sectionName="projects" entry={{ name: 'Trading engine' }} onChange={vi.fn()} />);

    expect(screen.getByLabelText('Project density')).toHaveValue('auto');
  });

  it('reflects an existing level', () => {
    render(
      <NormalEntryForm
        sectionName="projects"
        entry={{ name: 'Trading engine', projectLevel: 'top3' }}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByLabelText('Project density')).toHaveValue('top3');
  });

  it('writes the chosen level, and clears it for auto', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <NormalEntryForm sectionName="projects" entry={{ name: 'Trading engine' }} onChange={onChange} />,
    );
    const picker = screen.getByLabelText('Project density');

    await user.selectOptions(picker, 'top5');
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ projectLevel: 'top5' }),
    );

    await user.selectOptions(picker, 'relevant');
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ projectLevel: 'relevant' }),
    );

    await user.selectOptions(picker, 'auto');
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ projectLevel: undefined }),
    );
  });
});
