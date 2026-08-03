import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '../../test/test-utils'
import ResumeShapeCard from './ResumeShapeCard'

const defaults = {
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
  certificationsEnabled: true,
  certificationsMin: 0,
  certificationsMax: 0,
}

const getResumeShape = vi.fn()
const putResumeShape = vi.fn()
const resetResumeShape = vi.fn()

vi.mock('../../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api')>()
  return {
    ...actual,
    api: {
      settings: {
        getResumeShape: () => getResumeShape(),
        putResumeShape: (body: unknown) => putResumeShape(body),
        resetResumeShape: () => resetResumeShape(),
      },
    },
  }
})

describe('ResumeShapeCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getResumeShape.mockResolvedValue({ ...defaults })
    putResumeShape.mockImplementation((body) => Promise.resolve(body))
    resetResumeShape.mockResolvedValue({ ...defaults })
  })

  it('renders every configurable value with its current setting and allowed range', async () => {
    renderWithProviders(<ResumeShapeCard />)

    await waitFor(() => {
      expect(screen.getByLabelText('Summary sentences')).toHaveValue(4)
    })
    // Every value is visible in this single place, each with its range.
    expect(screen.getByText('Summary sentences (1-12)')).toBeInTheDocument()
    expect(screen.getByText('Max skill groups (0-20)')).toBeInTheDocument()
    expect(screen.getByText('Min bullets per job (1-20)')).toBeInTheDocument()
    expect(screen.getByText('Max bullets per job (1-20)')).toBeInTheDocument()
    expect(screen.getByText('Target pages (1-3)')).toBeInTheDocument()
    expect(screen.getByText('Min projects (0-20)')).toBeInTheDocument()
    expect(screen.getByText('Max projects (0-20)')).toBeInTheDocument()
    expect(screen.getByText('Max bullets per project (0-10)')).toBeInTheDocument()
    expect(screen.getByText('Min certifications (0-20)')).toBeInTheDocument()
    expect(screen.getByText('Max certifications (0-20)')).toBeInTheDocument()

    expect(screen.getByLabelText('Max bullets per job')).toHaveValue(10)
    expect(screen.getByLabelText('Target pages')).toHaveValue(2)
    expect(screen.getByLabelText('Include skills section')).toBeChecked()
    expect(screen.getByLabelText('Include projects section')).toBeChecked()
    expect(screen.getByLabelText('Include certifications section')).toBeChecked()
  })

  it('enables the save button when only certificationsEnabled is toggled', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ResumeShapeCard />)

    await waitFor(() => expect(screen.getByLabelText('Include certifications section')).toBeChecked())
    expect(screen.getByRole('button', { name: 'save' })).toBeDisabled()

    await user.click(screen.getByLabelText('Include certifications section'))

    expect(screen.getByRole('button', { name: 'save' })).toBeEnabled()
  })

  it('surfaces the API rejection message without corrupting the displayed values', async () => {
    const user = userEvent.setup()
    putResumeShape.mockRejectedValue(new Error('targetPages must be between 1 and 3'))
    renderWithProviders(<ResumeShapeCard />)

    await waitFor(() => expect(screen.getByLabelText('Target pages')).toHaveValue(2))
    const pages = screen.getByLabelText('Target pages')
    await user.clear(pages)
    await user.type(pages, '4')
    await user.click(screen.getByRole('button', { name: 'save' }))

    await waitFor(() => {
      expect(screen.getByText(/targetPages must be between 1 and 3/)).toBeInTheDocument()
    })
    // The other values are untouched: nothing was stored.
    expect(screen.getByLabelText('Summary sentences')).toHaveValue(4)
    expect(screen.getByLabelText('Max bullets per job')).toHaveValue(10)
  })

  it('restores the defaults in the UI after a reset', async () => {
    const user = userEvent.setup()
    getResumeShape.mockResolvedValue({ ...defaults, targetPages: 1, summaryLines: 8 })
    renderWithProviders(<ResumeShapeCard />)

    await waitFor(() => expect(screen.getByLabelText('Target pages')).toHaveValue(1))
    expect(screen.getByLabelText('Summary sentences')).toHaveValue(8)

    // After the reset the refetch serves the defaults again.
    getResumeShape.mockResolvedValue({ ...defaults })
    await user.click(screen.getByRole('button', { name: 'reset to defaults' }))

    await waitFor(() => expect(screen.getByLabelText('Target pages')).toHaveValue(2))
    expect(screen.getByLabelText('Summary sentences')).toHaveValue(4)
    expect(resetResumeShape).toHaveBeenCalled()
  })
})
