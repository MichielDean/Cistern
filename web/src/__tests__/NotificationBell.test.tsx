import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { NotificationBell } from '../components/NotificationBell';
import type { Notification } from '../api/notifications';

const baseNotification: Notification = {
  id: 1,
  event_row_id: 10,
  droplet_id: 'drp-1',
  event_type: 'stall',
  title: 'Parser droplet: stalled',
  detail: 'no heartbeat for 10m',
  created_at: new Date(Date.now() - 30_000).toISOString(),
  read_at: null,
};

function renderBell(props: Partial<Parameters<typeof NotificationBell>[0]> = {}) {
  return render(
    <MemoryRouter>
      <NotificationBell
        notifications={props.notifications ?? [baseNotification]}
        unreadCount={props.unreadCount ?? 1}
        connected={props.connected ?? true}
        onMarkRead={props.onMarkRead ?? vi.fn()}
        onMarkAllRead={props.onMarkAllRead ?? vi.fn()}
      />
    </MemoryRouter>,
  );
}

describe('NotificationBell', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the unread badge count', () => {
    renderBell({ unreadCount: 3 });
    expect(screen.getByTestId('notification-badge')).toHaveTextContent('3');
  });

  it('hides the badge when unread count is zero', () => {
    renderBell({ unreadCount: 0, notifications: [] });
    expect(screen.queryByTestId('notification-badge')).toBeNull();
  });

  it('opens the panel on click and lists notifications', async () => {
    renderBell();
    fireEvent.click(screen.getByTestId('notification-bell'));
    const panel = await waitFor(() => screen.getByTestId('notification-panel'));
    expect(panel).toBeInTheDocument();
    expect(screen.getByText('Parser droplet: stalled')).toBeInTheDocument();
  });

  it('marks a notification read when clicked', async () => {
    const onMarkRead = vi.fn();
    renderBell({ onMarkRead });
    fireEvent.click(screen.getByTestId('notification-bell'));
    fireEvent.click(await screen.findByText('Parser droplet: stalled'));
    expect(onMarkRead).toHaveBeenCalledWith(1);
  });

  it('calls mark-all-read from the panel header', async () => {
    const onMarkAllRead = vi.fn();
    renderBell({ onMarkAllRead });
    fireEvent.click(screen.getByTestId('notification-bell'));
    fireEvent.click(await screen.findByText('Mark all read'));
    expect(onMarkAllRead).toHaveBeenCalledTimes(1);
  });

  it('renders an empty state with no notifications', async () => {
    renderBell({ notifications: [], unreadCount: 0 });
    fireEvent.click(screen.getByTestId('notification-bell'));
    expect(await screen.findByText('No notifications yet')).toBeInTheDocument();
  });
});
