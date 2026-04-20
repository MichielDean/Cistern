import { useState, useRef, useCallback, useEffect } from 'react';
import { StatusIndicator } from '../components/StatusIndicator';
import { ActionButton } from '../components/ActionButton';
import { useCastellariusStatus, castellariusAction } from '../api/castellarius';

interface Toast {
  message: string;
  type: 'success' | 'error';
}

export function CastellariusPage() {
  const { status, loading, error, refresh } = useCastellariusStatus(5000);
  const [toast, setToast] = useState<Toast | null>(null);
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const showToast = useCallback((message: string, type: Toast['type']) => {
    if (toastTimerRef.current !== null) {
      clearTimeout(toastTimerRef.current);
    }
    setToast({ message, type });
    toastTimerRef.current = setTimeout(() => {
      toastTimerRef.current = null;
      setToast(null);
    }, 3000);
  }, []);

  useEffect(() => {
    return () => {
      if (toastTimerRef.current !== null) {
        clearTimeout(toastTimerRef.current);
      }
    };
  }, []);

  const handleAction = async (action: 'start' | 'stop' | 'restart') => {
    try {
      await castellariusAction(action);
      showToast(`${action} succeeded`, 'success');
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : `${action} failed`, 'error');
    }
  };

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
      <div className="flex items-center justify-center h-full">
        <div className="text-cistern-muted font-mono">Loading…</div>
      </div>
    );
  }

  if (!status) return null;

  const indicatorStatus = status.running ? 'running' : 'stopped';
  const uptimeStr = status.uptime_seconds != null
    ? `${Math.floor(status.uptime_seconds / 3600)}h${Math.floor((status.uptime_seconds % 3600) / 60).toString().padStart(2, '0')}m`
    : null;

  return (
    <div className="flex-1 overflow-y-auto p-4 md:p-6 space-y-6">
      <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
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

      <section>
        <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">Aqueducts</h2>
        <div className="space-y-3">
          {status.aqueducts.length === 0 && (
            <div className="text-cistern-muted text-sm font-mono">No aqueducts active</div>
          )}
          {status.aqueducts.map((aq) => (
            <AqueductRow key={aq.name} aqueduct={aq} />
          ))}
        </div>
      </section>

      <div className="flex items-center gap-2">
        <StatusIndicator status={status.farm_running ? 'running' : 'stopped'} label={`Farm ${status.farm_running ? 'Running' : 'Stopped'}`} />
      </div>

      {toast && (
        <div className={`fixed bottom-4 right-4 bg-cistern-surface border border-cistern-border rounded-lg p-3 font-mono text-sm ${
          toast.type === 'success' ? 'text-cistern-green' : 'text-cistern-red'
        }`}>
          {toast.message}
        </div>
      )}
    </div>
  );
}

function AqueductRow({ aqueduct }: { aqueduct: { name: string; status: string; droplet_id: string | null; droplet_title: string | null; current_step: string | null; elapsed: number } }) {
  const elapsedSec = Math.floor(aqueduct.elapsed / 1e9);
  const elapsedStr = elapsedSec >= 3600
    ? `${Math.floor(elapsedSec / 3600)}h${Math.floor((elapsedSec % 3600) / 60).toString().padStart(2, '0')}m`
    : `${Math.floor(elapsedSec / 60)}:${(elapsedSec % 60).toString().padStart(2, '0')}`;

  return (
    <div className={`bg-cistern-surface border rounded-lg p-4 ${
      aqueduct.status === 'flowing' ? 'border-cistern-accent/40' : 'border-cistern-border'
    }`}>
      <div className="flex items-center justify-between mb-2">
        <a href="/app/" className="font-mono font-bold text-cistern-fg hover:text-cistern-accent">
          {aqueduct.name}
        </a>
        <span className={`text-xs font-mono px-2 py-0.5 rounded-full ${
          aqueduct.status === 'flowing'
            ? 'bg-cistern-green/20 text-cistern-green'
            : 'bg-cistern-muted/20 text-cistern-muted'
        }`}>
          {aqueduct.status}
        </span>
      </div>
      {aqueduct.droplet_id && aqueduct.status === 'flowing' && (
        <div className="text-sm font-mono">
          <span className="text-cistern-green">{aqueduct.droplet_id}</span>
          {aqueduct.droplet_title && (
            <>
              <span className="text-cistern-muted mx-2">·</span>
              <span className="text-cistern-fg">{aqueduct.droplet_title}</span>
            </>
          )}
          {aqueduct.current_step && (
            <>
              <span className="text-cistern-muted mx-2">·</span>
              <span className="text-cistern-accent">{aqueduct.current_step}</span>
            </>
          )}
          <span className="text-cistern-muted mx-2">·</span>
          <span className="text-cistern-muted">{elapsedStr}</span>
        </div>
      )}
    </div>
  );
}