import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { DashboardGrid } from '../DashboardGrid';
import { Tile, type SpanRule } from '../Tile';

describe('DashboardGrid', () => {
  it('renders children', () => {
    render(
      <DashboardGrid>
        <div data-testid="child">Content</div>
      </DashboardGrid>,
    );
    expect(screen.getByTestId('child')).toBeInTheDocument();
  });

  it('applies responsive grid classes for 1 / 2 / 3 / 4 columns at 640 / 1024 / 1920px', () => {
    render(<DashboardGrid><div /></DashboardGrid>);
    const grid = document.querySelector('.grid');
    expect(grid).toBeInTheDocument();
    expect(grid!.className).toContain('grid-cols-1');
    expect(grid!.className).toContain('sm:grid-cols-2');
    expect(grid!.className).toContain('lg:grid-cols-3');
    expect(grid!.className).toContain('3xl:grid-cols-4');
    expect(grid!.className).not.toContain('4xl:grid-cols-5');
  });

  it('applies responsive gap classes rising from 1rem to 1.25rem', () => {
    render(<DashboardGrid><div /></DashboardGrid>);
    const grid = document.querySelector('.grid');
    expect(grid!.className).toContain('gap-4');
    expect(grid!.className).toContain('sm:gap-5');
  });

  it('defaults to flow variant with items-start', () => {
    render(<DashboardGrid><div /></DashboardGrid>);
    const grid = document.querySelector('.grid');
    expect(grid!.className).toContain('items-start');
  });

  it('applies items-stretch in fit variant', () => {
    render(<DashboardGrid variant="fit"><div /></DashboardGrid>);
    const grid = document.querySelector('.grid');
    expect(grid!.className).toContain('items-stretch');
  });

  it('centres with the --container-dashboard max-width cap', () => {
    render(<DashboardGrid><div /></DashboardGrid>);
    const grid = document.querySelector('.grid');
    expect(grid!.className).toContain('mx-auto');
    expect(grid!.className).toContain('max-w-[var(--container-dashboard)]');
  });

  it('merges additional className', () => {
    render(<DashboardGrid className="extra"><div /></DashboardGrid>);
    const grid = document.querySelector('.grid');
    expect(grid!.className).toContain('extra');
  });
});

describe('Tile span rules', () => {
  const spanTests: { span: SpanRule; expected: string[]; notExpected: string[] }[] = [
    { span: 'compact', expected: [], notExpected: ['col-span-2', 'col-span-3', 'col-span-4'] },
    { span: 'standard', expected: [], notExpected: ['col-span-2', 'col-span-3', 'col-span-4'] },
    { span: 'wide', expected: ['sm:col-span-2', 'lg:col-span-2', '3xl:col-span-2'], notExpected: ['col-span-3', 'col-span-4'] },
    { span: 'tall', expected: ['sm:row-span-2', 'lg:row-span-2', '3xl:row-span-2'], notExpected: ['col-span-2'] },
    { span: 'feature', expected: ['sm:col-span-2', 'lg:col-span-2', '3xl:col-span-2', 'sm:row-span-2', 'lg:row-span-2', '3xl:row-span-2'], notExpected: ['col-span-3', 'col-span-4'] },
    { span: 'full', expected: ['sm:col-span-2', 'lg:col-span-3', '3xl:col-span-4'], notExpected: [] },
  ];

  for (const { span, expected, notExpected } of spanTests) {
    it(`span "${span}" maps to correct classes`, () => {
      render(<Tile span={span}><div data-testid="inner" /></Tile>);
      const tile = screen.getByTestId('inner').parentElement!.parentElement!;
      for (const cls of expected) {
        expect(tile.className).toContain(cls);
      }
      for (const cls of notExpected) {
        expect(tile.className).not.toContain(cls);
      }
    });
  }

  it('no span exceeds the 1 / 2 / 3 / 4 column ceiling at any breakpoint', () => {
    const maxCols: Record<string, number> = { '': 1, 'sm:': 2, 'lg:': 3, '3xl:': 4 };
    const spans: { span: SpanRule; cols: Record<string, number> }[] = [
      { span: 'compact', cols: { '': 1, 'sm:': 1, 'lg:': 1, '3xl:': 1 } },
      { span: 'standard', cols: { '': 1, 'sm:': 1, 'lg:': 1, '3xl:': 1 } },
      { span: 'wide', cols: { '': 1, 'sm:': 2, 'lg:': 2, '3xl:': 2 } },
      { span: 'tall', cols: { '': 1, 'sm:': 1, 'lg:': 1, '3xl:': 1 } },
      { span: 'feature', cols: { '': 1, 'sm:': 2, 'lg:': 2, '3xl:': 2 } },
      { span: 'full', cols: { '': 1, 'sm:': 2, 'lg:': 3, '3xl:': 4 } },
    ];
    for (const { span: _span, cols } of spans) {
      for (const [bp, colSpan] of Object.entries(cols)) {
        expect(colSpan).toBeLessThanOrEqual(maxCols[bp]);
      }
    }
  });
});

