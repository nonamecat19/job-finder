import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '../../test/test-utils'
import SettingsPage from './SettingsPage'

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return {
    ...actual,
    api: {
      settings: {
        getAiFeatures: vi.fn().mockResolvedValue([]),
        getResumeShape: vi.fn().mockResolvedValue({
          summaryLines: 4,
          skillsEnabled: true,
          skillsMaxGroups: 0,
          experienceBulletsMin: 8,
          experienceBulletsMax: 10,
          targetPages: 2,
          projectsEnabled: true,
          projectsMin: 0,
          projectsMax: 0,
          projectBulletsMax: 0,
        }),
      },
    },
  }
})

describe('SettingsPage', () => {
  it('renders the settings page without the AI models tile', async () => {
    renderWithProviders(<SettingsPage />)
    await waitFor(() => {
      expect(screen.getByText('Settings')).toBeInTheDocument()
    })
    expect(screen.queryByText('AI models')).not.toBeInTheDocument()
    expect(screen.getByText('AI features')).toBeInTheDocument()
    expect(screen.getByText('Resume shape')).toBeInTheDocument()
    expect(screen.getByText('Danger zone')).toBeInTheDocument()
  })
})
