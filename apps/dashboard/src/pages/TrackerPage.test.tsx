import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '../test/test-utils'
import { mockApplication, mockJob } from '../test/factories'
import { api } from '../api'
import TrackerPage from './TrackerPage'

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

beforeEach(() => {
  vi.mocked(api.applications.list).mockResolvedValue([
    mockApplication({ id: 'app-1', status: 'shortlisted', job: mockJob({ id: 'j1', dedupeKey: 'd1', sourceKey: 'remotive', title: 'React Dev', company: 'Acme', location: null, remote: true, salaryRaw: null, url: 'https://example.com', description: 'test', postedAt: null, ingestedAt: '2025-01-15T12:00:00Z', status: 'shortlisted', matchResult: { id: 'm1', jobId: 'j1', similarity: 0.9, score: 88, matchedSkills: ['React'], missingSkills: [], summary: null, redFlags: null, model: 'test', createdAt: '2025-01-15T12:00:00Z' } }) }),
    mockApplication({ id: 'app-2', status: 'applied', job: mockJob({ id: 'j2', dedupeKey: 'd2', sourceKey: 'djinni', title: 'Go Dev', company: 'Beta Inc', location: 'Kyiv', remote: false, salaryRaw: '$80k', url: 'https://example.com/2', description: 'test', postedAt: null, ingestedAt: '2025-01-15T12:00:00Z', status: 'applied', matchResult: undefined }) }),
  ])
})

describe('TrackerPage', () => {
  it('renders heading', async () => {
    renderWithProviders(<TrackerPage />)
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Application tracker' })).toBeInTheDocument()
    })
  })

  it('renders kanban columns', async () => {
    renderWithProviders(<TrackerPage />)
    await waitFor(() => {
      expect(screen.getByText('Shortlisted')).toBeInTheDocument()
      expect(screen.getByText('Docs ready')).toBeInTheDocument()
      expect(screen.getByText('Applied')).toBeInTheDocument()
      expect(screen.getByText('Interview')).toBeInTheDocument()
      expect(screen.getByText('Offer')).toBeInTheDocument()
      expect(screen.getByText('Rejected')).toBeInTheDocument()
    })
  })

  it('renders job cards in correct columns', async () => {
    renderWithProviders(<TrackerPage />)
    await waitFor(() => {
      expect(screen.getByText('Acme')).toBeInTheDocument()
      expect(screen.getByText('React Dev')).toBeInTheDocument()
      expect(screen.getByText('Beta Inc')).toBeInTheDocument()
      expect(screen.getByText('Go Dev')).toBeInTheDocument()
    })
  })

  it('shows column count', async () => {
    renderWithProviders(<TrackerPage />)
    await waitFor(() => {
      const counts = screen.getAllByText('(1)')
      expect(counts.length).toBeGreaterThanOrEqual(1)
    })
  })

  it('shows loading state', () => {
    vi.mocked(api.applications.list).mockReturnValue(new Promise(() => {}))
    renderWithProviders(<TrackerPage />)
    expect(screen.getByText('loading tracker…')).toBeInTheDocument()
  })
})
