import { useState, useEffect, useCallback, useRef } from 'react';
import { getAuthHeaders } from '../hooks/useAuth';
import type { DoctorResult, DoctorCheck } from './types';

export async function fetchDoctor(fix = false): Promise<DoctorResult> {
  const url = fix ? '/api/doctor?fix=true' : '/api/doctor';
  const resp = await fetch(url, { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`doctor: ${resp.status}`);
  const data = await resp.json();
  if (data.checks === undefined) {
    const checks: DoctorCheck[] = [
      {
        name: 'Config Valid',
        status: data.config_ok ? 'pass' : 'fail',
        message: data.config_ok ? 'Configuration is valid' : 'Configuration has errors',
        category: 'Config',
      },
    ];
    const repos: DoctorCheck[] = Array.isArray(data.repos)
      ? data.repos.map((r: { name: string; url: string }) => ({
          name: `Repo: ${r.name}`,
          status: 'pass' as const,
          message: r.url,
          category: 'Repos',
        }))
      : [];
    const allChecks = [...checks, ...repos];
    return {
      checks: allChecks,
      summary: {
        total: allChecks.length,
        passed: allChecks.filter(c => c.status === 'pass').length,
      },
      timestamp: new Date().toISOString(),
    };
  }
  return data as DoctorResult;
}

export function useDoctor() {
  const [result, setResult] = useState<DoctorResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const mountedRef = useRef(true);

  const runDoctor = useCallback(async (fix = false) => {
    setLoading(true);
    setError(null);
    try {
      const data = await fetchDoctor(fix);
      if (!mountedRef.current) return;
      setResult(data);
    } catch (err) {
      if (!mountedRef.current) return;
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      if (mountedRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    runDoctor();
    return () => {
      mountedRef.current = false;
    };
  }, [runDoctor]);

  const rerun = useCallback(() => runDoctor(false), [runDoctor]);
  const fix = useCallback(() => runDoctor(true), [runDoctor]);

  return { result, loading, error, rerun, fix };
}