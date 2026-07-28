import { render, screen } from '@testing-library/react'
import { DashboardGrid } from '../DashboardGrid'
import { GridCard } from '../GridCard'

describe('DashboardGrid', () => {
  it('renders children', () => {
    render(
      <DashboardGrid>
        <div data-testid="child">Content</div>
      </DashboardGrid>,
    )
    expect(screen.getByTestId('child')).toBeInTheDocument()
  })

  it('applies responsive grid classes', () => {
    render(<DashboardGrid><div /></DashboardGrid>)
    const grid = document.querySelector('.grid')
    expect(grid).toBeInTheDocument()
    expect(grid!.className).toContain('grid')
    expect(grid!.className).toContain('md:grid-cols-2')
    expect(grid!.className).toContain('lg:grid-cols-3')
    expect(grid!.className).toContain('items-start')
  })

  it('applies responsive gap classes', () => {
    render(<DashboardGrid><div /></DashboardGrid>)
    const grid = document.querySelector('.grid')
    expect(grid!.className).toContain('gap-3')
    expect(grid!.className).toContain('lg:gap-5')
  })

  it('merges additional className', () => {
    render(<DashboardGrid className="extra"><div /></DashboardGrid>)
    const grid = document.querySelector('.grid')
    expect(grid!.className).toContain('extra')
  })
})

describe('GridCard', () => {
  it('renders children', () => {
    render(
      <GridCard span="narrow">
        <div data-testid="card-child">Card Content</div>
      </GridCard>,
    )
    expect(screen.getByTestId('card-child')).toBeInTheDocument()
  })

  it('defaults to narrow span', () => {
    render(<GridCard><div /></GridCard>)
    const card = document.querySelector('.min-w-0')
    expect(card!.className).not.toContain('col-span-2')
    expect(card!.className).not.toContain('col-span-3')
  })

  it('applies wide span with responsive classes', () => {
    render(<GridCard span="wide"><div /></GridCard>)
    const card = document.querySelector('.min-w-0')
    expect(card!.className).toContain('md:col-span-2')
    expect(card!.className).toContain('lg:col-span-2')
  })

  it('applies full span with responsive classes', () => {
    render(<GridCard span="full"><div /></GridCard>)
    const card = document.querySelector('.min-w-0')
    expect(card!.className).toContain('md:col-span-2')
    expect(card!.className).toContain('lg:col-span-3')
  })

  it('does not apply full span classes for narrow', () => {
    render(<GridCard span="narrow"><div /></GridCard>)
    const card = document.querySelector('.min-w-0')
    expect(card!.className).not.toContain('col-span-2')
    expect(card!.className).not.toContain('col-span-3')
  })

  it('has min-w-0 overflow safeguard', () => {
    render(<GridCard span="wide"><div /></GridCard>)
    const card = document.querySelector('.min-w-0')
    expect(card!.className).toContain('min-w-0')
  })

  it('renders multiple cards with different spans without overlap classes', () => {
    render(
      <DashboardGrid>
        <GridCard span="full"><div data-testid="full" /></GridCard>
        <GridCard span="wide"><div data-testid="wide" /></GridCard>
        <GridCard span="narrow"><div data-testid="narrow" /></GridCard>
      </DashboardGrid>,
    )
    const full = screen.getByTestId('full').parentElement!
    const wide = screen.getByTestId('wide').parentElement!
    const narrow = screen.getByTestId('narrow').parentElement!

    expect(full.className).toContain('md:col-span-2')
    expect(full.className).toContain('lg:col-span-3')
    expect(wide.className).toContain('md:col-span-2')
    expect(wide.className).toContain('lg:col-span-2')
    expect(narrow.className).not.toContain('col-span-2')
    expect(narrow.className).not.toContain('col-span-3')
  })
})
