import { useState, useEffect } from 'react';
import type { DashboardData } from '../api/types';

interface HeaderProps {
  data: DashboardData | null;
  connected: boolean;
  onMenuClick: () => void;
}

export function Header({ data, connected, onMenuClick }: HeaderProps) {
  const [justConnected, setJustConnected] = useState(false);
  const [prevConnected, setPrevConnected] = useState(connected);

  useEffect(() => {
    if (connected && !prevConnected) {
      setJustConnected(true);
      const timer = setTimeout(() => setJustConnected(false), 2000);
      setPrevConnected(connected);
      return () => clearTimeout(timer);
    }
    setPrevConnected(connected);
  }, [connected, prevConnected]);

  return (
    <header className="h-[60px] bg-cistern-surface border-b border-cistern-border flex items-center px-4 gap-4 shrink-0" role="banner">
      <button
        onClick={onMenuClick}
        className="md:hidden text-cistern-muted hover:text-cistern-fg transition-colors"
        aria-label="Open menu"
      >
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M3 5h14M3 10h14M3 15h14" />
        </svg>
      </button>
      <div className="hidden md:flex items-center gap-2">
        <span className="text-cistern-accent font-mono font-bold text-lg">Cistern</span>
      </div>
      <div className="flex-1" />
      {data && (
        <div className="flex items-center gap-4 text-sm font-mono">
          <StatusPill label="Flowing" count={data.flowing_count} color="green" />
          <StatusPill label="Queued" count={data.queued_count} color="yellow" />
          <StatusPill label="Delivered" count={data.done_count} color="blue" />
        </div>
      )}
      <div className="flex items-center gap-2">
        <div className={`w-2 h-2 rounded-full ${connected ? 'bg-cistern-green' : 'bg-cistern-red'}`} />
        <span className="text-xs text-cistern-muted hidden sm:inline">
          {justConnected ? 'Connected' : connected ? 'Live' : 'Reconnecting...'}
        </span>
      </div>
      <a
        href="/"
        className="hidden sm:inline text-xs text-cistern-muted hover:text-cistern-accent transition-colors border border-cistern-border rounded px-2 py-1"
        title="Switch to terminal dashboard"
      >
        Classic Dashboard
      </a>
    </header>
  );
}

function StatusPill({ label, count, color }: { label: string; count: number; color: 'green' | 'yellow' | 'blue' }) {
  const colorClasses = {
    green: 'bg-cistern-green/20 text-cistern-green',
    yellow: 'bg-cistern-yellow/20 text-cistern-yellow',
    blue: 'bg-cistern-accent/20 text-cistern-accent',
  };
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs ${colorClasses[color]}`}>
      {label} {count}
    </span>
  );
}