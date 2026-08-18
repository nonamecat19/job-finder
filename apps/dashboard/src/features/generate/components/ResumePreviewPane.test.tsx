import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import userEvent from '@testing-library/user-event';
import type { GenerationRunDto } from '@job-finder/shared';
import { renderWithProviders, screen, waitFor } from '../../../test/test-utils';
import { api } from '../../../lib/api';
import { buildTypst } from '../wasm/rendercvWasm';
import { compilePdf } from '../wasm/typstWasi';
import ResumePreviewPane from './ResumePreviewPane';

vi.mock('../../../lib/api', () => ({
  api: {
    documents: { pdfUrl: (id: string) => `/api/documents/${id}/pdf` },
    generations: { previewDocument: vi.fn() },
  },
}));
vi.mock('../wasm/rendercvWasm', () => ({ buildTypst: vi.fn() }));
vi.mock('../wasm/typstWasi', () => ({ compilePdf: vi.fn() }));

function baseRun(overrides: Partial<GenerationRunDto> = {}): GenerationRunDto {
  return {
    id: 'run-1',
    state: 'ready',
    vacancy: { company: 'Acme', title: 'Senior Engineer' },
    groundingLevel: 'moderate',
    summarySubstituted: false,
    masterChanged: false,
    shapeConfig: {
      summaryLines: 4,
      summaryEnabled: true,
      skillsEnabled: true,
      skillsMaxGroups: 0,
      experienceEnabled: true,
      experienceBulletsMin: 8,
      experienceBulletsMax: 10,
      targetPages: 1,
      projectsEnabled: true,
      projectsMin: 0,
      projectsMax: 0,
      projectBulletsMax: 0,
      certificationsEnabled: true,
      certificationsMin: 0,
      certificationsMax: 0,
      educationEnabled: true,
      fontSize: 10,
    },
    export: { status: '' },
    sections: [
      {
        id: 'sec-1',
        kind: 'experience',
        entryKey: 'Acme Inc.',
        entryLabel: 'Acme Inc. — Senior Engineer',
        position: 1,
        targetCount: 8,
        state: 'ready',
        fallbackUsed: false,
        enabled: true,
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
        ],
      },
    ],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('ResumePreviewPane', () => {
  beforeEach(() => {
    vi.mocked(api.generations.previewDocument).mockReset();
    vi.mocked(buildTypst).mockReset();
    vi.mocked(compilePdf).mockReset();
    vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:pdf-1'), revokeObjectURL: vi.fn() });
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  async function mockHappyPath(sectionsHash = 'sha256:abc') {
    vi.mocked(api.generations.previewDocument).mockResolvedValue({ yaml: 'cv: {}', sectionsHash });
    vi.mocked(buildTypst).mockResolvedValue('#typst source');
    vi.mocked(compilePdf).mockResolvedValue(new Uint8Array([1, 2, 3]));
  }

  it('shows an empty state with no run', () => {
    renderWithProviders(<ResumePreviewPane run={undefined} profile={undefined} />);
    expect(screen.getByText(/generate a resume to preview it here/i)).toBeInTheDocument();
    expect(api.generations.previewDocument).not.toHaveBeenCalled();
  });

  it('shows a rendering message while the run itself is still running', () => {
    renderWithProviders(<ResumePreviewPane run={baseRun({ state: 'running' })} profile={undefined} />);
    expect(screen.getByText(/rendering your resume/i)).toBeInTheDocument();
    expect(api.generations.previewDocument).not.toHaveBeenCalled();
  });

  it('renders the real PDF preview once the debounced pipeline resolves', async () => {
    await mockHappyPath();
    renderWithProviders(<ResumePreviewPane run={baseRun()} profile={undefined} />);

    await vi.advanceTimersByTimeAsync(500);
    // The preview is drawn in-page (PdfPreviewCanvas) rather than handed to an
    // <iframe>, so what a ready render puts on screen is the canvas surface.
    await screen.findByTestId('resume-preview');
    expect(api.generations.previewDocument).toHaveBeenCalledWith('run-1');
  });

  it('shows a clear error state, and export stays available, when the pipeline fails', async () => {
    vi.mocked(api.generations.previewDocument).mockRejectedValue(new Error('typstwasm exited 1'));
    const onExport = vi.fn();
    renderWithProviders(<ResumePreviewPane run={baseRun()} profile={undefined} onExport={onExport} />);

    await vi.advanceTimersByTimeAsync(500);
    await waitFor(() => expect(screen.getByText(/preview isn't available/i)).toBeInTheDocument());
    expect(screen.getByText(/typstwasm exited 1/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /export pdf/i })).toBeEnabled();
  });

  it('short-circuits to the fallback state in a browser without WebAssembly (FR-007)', async () => {
    const originalWebAssembly = globalThis.WebAssembly;
    // @ts-expect-error -- simulating an unsupported browser
    delete globalThis.WebAssembly;
    try {
      const onExport = vi.fn();
      renderWithProviders(<ResumePreviewPane run={baseRun()} profile={undefined} onExport={onExport} />);
      await vi.advanceTimersByTimeAsync(500);
      await waitFor(() => expect(screen.getByText(/preview isn't available/i)).toBeInTheDocument());
      expect(screen.getByText(/doesn't support the in-browser preview/i)).toBeInTheDocument();
      expect(api.generations.previewDocument).not.toHaveBeenCalled();
      expect(screen.getByRole('button', { name: /export pdf/i })).toBeEnabled();
    } finally {
      globalThis.WebAssembly = originalWebAssembly;
    }
  });

  it('coalesces rapid successive edits into a single render (FR-010)', async () => {
    await mockHappyPath();
    const { rerender } = renderWithProviders(<ResumePreviewPane run={baseRun()} profile={undefined} />);

    // Three edits in quick succession, each well inside the debounce window.
    for (let i = 0; i < 3; i++) {
      rerender(
        <ResumePreviewPane
          run={baseRun({
            sections: [
              {
                id: 'sec-1',
                kind: 'experience',
                entryKey: 'Acme Inc.',
                entryLabel: 'Acme Inc. — Senior Engineer',
                position: 1,
                targetCount: 8,
                state: 'ready',
                fallbackUsed: false,
                enabled: true,
                items: [
                  {
                    id: 'item-1',
                    origin: 'profile',
                    kind: 'achievement',
                    text: `Shipped the thing v${i}`,
                    sourceIndex: 0,
                    rank: 0,
                    position: 0,
                    selected: true,
                    edited: true,
                    unavailable: false,
                  },
                ],
              },
            ],
          })}
          profile={undefined}
        />,
      );
      await vi.advanceTimersByTimeAsync(50);
    }

    await vi.advanceTimersByTimeAsync(500);
    await screen.findByTestId('resume-preview');
    expect(api.generations.previewDocument).toHaveBeenCalledTimes(1);
  });

  it('skips a redundant WASM render when two edits resolve to the same sectionsHash', async () => {
    await mockHappyPath('sha256:same');
    const { rerender } = renderWithProviders(<ResumePreviewPane run={baseRun()} profile={undefined} />);
    await vi.advanceTimersByTimeAsync(500);
    await screen.findByTestId('resume-preview');
    expect(buildTypst).toHaveBeenCalledTimes(1);

    // A real content change (so the effect actually re-schedules) that the
    // server nonetheless assembles to the same effective document — e.g. a
    // toggle flipped off then back on — reported via the same sectionsHash.
    rerender(
      <ResumePreviewPane
        run={baseRun({
          sections: [
            {
              id: 'sec-1',
              kind: 'experience',
              entryKey: 'Acme Inc.',
              entryLabel: 'Acme Inc. — Senior Engineer',
              position: 1,
              targetCount: 8,
              state: 'ready',
              fallbackUsed: false,
              enabled: true,
              items: [
                {
                  id: 'item-1',
                  origin: 'profile',
                  kind: 'achievement',
                  text: 'Shipped the thing',
                  sourceIndex: 0,
                  rank: 0,
                  position: 0,
                  selected: false,
                  edited: false,
                  unavailable: false,
                },
              ],
            },
          ],
        })}
        profile={undefined}
      />,
    );
    await vi.advanceTimersByTimeAsync(500);

    expect(api.generations.previewDocument).toHaveBeenCalledTimes(2);
    expect(buildTypst).toHaveBeenCalledTimes(1); // not called again — sectionsHash matched
  });

  it('shows the overflow report once export is blocked', async () => {
    await mockHappyPath();
    const { rerender } = renderWithProviders(<ResumePreviewPane run={baseRun()} profile={undefined} />);
    await vi.advanceTimersByTimeAsync(500);
    await screen.findByTestId('resume-preview');
    expect(screen.queryByTestId('overflow-report')).not.toBeInTheDocument();

    rerender(
      <ResumePreviewPane
        run={baseRun({
          export: {
            status: 'blocked',
            report: { pagesRendered: 2, pagesTarget: 1, candidates: [] },
          },
        })}
        profile={undefined}
      />,
    );
    expect(screen.getByTestId('overflow-report')).toBeInTheDocument();
  });

  it('renders warnings and the export control in the preview footer', async () => {
    await mockHappyPath();
    const onExport = vi.fn();
    renderWithProviders(
      <ResumePreviewPane
        run={baseRun()}
        profile={undefined}
        onExport={onExport}
        warnings={['This resume has no skills — every skill group is switched off.']}
      />,
    );

    expect(screen.getByText(/this resume has no skills/i)).toBeInTheDocument();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(screen.getByRole('button', { name: /export pdf/i }));
    expect(onExport).toHaveBeenCalledTimes(1);
  });

  it('offers a download once exported', async () => {
    await mockHappyPath();
    renderWithProviders(
      <ResumePreviewPane
        run={baseRun()}
        profile={undefined}
        onExport={vi.fn()}
        exportState={{ status: 'exported', documentId: 'doc-1' }}
      />,
    );
    expect(screen.getByRole('link', { name: /download pdf/i })).toBeInTheDocument();
  });
});
