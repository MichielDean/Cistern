import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useDashboardEvents } from '../hooks/useDashboardEvents';
import type { DashboardData } from '../api/types';

const mockDashboardData: DashboardData = {
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

describe('useDashboardEvents', () => {
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

  it('connects to SSE endpoint on mount', () => {
    renderHook(() => useDashboardEvents());
    expect(EventSource).toHaveBeenCalledWith('/api/dashboard/events');
  });

  it('includes auth token in SSE URL when key is stored', () => {
    localStorage.setItem('cistern_api_key', 'test-key');
    renderHook(() => useDashboardEvents());
    expect(EventSource).toHaveBeenCalledWith('/api/dashboard/events?token=test-key');
    localStorage.clear();
  });

  it('receives dashboard data from SSE messages', () => {
    const { result } = renderHook(() => useDashboardEvents());

    act(() => {
      mockEventSource.onopen?.();
      mockEventSource.onmessage?.({ data: JSON.stringify(mockDashboardData) });
    });

    expect(result.current.data).toEqual(mockDashboardData);
    expect(result.current.connected).toBe(true);
  });

  it('handles connection errors and reconnects with exponential backoff', () => {
    vi.useFakeTimers();
    renderHook(() => useDashboardEvents());

    act(() => {
      mockEventSource.onerror?.();
    });

    expect(EventSource).toHaveBeenCalledTimes(1);

    act(() => {
      vi.advanceTimersByTime(1000);
    });

    expect(EventSource).toHaveBeenCalledTimes(2);
  });

  it('increases backoff delay on successive failures', () => {
    vi.useFakeTimers();
    renderHook(() => useDashboardEvents());

    act(() => {
      mockEventSource.onerror?.();
    });

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(EventSource).toHaveBeenCalledTimes(2);

    act(() => {
      mockEventSource.onerror?.();
    });

    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(EventSource).toHaveBeenCalledTimes(3);
  });

  it('resets backoff after successful connection', () => {
    vi.useFakeTimers();
    renderHook(() => useDashboardEvents());

    act(() => {
      mockEventSource.onerror?.();
    });

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(EventSource).toHaveBeenCalledTimes(2);

    act(() => {
      mockEventSource.onopen?.();
    });

    act(() => {
      mockEventSource.onerror?.();
    });

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(EventSource).toHaveBeenCalledTimes(3);

    vi.useRealTimers();
  });

  it('does not reconnect after unmount', () => {
    vi.useFakeTimers();
    const { unmount } = renderHook(() => useDashboardEvents());

    unmount();

    vi.advanceTimersByTime(3000);
    vi.useRealTimers();

    expect(EventSource).toHaveBeenCalledTimes(1);
  });

  it('does not connect when disabled', () => {
    renderHook(() => useDashboardEvents({ enabled: false }));
    expect(EventSource).not.toHaveBeenCalled();
  });
});