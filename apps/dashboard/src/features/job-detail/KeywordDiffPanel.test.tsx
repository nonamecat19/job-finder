import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { KeywordDiffResponse } from '@job-finder/shared'
import { renderWithProviders } from '../../test/test-utils'
import { api } from '../../lib/api'
import KeywordDiffPanel from './KeywordDiffPanel'

vi.mock('../../lib/api', () => ({
  api: {
    jobs: { keywordDiff: vi.fn() },
  },
}))

function mockDiff(overrides: Partial<KeywordDiffResponse> = {}): KeywordDiffResponse {
  return {
    jobId: 'job-1',
    matched: [
      { term: 'kubernetes', canonical: 'Kubernetes', polarity: 'required', normalized: 'kubernet', matchType: 'exact' },
      { term: 'typescript', canonical: 'TypeScript', polarity: 'preferred', normalized: 'typescript' },
    ],
    missingRequired: [
      { term: 'docker', canonical: 'Docker', polarity: 'required', normalized: 'docker' },
      { term: 'kafka', canonical: 'Kafka', polarity: 'required', normalized: 'kafka' },
    ],
    missingPreferred: [{ term: 'grpc', canonical: 'gRPC', polarity: 'preferred', normalized: 'grpc' }],
    metadata: { totalRequired: 3, totalPreferred: 2, matchedRequired: 1, matchedPreferred: 1, coveragePct: 40 },
    suggestions: [
      {
        term: 'docker',
        canonical: 'Docker',
        rephrase: 'Built and shipped containerized services in CI pipelines',
        sourceBullet: 'Set up CI pipelines',
      },
      { term: 'kafka', canonical: 'Kafka', rephrase: null, reason: 'no-honest-rephrase-available' },
    ],
    ...overrides,
  }
}

describe('KeywordDiffPanel', () => {
  beforeEach(() => {
    vi.mocked(api.jobs.keywordDiff).mockResolvedValue(mockDiff())
  })

  it('renders the three term groups with their terms', async () => {
    renderWithProviders(<KeywordDiffPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText('Matched')).toBeInTheDocument()
    })
    expect(screen.getByText('Missing — required')).toBeInTheDocument()
    expect(screen.getByText('Missing — preferred')).toBeInTheDocument()

    expect(screen.getByText('Kubernetes')).toBeInTheDocument()
    expect(screen.getByText('TypeScript')).toBeInTheDocument()
    expect(screen.getByText('Docker')).toBeInTheDocument()
    expect(screen.getByText('Kafka')).toBeInTheDocument()
    expect(screen.getByText('gRPC')).toBeInTheDocument()
  })

  it('shows the inline rephrase suggestion and its source bullet', async () => {
    renderWithProviders(<KeywordDiffPanel jobId="job-1" />)

    await waitFor(() => {
      expect(
        screen.getByText('Built and shipped containerized services in CI pipelines'),
      ).toBeInTheDocument()
    })
    expect(screen.getByText(/Set up CI pipelines/)).toBeInTheDocument()
    // The no-honest-rephrase term shows the advisory note instead of text.
    expect(screen.getByText(/No honest rephrase available/i)).toBeInTheDocument()
  })

  it('copies the suggestion to the clipboard', async () => {
    // userEvent.setup() installs a clipboard stub on navigator; spy on it.
    const user = userEvent.setup()
    const writeText = vi.spyOn(navigator.clipboard, 'writeText')

    renderWithProviders(<KeywordDiffPanel jobId="job-1" />)

    const copyBtn = await screen.findByRole('button', { name: /copy suggestion/i })
    await user.click(copyBtn)

    expect(writeText).toHaveBeenCalledWith('Built and shipped containerized services in CI pipelines')
  })

  it('renders nothing when the diff has not been computed', async () => {
    vi.mocked(api.jobs.keywordDiff).mockRejectedValue(new Error('404'))
    const { container } = renderWithProviders(<KeywordDiffPanel jobId="job-1" />)

    await waitFor(() => {
      expect(container).toBeEmptyDOMElement()
    })
  })
})
