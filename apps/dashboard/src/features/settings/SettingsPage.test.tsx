import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '../../test/test-utils'
import { api } from '../../lib/api'
import SettingsPage from './SettingsPage'

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return {
    ...actual,
    api: {
      ext: { bootstrap: vi.fn() },
      settings: {
        getLlm: vi
          .fn()
          .mockResolvedValue({ credentialConfigured: false, openRouterCredentialConfigured: false, tasks: [] }),
        putLlm: vi.fn(),
        llmModels: vi.fn().mockResolvedValue({ cerebras: [], openrouter: [] }),
        getAiFeatures: vi.fn().mockResolvedValue([]),
      },
    },
  }
})

describe('SettingsPage', () => {
  it('renders the pairing card without generating a code up front', () => {
    renderWithProviders(<SettingsPage />)
    expect(screen.getByRole('button', { name: 'Generate pairing code' })).toBeInTheDocument()
    expect(screen.queryByTestId('bootstrap-code')).not.toBeInTheDocument()
    expect(api.ext.bootstrap).not.toHaveBeenCalled()
  })

  it('generates and displays a pairing code on click', async () => {
    const user = userEvent.setup()
    vi.mocked(api.ext.bootstrap).mockResolvedValue({
      code: 'abc123xyz',
      expiresAt: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
    })

    renderWithProviders(<SettingsPage />)
    await user.click(screen.getByRole('button', { name: 'Generate pairing code' }))

    await waitFor(() => {
      expect(screen.getByTestId('bootstrap-code')).toHaveTextContent('abc123xyz')
    })
    expect(api.ext.bootstrap).toHaveBeenCalledTimes(1)
  })

  it('shows an error state when bootstrap fails', async () => {
    const user = userEvent.setup()
    vi.mocked(api.ext.bootstrap).mockRejectedValue(new Error('network down'))

    renderWithProviders(<SettingsPage />)
    await user.click(screen.getByRole('button', { name: 'Generate pairing code' }))

    await waitFor(() => {
      expect(screen.getByText(/network down/)).toBeInTheDocument()
    })
  })
})
