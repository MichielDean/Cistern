import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { DropletTable } from '../components/DropletTable';
import type { Droplet } from '../api/types';

const mockDroplets: Droplet[] = [
  {
    id: 'ct-abc123',
    repo: 'cistern',
    title: 'Test droplet',
    description: 'A test',
    priority: 1,
    complexity: 2,
    status: 'in_progress',
    assignee: 'implement',
    current_cataractae: 'implement',
    created_at: '2026-04-19T00:00:00Z',
    updated_at: '2026-04-19T00:00:00Z',
  },
  {
    id: 'ct-def456',
    repo: 'other-repo',
    title: 'Another droplet',
    description: '',
    priority: 3,
    complexity: 1,
    status: 'open',
    assignee: '',
    current_cataractae: '',
    created_at: '2026-04-18T00:00:00Z',
    updated_at: '2026-04-18T00:00:00Z',
  },
];

describe('DropletTable', () => {
  it('renders table with droplet data', () => {
    render(<DropletTable droplets={mockDroplets} onRowClick={vi.fn()} />);

    expect(screen.getByText('Test droplet')).toBeInTheDocument();
    expect(screen.getByText('Another droplet')).toBeInTheDocument();
    expect(screen.getByText('ct-abc123')).toBeInTheDocument();
    expect(screen.getByText('ct-def456')).toBeInTheDocument();
  });

  it('renders column headers', () => {
    render(<DropletTable droplets={mockDroplets} onRowClick={vi.fn()} />);

    expect(screen.getByText('ID')).toBeInTheDocument();
    expect(screen.getByText('Title')).toBeInTheDocument();
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('Step')).toBeInTheDocument();
    expect(screen.getByText('Priority')).toBeInTheDocument();
    expect(screen.getByText('Age')).toBeInTheDocument();
  });

  it('calls onRowClick when a row is clicked', () => {
    const onRowClick = vi.fn();
    render(<DropletTable droplets={mockDroplets} onRowClick={onRowClick} />);

    fireEvent.click(screen.getByText('Test droplet'));
    expect(onRowClick).toHaveBeenCalledWith('ct-abc123');
  });

  it('shows "--" when current_cataractae is empty', () => {
    render(<DropletTable droplets={mockDroplets} onRowClick={vi.fn()} />);

    const stepCells = screen.getAllByText('--');
    expect(stepCells.length).toBeGreaterThanOrEqual(1);
  });

  it('shows empty state when no droplets', () => {
    render(<DropletTable droplets={[]} onRowClick={vi.fn()} />);

    expect(screen.getByText('No droplets found')).toBeInTheDocument();
  });
});