import { useCallback, useEffect, useRef, useState } from 'react';
import { StatusIndicator } from '../components/StatusIndicator';
import { ActionButton } from '../components/ActionButton';
import { LogViewer } from '../components/LogViewer';
import { useToast } from '../components/Toast';
import { SkeletonCard } from '../components/LoadingSkeleton';
import { useCastellariusStatus, castellariusAction } from '../api/castellarius';
import { fetchLogHistory, createLogEventSource } from '../api/logs';
import type { LogEntry } from '../api/types';

export function CastellariusPage() {
  const { status, loading, error, refresh } = useCastellariusStatus(5000);
  const { addToast } = useToast();
  const [logEntries, setLogEntries] = useState<LogEntry[]>([]);
  const [logError, setLogError] = useState<Error | null>(null);
  const [logLoading, setLogLoading] = useState(true);
  const [autoScroll, setAutoScroll] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const esRef = useRef<EventSource | null>(null);
  const lastLine = useRef(0);

  const handleAction = useCallback(async (action: 'start' | 'stop' | 'restart') => {
    try {
      await castellariusAction(action);
      addToast(`${action} succeeded`, 'success');
      refresh();
    } catch (err) {
      addToast(err instanceof Error ? err.message : `${action} failed`, 'error');
    }
  }, [addToast, refresh]);

  useEffect(() => {
    setLogLoading(true);
    setLogError(null);
    fetchLogHistory(500, 'castellarius').then((entries) => {
      lastLine.current = entries.length > 0 ? entries[entries.length - 1].line : 0;
      setLogEntries(entries);
    }).catch((err) => {
      setLogError(err instanceof Error ? err : new Error(String(err)));
    }).finally(() => setLogLoading(false));

    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }
    esRef.current = createLogEventSource('castellarius', (entry) => {
      if (entry.line <= lastLine.current) return;
      setLogEntries((prev) => [...prev, entry]);
    }, (err) => {
      setLogError(err);
    });

    return () => {
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
    };
  }, []);

  if (error && !status) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-cistern-red text-lg font-mono mb-2">Connection Error</div>
          <div className="text-cistern-muted text-sm">{error.message}</div>
        </div>
      </div>
    );
  }

  if (loading && !status) {
    return (
      <div className="flex-1 p-4 md:p-6 space-y-6">
        <SkeletonCard lines={4} />
        <SkeletonCard lines={3} />
      </div>
    );
  }

  if (!status) return null;

  const indicatorStatus = status.running ? 'running' : 'stopped';
  const uptimeStr = status.uptime_seconds != null
    ? `${Math.floor(status.uptime_seconds / 3600)}h${Math.floor((status.uptime_seconds % 3600) / 60).toString().padStart(2, '0')}m`
    : null;

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-y-auto p-4 md:p-6 space-y-4">
      <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4 shrink-0">
        <div className="flex items-center justify-between mb-4">
          <StatusIndicator status={indicatorStatus} label="Castellarius" size="lg" />
          <div className="flex items-center gap-2">
            <ActionButton
              label="Start"
              onClick={() => handleAction('start')}
              variant="primary"
              disabled={status.running}
            />
            <ActionButton
              label="Stop"
              onClick={() => handleAction('stop')}
              variant="danger"
              disabled={!status.running}
            />
            <ActionButton
              label="Restart"
              onClick={() => handleAction('restart')}
              variant="danger"
              disabled={!status.running}
              confirm="Restart Castellarius? Active droplets will be interrupted."
            />
          </div>
        </div>
        <div className="flex items-center gap-4 text-sm text-cistern-muted font-mono">
          {status.pid != null && <span>PID: {status.pid}</span>}
          {uptimeStr && <span>Uptime: {uptimeStr}</span>}
        </div>
      </section>

      <section className="flex-1 flex flex-col min-h-0">
        <div className="flex items-center justify-between mb-3 shrink-0">
          <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider">Castellarius Log</h2>
          <div className="flex items-center gap-2">
            <input
              type="text"
              placeholder="Filter..."
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
          </div>
        </div>
        <div className="flex-1 min-h-0 bg-cistern-surface border border-cistern-border rounded-lg">
          {logError && logEntries.length === 0 ? (
            <div className="flex items-center justify-center h-full text-cistern-muted text-sm font-mono">{logError.message}</div>
          ) : logLoading && logEntries.length === 0 ? (
            <div className="flex items-center justify-center h-full text-cistern-muted text-sm font-mono">Loading...</div>
          ) : (
            <LogViewer
              entries={logEntries}
              autoScroll={autoScroll}
              onAutoScrollChange={setAutoScroll}
              maxHeight="100%"
              searchQuery={searchQuery}
            />
          )}
        </div>
      </section>
    </div>
  );
}