import { getAuthHeaders, getAuthParams } from '../hooks/useAuth';
import type { LogSourceInfo, LogEntry } from './types';

interface SSELogEvent {
  line: number;
  text: string;
}

export async function fetchLogHistory(
  lines = 500,
  source = 'castellarius',
): Promise<string[]> {
  const auth = getAuthParams();
  const encodedSource = encodeURIComponent(source);
  const url = auth
    ? `/api/logs?lines=${lines}&source=${encodedSource}&${auth}`
    : `/api/logs?lines=${lines}&source=${encodedSource}`;
  const resp = await fetch(url, { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`logs: ${resp.status}`);
  return resp.json();
}

export function createLogEventSource(
  source: string,
  onEntry: (entry: LogEntry) => void,
  onError: (err: Error) => void,
): EventSource {
  const auth = getAuthParams();
  const encodedSource = encodeURIComponent(source);
  const url = auth
    ? `/api/logs/events?source=${encodedSource}&${auth}`
    : `/api/logs/events?source=${encodedSource}`;
  const es = new EventSource(url);
  es.onmessage = (e) => {
    try {
      const event: SSELogEvent = JSON.parse(e.data);
      const level = parseLevel(event.text);
      onEntry({
        line: event.line,
        level,
        text: event.text,
        raw: event.text,
      });
    } catch {
      const text = e.data;
      onEntry({ line: 1, level: '', text, raw: text });
    }
  };
  es.onerror = () => {
    onError(new Error('log stream error'));
    es.close();
  };
  return es;
}

const levelPattern = /\b(INFO|WARN|ERROR|DEBUG)\b/;

function parseLevel(raw: string): LogEntry['level'] {
  const m = raw.match(levelPattern);
  if (m) return m[1] as LogEntry['level'];
  return '';
}

export async function fetchLogSources(): Promise<LogSourceInfo[]> {
  const resp = await fetch('/api/logs/sources', { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`log sources: ${resp.status}`);
  return resp.json();
}