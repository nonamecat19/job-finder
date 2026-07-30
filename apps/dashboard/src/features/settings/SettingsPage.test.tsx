import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '../../test/test-utils'
import SettingsPage from './SettingsPage'

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return {
    ...actual,
    api: {
      settings: {
        getLlm: vi
          .fn()
          .mockResolvedValue({ credentialConfigured: false, tasks: [] }),
        putLlm: vi.fn(),
        llmModels: vi.fn().mockResolvedValue({ cerebras: [] }),
        getAiFeatures: vi.fn().mockResolvedValue([]),
      },
    },
  }
})

describe('SettingsPage', () => {
  it('renders the settings page', async () => {
    renderWithProviders(<SettingsPage />)
    await waitFor(() => {
      expect(screen.getByText('Settings')).toBeInTheDocument()
    })
  })
})
