import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { InterviewPrepPack } from '@job-finder/shared'
import { renderWithProviders } from '../../test/test-utils'
import { api } from '../../lib/api'
import PrepPackPanel from './PrepPackPanel'

vi.mock('../../lib/api', () => ({
  api: {
    jobs: { interviewPrep: vi.fn() },
  },
}))

const windowPrint = vi.spyOn(window, 'print').mockImplementation(() => {})

function mockPack(overrides: Partial<InterviewPrepPack> = {}): InterviewPrepPack {
  return {
    jobId: 'job-1',
    generatedAt: '2026-07-21T12:00:00Z',
    questions: [
      {
        id: 'q1',
        text: 'The role requires Kubernetes. How would you approach zero-downtime deployments?',
        category: 'technical',
        source: 'required_skill',
        sourceExcerpt: 'Kubernetes experience required',
        mappedStories: [
          {
            storyId: 's1',
            storyTitle: 'Led migration to Kubernetes',
            relevanceScore: 0.85,
            matchedSkills: ['Kubernetes', 'Docker'],
            excerpt: 'Led the migration of a monolith to Kubernetes, reducing downtime by 90%.',
          },
        ],
      },
      {
        id: 'q2',
        text: 'Tell me about a time you led a cross-functional team.',
        category: 'behavioral',
        source: 'responsibility',
        sourceExcerpt: 'Lead cross-functional engineering teams',
        mappedStories: [],
      },
    ],
    companyNews: [
      { kind: 'funding', label: 'Series B', value: '$50M', source: 'Crunchbase', fetchedAt: '2026-07-01T00:00:00Z' },
    ],
    keywordGap: {
      missingRequired: ['Docker'],
      missingPreferred: ['Kafka'],
      coveragePct: 40,
      gapAwarenessTips: ['Docker is a core requirement — mention your container experience.'],
    },
    metadata: {
      totalQuestions: 2,
      coveredQuestions: 1,
      uncoveredQuestions: 1,
      staleNews: false,
    },
    ...overrides,
  }
}

describe('PrepPackPanel', () => {
  beforeEach(() => {
    vi.mocked(api.jobs.interviewPrep).mockResolvedValue(mockPack())
  })

  it('renders the section title', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText('Interview prep pack')).toBeInTheDocument()
    })
  })

  it('renders interview questions', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(
        screen.getByText(/The role requires Kubernetes/),
      ).toBeInTheDocument()
      expect(
        screen.getByText(/Tell me about a time/),
      ).toBeInTheDocument()
    })
  })

  it('renders question category chips', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText('Technical')).toBeInTheDocument()
      expect(screen.getByText('Behavioral')).toBeInTheDocument()
    })
  })

  it('renders uncovered gap indicator per question', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText('uncovered')).toBeInTheDocument()
    })
  })

  it('renders the uncovered gaps banner', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText(/1 uncovered question/)).toBeInTheDocument()
    })
  })

  it('renders the "No matching story" callout for uncovered questions', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText('No matching story')).toBeInTheDocument()
    })
  })

  it('renders story mappings for covered questions', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText('Led migration to Kubernetes')).toBeInTheDocument()
    })
  })

  it('renders story relevance score', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText('85')).toBeInTheDocument()
    })
  })

  it('renders company news briefing', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText('Company news briefing')).toBeInTheDocument()
      expect(screen.getByText('Series B')).toBeInTheDocument()
      expect(screen.getByText('$50M')).toBeInTheDocument()
      expect(screen.getByText('Crunchbase')).toBeInTheDocument()
    })
  })

  it('renders skill gap awareness section', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText('Skill gap awareness')).toBeInTheDocument()
      expect(screen.getByText((c) => c.includes('40%') && c.includes('keyword coverage'))).toBeInTheDocument()
      expect(screen.getAllByText('Docker').length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('Kafka')).toBeInTheDocument()
    })
  })

  it('renders the print button and triggers window.print', async () => {
    const user = userEvent.setup()
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    const printBtn = await screen.findByRole('button', { name: /print prep pack/i })
    await user.click(printBtn)

    expect(windowPrint).toHaveBeenCalled()
  })

  it('renders coverage summary in the header', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText(/1\/2 covered/)).toBeInTheDocument()
    })
  })

  it('shows generated timestamp', async () => {
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText(/Generated/)).toBeInTheDocument()
    })
  })

  it('shows empty state when the pack has not been computed', async () => {
    vi.mocked(api.jobs.interviewPrep).mockRejectedValue(new Error('404'))
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText(/no interview prep available yet/i)).toBeInTheDocument()
    })
  })

  it('renders empty-state text when there are no questions', async () => {
    vi.mocked(api.jobs.interviewPrep).mockResolvedValue(mockPack({
      questions: [],
      metadata: { totalQuestions: 0, coveredQuestions: 0, uncoveredQuestions: 0, staleNews: false },
    }))
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(
        screen.getByText(/No interview questions derived for this job description yet/),
      ).toBeInTheDocument()
    })
  })

  it('does not render uncovered banner when all questions are covered', async () => {
    vi.mocked(api.jobs.interviewPrep).mockResolvedValue(mockPack({
      questions: [{
        id: 'q1',
        text: 'The role requires Kubernetes. How would you approach zero-downtime deployments?',
        category: 'technical',
        source: 'required_skill',
        sourceExcerpt: 'Kubernetes experience required',
        mappedStories: [{
          storyId: 's1', storyTitle: 'Led migration to Kubernetes', relevanceScore: 0.85,
          matchedSkills: ['Kubernetes', 'Docker'], excerpt: 'Led migration.',
        }],
      }],
      metadata: { totalQuestions: 1, coveredQuestions: 1, uncoveredQuestions: 0, staleNews: false },
    }))
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.queryByText(/uncovered question/)).not.toBeInTheDocument()
      expect(screen.queryByText('uncovered')).not.toBeInTheDocument()
    })
  })

  it('collapses and expands a question card', async () => {
    const user = userEvent.setup()
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getAllByText(/from JD:/).length).toBe(2)
    })

    const questionHeader = screen.getByText(/The role requires Kubernetes/).closest('button')!
    await user.click(questionHeader)

    expect(screen.getAllByText(/from JD:/).length).toBe(1)

    await user.click(questionHeader)
    expect(screen.getAllByText(/from JD:/).length).toBe(2)
  })

  it('shows stale news warning when metadata indicates staleness', async () => {
    vi.mocked(api.jobs.interviewPrep).mockResolvedValue(mockPack({
      metadata: { totalQuestions: 1, coveredQuestions: 1, uncoveredQuestions: 0, staleNews: true },
    }))
    renderWithProviders(<PrepPackPanel jobId="job-1" />)

    await waitFor(() => {
      expect(screen.getByText(/News data may be stale/)).toBeInTheDocument()
    })
  })
})
