import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ManualAddResultDto } from '@job-finder/shared';
import { AddVacancyForm } from './AddVacancyForm';
import { mockJob } from '../../test/factories';

const navigate = vi.fn();

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return { ...actual, useNavigate: () => navigate };
});

vi.mock('./hooks', () => ({ useAddVacancyByUrl: vi.fn(), useSaveManualVacancy: vi.fn() }));

import { useAddVacancyByUrl, useSaveManualVacancy } from './hooks';

const POSTING_URL = 'https://djinni.co/jobs/123456-senior-go-engineer/';

// mockAdd stands in for the mutation: `result` is what the server answered, and
// the mutate spy records what the form submitted.
function mockAdd({
  result,
  isPending = false,
  isError = false,
}: {
  result?: ManualAddResultDto;
  isPending?: boolean;
  isError?: boolean;
} = {}) {
  const mutate = vi.fn((_url: string, opts?: { onSuccess?: (r: ManualAddResultDto) => void }) => {
    if (result) opts?.onSuccess?.(result);
  });
  vi.mocked(useAddVacancyByUrl).mockReturnValue({ mutate, isPending, isError, data: result } as any);
  vi.mocked(useSaveManualVacancy).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
  return mutate;
}

function renderForm(props: Parameters<typeof AddVacancyForm>[0] = {}) {
  return render(
    <MemoryRouter>
      <AddVacancyForm {...props} />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('AddVacancyForm outcomes', () => {
  it('reports the created vacancy and hands its id back for highlighting', async () => {
    const job = mockJob({ id: 'job-new', title: 'Senior Go Engineer' });
    mockAdd({ result: { outcome: 'created', job } as ManualAddResultDto });
    const onCreated = vi.fn();

    renderForm({ onCreated });
    await userEvent.type(screen.getByLabelText('Posting URL'), POSTING_URL);
    await userEvent.click(screen.getByRole('button', { name: /add/i }));

    expect(onCreated).toHaveBeenCalledWith('job-new');
    expect(screen.getByRole('status')).toHaveTextContent('Senior Go Engineer');
  });

  it('navigates to the existing vacancy on a duplicate', async () => {
    const job = mockJob({ id: 'job-existing' });
    mockAdd({
      result: {
        outcome: 'duplicate',
        job,
        reason: 'This vacancy is already in your feed.',
      } as ManualAddResultDto,
    });

    renderForm();
    await userEvent.type(screen.getByLabelText('Posting URL'), POSTING_URL);
    await userEvent.click(screen.getByRole('button', { name: /add/i }));

    expect(navigate).toHaveBeenCalledWith('/jobs/job-existing');
  });

  it('shows the server reason verbatim on a failure', async () => {
    mockAdd({
      result: {
        outcome: 'failed',
        kind: 'blocked',
        reason: 'djinni.co returned a bot challenge; the posting could not be read.',
      } as ManualAddResultDto,
    });

    renderForm();
    await userEvent.type(screen.getByLabelText('Posting URL'), POSTING_URL);
    await userEvent.click(screen.getByRole('button', { name: /add/i }));

    expect(screen.getByRole('alert')).toHaveTextContent(
      'djinni.co returned a bot challenge; the posting could not be read.',
    );
  });

  it('points a search URL at subscriptions rather than dead-ending', async () => {
    mockAdd({
      result: {
        outcome: 'failed',
        kind: 'not_a_posting',
        reason: 'djinni.co is a known source, but this URL is a search page.',
      } as ManualAddResultDto,
    });

    renderForm();
    await userEvent.type(screen.getByLabelText('Posting URL'), 'https://djinni.co/jobs/?search_type=basic-search');
    await userEvent.click(screen.getByRole('button', { name: /add/i }));

    expect(screen.getByRole('alert')).toHaveTextContent(/subscription/i);
  });

  it('hands a needs_fill_in result to the caller with its draft', async () => {
    const result = {
      outcome: 'needs_fill_in',
      kind: 'incomplete',
      reason: 'The page was read but is missing description.',
      draft: { url: POSTING_URL, title: 'Senior Go Engineer', remote: true },
    } as ManualAddResultDto;
    mockAdd({ result });
    const onNeedsFillIn = vi.fn();

    renderForm({ onNeedsFillIn });
    await userEvent.type(screen.getByLabelText('Posting URL'), POSTING_URL);
    await userEvent.click(screen.getByRole('button', { name: /add/i }));

    expect(onNeedsFillIn).toHaveBeenCalledWith(result);
  });
});

describe('AddVacancyForm submission guards', () => {
  it('disables submit while a request is in flight so a double-click cannot double-submit', () => {
    mockAdd({ isPending: true });

    renderForm();

    expect(screen.getByRole('button', { name: /adding/i })).toBeDisabled();
    expect(screen.getByLabelText('Posting URL')).toBeDisabled();
  });

  it('does not submit an empty URL', async () => {
    const mutate = mockAdd();

    renderForm();
    await userEvent.click(screen.getByRole('button', { name: /add/i }));

    expect(mutate).not.toHaveBeenCalled();
  });

  it('trims the submitted URL', async () => {
    const mutate = mockAdd();

    renderForm();
    await userEvent.type(screen.getByLabelText('Posting URL'), `  ${POSTING_URL}  `);
    await userEvent.click(screen.getByRole('button', { name: /add/i }));

    expect(mutate).toHaveBeenCalledWith(POSTING_URL, expect.anything());
  });

  it('reports a transport failure without pretending the vacancy was added', () => {
    mockAdd({ isError: true });

    renderForm();

    expect(screen.getByRole('alert')).toHaveTextContent(/could not be added/i);
  });
});
