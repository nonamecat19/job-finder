import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '../test/test-utils'
import { mockProfile } from '../test/factories'
import { api } from '../api'
import ProfilePage from './ProfilePage'

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
  vi.mocked(api.profiles.list).mockResolvedValue([
    mockProfile({ name: 'Backend profile', document: { basics: { name: 'Jane' }, skills: [{ name: 'Go' }], work: [] } }),
  ])
})

describe('ProfilePage', () => {
  it('shows loading state', () => {
    vi.mocked(api.profiles.list).mockReturnValue(new Promise(() => {}))
    renderWithProviders(<ProfilePage />)
    expect(screen.getByText('loading profiles…')).toBeInTheDocument()
  })

  it('renders heading', async () => {
    renderWithProviders(<ProfilePage />)
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Master profile' })).toBeInTheDocument()
    })
  })

  it('renders profile name input', async () => {
    renderWithProviders(<ProfilePage />)
    await waitFor(() => {
      const nameInput = screen.getByDisplayValue('Backend profile')
      expect(nameInput).toBeInTheDocument()
    })
  })

  it('renders JSON Resume textarea', async () => {
    renderWithProviders(<ProfilePage />)
    await waitFor(() => {
      expect(screen.getByText('Document (JSON Resume format)')).toBeInTheDocument()
      const textareas = screen.getAllByRole('textbox')
      const jsonTextarea = textareas.find((t) => t.className.includes('font-mono'))
      expect(jsonTextarea).toBeInTheDocument()
    })
  })

  it('renders extra notes textarea', async () => {
    renderWithProviders(<ProfilePage />)
    await waitFor(() => {
      expect(screen.getByText(/Extra notes/)).toBeInTheDocument()
    })
  })

  it('renders save button', async () => {
    renderWithProviders(<ProfilePage />)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Save profile' })).toBeInTheDocument()
    })
  })

  it('renders new profile button', async () => {
    renderWithProviders(<ProfilePage />)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: '+ new' })).toBeInTheDocument()
    })
  })

  it('renders import PDF button', async () => {
    renderWithProviders(<ProfilePage />)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Import resume PDF' })).toBeInTheDocument()
    })
  })

  it('renders profile selector dropdown', async () => {
    renderWithProviders(<ProfilePage />)
    await waitFor(() => {
      expect(screen.getByDisplayValue('Backend profile')).toBeInTheDocument()
    })
  })
})
