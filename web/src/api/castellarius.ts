import { useState, useEffect, useCallback, useRef } from 'react';
import { getAuthHeaders } from '../hooks/useAuth';
import type { CastellariusStatus } from './types';

export async function fetchCastellariusStatus(): Promise<CastellariusStatus> {
  const resp = await fetch('/api/castellarius/status', { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`castellarius status: ${resp.status}`);
  const data = await resp.json();
  if (data.aqueducts === undefined) {
    return {
      running: data.status === 'ok',
      pid: null,
      uptime_seconds: null,
      aqueducts: [],
      farm_running: false,
    };
  }
  return data as CastellariusStatus;
}

export async function castellariusAction(action: 'start' | 'stop' | 'restart'): Promise<void> {
  const resp = await fetch(`/api/castellarius/${action}`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!resp.ok) {
    const body = await resp.json().catch(() => ({ message: resp.statusText }));
    throw new Error(body.message || `castellarius ${action} failed: ${resp.status}`);
  }
}

export function useCastellariusStatus(intervalMs = 5000) {
  const [status, setStatus] = useState<CastellariusStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const mountedRef = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const data = await fetchCastellariusStatus();
      if (!mountedRef.current) return;
      setStatus(data);
      setError(null);
    } catch (err) {
      if (!mountedRef.current) return;
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      if (mountedRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    refresh();
    const id = setInterval(refresh, intervalMs);
    return () => {
      mountedRef.current = false;
      clearInterval(id);
    };
  }, [refresh, intervalMs]);

  return { status, loading, error, refresh };
}