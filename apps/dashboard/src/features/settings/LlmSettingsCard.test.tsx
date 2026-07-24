import { screen, waitFor, within } from '@testing-library/react'
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

function seededSettings(credentialConfigured = true, openRouterCredentialConfigured = true) {
  return {
    credentialConfigured,
    openRouterCredentialConfigured,
    tasks: TASKS.map((taskKey) => ({ taskKey, provider: 'ollama', model: '' })),
  }
}

function curatedModels() {
  return {
    cerebras: [
      { id: 'gpt-oss-120b', label: 'GPT-OSS 120B', isDefault: true },
      { id: 'llama-3.3-70b', label: 'Llama 3.3 70B', isDefault: false },
    ],
    openrouter: [
      { id: 'deepseek/deepseek-chat-v3-0324:free', label: 'DeepSeek V3 (free)', isDefault: true },
      { id: 'deepseek/deepseek-r1:free', label: 'DeepSeek R1 (free)', isDefault: false },
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
      openRouterCredentialConfigured: true,
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
    expect(screen.queryByTestId('openrouter-credential-banner')).not.toBeInTheDocument()
  })

  it('switch-all to OpenRouter PUTs every task with provider openrouter', async () => {
    const user = userEvent.setup()
    vi.mocked(api.settings.getLlm).mockResolvedValue(seededSettings())
    vi.mocked(api.settings.putLlm).mockResolvedValue(seededSettings())
    renderWithProviders(<LlmSettingsCard />)

    await waitFor(() => screen.getByText('Job matching / scoring'))
    await user.click(screen.getByRole('button', { name: 'Switch all to OpenRouter' }))

    await waitFor(() => {
      expect(api.settings.putLlm).toHaveBeenCalledTimes(1)
    })
    const sentTasks = vi.mocked(api.settings.putLlm).mock.calls[0][0]
    expect(sentTasks).toHaveLength(TASKS.length)
    for (const t of sentTasks) {
      expect(t.provider).toBe('openrouter')
      expect(t.model).toBe('')
    }
  })

  it('offers the OpenRouter model list when a task is set to openrouter', async () => {
    vi.mocked(api.settings.getLlm).mockResolvedValue({
      credentialConfigured: true,
      openRouterCredentialConfigured: true,
      tasks: TASKS.map((taskKey) => ({
        taskKey,
        provider: taskKey === 'match' ? 'openrouter' : 'ollama',
        model: '',
      })),
    })
    renderWithProviders(<LlmSettingsCard />)

    await waitFor(() => screen.getByText('Job matching / scoring'))
    const modelSelect = screen.getByRole('combobox', { name: 'Job matching / scoring model' })
    expect(modelSelect).toBeEnabled()
    expect(within(modelSelect).getByRole('option', { name: 'DeepSeek R1 (free)' })).toBeInTheDocument()
    // Cerebras models must not leak into an OpenRouter task's model list.
    expect(within(modelSelect).queryByRole('option', { name: 'GPT-OSS 120B' })).not.toBeInTheDocument()
  })

  it('shows a credential-missing banner when an OpenRouter task is set but no key is configured', async () => {
    vi.mocked(api.settings.getLlm).mockResolvedValue({
      credentialConfigured: true,
      openRouterCredentialConfigured: false,
      tasks: TASKS.map((taskKey) => ({
        taskKey,
        provider: taskKey === 'ghost' ? 'openrouter' : 'ollama',
        model: '',
      })),
    })
    renderWithProviders(<LlmSettingsCard />)

    await waitFor(() => {
      expect(screen.getByTestId('openrouter-credential-banner')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('cerebras-credential-banner')).not.toBeInTheDocument()
  })
})
