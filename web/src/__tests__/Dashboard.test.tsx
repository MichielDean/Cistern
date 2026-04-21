import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Dashboard } from '../pages/Dashboard';
import * as DashboardContext from '../context/DashboardContext';
import type { DashboardData } from '../api/types';

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
  farm_running: true,
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
  farm_running: true,
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

    expect(() => render(<Dashboard />)).not.toThrow();
  });

  it('renders without crashing when API returns empty arrays', () => {
    mockUseDashboard(dataWithEmptyArrays);

    expect(() => render(<Dashboard />)).not.toThrow();
  });

  it('shows the Aqueducts heading for empty data', () => {
    mockUseDashboard(dataWithEmptyArrays);

    render(<Dashboard />);
    expect(screen.getByText('Aqueducts')).toBeDefined();
  });

  it('shows pooled count as 0 when pooled_items is null', () => {
    mockUseDashboard(dataWithNullArrays);

    const { container } = render(<Dashboard />);
    const pooledText = container.querySelector('.text-cistern-red');
    expect(pooledText?.textContent).toBe('0');
  });
});