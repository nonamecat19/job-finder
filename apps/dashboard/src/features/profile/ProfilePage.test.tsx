import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '../../test/test-utils'
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

beforeEach(() => {
  vi.mocked(api.profiles.configStatus).mockResolvedValue({ hasConfig: false, hasExistingContent: false })
})

describe('ProfilePage', () => {
  // FR-012: a profile with no config/sections yet must render as editable,
  // not as an error or blank screen (spec 009 quickstart.md Scenario 1).
  it('renders an empty resume as editable, not an error state', async () => {
    vi.mocked(api.profiles.list).mockResolvedValue([
      { id: 'p1', name: 'Jane Doe', hasConfig: false, extraNotes: null, updatedAt: new Date().toISOString() },
    ])
    vi.mocked(api.profiles.getResume).mockResolvedValue(emptyResume)

    renderWithProviders(<ProfilePage />)

    await waitFor(() => {
      expect(screen.getByText(/no sections yet/i)).toBeInTheDocument()
    })
    expect(document.querySelector('.bg-danger-soft')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Name')).toHaveValue('Jane Doe')
  })

  // FR-001/FR-012: a brand-new user must land directly on the full editable
  // form + import button, never gated behind a "name it first" step — a
  // blank profile is created silently in the background.
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

    renderWithProviders(<ProfilePage />)

    await waitFor(() => expect(api.profiles.create).toHaveBeenCalledWith({ name: 'My Profile' }))
    expect(screen.queryByPlaceholderText('e.g. Jane Doe')).not.toBeInTheDocument()

    await waitFor(() => expect(screen.getByRole('button', { name: /import config/i })).toBeInTheDocument())
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
  })

  // FR-010: uploading a new config over a profile that already has resume
  // content must be gated behind an explicit confirmation.
  it('requires confirmation before a config upload overwrites existing content', async () => {
    vi.mocked(api.profiles.list).mockResolvedValue([
      { id: 'p1', name: 'Jane Doe', hasConfig: true, extraNotes: null, updatedAt: new Date().toISOString() },
    ])
    vi.mocked(api.profiles.configStatus).mockResolvedValue({ hasConfig: true, hasExistingContent: true })
    vi.mocked(api.profiles.getResume).mockResolvedValue({
      resume: { name: 'Jane Doe', sections: [{ name: 'experience', entryType: 'experience', entries: [{ company: 'Acme' }] }] },
    })

    renderWithProviders(<ProfilePage />)
    await waitFor(() => expect(screen.getByText(/Profile: Jane Doe/)).toBeInTheDocument())

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
