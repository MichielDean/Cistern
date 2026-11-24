import { apiFetch } from './shared';

export interface Notification {
  id: number;
  event_row_id: number;
  droplet_id: string;
  event_type: string;
  title: string;
  detail: string;
  created_at: string;
  read_at: string | null;
}

export interface NotificationListResponse {
  notifications: Notification[];
  unread_count: number;
}

export function fetchNotifications(onlyUnread = false, limit = 50): Promise<NotificationListResponse> {
  const params = new URLSearchParams();
  if (onlyUnread) params.set('unread', '1');
  params.set('limit', String(limit));
  return apiFetch<NotificationListResponse>(`/api/notifications?${params.toString()}`);
}

export function markNotificationRead(id: number): Promise<Notification> {
  return apiFetch<Notification>(`/api/notifications/${id}/read`, { method: 'POST' });
}

export function markAllNotificationsRead(): Promise<{ marked: number }> {
  return apiFetch<{ marked: number }>('/api/notifications/read-all', { method: 'POST' });
}

// Frame pushed over /ws/notifications.
export interface NotificationWSFrame {
  type: 'notification' | 'unread_count';
  count?: number;
  notification?: Notification;
}
