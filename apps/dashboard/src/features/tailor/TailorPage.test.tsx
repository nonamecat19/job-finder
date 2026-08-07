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
}));

import {
  useAdHocDocuments,
  useGenerateCoverLetter,
  useSaveAdHocDocument,
  useTailorDocuments,
} from './hooks';

const mockedUseAdHocDocuments = vi.mocked(useAdHocDocuments);
const mockedUseTailorDocuments = vi.mocked(useTailorDocuments);
const mockedUseGenerateCoverLetter = vi.mocked(useGenerateCoverLetter);
const mockedUseSaveAdHocDocument = vi.mocked(useSaveAdHocDocument);

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
});
