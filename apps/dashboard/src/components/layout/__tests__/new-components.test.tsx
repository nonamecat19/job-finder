import { render, screen, fireEvent } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { Briefcase } from 'lucide-react';
import { IconTile, type IconTileTint } from '../IconTile';
import { StatTile } from '../StatTile';
import { ListRow } from '../ListRow';

describe('IconTile', () => {
  it('renders the given lucide glyph', () => {
    render(<IconTile icon={Briefcase} />);
    const svg = document.querySelector('svg');
    expect(svg).toBeInTheDocument();
  });

  const tints: IconTileTint[] = ['violet', 'blue', 'mint', 'amber', 'rose'];
  for (const tint of tints) {
    it(`maps tint "${tint}" to its bg/fg token utilities`, () => {
      render(<IconTile icon={Briefcase} tint={tint} />);
      const box = document.querySelector('svg')!.parentElement!;
      expect(box.className).toContain(`bg-tint-${tint}`);
      expect(box.className).toContain(`text-tint-${tint}-fg`);
    });
  }

  it('defaults to the blue tint', () => {
    render(<IconTile icon={Briefcase} />);
    const box = document.querySelector('svg')!.parentElement!;
    expect(box.className).toContain('bg-tint-blue');
  });

  it('sizes the glyph 12 / 16 / 20px for sm / md / lg', () => {
    const { rerender } = render(<IconTile icon={Briefcase} size="sm" />);
    expect(document.querySelector('svg')!.getAttribute('width')).toBe('12');

    rerender(<IconTile icon={Briefcase} size="md" />);
    expect(document.querySelector('svg')!.getAttribute('width')).toBe('16');

    rerender(<IconTile icon={Briefcase} size="lg" />);
    expect(document.querySelector('svg')!.getAttribute('width')).toBe('20');
  });

  it('uses a 2px stroke', () => {
    render(<IconTile icon={Briefcase} />);
    expect(document.querySelector('svg')!.getAttribute('stroke-width')).toBe('2');
  });

  it('is a 0.75rem-radius rounded square', () => {
    render(<IconTile icon={Briefcase} />);
    const box = document.querySelector('svg')!.parentElement!;
    expect(box.className).toContain('rounded-lg');
  });

  it('merges additional className', () => {
    render(<IconTile icon={Briefcase} className="extra" />);
    const box = document.querySelector('svg')!.parentElement!;
    expect(box.className).toContain('extra');
  });
});

describe('StatTile', () => {
  it('renders caption and figure', () => {
    render(<StatTile caption="Applications" value="128" />);
    expect(screen.getByText('Applications')).toBeInTheDocument();
    expect(screen.getByText('128')).toBeInTheDocument();
  });

  it('renders an up delta with success colour', () => {
    render(<StatTile caption="Applications" value="128" delta={{ value: '+12%', direction: 'up' }} />);
    const delta = screen.getByText('+12%').closest('span')!;
    expect(delta.className).toContain('text-success');
  });

  it('renders a down delta with danger colour', () => {
    render(<StatTile caption="Applications" value="128" delta={{ value: '-4%', direction: 'down' }} />);
    const delta = screen.getByText('-4%').closest('span')!;
    expect(delta.className).toContain('text-danger');
  });

  it('renders no delta when omitted', () => {
    render(<StatTile caption="Applications" value="128" />);
    expect(screen.queryByText(/%/)).not.toBeInTheDocument();
  });

  it('renders an IconTile when icon is provided', () => {
    render(<StatTile caption="Applications" value="128" icon={Briefcase} tint="mint" />);
    const svg = document.querySelector('svg')!;
    expect(svg.parentElement!.className).toContain('bg-tint-mint');
  });

  it('renders no icon tile when icon is omitted', () => {
    render(<StatTile caption="Applications" value="128" />);
    expect(document.querySelector('svg')).not.toBeInTheDocument();
  });
});

describe('ListRow', () => {
  it('renders title and meta', () => {
    render(<ListRow title="Acme Corp" meta="Applied 3 days ago" />);
    expect(screen.getByText('Acme Corp')).toBeInTheDocument();
    expect(screen.getByText('Applied 3 days ago')).toBeInTheDocument();
  });

  it('renders leading and aside nodes', () => {
    render(<ListRow leading={<span data-testid="lead" />} title="Acme Corp" aside={<span>$120k</span>} />);
    expect(screen.getByTestId('lead')).toBeInTheDocument();
    expect(screen.getByText('$120k')).toBeInTheDocument();
  });

  it('applies the surface-tertiary selected fill, not an accent border', () => {
    render(<ListRow title="Acme Corp" selected />);
    const row = screen.getByText('Acme Corp').closest('[aria-selected], div')!;
    const outer = row.parentElement!.parentElement!;
    expect(outer.className).toContain('bg-surface-tertiary');
    expect(outer.className).not.toContain('border-accent');
  });

  it('is not selected by default', () => {
    render(<ListRow title="Acme Corp" />);
    const outer = screen.getByText('Acme Corp').parentElement!.parentElement!;
    expect(outer.className).not.toContain('bg-surface-tertiary');
  });

  it('is a button role and fires onClick when interactive', () => {
    const onClick = vi.fn();
    render(<ListRow title="Acme Corp" onClick={onClick} />);
    const row = screen.getByRole('button');
    fireEvent.click(row);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it('is not a button role when onClick is omitted', () => {
    render(<ListRow title="Acme Corp" />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('renders no dividers between rows in a list', () => {
    render(
      <div>
        <ListRow title="Row one" />
        <ListRow title="Row two" />
      </div>,
    );
    expect(document.querySelector('hr')).not.toBeInTheDocument();
    for (const row of document.querySelectorAll('[class*="rounded-lg"]')) {
      expect(row.className).not.toContain('border-t');
      expect(row.className).not.toContain('divide-y');
    }
  });
});
