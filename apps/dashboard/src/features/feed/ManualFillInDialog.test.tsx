import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ManualAddResultDto, ManualVacancyDraftDto } from '@job-finder/shared';
import { ManualFillInDialog } from './ManualFillInDialog';
import { mockJob } from '../../test/factories';

vi.mock('./hooks', () => ({ useSaveManualVacancy: vi.fn() }));

import { useSaveManualVacancy } from './hooks';

const DRAFT: ManualVacancyDraftDto = {
  url: 'https://careers.example.com/jobs/42',
  sourceKey: 'djinni',
  title: 'Senior Go Engineer',
  company: 'NovaTech',
  remote: true,
};

function mockSave(result?: ManualAddResultDto, isPending = false) {
  const mutate = vi.fn((_body: unknown, opts?: { onSuccess?: (r: ManualAddResultDto) => void }) => {
    if (result) opts?.onSuccess?.(result);
  });
  vi.mocked(useSaveManualVacancy).mockReturnValue({ mutate, isPending } as any);
  return mutate;
}

function renderDialog(draft: ManualVacancyDraftDto = DRAFT, onClose = vi.fn(), onCreated = vi.fn()) {
  render(
    <MemoryRouter>
      <ManualFillInDialog draft={draft} onClose={onClose} onCreated={onCreated} />
    </MemoryRouter>,
  );
  return { onClose, onCreated };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('ManualFillInDialog', () => {
  it('pre-fills from the draft so the operator completes rather than retypes', () => {
    mockSave();

    renderDialog();

    expect(screen.getByLabelText('Title')).toHaveValue('Senior Go Engineer');
    expect(screen.getByLabelText('Company')).toHaveValue('NovaTech');
    expect(screen.getByLabelText('Description')).toHaveValue('');
    expect(screen.getByText(DRAFT.url)).toBeInTheDocument();
  });

  it('names every missing required field and does not save', async () => {
    const mutate = mockSave();

    renderDialog({ url: DRAFT.url, remote: false });
    await userEvent.click(screen.getByRole('button', { name: /save vacancy/i }));

    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('Title');
    expect(alert).toHaveTextContent('Company');
    expect(alert).toHaveTextContent('Description');
    expect(mutate).not.toHaveBeenCalled();
  });

  it('saves what the operator completed and closes', async () => {
    const job = mockJob({ id: 'job-filled' });
    const mutate = mockSave({ outcome: 'created', job } as ManualAddResultDto);
    const { onClose, onCreated } = renderDialog();

    await userEvent.type(screen.getByLabelText('Description'), 'Own the ledger service.');
    await userEvent.click(screen.getByRole('button', { name: /save vacancy/i }));

    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        url: DRAFT.url,
        sourceKey: 'djinni',
        title: 'Senior Go Engineer',
        company: 'NovaTech',
        description: 'Own the ledger service.',
      }),
      expect.anything(),
    );
    expect(onCreated).toHaveBeenCalledWith('job-filled');
    expect(onClose).toHaveBeenCalled();
  });

  it('disables save while the request is in flight', () => {
    mockSave(undefined, true);

    renderDialog();

    expect(screen.getByRole('button', { name: /saving/i })).toBeDisabled();
  });

  it('closes without saving when cancelled', async () => {
    const mutate = mockSave();
    const { onClose } = renderDialog();

    await userEvent.click(screen.getByRole('button', { name: /cancel/i }));

    expect(onClose).toHaveBeenCalled();
    expect(mutate).not.toHaveBeenCalled();
  });
});
