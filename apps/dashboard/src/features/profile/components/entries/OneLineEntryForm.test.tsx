import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Entry } from '@job-finder/shared'
import { OneLineEntryForm } from './OneLineEntryForm'

describe('OneLineEntryForm', () => {
  it('shows a density picker only for the skills section', () => {
    const onChange = vi.fn()
    render(
      <OneLineEntryForm
        sectionName="skills"
        entry={{ label: 'Backend', details: 'Go, Node.js' }}
        onChange={onChange}
      />,
    )
    expect(screen.getByLabelText('Skill density')).toBeInTheDocument()
  })

  it('does not show the density picker outside the skills section', () => {
    render(
      <OneLineEntryForm sectionName="certifications" entry={{ label: 'AWS' }} onChange={vi.fn()} />,
    )
    expect(screen.queryByLabelText('Skill density')).not.toBeInTheDocument()
  })

  it('defaults an unset level to all skills', () => {
    render(
      <OneLineEntryForm sectionName="skills" entry={{ label: 'Backend' }} onChange={vi.fn()} />,
    )
    expect(screen.getByLabelText('Skill density')).toHaveValue('all')
  })

  it('reflects an existing level', () => {
    render(
      <OneLineEntryForm
        sectionName="skills"
        entry={{ label: 'Backend', skillLevel: 'relevant' }}
        onChange={vi.fn()}
      />,
    )
    expect(screen.getByLabelText('Skill density')).toHaveValue('relevant')
  })

  it('writes the chosen level, and clears it for all', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <OneLineEntryForm
        sectionName="skills"
        entry={{ label: 'Backend', details: 'Go, Redis, Node.js' }}
        onChange={onChange}
      />,
    )
    const picker = screen.getByLabelText('Skill density')
    await user.selectOptions(picker, 'medium')
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ skillLevel: 'medium' }),
    )

    await user.selectOptions(picker, 'all')
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ skillLevel: undefined }),
    )
  })
})
