import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '../test/test-utils'
import { mockJob, mockJobListResponse, mockSource } from '../test/factories'
import { api } from '../api'
import FeedPage from './FeedPage'

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
    postage: { responseRate: vi.fn() },
    notifications: { list: vi.fn(), markSeen: vi.fn(), unseenCount: vi.fn() },
  },
}))

beforeEach(() => {
  vi.mocked(api.jobs.list).mockResolvedValue(mockJobListResponse())
  vi.mocked(api.sources.list).mockResolvedValue([mockSource()])
})

describe('FeedPage', () => {
  it('shows loading spinner initially', () => {
    vi.mocked(api.jobs.list).mockReturnValue(new Promise(() => {}))
    renderWithProviders(<FeedPage />)
    expect(screen.getByText('loading jobs…')).toBeInTheDocument()
  })

  it('shows empty state when no jobs', async () => {
    vi.mocked(api.jobs.list).mockResolvedValue(mockJobListResponse({ items: [], total: 0 }))
    renderWithProviders(<FeedPage />)
    await waitFor(() => {
      expect(screen.getByText(/No jobs yet/)).toBeInTheDocument()
    })
  })

  it('renders job list with score badge, title, company', async () => {
    const job = mockJob({ matchResult: { id: 'm1', jobId: 'test-job-1', similarity: 0.85, score: 92, matchedSkills: ['React'], missingSkills: [], summary: null, redFlags: null, model: 'test', createdAt: '2025-01-15T12:00:00Z' } })
    vi.mocked(api.jobs.list).mockResolvedValue(mockJobListResponse({ items: [job] }))

    renderWithProviders(<FeedPage />)

    await waitFor(() => {
      expect(screen.getByText('92')).toBeInTheDocument()
      expect(screen.getByText('Senior React Developer')).toBeInTheDocument()
      expect(screen.getByText(/Acme Corp/)).toBeInTheDocument()
    })
  })

  it('renders source chip', async () => {
    renderWithProviders(<FeedPage />)
    await waitFor(() => {
      const chips = screen.getAllByText('remotive')
      expect(chips.length).toBeGreaterThanOrEqual(1)
    })
  })

  it('search input filters jobs', async () => {
    const user = userEvent.setup()
    renderWithProviders(<FeedPage />)

    await waitFor(() => {
      expect(screen.getByText('Senior React Developer')).toBeInTheDocument()
    })

    const searchInput = screen.getByPlaceholderText('Search title/company…')
    await user.type(searchInput, 'backend')

    await waitFor(() => {
      expect(api.jobs.list).toHaveBeenCalledWith(
        expect.objectContaining({ q: 'backend' }),
      )
    })
  })

  it('source select filters jobs', async () => {
    const user = userEvent.setup()
    renderWithProviders(<FeedPage />)

    await waitFor(() => {
      expect(screen.getByText('Senior React Developer')).toBeInTheDocument()
    })

    const sourceSelect = screen.getByDisplayValue('all sources')
    await user.selectOptions(sourceSelect, 'remotive')

    await waitFor(() => {
      expect(api.jobs.list).toHaveBeenCalledWith(
        expect.objectContaining({ source: 'remotive' }),
      )
    })
  })

  it('min score select filters jobs', async () => {
    const user = userEvent.setup()
    renderWithProviders(<FeedPage />)

    await waitFor(() => {
      expect(screen.getByText('Senior React Developer')).toBeInTheDocument()
    })

    const minScoreSelect = screen.getByDisplayValue('any score')
    await user.selectOptions(minScoreSelect, '80')

    await waitFor(() => {
      expect(api.jobs.list).toHaveBeenCalledWith(
        expect.objectContaining({ minScore: 80 }),
      )
    })
  })

  it('remote checkbox filters jobs', async () => {
    const user = userEvent.setup()
    renderWithProviders(<FeedPage />)

    await waitFor(() => {
      expect(screen.getByText('Senior React Developer')).toBeInTheDocument()
    })

    const remoteCheckbox = screen.getByRole('checkbox', { name: 'remote' })
    await user.click(remoteCheckbox)

    await waitFor(() => {
      expect(api.jobs.list).toHaveBeenCalledWith(
        expect.objectContaining({ remote: true }),
      )
    })
  })

  it('shows pagination when total > pageSize', async () => {
    vi.mocked(api.jobs.list).mockResolvedValue(
      mockJobListResponse({ total: 40, pageSize: 20, page: 1 }),
    )
    renderWithProviders(<FeedPage />)

    await waitFor(() => {
      expect(screen.getByText(/prev/)).toBeInTheDocument()
      expect(screen.getByText(/next/)).toBeInTheDocument()
      expect(screen.getByText(/page 1 \/ 2/)).toBeInTheDocument()
    })
  })

  it('prev button is disabled on first page', async () => {
    vi.mocked(api.jobs.list).mockResolvedValue(
      mockJobListResponse({ total: 40, pageSize: 20, page: 1 }),
    )
    renderWithProviders(<FeedPage />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /prev/ })).toBeDisabled()
    })
  })

  it('shows posting age on job card', async () => {
    const recent = new Date(Date.now() - 2 * 3600000).toISOString() // 2h ago
    const job = mockJob({ postedAt: recent })
    vi.mocked(api.jobs.list).mockResolvedValue(mockJobListResponse({ items: [job] }))

    renderWithProviders(<FeedPage />)

    await waitFor(() => {
      expect(screen.getByText(/2h ago/)).toBeInTheDocument()
    })
  })

  it('does not show posting age when postedAt is null', async () => {
    const job = mockJob({ postedAt: null })
    vi.mocked(api.jobs.list).mockResolvedValue(mockJobListResponse({ items: [job] }))

    renderWithProviders(<FeedPage />)

    await waitFor(() => {
      expect(screen.getByText('Senior React Developer')).toBeInTheDocument()
    })
    expect(screen.queryByText(/h ago/)).not.toBeInTheDocument()
    expect(screen.queryByText(/d ago/)).not.toBeInTheDocument()
  })
})
