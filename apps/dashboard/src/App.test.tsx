import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from './test/test-utils'
import { api } from './api'
import App from './App'

vi.mock('./api', () => ({
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
    profiles: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      remove: vi.fn(),
      uploadConfig: vi.fn(),
      configStatus: vi.fn(),
    },
    documents: { update: vi.fn(), pdfUrl: vi.fn() },
    subscriptions: { list: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn(), run: vi.fn() },
    applications: { list: vi.fn(), update: vi.fn() },
    stats: vi.fn(),
  },
}))

beforeEach(() => {
  vi.mocked(api.jobs.list).mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.mocked(api.sources.list).mockResolvedValue([])
  vi.mocked(api.searches.list).mockResolvedValue([])
  vi.mocked(api.searches.recentRuns).mockResolvedValue([])
  vi.mocked(api.profiles.list).mockResolvedValue([])
  vi.mocked(api.profiles.configStatus).mockResolvedValue({ hasConfig: true })
  vi.mocked(api.subscriptions.list).mockResolvedValue([])
  vi.mocked(api.applications.list).mockResolvedValue([])
})

describe('App', () => {
  it('renders nav links for Feed, Tracker, Sources, Profile', () => {
    renderWithProviders(<App />)
    expect(screen.getByRole('link', { name: 'Feed' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Tracker' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Sources' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Profile' })).toBeInTheDocument()
  })

  it('defaults to FeedPage', async () => {
    renderWithProviders(<App />)
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search title/company…')).toBeInTheDocument()
    })
  })

  it('navigates to Sources page on click', async () => {
    const user = userEvent.setup()
    renderWithProviders(<App />)
    await user.click(screen.getByRole('link', { name: 'Sources' }))
    expect(screen.getByRole('heading', { name: 'Job sources' })).toBeInTheDocument()
  })

  it('navigates to Tracker page on click', async () => {
    const user = userEvent.setup()
    renderWithProviders(<App />)
    await user.click(screen.getByRole('link', { name: 'Tracker' }))
    expect(screen.getByRole('heading', { name: 'Application tracker' })).toBeInTheDocument()
  })

  it('navigates to Profile page on click', async () => {
    const user = userEvent.setup()
    renderWithProviders(<App />)
    await user.click(screen.getByRole('link', { name: 'Profile' }))
    expect(screen.getByRole('heading', { name: 'Profile' })).toBeInTheDocument()
  })
})
