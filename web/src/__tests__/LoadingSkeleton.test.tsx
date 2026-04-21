import { describe, it, expect } from 'vitest';
import { LoadingSkeleton, SkeletonLine, SkeletonCard, SkeletonTable } from '../components/LoadingSkeleton';
import { render, screen } from '@testing-library/react';

describe('LoadingSkeleton', () => {
  it('renders with default styles', () => {
    const { container } = render(<LoadingSkeleton />);
    expect(container.firstChild).toBeTruthy();
    expect(container.firstChild).toHaveClass('animate-pulse');
  });

  it('applies custom className', () => {
    const { container } = render(<LoadingSkeleton className="h-8 w-32" />);
    expect(container.firstChild).toHaveClass('h-8', 'w-32');
  });
});

describe('SkeletonLine', () => {
  it('renders a line with default width', () => {
    const { container } = render(<SkeletonLine />);
    const el = container.firstChild as HTMLElement;
    expect(el).toHaveClass('animate-pulse');
    expect(el.style.width).toBe('100%');
  });

  it('renders a line with custom width', () => {
    const { container } = render(<SkeletonLine width="60%" />);
    const el = container.firstChild as HTMLElement;
    expect(el.style.width).toBe('60%');
  });
});

describe('SkeletonCard', () => {
  it('renders a card with default lines', () => {
    const { container } = render(<SkeletonCard />);
    const lines = container.querySelectorAll('.animate-pulse');
    expect(lines.length).toBe(3);
  });

  it('renders a card with specified number of lines', () => {
    const { container } = render(<SkeletonCard lines={5} />);
    const lines = container.querySelectorAll('.animate-pulse');
    expect(lines.length).toBe(5);
  });
});

describe('SkeletonTable', () => {
  it('renders a table with default rows and cols', () => {
    const { container } = render(<SkeletonTable />);
    const rows = container.querySelectorAll('.border-t');
    expect(rows.length).toBe(5);
  });

  it('renders a table with specified rows and cols', () => {
    const { container } = render(<SkeletonTable rows={3} cols={2} />);
    const rows = container.querySelectorAll('.border-t');
    expect(rows.length).toBe(3);
  });
});