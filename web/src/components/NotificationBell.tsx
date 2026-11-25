import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { Notification } from '../api/notifications';

const EVENT_COLORS: Record<string, string> = {
  stall: 'text-cistern-red',
  circuit_breaker: 'text-cistern-red',
  no_route: 'text-cistern-yellow',
  recovery: 'text-cistern-green',
  delivered: 'text-cistern-green',
  recirculate: 'text-cistern-yellow',
};

function timeAgo(iso: string): string {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

interface NotificationBellProps {
  notifications: Notification[];
  unreadCount: number;
  connected: boolean;
  onMarkRead: (id: number) => void;
  onMarkAllRead: () => void;
}

// NotificationBell renders the header bell with an unread badge and a
// dropdown history panel. Clicking a notification marks it read and opens
// the droplet it came from.
export function NotificationBell({
  notifications,
  unreadCount,
  connected,
  onMarkRead,
  onMarkAllRead,
}: NotificationBellProps) {
  const [open, setOpen] = useState(false);
  const [, setTimeTick] = useState(0);
  const panelRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const navigate = useNavigate();

  // Keep the relative "x ago" timestamps honest while the panel is open.
  useEffect(() => {
    if (!open) return;
    const timer = setInterval(() => setTimeTick((t) => t + 1), 30_000);
    return () => clearInterval(timer);
  }, [open]);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (
        panelRef.current && !panelRef.current.contains(e.target as Node) &&
        buttonRef.current && !buttonRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onDown);
    return () => document.removeEventListener('mousedown', onDown);
  }, [open]);

  // Close on Escape.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open]);

  return (
    <div className="relative">
      <button
        ref={buttonRef}
        onClick={() => setOpen((v) => !v)}
        className="relative text-cistern-muted hover:text-cistern-fg transition-colors p-1"
        aria-label={`Notifications${unreadCount > 0 ? `, ${unreadCount} unread` : ''}`}
        aria-expanded={open}
        data-testid="notification-bell"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
          <path d="M13.7 21a2 2 0 0 1-3.4 0" />
        </svg>
        {unreadCount > 0 && (
          <span
            className="absolute -top-1 -right-1 min-w-[18px] h-[18px] px-1 flex items-center justify-center rounded-full bg-cistern-red text-white text-[10px] font-bold leading-none"
            data-testid="notification-badge"
          >
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div
          ref={panelRef}
          className="absolute right-0 top-full mt-2 w-80 max-w-[calc(100vw-2rem)] bg-cistern-surface border border-cistern-border rounded-md shadow-lg z-50"
          data-testid="notification-panel"
        >
          <div className="flex items-center justify-between px-3 py-2 border-b border-cistern-border">
            <span className="text-xs font-mono text-cistern-muted">
              Notifications
              <span className={`ml-2 inline-block w-1.5 h-1.5 rounded-full ${connected ? 'bg-cistern-green' : 'bg-cistern-red'}`} />
            </span>
            {unreadCount > 0 && (
              <button
                onClick={onMarkAllRead}
                className="text-xs text-cistern-muted hover:text-cistern-accent transition-colors"
              >
                Mark all read
              </button>
            )}
          </div>
          <div className="max-h-80 overflow-y-auto">
            {notifications.length === 0 ? (
              <div className="px-3 py-6 text-center text-xs text-cistern-muted">No notifications yet</div>
            ) : (
              notifications.map((n) => (
                <button
                  key={n.id}
                  onClick={() => {
                    if (!n.read_at) onMarkRead(n.id);
                    if (n.droplet_id) {
                      navigate(`/app/droplets/${n.droplet_id}`);
                      setOpen(false);
                    }
                  }}
                  className={`block w-full text-left px-3 py-2 border-b border-cistern-border/50 hover:bg-cistern-border/20 transition-colors ${
                    n.read_at ? 'opacity-60' : ''
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className={`text-xs font-medium truncate ${EVENT_COLORS[n.event_type] ?? 'text-cistern-fg'}`}>
                      {n.title}
                    </span>
                    {!n.read_at && <span className="w-1.5 h-1.5 rounded-full bg-cistern-red shrink-0" />}
                  </div>
                  {n.detail && (
                    <p className="text-xs text-cistern-muted mt-0.5 line-clamp-2">{n.detail}</p>
                  )}
                  <div className="text-[10px] text-cistern-muted/70 mt-1 font-mono">{timeAgo(n.created_at)}</div>
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
