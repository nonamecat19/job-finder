import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ScoreBadge, Chip, Spinner, Button } from './ui'

describe('ScoreBadge', () => {
  it('renders em-dash for null score', () => {
    render(<ScoreBadge score={null} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders em-dash for undefined score', () => {
    render(<ScoreBadge />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders emerald class for score >= 80', () => {
    render(<ScoreBadge score={85} />)
    const badge = screen.getByText('85')
    expect(badge.className).toContain('success')
  })

  it('renders lime class for score >= 60', () => {
    render(<ScoreBadge score={65} />)
    const badge = screen.getByText('65')
    expect(badge.className).toContain('primary')
  })

  it('renders amber class for score >= 40', () => {
    render(<ScoreBadge score={45} />)
    const badge = screen.getByText('45')
    expect(badge.className).toContain('warning')
  })

  it('renders rose class for score < 40', () => {
    render(<ScoreBadge score={20} />)
    const badge = screen.getByText('20')
    expect(badge.className).toContain('danger')
  })
})

describe('Chip', () => {
  it('renders with default slate tone', () => {
    render(<Chip>React</Chip>)
    const chip = screen.getByText('React')
    expect(chip.className).toContain('muted')
  })

  it('renders with green tone', () => {
    render(<Chip tone="green">TypeScript</Chip>)
    const chip = screen.getByText('TypeScript')
    expect(chip.className).toContain('success')
  })

  it('renders with red tone', () => {
    render(<Chip tone="red">Go</Chip>)
    const chip = screen.getByText('Go')
    expect(chip.className).toContain('danger')
  })
})

describe('Spinner', () => {
  it('renders without label', () => {
    const { container } = render(<Spinner />)
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders with label', () => {
    render(<Spinner label="loading…" />)
    expect(screen.getByText('loading…')).toBeInTheDocument()
  })
})

describe('Button', () => {
  it('renders primary variant by default', () => {
    render(<Button>Click me</Button>)
    const btn = screen.getByRole('button', { name: 'Click me' })
    expect(btn.className).toContain('bg-primary')
  })

  it('renders secondary variant', () => {
    render(<Button variant="secondary">Secondary</Button>)
    const btn = screen.getByRole('button', { name: 'Secondary' })
    expect(btn.className).toContain('border-border')
  })

  it('renders danger variant', () => {
    render(<Button variant="danger">Danger</Button>)
    const btn = screen.getByRole('button', { name: 'Danger' })
    expect(btn.className).toContain('bg-danger')
  })

  it('renders ghost variant', () => {
    render(<Button variant="ghost">Ghost</Button>)
    const btn = screen.getByRole('button', { name: 'Ghost' })
    expect(btn.className).toContain('text-muted')
  })

  it('is disabled when disabled prop is true', () => {
    render(<Button disabled>Disabled</Button>)
    expect(screen.getByRole('button', { name: 'Disabled' })).toBeDisabled()
  })

  it('fires onClick handler', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<Button onClick={onClick}>Click</Button>)
    await user.click(screen.getByRole('button', { name: 'Click' }))
    expect(onClick).toHaveBeenCalledOnce()
  })
})
