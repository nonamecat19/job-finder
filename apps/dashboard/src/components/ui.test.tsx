import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ScoreBadge, GhostBadge, Chip, Spinner, Button, SkeletonLine, SkeletonBlock, SkeletonCircle, LoadingRegion } from './ui'

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
    expect(badge.className).toContain('accent')
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

describe('GhostBadge', () => {
  it('renders nothing when no signal exists (undefined)', () => {
    const { container } = render(<GhostBadge />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when score is null', () => {
    const { container } = render(<GhostBadge score={null} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing below 50', () => {
    const { container } = render(<GhostBadge score={49} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders yellow badge for 50-79', () => {
    render(<GhostBadge score={62} />)
    const badge = screen.getByText(/62/)
    expect(badge.className).toContain('warning')
  })

  it('renders yellow badge at the 79 boundary', () => {
    render(<GhostBadge score={79} />)
    expect(screen.getByText(/79/).className).toContain('warning')
  })

  it('renders red badge for 80-100', () => {
    render(<GhostBadge score={85} />)
    const badge = screen.getByText(/85/)
    expect(badge.className).toContain('danger')
  })

  it('renders red badge at the 80 boundary', () => {
    render(<GhostBadge score={80} />)
    expect(screen.getByText(/80/).className).toContain('danger')
  })

  // SC-008: an unscored card must render identically to today — verified
  // here as "GhostBadge renders nothing", so it contributes zero DOM/markup
  // next to ScoreBadge for a job with no ghost signal.
  it('an unscored job renders the same DOM as ScoreBadge alone (no ghost markup added)', () => {
    const { container: withGhost } = render(
      <>
        <ScoreBadge score={72} />
        <GhostBadge score={undefined} />
      </>,
    )
    const { container: withoutGhost } = render(<ScoreBadge score={72} />)
    expect(withGhost.innerHTML).toBe(withoutGhost.innerHTML)
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

describe('SkeletonLine', () => {
  it('renders a pulsing placeholder', () => {
    const { container } = render(<SkeletonLine />)
    expect(container.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('is hidden from assistive tech', () => {
    const { container } = render(<SkeletonLine />)
    expect(container.firstChild).toHaveAttribute('aria-hidden', 'true')
  })
})

describe('SkeletonBlock', () => {
  it('renders a pulsing placeholder hidden from assistive tech', () => {
    const { container } = render(<SkeletonBlock className="h-24 w-full" />)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('animate-pulse')
    expect(el).toHaveAttribute('aria-hidden', 'true')
  })
})

describe('SkeletonCircle', () => {
  it('renders a pulsing circular placeholder', () => {
    const { container } = render(<SkeletonCircle />)
    const el = container.firstChild as HTMLElement
    expect(el.className).toContain('animate-pulse')
    expect(el.className).toContain('rounded-full')
  })
})

describe('LoadingRegion', () => {
  it('exposes a status role and busy state with an accessible label', () => {
    render(
      <LoadingRegion label="loading jobs…">
        <SkeletonBlock className="h-10 w-full" />
      </LoadingRegion>,
    )
    const region = screen.getByRole('status')
    expect(region).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByText('loading jobs…')).toBeInTheDocument()
  })
})

describe('Button', () => {
  it('renders primary variant by default', () => {
    render(<Button>Click me</Button>)
    const btn = screen.getByRole('button', { name: 'Click me' })
    expect(btn.className).toContain('bg-accent')
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
