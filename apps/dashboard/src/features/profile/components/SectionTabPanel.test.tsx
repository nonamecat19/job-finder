import { screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '@testing-library/react'
import type { Section } from '@job-finder/shared'
import { SectionTabPanel } from './SectionTabPanel'

const baseSection: Section = { name: 'experience', entryType: 'experience', entries: [{ company: 'Acme' }] }

describe('SectionTabPanel', () => {
  it('calls onMove(-1) and onMove(1) from the move buttons', async () => {
    const onMove = vi.fn()
    const user = userEvent.setup()
    render(
      <SectionTabPanel
        section={baseSection}
        canMoveUp
        canMoveDown
        onRename={vi.fn()}
        onMove={onMove}
        onDelete={vi.fn()}
        onChange={vi.fn()}
      />,
    )

    await user.click(screen.getByLabelText('move section up'))
    await user.click(screen.getByLabelText('move section down'))

    expect(onMove).toHaveBeenNthCalledWith(1, -1)
    expect(onMove).toHaveBeenNthCalledWith(2, 1)
  })

  it('requires confirmation before deleting a section', async () => {
    const onDelete = vi.fn()
    const user = userEvent.setup()
    render(
      <SectionTabPanel
        section={baseSection}
        canMoveUp
        canMoveDown
        onRename={vi.fn()}
        onMove={vi.fn()}
        onDelete={onDelete}
        onChange={vi.fn()}
      />,
    )

    await user.click(screen.getByLabelText('delete section'))
    expect(onDelete).not.toHaveBeenCalled()
    expect(screen.getByText(/Delete section\?/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(onDelete).toHaveBeenCalledTimes(1)
  })

  it('renames the section via the name input', async () => {
    const onRename = vi.fn()
    render(
      <SectionTabPanel
        section={baseSection}
        canMoveUp={false}
        canMoveDown={false}
        onRename={onRename}
        onMove={vi.fn()}
        onDelete={vi.fn()}
        onChange={vi.fn()}
      />,
    )

    fireEvent.change(screen.getByLabelText('section name'), { target: { value: 'volunteering' } })

    expect(onRename).toHaveBeenCalledWith('volunteering')
  })
})
