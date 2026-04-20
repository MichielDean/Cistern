import { useCallback, useEffect, useRef, useState } from 'react';
import type { DashboardData } from '../api/types';
import { getAuthParams } from './useAuth';

interface UseDashboardEventsOptions {
  onData?: (data: DashboardData) => void;
  onError?: (error: Error) => void;
  enabled?: boolean;
}

export function useDashboardEvents(options: UseDashboardEventsOptions = {}) {
  const { onData, onError, enabled = true } = options;
  const [data, setData] = useState<DashboardData | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const esRef = useRef<EventSource | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>();
  const mountedRef = useRef(true);
  const onDataRef = useRef(onData);
  const onErrorRef = useRef(onError);
  const enabledRef = useRef(enabled);

  useEffect(() => { onDataRef.current = onData; }, [onData]);
  useEffect(() => { onErrorRef.current = onError; }, [onError]);
  useEffect(() => { enabledRef.current = enabled; }, [enabled]);

  const retryCountRef = useRef(0);

  const connect = useCallback(() => {
    if (esRef.current) {
      esRef.current.close();
    }

    const authParams = getAuthParams();
    const url = authParams ? `/api/dashboard/events?${authParams}` : '/api/dashboard/events';
    const es = new EventSource(url);

    es.onopen = () => {
      if (!mountedRef.current) return;
      retryCountRef.current = 0;
      setConnected(true);
      setError(null);
    };

    es.onmessage = (e) => {
      if (!mountedRef.current) return;
      try {
        const parsed: DashboardData = JSON.parse(e.data);
        setData(parsed);
        onDataRef.current?.(parsed);
      } catch (err) {
        console.warn('Failed to parse SSE message:', err, e.data);
      }
    };

    es.onerror = () => {
      if (!mountedRef.current) return;
      setConnected(false);
      const err = new Error('SSE connection lost');
      setError(err);
      onErrorRef.current?.(err);
      es.close();
      esRef.current = null;
      if (enabledRef.current) {
        const delay = Math.min(1000 * Math.pow(2, retryCountRef.current), 30000);
        retryCountRef.current++;
        clearTimeout(reconnectTimer.current);
        reconnectTimer.current = setTimeout(() => {
          if (mountedRef.current) {
            connect();
          }
        }, delay);
      }
    };

    esRef.current = es;
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    if (!enabled) {
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
      return;
    }
    connect();
    return () => {
      mountedRef.current = false;
      clearTimeout(reconnectTimer.current);
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
    };
  }, [connect, enabled]);

  return { data, connected, error };
}