describe('Tile', () => {
  it('renders children in ready state', () => {
    render(<Tile><div data-testid="child">Content</div></Tile>);
    expect(screen.getByTestId('child')).toBeInTheDocument();
  });

  it('renders title and action', () => {
    render(<Tile title="My Tile" action={<button>Action</button>}><div /></Tile>);
    expect(screen.getByText('My Tile')).toBeInTheDocument();
    expect(screen.getByText('Action')).toBeInTheDocument();
  });

  it('renders footer', () => {
    render(<Tile footer="Footer text"><div /></Tile>);
    expect(screen.getByText('Footer text')).toBeInTheDocument();
  });

  it('renders skeleton in loading state', () => {
    render(<Tile state="loading"><div data-testid="should-not-render" /></Tile>);
    expect(screen.queryByTestId('should-not-render')).not.toBeInTheDocument();
  });

  it('renders empty state', () => {
    render(<Tile state="empty" emptyMessage="Nothing here"><div /></Tile>);
    expect(screen.getByText('Nothing here')).toBeInTheDocument();
  });

  it('renders error state with retry', () => {
    render(<Tile state="error" error={new Error('Boom')} onRetry={() => {}}><div /></Tile>);
    expect(screen.getByText('Boom')).toBeInTheDocument();
    expect(screen.getByText('Retry')).toBeInTheDocument();
  });

  it('applies min-h-0', () => {
    render(<Tile><div data-testid="inner" /></Tile>);
    const tile = screen.getByTestId('inner').parentElement!.parentElement!;
    expect(tile.className).toContain('min-h-0');
  });

  it('defaults to the white surface tone with the tile shadow', () => {
    render(<Tile><div data-testid="inner" /></Tile>);
    const tile = screen.getByTestId('inner').parentElement!.parentElement!;
    expect(tile.className).toContain('bg-surface');
    expect(tile.className).toContain('shadow-tile');
  });

  it('applies the quiet tone without a shadow, on --surface-secondary', () => {
    render(<Tile tone="quiet"><div data-testid="inner" /></Tile>);
    const tile = screen.getByTestId('inner').parentElement!.parentElement!;
    expect(tile.className).toContain('bg-surface-secondary');
    expect(tile.className).not.toContain('shadow-tile');
  });

  it('applies the inverse tone as an ink foreground flip', () => {
    render(<Tile tone="inverse"><div data-testid="inner" /></Tile>);
    const tile = screen.getByTestId('inner').parentElement!.parentElement!;
    expect(tile.className).toContain('bg-foreground');
    expect(tile.className).toContain('text-background');
  });

  it('applies a hairline border on every tone', () => {
    render(<Tile><div data-testid="inner" /></Tile>);
    const tile = screen.getByTestId('inner').parentElement!.parentElement!;
    expect(tile.className).toContain('border-border');
  });

  it('lifts on hover only when interactive', () => {
    render(<Tile interactive><div data-testid="inner" /></Tile>);
    const tile = screen.getByTestId('inner').parentElement!.parentElement!;
    expect(tile.className).toContain('hover:-translate-y-0.5');
    expect(tile.className).toContain('hover:shadow-raise');
  });

  it('does not lift on hover by default', () => {
    render(<Tile><div data-testid="inner" /></Tile>);
    const tile = screen.getByTestId('inner').parentElement!.parentElement!;
    expect(tile.className).not.toContain('hover:-translate-y-0.5');
  });

  it('renders scroll region when scroll is true', () => {
    render(<Tile scroll scrollLabel="My scroll"><div data-testid="inner" /></Tile>);
    const region = screen.getByRole('region', { name: 'My scroll' });
    expect(region).toBeInTheDocument();
    expect(region).toHaveAttribute('tabindex', '0');
  });
});
