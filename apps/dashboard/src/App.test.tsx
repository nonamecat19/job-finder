import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from './test/test-utils'
import { api } from './lib/api'
import App from './App'

vi.mock('./lib/api', () => ({
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
    postage: { responseRate: vi.fn() },
    notifications: { list: vi.fn(), markSeen: vi.fn(), unseenCount: vi.fn() },
    roster: { list: vi.fn(), register: vi.fn(), remove: vi.fn(), candidates: vi.fn(), accept: vi.fn(), reject: vi.fn(), discover: vi.fn() },
    hosts: { retrievalStatus: vi.fn(), clearRungPreference: vi.fn(), clearCookies: vi.fn(), overrideCoolingOff: vi.fn() },
  },
}))

beforeEach(() => {
  vi.mocked(api.jobs.list).mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.mocked(api.sources.list).mockResolvedValue([])
  vi.mocked(api.searches.list).mockResolvedValue([])
  vi.mocked(api.searches.recentRuns).mockResolvedValue([])
  vi.mocked(api.profiles.list).mockResolvedValue([])
  vi.mocked(api.profiles.configStatus).mockResolvedValue({ hasConfig: true, hasExistingContent: true })
  vi.mocked(api.subscriptions.list).mockResolvedValue([])
  vi.mocked(api.applications.list).mockResolvedValue([])
  vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 0 })
  vi.mocked(api.notifications.list).mockResolvedValue([])
  vi.mocked(api.roster.list).mockResolvedValue({ employers: [] })
  vi.mocked(api.roster.candidates).mockResolvedValue({ candidates: [] })
  vi.mocked(api.postage.responseRate).mockResolvedValue({
    buckets: [],
    totalApps: 0,
    globalState: 'prior',
    priorRate: 0.2,
    priorLabel: 'Typical baseline',
    thresholdMsg: null,
  })
})

describe('App', () => {
  it('renders nav links for Feed, Tracker, Sources, Profile', () => {
    renderWithProviders(<App />)
    expect(screen.getByRole('link', { name: 'Feed' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Tracker' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Sources' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Profile' })).toBeInTheDocument()
  })

  it('renders exactly one link per nav label (no duplicate nav DOM)', () => {
    renderWithProviders(<App />)
    for (const label of ['Feed', 'Tracker', 'Status', 'Sources', 'Profile']) {
      expect(screen.getAllByRole('link', { name: label })).toHaveLength(1)
    }
    expect(screen.getAllByRole('navigation')).toHaveLength(1)
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
    expect(screen.getByRole('heading', { name: 'Sources & searches' })).toBeInTheDocument()
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
