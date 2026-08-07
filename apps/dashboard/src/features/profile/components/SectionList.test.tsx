import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '@testing-library/react'
import type { Section } from '@job-finder/shared'
import { SectionList } from './SectionList'

const baseSections: Section[] = [
  { name: 'education', entryType: 'education', entries: [{ institution: 'MIT' }] },
  { name: 'experience', entryType: 'experience', entries: [{ company: 'Acme' }] },
]

describe('SectionList', () => {
  it('moves a section down via the move-down button and calls onChange with new order', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<SectionList sections={baseSections} onChange={onChange} />)

    await user.click(screen.getAllByLabelText('move section down')[0])

    expect(onChange).toHaveBeenCalledWith([baseSections[1], baseSections[0]])
  })

  it('requires confirmation before deleting a section', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<SectionList sections={baseSections} onChange={onChange} />)

    await user.click(screen.getAllByLabelText('delete section')[0])
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByText(/Delete section\?/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(onChange).toHaveBeenCalledWith([baseSections[1]])
  })

  it('adds a custom-named section with the chosen entry type', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<SectionList sections={[]} onChange={onChange} />)

    await user.type(screen.getByPlaceholderText(/New section name/i), 'Volunteering')
    await user.click(screen.getByRole('button', { name: /add section/i }))

    expect(onChange).toHaveBeenCalledWith([{ name: 'Volunteering', entryType: 'bullet', entries: [] }])
  })
})
