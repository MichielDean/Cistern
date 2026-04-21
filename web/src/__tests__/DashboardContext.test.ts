import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { createElement } from 'react';
import { DashboardProvider, useDashboard } from '../context/DashboardContext';

const mockDashboardData = {
  cataractae_count: 1,
  flowing_count: 1,
  queued_count: 0,
  done_count: 5,
  cataractae: [],
  unassigned_items: [],
  cistern_items: [],
  pooled_items: [],
  recent_items: [],
  blocked_by_map: {},
  flow_activities: [],
  castellarius_running: true,
  fetched_at: '2026-04-19T00:00:00Z',
};

describe('DashboardContext', () => {
  let mockEventSource: {
    onopen: (() => void) | null;
    onmessage: ((e: { data: string }) => void) | null;
    onerror: (() => void) | null;
    close: () => void;
    url: string;
  };

  beforeEach(() => {
    mockEventSource = {
      onopen: null,
      onmessage: null,
      onerror: null,
      close: vi.fn(),
      url: '',
    };
    vi.stubGlobal('EventSource', vi.fn((url: string) => {
      mockEventSource.url = url;
      return mockEventSource;
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('provides data from SSE through context', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      createElement(DashboardProvider, null, children);

    const { result } = renderHook(() => useDashboard(), { wrapper });

    act(() => {
      mockEventSource.onopen?.();
      mockEventSource.onmessage?.({ data: JSON.stringify(mockDashboardData) });
    });

    expect(result.current.data).toEqual(mockDashboardData);
    expect(result.current.connected).toBe(true);
  });

  it('returns default values outside provider', () => {
    const { result } = renderHook(() => useDashboard());
    expect(result.current.data).toBeNull();
    expect(result.current.connected).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('reflects SSE connection errors through context', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      createElement(DashboardProvider, null, children);

    const { result } = renderHook(() => useDashboard(), { wrapper });

    act(() => {
      mockEventSource.onopen?.();
    });
    expect(result.current.connected).toBe(true);

    act(() => {
      mockEventSource.onerror?.();
    });
    expect(result.current.connected).toBe(false);
    expect(result.current.error).toBeInstanceOf(Error);
  });
});