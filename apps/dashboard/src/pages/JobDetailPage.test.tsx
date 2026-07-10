import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '../test/test-utils'
import { mockJob, mockDocument } from '../test/factories'
import { api } from '../api'
import JobDetailPage from './JobDetailPage'

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useParams: () => ({ id: 'test-job-1' }) }
})

vi.mock('../api', () => ({
  api: {
    jobs: {
      list: vi.fn(),
      get: vi.fn(),
      shortlist: vi.fn(),
      hide: vi.fn(),
      generate: vi.fn(),
      documents: vi.fn(),
    },
    sources: { list: vi.fn(), update: vi.fn(), test: vi.fn(), enrich: vi.fn() },
    searches: { list: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn(), run: vi.fn(), recentRuns: vi.fn() },
    profiles: { list: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn(), import: vi.fn() },
    documents: { update: vi.fn(), pdfUrl: vi.fn() },
    subscriptions: { list: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn(), run: vi.fn() },
    applications: { list: vi.fn(), update: vi.fn() },
    stats: vi.fn(),
  },
}))

const jobWithDocs = {
  ...mockJob(),
  matchResult: {
    id: 'm1', jobId: 'test-job-1', similarity: 0.82, score: 78,
    matchedSkills: ['React', 'TypeScript'], missingSkills: ['Go'],
    summary: 'Good fit', redFlags: ['Requires on-site'], model: 'test',
    createdAt: '2025-01-15T12:00:00Z',
  },
}

beforeEach(() => {
  vi.mocked(api.jobs.get).mockResolvedValue({ ...jobWithDocs, documents: [], application: null })
  vi.mocked(api.jobs.documents).mockResolvedValue([])
})

describe('JobDetailPage', () => {
  it('shows loading state', () => {
    vi.mocked(api.jobs.get).mockReturnValue(new Promise(() => {}))
    renderWithProviders(<JobDetailPage />)
    expect(screen.getByText('loading job…')).toBeInTheDocument()
  })

  it('renders job title and company', async () => {
    renderWithProviders(<JobDetailPage />)
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Senior React Developer' })).toBeInTheDocument()
      expect(screen.getByText(/Acme Corp/)).toBeInTheDocument()
    })
  })

  it('renders match analysis section', async () => {
    renderWithProviders(<JobDetailPage />)
    await waitFor(() => {
      expect(screen.getByText('Fit')).toBeInTheDocument()
      expect(screen.getByText('78')).toBeInTheDocument()
      expect(screen.getByText('Good fit')).toBeInTheDocument()
    })
  })

  it('renders matched and missing skill chips', async () => {
    renderWithProviders(<JobDetailPage />)
    await waitFor(() => {
      expect(screen.getByText('React')).toBeInTheDocument()
      expect(screen.getByText('TypeScript')).toBeInTheDocument()
      expect(screen.getByText('Go')).toBeInTheDocument()
    })
  })

  it('renders red flags', async () => {
    renderWithProviders(<JobDetailPage />)
    await waitFor(() => {
      expect(screen.getByText('Requires on-site')).toBeInTheDocument()
    })
  })

  it('renders generate buttons', async () => {
    renderWithProviders(<JobDetailPage />)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Generate resume' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Generate cover letter' })).toBeInTheDocument()
    })
  })

  it('renders job description', async () => {
    renderWithProviders(<JobDetailPage />)
    await waitFor(() => {
      expect(screen.getByText('Build amazing things')).toBeInTheDocument()
    })
  })

  it('renders back to feed link', async () => {
    renderWithProviders(<JobDetailPage />)
    await waitFor(() => {
      expect(screen.getByText('← back to feed')).toBeInTheDocument()
    })
  })

  it('renders document section', async () => {
    renderWithProviders(<JobDetailPage />)
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Documents' })).toBeInTheDocument()
    })
  })
})
