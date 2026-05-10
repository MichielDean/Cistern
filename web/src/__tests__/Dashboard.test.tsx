import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { Dashboard } from '../pages/Dashboard';
import * as DashboardContext from '../context/DashboardContext';
import type { DashboardData } from '../api/types';

function renderWithRouter(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

function mockUseDashboard(data: DashboardData | null, error: Error | null = null) {
  vi.spyOn(DashboardContext, 'useDashboard').mockReturnValue({
    data,
    connected: data !== null,
    error,
  });
}

const dataWithNullArrays = {
  cataractae_count: 0,
  flowing_count: 0,
  queued_count: 0,
  done_count: 0,
  cataractae: null as unknown as DashboardData['cataractae'],
  unassigned_items: null as unknown as DashboardData['unassigned_items'],
  cistern_items: null as unknown as DashboardData['cistern_items'],
  pooled_items: null as unknown as DashboardData['pooled_items'],
  recent_items: null as unknown as DashboardData['recent_items'],
  blocked_by_map: null as unknown as DashboardData['blocked_by_map'],
  flow_activities: null as unknown as DashboardData['flow_activities'],
  pool_reasons: null as unknown as DashboardData['pool_reasons'],
  castellarius_running: true,
  fetched_at: '2026-04-21T00:00:00Z',
};

const dataWithEmptyArrays: DashboardData = {
  cataractae_count: 0,
  flowing_count: 0,
  queued_count: 0,
  done_count: 0,
  cataractae: [],
  unassigned_items: [],
  cistern_items: [],
  pooled_items: [],
  recent_items: [],
  blocked_by_map: {},
  flow_activities: [],
  pool_reasons: {},
  castellarius_running: true,
  fetched_at: '2026-04-21T00:00:00Z',
};

describe('Dashboard null-array regression', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders without crashing when API returns null arrays', () => {
    mockUseDashboard(dataWithNullArrays);

    expect(() => renderWithRouter(<Dashboard />)).not.toThrow();
  });

  it('renders without crashing when API returns empty arrays', () => {
    mockUseDashboard(dataWithEmptyArrays);

    expect(() => renderWithRouter(<Dashboard />)).not.toThrow();
  });

  it('shows the Aqueducts heading for empty data', () => {
    mockUseDashboard(dataWithEmptyArrays);

    renderWithRouter(<Dashboard />);
    expect(screen.getByText('Aqueducts')).toBeDefined();
  });

  it('shows pooled count as 0 when pooled_items is null', () => {
    mockUseDashboard(dataWithNullArrays);

    const { container } = renderWithRouter(<Dashboard />);
    const pooledText = container.querySelector('.text-cistern-red');
    expect(pooledText?.textContent).toBe('0');
  });
});

describe('PooledSection expandable rows', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('shows pool reason when pooled item row is clicked', async () => {
    const pooledItem = {
      id: 'ci-abc123',
      repo: 'myrepo',
      title: 'Stuck droplet',
      description: '',
      priority: 1,
      complexity: 1,
      status: 'pooled' as const,
      assignee: '',
      current_cataractae: '',
      created_at: '2026-04-21T00:00:00Z',
      updated_at: '2026-04-21T00:00:00Z',
    };
    const dataWithPooled: DashboardData = {
      ...dataWithEmptyArrays,
      pooled_items: [pooledItem],
      pool_reasons: { 'ci-abc123': 'blocked by upstream dependency' },
    };

    mockUseDashboard(dataWithPooled);
    renderWithRouter(<Dashboard />);

    const row = screen.getByText('Stuck droplet');
    await fireEvent.click(row);

    expect(screen.getByText('blocked by upstream dependency')).toBeDefined();
  });

  it('shows no reason recorded when pool reason is missing', async () => {
    const pooledItem = {
      id: 'ci-def456',
      repo: 'myrepo',
      title: 'No reason droplet',
      description: '',
      priority: 1,
      complexity: 1,
      status: 'pooled' as const,
      assignee: '',
      current_cataractae: '',
      created_at: '2026-04-21T00:00:00Z',
      updated_at: '2026-04-21T00:00:00Z',
    };
    const dataWithPooledNoReason: DashboardData = {
      ...dataWithEmptyArrays,
      pooled_items: [pooledItem],
      pool_reasons: {},
    };

    mockUseDashboard(dataWithPooledNoReason);
    renderWithRouter(<Dashboard />);

    const row = screen.getByText('No reason droplet');
    await fireEvent.click(row);

    expect(screen.getByText('No reason recorded')).toBeDefined();
  });
});