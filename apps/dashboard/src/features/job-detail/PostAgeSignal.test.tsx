import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '../../test/test-utils'
import { mockPostAgeResponse } from '../../test/factories'
import { api } from '../../api'
import PostAgeSignal from './PostAgeSignal'

vi.mock('../../api', () => ({
  api: {
    postage: { responseRate: vi.fn() },
  },
}))

describe('PostAgeSignal', () => {
  it('shows loading spinner initially', () => {
    vi.mocked(api.postage.responseRate).mockReturnValue(new Promise(() => {}))
    renderWithProviders(<PostAgeSignal />)
    expect(screen.getByText('loading response rate…')).toBeInTheDocument()
  })

  it('renders nothing on error', async () => {
    vi.mocked(api.postage.responseRate).mockRejectedValue(new Error('fail'))
    const { container } = renderWithProviders(<PostAgeSignal />)
    await waitFor(() => {
      expect(container).toBeEmptyDOMElement()
    })
  })

  it('renders observed buckets with rates', async () => {
    vi.mocked(api.postage.responseRate).mockResolvedValue(mockPostAgeResponse())
    renderWithProviders(<PostAgeSignal />)

    await waitFor(() => {
      expect(screen.getByText('Response rate by application timing')).toBeInTheDocument()
      expect(screen.getByText('42%')).toBeInTheDocument()
      expect(screen.getByText('25%')).toBeInTheDocument()
    })
    expect(screen.getByText('fresh')).toBeInTheDocument()
    expect(screen.getByText('recent')).toBeInTheDocument()
    expect(screen.getByText('(12)')).toBeInTheDocument()
    expect(screen.getByText('(8)')).toBeInTheDocument()
  })

  it('renders not enough data chip for insufficient buckets', async () => {
    vi.mocked(api.postage.responseRate).mockResolvedValue(
      mockPostAgeResponse({
        buckets: [
          { bucket: 'fresh', n: 3, responses: 0, rate: null, state: 'insufficient' },
        ],
        totalApps: 3,
        globalState: 'prior',
        priorRate: 0.2,
        priorLabel: 'Typical baseline — not yet your data',
        thresholdMsg: 'Need 27 more applications before showing your personal response rate.',
      }),
    )
    renderWithProviders(<PostAgeSignal />)

    await waitFor(() => {
      expect(screen.getByText('not enough data')).toBeInTheDocument()
      expect(screen.getByText(/Need 27 more applications/)).toBeInTheDocument()
    })
  })

  it('renders prior state with baseline rate', async () => {
    vi.mocked(api.postage.responseRate).mockResolvedValue(
      mockPostAgeResponse({
        buckets: [],
        totalApps: 0,
        globalState: 'prior',
        priorRate: 0.2,
        priorLabel: 'Typical baseline — not yet your data',
        thresholdMsg: 'Need 30 applications before showing your personal response rate.',
      }),
    )
    renderWithProviders(<PostAgeSignal />)

    await waitFor(() => {
      expect(screen.getByText(/Need 30 applications/)).toBeInTheDocument()
      expect(screen.getByText(/20% baseline/)).toBeInTheDocument()
    })
  })

  it('renders unknown bucket label', async () => {
    vi.mocked(api.postage.responseRate).mockResolvedValue(
      mockPostAgeResponse({
        buckets: [
          { bucket: 'unknown', n: 2, responses: 0, rate: null, state: 'insufficient' },
        ],
      }),
    )
    renderWithProviders(<PostAgeSignal />)

    await waitFor(() => {
      expect(screen.getByText('Posting date unknown')).toBeInTheDocument()
    })
  })

  it('renders dash for prior state buckets', async () => {
    vi.mocked(api.postage.responseRate).mockResolvedValue(
      mockPostAgeResponse({
        buckets: [
          { bucket: 'fresh', n: 5, responses: 1, rate: null, state: 'prior' },
        ],
        globalState: 'prior',
      }),
    )
    renderWithProviders(<PostAgeSignal />)

    await waitFor(() => {
      expect(screen.getByText('—')).toBeInTheDocument()
    })
  })
})
