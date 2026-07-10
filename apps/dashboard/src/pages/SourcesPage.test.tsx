import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '../test/test-utils'
import { mockSource, mockSavedSearch, mockSubscription, mockSourceRun } from '../test/factories'
import { api } from '../api'
import SourcesPage from './SourcesPage'

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
  vi.mocked(api.sources.list).mockResolvedValue([
    mockSource({ key: 'remotive' }),
    mockSource({ id: 'src-2', key: 'djinni', kind: 'scrape' }),
  ])
  vi.mocked(api.searches.list).mockResolvedValue([
    mockSavedSearch({ name: 'React remote', query: { keywords: 'React', remote: true } }),
  ])
  vi.mocked(api.subscriptions.list).mockResolvedValue([
    mockSubscription({ sourceKey: 'remotive', name: 'Remotive RSS' }),
  ])
  vi.mocked(api.searches.recentRuns).mockResolvedValue([
    mockSourceRun({ sourceKey: 'remotive', found: 10, new: 3 }),
  ])
})

describe('SourcesPage', () => {
  it('renders job sources heading', async () => {
    renderWithProviders(<SourcesPage />)
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Job sources' })).toBeInTheDocument()
    })
  })

  it('renders source keys', async () => {
    renderWithProviders(<SourcesPage />)
    await waitFor(() => {
      const remotive = screen.getAllByText('remotive')
      expect(remotive.length).toBeGreaterThanOrEqual(1)
      const djinni = screen.getAllByText('djinni')
      expect(djinni.length).toBeGreaterThanOrEqual(1)
    })
  })

  it('renders enabled checkboxes', async () => {
    renderWithProviders(<SourcesPage />)
    await waitFor(() => {
      const checkboxes = screen.getAllByRole('checkbox', { name: 'enabled' })
      expect(checkboxes.length).toBeGreaterThanOrEqual(2)
    })
  })

  it('renders test buttons', async () => {
    renderWithProviders(<SourcesPage />)
    await waitFor(() => {
      const testButtons = screen.getAllByRole('button', { name: 'Test' })
      expect(testButtons.length).toBeGreaterThanOrEqual(2)
    })
  })

  it('renders saved searches section', async () => {
    renderWithProviders(<SourcesPage />)
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Saved searches' })).toBeInTheDocument()
      expect(screen.getByText('React remote')).toBeInTheDocument()
    })
  })

  it('renders recent runs table', async () => {
    renderWithProviders(<SourcesPage />)
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Recent runs' })).toBeInTheDocument()
      const remotive = screen.getAllByText('remotive')
      expect(remotive.length).toBeGreaterThanOrEqual(1)
    })
  })
})
