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

  it('defaults an unset level to only the skills the job asks for', () => {
    render(
      <OneLineEntryForm sectionName="skills" entry={{ label: 'Backend' }} onChange={vi.fn()} />,
    )
    expect(screen.getByLabelText('Skill density')).toHaveValue('relevant')
  })

  it('reflects an existing level', () => {
    render(
      <OneLineEntryForm
        sectionName="skills"
        entry={{ label: 'Backend', skillLevel: 'top10' }}
        onChange={vi.fn()}
      />,
    )
    expect(screen.getByLabelText('Skill density')).toHaveValue('top10')
  })

  it('writes the chosen level, and clears it for the relevant default', async () => {
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
    await user.selectOptions(picker, 'top5')
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ skillLevel: 'top5' }),
    )

    await user.selectOptions(picker, 'top10')
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ skillLevel: 'top10' }),
    )

    await user.selectOptions(picker, 'top15')
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ skillLevel: 'top15' }),
    )

    await user.selectOptions(picker, 'top20')
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ skillLevel: 'top20' }),
    )

    await user.selectOptions(picker, 'all')
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ skillLevel: 'all' }),
    )

    await user.selectOptions(picker, 'relevant')
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ skillLevel: undefined }),
    )
  })

  it('renders skills as badges in the skills section', () => {
    render(
      <OneLineEntryForm
        sectionName="skills"
        entry={{ label: 'Backend', details: 'Go, Node.js' }}
        onChange={vi.fn()}
      />,
    )
    expect(screen.getByText('Go')).toBeInTheDocument()
    expect(screen.getByText('Node.js')).toBeInTheDocument()
  })

  it('removes a skill badge and joins the rest back to a details string', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <OneLineEntryForm
        sectionName="skills"
        entry={{ label: 'Backend', details: 'Go, Node.js, Rust' }}
        onChange={onChange}
      />,
    )
    await user.click(screen.getByLabelText('remove Node.js'))
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ details: 'Go, Rust' }),
    )
  })

  it('adds a skill by typing and pressing enter', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <OneLineEntryForm
        sectionName="skills"
        entry={{ label: 'Backend', details: 'Go, Node.js' }}
        onChange={onChange}
      />,
    )
    await user.type(screen.getByLabelText('Skill details'), 'Rust{Enter}')
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining<Partial<Entry>>({ details: 'Go, Node.js, Rust' }),
    )
  })
})
