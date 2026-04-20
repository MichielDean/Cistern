import { useState, useEffect, useRef, useCallback } from 'react';
import { LogViewer } from '../components/LogViewer';
import { fetchLogHistory, createLogEventSource, fetchLogSources } from '../api/logs';
import type { LogEntry, LogSourceInfo } from '../api/types';

export function LogsPage() {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [sources, setSources] = useState<LogSourceInfo[]>([]);
  const [activeSource, setActiveSource] = useState('castellarius');
  const [autoScroll, setAutoScroll] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const esRef = useRef<EventSource | null>(null);
  const sourceRef = useRef(activeSource);
  const lastHistoryLine = useRef(0);

  useEffect(() => {
    sourceRef.current = activeSource;
  }, [activeSource]);

  useEffect(() => {
    fetchLogSources().then(setSources).catch(() => {});
  }, []);

  const loadHistory = useCallback(async (source: string) => {
    setLoading(true);
    setError(null);
    try {
      const logEntries = await fetchLogHistory(500, source);
      if (sourceRef.current !== source) return;
      lastHistoryLine.current = logEntries.length > 0 ? logEntries[logEntries.length - 1].line : 0;
      setEntries(logEntries);
    } catch (err) {
      if (sourceRef.current !== source) return;
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      if (sourceRef.current === source) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const source = activeSource;
    lastHistoryLine.current = 0;
    setEntries([]);
    loadHistory(source);

    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }

    esRef.current = createLogEventSource(
      source,
      (entry) => {
        if (sourceRef.current !== source) return;
        if (entry.line <= lastHistoryLine.current) return;
        setEntries(prev => [...prev, entry]);
      },
      (err) => {
        if (sourceRef.current !== source) return;
        setError(err);
      },
    );

    return () => {
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
    };
  }, [activeSource, loadHistory]);

  const handleSourceChange = (source: string) => {
    setActiveSource(source);
    setAutoScroll(true);
  };

  const clearEntries = () => {
    setEntries([]);
  };

  const activeSourceInfo = sources.find(s => s.name === activeSource);

  if (error && entries.length === 0) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-cistern-red text-lg font-mono mb-2">Error</div>
          <div className="text-cistern-muted text-sm">{error.message}</div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-4 px-4 md:px-6 py-3 border-b border-cistern-border bg-cistern-surface/50">
        <select
          value={activeSource}
          onChange={e => handleSourceChange(e.target.value)}
          className="bg-cistern-surface border border-cistern-border text-cistern-fg font-mono text-sm rounded-md px-2 py-1"
        >
          {sources.length === 0 && <option value="castellarius">castellarius</option>}
          {sources.map(s => (
            <option key={s.name} value={s.name}>{s.name}</option>
          ))}
        </select>
        {activeSourceInfo && (
          <div className="text-xs text-cistern-muted font-mono">
            {formatBytes(activeSourceInfo.size_bytes)} · {new Date(activeSourceInfo.last_modified).toLocaleString()}
          </div>
        )}
        <div className="flex-1" />
        <input
          type="text"
          placeholder="Filter…"
          value={searchQuery}
          onChange={e => setSearchQuery(e.target.value)}
          className="bg-cistern-bg border border-cistern-border text-cistern-fg font-mono text-xs px-2 py-1 rounded-md w-32"
        />
        <label className="flex items-center gap-1 text-xs font-mono text-cistern-muted cursor-pointer">
          <input
            type="checkbox"
            checked={autoScroll}
            onChange={e => setAutoScroll(e.target.checked)}
            className="accent-cistern-accent"
          />
          Auto-scroll
        </label>
        <button
          onClick={clearEntries}
          className="text-xs font-mono text-cistern-muted hover:text-cistern-fg border border-cistern-border rounded-md px-2 py-1 transition-colors"
        >
          Clear
        </button>
      </div>
      <div className="flex-1 overflow-hidden">
        {loading && entries.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-cistern-muted font-mono">Loading logs…</div>
          </div>
        ) : (
          <LogViewer
            entries={entries}
            autoScroll={autoScroll}
            onAutoScrollChange={setAutoScroll}
            maxHeight="100%"
            searchQuery={searchQuery}
          />
        )}
      </div>
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}