import { getAuthHeaders, getAuthParams } from '../hooks/useAuth';
import type { LogSourceInfo } from './types';

export async function fetchLogHistory(
  lines = 500,
  source = 'castellarius',
): Promise<string[]> {
  const auth = getAuthParams();
  const url = auth
    ? `/api/logs?lines=${lines}&source=${source}&${auth}`
    : `/api/logs?lines=${lines}&source=${source}`;
  const resp = await fetch(url, { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`logs: ${resp.status}`);
  return resp.json();
}

export function createLogEventSource(
  source: string,
  onLine: (line: string) => void,
  onError: (err: Error) => void,
): EventSource {
  const auth = getAuthParams();
  const url = auth
    ? `/api/logs/events?source=${source}&${auth}`
    : `/api/logs/events?source=${source}`;
  const es = new EventSource(url);
  es.onmessage = (e) => onLine(e.data);
  es.onerror = () => {
    onError(new Error('log stream error'));
    es.close();
  };
  return es;
}

export async function fetchLogSources(): Promise<LogSourceInfo[]> {
  const resp = await fetch('/api/logs/sources', { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`log sources: ${resp.status}`);
  return resp.json();
}