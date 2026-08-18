import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { GenerationRunDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../../test/test-utils';
import { api } from '../../../lib/api';
import { buildTypst } from '../wasm/rendercvWasm';
import { compilePdf } from '../wasm/typstWasi';
import { loadPdfjs } from '../preview/pdfjs';
import ResumePreviewPane from './ResumePreviewPane';

vi.mock('../../../lib/api', () => ({
  api: {
    documents: { pdfUrl: (id: string) => `/api/documents/${id}/pdf` },
    generations: { previewDocument: vi.fn() },
  },
}));
vi.mock('../wasm/rendercvWasm', () => ({ buildTypst: vi.fn() }));
vi.mock('../wasm/typstWasi', () => ({ compilePdf: vi.fn() }));

vi.mock('../preview/pdfjs', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../preview/pdfjs')>()),
  loadPdfjs: vi.fn(),
}));

function fakePdfjs(numPages: number) {
  const page = {
    getViewport: () => ({ width: 595, height: 842, transform: [1, 0, 0, 1, 0, 0] }),
    getTextContent: async () => ({ items: [] }),
    render: () => ({ promise: Promise.resolve(), cancel: () => {} }),
  };
  return {
    getDocument: () => ({
      promise: Promise.resolve({ numPages, getPage: async () => page }),
      destroy: async () => {},
    }),
    Util: { transform: (a: number[]) => a },
  };
}

function baseRun(targetPages: number): GenerationRunDto {
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
      targetPages,
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
    sections: [],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
  };
}

describe('ResumePreviewPane page budget', () => {
  beforeEach(() => {
    vi.mocked(api.generations.previewDocument).mockResolvedValue({ yaml: 'cv: {}', sectionsHash: 'sha256:abc' });
    vi.mocked(buildTypst).mockResolvedValue('#typst source');
    vi.mocked(compilePdf).mockResolvedValue(new Uint8Array([1, 2, 3]));
    vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:pdf-1'), revokeObjectURL: vi.fn() });
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('reports the rendered page count against the shape target', async () => {
    vi.mocked(loadPdfjs).mockResolvedValue(fakePdfjs(2) as unknown as Awaited<ReturnType<typeof loadPdfjs>>);
    renderWithProviders(<ResumePreviewPane run={baseRun(2)} profile={undefined} />);

    await vi.advanceTimersByTimeAsync(500);
    const budget = await screen.findByTestId('page-budget');
    expect(budget).toHaveTextContent('2 pages of 2 target');
    expect(budget).not.toHaveTextContent('over budget');
  });

  it('flags a render that is over the target', async () => {
    vi.mocked(loadPdfjs).mockResolvedValue(fakePdfjs(3) as unknown as Awaited<ReturnType<typeof loadPdfjs>>);
    renderWithProviders(<ResumePreviewPane run={baseRun(2)} profile={undefined} />);

    await vi.advanceTimersByTimeAsync(500);
    const budget = await screen.findByTestId('page-budget');
    expect(budget).toHaveTextContent('3 pages of 2 target');
    expect(budget).toHaveTextContent('over budget');
  });
});
