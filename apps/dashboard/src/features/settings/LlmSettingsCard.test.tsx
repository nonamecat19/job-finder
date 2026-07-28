import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '../../test/test-utils'
import { api } from '../../lib/api'
import LlmSettingsCard from './LlmSettingsCard'

vi.mock('../../lib/api', () => ({
  api: {
    settings: {
      getLlm: vi.fn(),
      putLlm: vi.fn(),
      llmModels: vi.fn(),
    },
  },
}))

const TASKS = ['match', 'generation', 'rephrase', 'ghost', 'default']

function seededSettings(credentialConfigured = true) {
  return {
    credentialConfigured,
    tasks: TASKS.map((taskKey) => ({ taskKey, provider: 'ollama', model: '' })),
  }
}

function curatedModels() {
  return {
    cerebras: [
      { id: 'gpt-oss-120b', label: 'GPT-OSS 120B', isDefault: true },
      { id: 'llama-3.3-70b', label: 'Llama 3.3 70B', isDefault: false },
    ],
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.settings.llmModels).mockResolvedValue(curatedModels())
})

describe('LlmSettingsCard', () => {
  it('renders a row per task with provider Ollama by default', async () => {
    vi.mocked(api.settings.getLlm).mockResolvedValue(seededSettings())
    renderWithProviders(<LlmSettingsCard />)

    await waitFor(() => {
      expect(screen.getByText('Job matching / scoring')).toBeInTheDocument()
    })
    for (const label of [
      'Job matching / scoring',
      'Resume & cover letter generation',
      'Keyword rephrase suggestions',
      'Ghost-job detection',
      'Other (recruiter, outreach, salary)',
    ]) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
  })

  it('switch-all to Cerebras PUTs every task with provider cerebras', async () => {
    const user = userEvent.setup()
    vi.mocked(api.settings.getLlm).mockResolvedValue(seededSettings())
    vi.mocked(api.settings.putLlm).mockResolvedValue(seededSettings())
    renderWithProviders(<LlmSettingsCard />)

    await waitFor(() => screen.getByText('Job matching / scoring'))
    await user.click(screen.getByRole('button', { name: 'Switch all to Cerebras' }))

    await waitFor(() => {
      expect(api.settings.putLlm).toHaveBeenCalledTimes(1)
    })
    const sentTasks = vi.mocked(api.settings.putLlm).mock.calls[0][0]
    expect(sentTasks).toHaveLength(TASKS.length)
    for (const t of sentTasks) {
      expect(t.provider).toBe('cerebras')
    }
  })

  it('changing one task provider PUTs only that task', async () => {
    const user = userEvent.setup()
    vi.mocked(api.settings.getLlm).mockResolvedValue(seededSettings())
    vi.mocked(api.settings.putLlm).mockResolvedValue(seededSettings())
    renderWithProviders(<LlmSettingsCard />)

    await waitFor(() => screen.getByText('Resume & cover letter generation'))
    const providerSelect = screen.getByRole('combobox', {
      name: 'Resume & cover letter generation provider',
    })
    await user.selectOptions(providerSelect, 'cerebras')

    await waitFor(() => {
      expect(api.settings.putLlm).toHaveBeenCalledTimes(1)
    })
    const sentTasks = vi.mocked(api.settings.putLlm).mock.calls[0][0]
    expect(sentTasks).toEqual([{ taskKey: 'generation', provider: 'cerebras', model: '' }])
  })

  it('model dropdown is disabled while provider is ollama', async () => {
    vi.mocked(api.settings.getLlm).mockResolvedValue(seededSettings())
    renderWithProviders(<LlmSettingsCard />)

    await waitFor(() => screen.getByText('Ghost-job detection'))
    const modelSelect = screen.getByRole('combobox', { name: 'Ghost-job detection model' })
    expect(modelSelect).toBeDisabled()
  })

  it('shows a credential-missing banner when a Cerebras task is set but no key is configured', async () => {
    vi.mocked(api.settings.getLlm).mockResolvedValue({
      credentialConfigured: false,
      tasks: TASKS.map((taskKey) => ({
        taskKey,
        provider: taskKey === 'ghost' ? 'cerebras' : 'ollama',
        model: '',
      })),
    })
    renderWithProviders(<LlmSettingsCard />)

    await waitFor(() => {
      expect(screen.getByTestId('cerebras-credential-banner')).toBeInTheDocument()
    })
  })

  it('hides the credential-missing banner when a credential is configured', async () => {
    vi.mocked(api.settings.getLlm).mockResolvedValue(seededSettings(true))
    renderWithProviders(<LlmSettingsCard />)

    await waitFor(() => screen.getByText('Job matching / scoring'))
    expect(screen.queryByTestId('cerebras-credential-banner')).not.toBeInTheDocument()
  })
})
