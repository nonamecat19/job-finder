import { describe, it, expect, vi, beforeEach } from 'vitest';
import userEvent from '@testing-library/user-event';
import type { GeneratedDocumentDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../test/test-utils';
import { mockDocument } from '../../test/factories';
import TailorPage from './TailorPage';

vi.mock('./hooks', () => ({
  useAdHocDocuments: vi.fn(),
  useTailorDocuments: vi.fn(),
  useGenerateCoverLetter: vi.fn(),
  useSaveAdHocDocument: vi.fn(),
  useSummaryModel: vi.fn(),
}));

import {
  useAdHocDocuments,
  useGenerateCoverLetter,
  useSaveAdHocDocument,
  useSummaryModel,
  useTailorDocuments,
} from './hooks';

const mockedUseAdHocDocuments = vi.mocked(useAdHocDocuments);
const mockedUseTailorDocuments = vi.mocked(useTailorDocuments);
const mockedUseGenerateCoverLetter = vi.mocked(useGenerateCoverLetter);
const mockedUseSaveAdHocDocument = vi.mocked(useSaveAdHocDocument);
const mockedUseSummaryModel = vi.mocked(useSummaryModel);

// 034: the menu the selector renders from. Mirrors the shape the API serves —
// options plus the current selection in one response.
const summaryMenu = {
  optionId: 'standard',
  options: [
    { id: 'standard', label: 'Standard', description: 'The balanced default.', cost: 'moderate', selfHosted: false, current: true },
    { id: 'premium', label: 'Premium', description: 'The strongest writer available.', cost: 'highest', selfHosted: false, current: false },
    { id: 'fast', label: 'Fast', description: 'Cheapest and quickest.', cost: 'lowest', selfHosted: false, current: false },
    { id: 'local', label: 'Self-hosted', description: 'Runs on your own machine.', cost: 'free', selfHosted: true, current: false },
  ],
};

function setupHooks(resume: GeneratedDocumentDto, coverLetter: GeneratedDocumentDto | null = null) {
  mockedUseAdHocDocuments.mockReturnValue({ data: [] } as any);

  const tailorMutate = vi.fn((_vars, opts) => opts?.onSuccess?.({ resume, coverLetter }));
  mockedUseTailorDocuments.mockReturnValue({ mutate: tailorMutate, isPending: false, isError: false } as any);

  const coverLetterMutate = vi.fn();
  mockedUseGenerateCoverLetter.mockReturnValue({
    mutate: coverLetterMutate,
    isPending: false,
    isError: false,
  } as any);

  mockedUseSaveAdHocDocument.mockReturnValue({ mutate: vi.fn() } as any);
  mockedUseSummaryModel.mockReturnValue({ data: summaryMenu } as any);

  return { tailorMutate, coverLetterMutate };
}

function resumeDoc(overrides: Partial<GeneratedDocumentDto> = {}): GeneratedDocumentDto {
  return mockDocument({
    id: 'resume-1',
    type: 'resume',
    summarySubstituted: false,
    selectionEscalated: false,
    ...overrides,
  });
}

async function runTailoring() {
  const user = userEvent.setup();
  renderWithProviders(<TailorPage />);
  await user.type(screen.getByPlaceholderText(/Paste the job posting text here/), 'Backend role');
  await user.click(screen.getByRole('button', { name: /generate resume/i }));
  return user;
}

describe('TailorPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows the summary substitution marker when the summary came from a fallback', async () => {
    setupHooks(resumeDoc({ summarySubstituted: true, summaryModel: 'claude-haiku-4.5' }));

    await runTailoring();

    expect(screen.getByText(/Summary written by a fallback model/i)).toBeInTheDocument();
    expect(screen.getByText('claude-haiku-4.5')).toBeInTheDocument();
  });

  it('does not show the marker when the configured summary model was used', async () => {
    setupHooks(resumeDoc({ summarySubstituted: false, summaryModel: 'claude-sonnet-5' }));

    await runTailoring();

    expect(screen.getByText('Result')).toBeInTheDocument();
    expect(screen.queryByText(/Summary written by a fallback model/i)).not.toBeInTheDocument();
  });

  it('offers a cover letter action instead of generating one automatically', async () => {
    const { coverLetterMutate } = setupHooks(resumeDoc());

    const user = await runTailoring();

    expect(screen.queryByText('Cover letter')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /generate cover letter/i }));
    expect(coverLetterMutate).toHaveBeenCalledWith('resume-1', expect.anything());
  });

  it('renders the cover letter once one exists', async () => {
    setupHooks(
      resumeDoc(),
      mockDocument({
        id: 'letter-1',
        type: 'cover_letter',
        content: { text: 'Dear hiring manager' },
        summarySubstituted: false,
        selectionEscalated: false,
      }),
    );

    await runTailoring();

    expect(screen.getByText('Cover letter')).toBeInTheDocument();
    expect(screen.getByText('Dear hiring manager')).toBeInTheDocument();
  });
  // 034: the summary-model selector.
  describe('summary model selector', () => {
    it('renders every option and preselects the stored choice', async () => {
      setupHooks(resumeDoc());
      renderWithProviders(<TailorPage />);

      const select = screen.getByLabelText('Summary writer') as HTMLSelectElement;
      expect(select.value).toBe('standard');
      for (const o of summaryMenu.options) {
        expect(screen.getByRole('option', { name: new RegExp(o.label) })).toBeInTheDocument();
      }
      // The description of the current option is what tells the user what they
      // are choosing between; a bare label would make the menu a guess.
      expect(screen.getByText('The balanced default.')).toBeInTheDocument();
    });

    it('sends the chosen option with the tailoring request', async () => {
      const { tailorMutate } = setupHooks(resumeDoc());
      const user = userEvent.setup();
      renderWithProviders(<TailorPage />);

      await user.selectOptions(screen.getByLabelText('Summary writer'), 'premium');
      await user.type(screen.getByPlaceholderText(/Paste the job posting text here/), 'Backend role');
      await user.click(screen.getByRole('button', { name: /generate resume/i }));

      expect(tailorMutate).toHaveBeenCalledWith(
        expect.objectContaining({ summaryOptionId: 'premium' }),
        expect.anything(),
      );
    });

    // Not sending the field at all is what makes an untouched selector
    // byte-identical to the pre-034 request (spec AC3).
    it('sends no option when the user does not touch the selector', async () => {
      const { tailorMutate } = setupHooks(resumeDoc());

      await runTailoring();

      expect(tailorMutate).toHaveBeenCalledWith(
        expect.objectContaining({ summaryOptionId: undefined }),
        expect.anything(),
      );
    });

    it('shows which option produced the finished resume', async () => {
      setupHooks(resumeDoc({ summaryOptionId: 'premium' } as any));

      await runTailoring();

      expect(screen.getByTestId('summary-option-used')).toHaveTextContent('Premium');
    });
  });
});
