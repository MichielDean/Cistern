import { useCallback, useEffect, useRef, useState } from 'react';
import { getAuthParams } from './useAuth';
import {
  fetchNotifications,
  markNotificationRead,
  markAllNotificationsRead,
  type Notification,
  type NotificationWSFrame,
} from '../api/notifications';

const WS_RECONNECT_BASE_MS = 1000;
const WS_RECONNECT_MAX_MS = 30000;

// useNotifications keeps the notification feed live: history list, unread
// badge count, and real-time pushes over /ws/notifications. Missed
// notifications are replayed on reconnect through the REST list endpoint, so
// no per-device delivery state is required.
export function useNotifications(enabled = true) {
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>();
  const retryCountRef = useRef(0);
  const mountedRef = useRef(true);
  const enabledRef = useRef(enabled);

  useEffect(() => { enabledRef.current = enabled; }, [enabled]);

  const refresh = useCallback(async () => {
    try {
      const resp = await fetchNotifications();
      if (!mountedRef.current) return;
      setNotifications(resp.notifications);
      setUnreadCount(resp.unread_count);
    } catch (err) {
      console.warn('Failed to load notifications:', err);
    }
  }, []);

  const markRead = useCallback(async (id: number) => {
    try {
      const updated = await markNotificationRead(id);
      if (!mountedRef.current) return;
      setNotifications((prev) => prev.map((n) => (n.id === id ? updated : n)));
      setUnreadCount((prev) => Math.max(0, prev - 1));
    } catch (err) {
      console.warn('Failed to mark notification read:', err);
    }
  }, []);

  const markAllRead = useCallback(async () => {
    try {
      const resp = await markAllNotificationsRead();
      if (!mountedRef.current) return;
      setNotifications((prev) => prev.map((n) => ({ ...n, read_at: n.read_at ?? new Date().toISOString() })));
      setUnreadCount(0);
      void resp;
    } catch (err) {
      console.warn('Failed to mark all notifications read:', err);
    }
  }, []);

  const connect = useCallback(() => {
    if (!enabledRef.current) return;
    if (wsRef.current) {
      wsRef.current.close();
    }

    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const authParams = getAuthParams();
    const url = `${proto}//${location.host}/ws/notifications${authParams ? `?${authParams}` : ''}`;
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      if (!mountedRef.current) return;
      retryCountRef.current = 0;
      setConnected(true);
      // Replay anything missed while disconnected.
      void refresh();
    };

    ws.onmessage = (e) => {
      if (!mountedRef.current) return;
      let frame: NotificationWSFrame;
      try {
        frame = JSON.parse(e.data);
      } catch {
        return;
      }
      switch (frame.type) {
        case 'unread_count':
          if (typeof frame.count === 'number') setUnreadCount(frame.count);
          break;
        case 'notification':
          if (frame.notification) {
            const n = frame.notification;
            setNotifications((prev) => (prev.some((p) => p.id === n.id) ? prev : [n, ...prev]));
            if (!n.read_at) setUnreadCount((prev) => prev + 1);
          }
          break;
      }
    };

    ws.onclose = () => {
      if (!mountedRef.current) return;
      setConnected(false);
      const delay = Math.min(WS_RECONNECT_BASE_MS * 2 ** retryCountRef.current, WS_RECONNECT_MAX_MS);
      retryCountRef.current += 1;
      reconnectTimer.current = setTimeout(connect, delay);
    };

    ws.onerror = () => ws.close();
  }, [refresh]);

  useEffect(() => {
    mountedRef.current = true;
    connect();

    // Safety net for dropped push frames: resync the badge when the tab
    // becomes visible again.
    const onVisible = () => {
      if (document.visibilityState === 'visible' && enabledRef.current) void refresh();
    };
    document.addEventListener('visibilitychange', onVisible);

    return () => {
      mountedRef.current = false;
      document.removeEventListener('visibilitychange', onVisible);
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
    };
  }, [connect, refresh]);

  return { notifications, unreadCount, connected, refresh, markRead, markAllRead };
}
