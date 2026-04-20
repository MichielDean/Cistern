import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StatusBadge } from '../components/StatusBadge';

describe('StatusBadge', () => {
  it('renders "In Progress" for in_progress status', () => {
    render(<StatusBadge status="in_progress" />);
    expect(screen.getByText('In Progress')).toBeInTheDocument();
  });

  it('renders "Open" for open status', () => {
    render(<StatusBadge status="open" />);
    expect(screen.getByText('Open')).toBeInTheDocument();
  });

  it('renders "Done" for done status', () => {
    render(<StatusBadge status="done" />);
    expect(screen.getByText('Done')).toBeInTheDocument();
  });

  it('renders "Pooled" for pooled status', () => {
    render(<StatusBadge status="pooled" />);
    expect(screen.getByText('Pooled')).toBeInTheDocument();
  });

  it('renders "Closed" for closed status', () => {
    render(<StatusBadge status="closed" />);
    expect(screen.getByText('Closed')).toBeInTheDocument();
  });

  it('renders raw status for unknown statuses', () => {
    render(<StatusBadge status="unknown_status" />);
    expect(screen.getByText('unknown_status')).toBeInTheDocument();
  });

  it('applies small size by default', () => {
    const { container } = render(<StatusBadge status="open" />);
    const badge = container.querySelector('span');
    expect(badge?.className).toContain('text-xs');
  });

  it('applies medium size when specified', () => {
    const { container } = render(<StatusBadge status="open" size="md" />);
    const badge = container.querySelector('span');
    expect(badge?.className).toContain('text-sm');
  });

  it('applies correct color classes for in_progress', () => {
    const { container } = render(<StatusBadge status="in_progress" />);
    const badge = container.querySelector('span');
    expect(badge?.className).toContain('text-cistern-accent');
  });

  it('applies correct color classes for open', () => {
    const { container } = render(<StatusBadge status="open" />);
    const badge = container.querySelector('span');
    expect(badge?.className).toContain('text-cistern-yellow');
  });

  it('applies correct color classes for pooled', () => {
    const { container } = render(<StatusBadge status="pooled" />);
    const badge = container.querySelector('span');
    expect(badge?.className).toContain('text-cistern-red');
  });

  it('applies muted styling for unknown status', () => {
    const { container } = render(<StatusBadge status="foobar" />);
    const badge = container.querySelector('span');
    expect(badge?.className).toContain('text-cistern-muted');
  });
});