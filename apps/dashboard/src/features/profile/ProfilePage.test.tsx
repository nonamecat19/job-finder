import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { api } from '../../lib/api'
import ProfilePage from './ProfilePage'

vi.mock('../../lib/api', () => ({
  api: {
    profiles: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      remove: vi.fn(),
      uploadConfig: vi.fn(),
      configStatus: vi.fn(),
      getResume: vi.fn(),
      updateResume: vi.fn(),
    },
  },
}))

const emptyResume = { resume: { name: 'Jane Doe', sections: [] } }

function renderProfile(initialPath = '/profile') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route path="/profile/:tab?" element={<ProfilePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.mocked(api.profiles.configStatus).mockResolvedValue({ hasConfig: false, hasExistingContent: false })
})

describe('ProfilePage', () => {
  it('renders an empty resume as editable, not an error state', async () => {
    vi.mocked(api.profiles.list).mockResolvedValue([
      { id: 'p1', name: 'Jane Doe', hasConfig: false, extraNotes: null, updatedAt: new Date().toISOString() },
    ])
    vi.mocked(api.profiles.getResume).mockResolvedValue(emptyResume)

    renderProfile()

    await waitFor(() => expect(screen.getByLabelText('Name')).toHaveValue('Jane Doe'))
    expect(document.querySelector('.bg-danger-soft')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: 'Add section' }))
    expect(screen.getByText(/no sections yet/i)).toBeInTheDocument()
  })

  it('auto-creates a blank profile and lands directly on the editable form when none exists', async () => {
    vi.mocked(api.profiles.list).mockResolvedValueOnce([]).mockResolvedValue([
      { id: 'p1', name: 'My Profile', hasConfig: false, extraNotes: null, updatedAt: new Date().toISOString() },
    ])
    vi.mocked(api.profiles.create).mockResolvedValue({
      id: 'p1',
      name: 'My Profile',
      hasConfig: false,
      extraNotes: null,
      updatedAt: new Date().toISOString(),
    })
    vi.mocked(api.profiles.getResume).mockResolvedValue({ resume: { name: '', sections: [] } })

    renderProfile()

    await waitFor(() => expect(api.profiles.create).toHaveBeenCalledWith({ name: 'My Profile' }))
    expect(screen.queryByPlaceholderText('e.g. Jane Doe')).not.toBeInTheDocument()

    await waitFor(() => expect(screen.getByLabelText('Name')).toBeInTheDocument())

    await userEvent.click(screen.getByRole('tab', { name: 'Config' }))
    expect(screen.getByRole('button', { name: /import config/i })).toBeInTheDocument()
  })

  it('adds a section via the Add section tab and opens it', async () => {
    vi.mocked(api.profiles.list).mockResolvedValue([
      { id: 'p1', name: 'Jane Doe', hasConfig: false, extraNotes: null, updatedAt: new Date().toISOString() },
    ])
    vi.mocked(api.profiles.getResume).mockResolvedValue(emptyResume)

    renderProfile()
    await waitFor(() => expect(screen.getByLabelText('Name')).toHaveValue('Jane Doe'))

    await userEvent.click(screen.getByRole('tab', { name: 'Add section' }))
    await userEvent.type(screen.getByPlaceholderText(/New section name/i), 'Volunteering')
    await userEvent.click(screen.getByRole('button', { name: /add section/i }))

    expect(await screen.findByRole('tab', { name: 'Volunteering' })).toBeInTheDocument()
    expect(screen.getByLabelText('section name')).toHaveValue('Volunteering')
  })

  it('requires confirmation before a config upload overwrites existing content', async () => {
    vi.mocked(api.profiles.list).mockResolvedValue([
      { id: 'p1', name: 'Jane Doe', hasConfig: true, extraNotes: null, updatedAt: new Date().toISOString() },
    ])
    vi.mocked(api.profiles.configStatus).mockResolvedValue({ hasConfig: true, hasExistingContent: true })
    vi.mocked(api.profiles.getResume).mockResolvedValue({
      resume: { name: 'Jane Doe', sections: [{ name: 'experience', entryType: 'experience', entries: [{ company: 'Acme' }] }] },
    })

    renderProfile()
    await waitFor(() => expect(screen.getByLabelText('Name')).toBeInTheDocument())

    const file = new File(['cv:\n  name: Jane'], 'config.yaml', { type: 'application/yaml' })
    const input = document.querySelector('input[type="file"]') as HTMLInputElement
    await userEvent.upload(input, file)

    const dialog = await screen.findByText(/Replace existing resume content/i)
    expect(dialog).toBeInTheDocument()
    expect(api.profiles.uploadConfig).not.toHaveBeenCalled()

    const confirmButton = within(dialog.closest('[role="dialog"]') ?? document.body).getByRole('button', { name: 'Replace' })
    await userEvent.click(confirmButton)
    await waitFor(() => expect(api.profiles.uploadConfig).toHaveBeenCalledWith(file))
  })
})
