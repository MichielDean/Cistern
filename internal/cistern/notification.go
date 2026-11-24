package cistern

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// NotificationEventTypes lists the event types that generate a dashboard
// notification. These are the health-relevant lifecycle events: stalls,
// circuit breaker trips, unroutable droplets, recoveries, recirculations and
// deliveries. Pure bookkeeping events (create, edit, pool, …) do not notify.
var NotificationEventTypes = map[string]bool{
	EventStall:          true,
	EventCircuitBreaker: true,
	EventNoRoute:        true,
	EventRecovery:       true,
	EventDelivered:      true,
	EventRecirculate:    true,
}

// notificationQualifiedEventTypesSQL is the SQL IN-list matching
// NotificationEventTypes. Keep the two in sync: the list limits the scan
// window so the promotion query stays cheap.
const notificationQualifiedEventTypesSQL = `'stall', 'circuit_breaker', 'no_route', 'recovery', 'delivered', 'recirculate'`

// Notification is one persistent dashboard notification. It wraps a single
// qualifying (droplet_id, event_type) row from the events table.
type Notification struct {
	ID         int64      `json:"id"`
	EventRowID int64      `json:"event_row_id"`
	DropletID  string     `json:"droplet_id"`
	EventType  string     `json:"event_type"`
	Title      string     `json:"title"`
	Detail     string     `json:"detail"`
	CreatedAt  time.Time  `json:"created_at"`
	ReadAt     *time.Time `json:"read_at"`
}

// notificationScanColumns is the column list shared by every notification
// SELECT. created_at and read_at are nullable in the general case; both scan
// through sql.NullTime via scanNotification.
const notificationScanColumns = `id, event_row_id, droplet_id, event_type, title, detail, created_at, read_at`

// scanNotification fills a Notification from one row already positioned at
// the notificationScanColumns order. row must be *sql.Row or *sql.Rows.
func scanNotification(row scanner) (Notification, error) {
	var (
		n         Notification
		createdAt sql.NullTime
		readAt    sql.NullTime
	)
	if err := row.Scan(&n.ID, &n.EventRowID, &n.DropletID, &n.EventType, &n.Title, &n.Detail, &createdAt, &readAt); err != nil {
		return Notification{}, err
	}
	if createdAt.Valid {
		n.CreatedAt = createdAt.Time
	}
	if readAt.Valid {
		t := readAt.Time
		n.ReadAt = &t
	}
	return n, nil
}

// scanner unifies *sql.Row and *sql.Rows for Scan. It is a structural subset
// of both concrete types, satisfied without any adapter.
type scanner interface {
	Scan(dest ...any) error
}

// SyncNotifications promotes every not-yet-promoted qualifying event row into
// the notifications table. Promotion is idempotent via the UNIQUE
// event_row_id column. It returns the notifications created by this call, in
// ascending event-row order.
func (c *Client) SyncNotifications() ([]Notification, error) {
	if c == nil {
		return nil, fmt.Errorf("cistern: sync notifications: nil client")
	}
	return c.syncNotifications()
}

// syncNotifications gathers the qualifying events first and closes the query
// before writing: the client caps sql.DB at one connection, so interleaving a
// read iterator with writes deadlocks.
func (c *Client) syncNotifications() ([]Notification, error) {
	type qualifyingEvent struct {
		rowid     int64
		dropletID string
		eventType string
		payload   string
		title     string
	}
	rows, err := c.db.Query(`
		SELECT e.rowid, e.droplet_id, e.event_type, e.payload, COALESCE(d.title, '')
		FROM events e
		LEFT JOIN droplets d ON d.id = e.droplet_id
		WHERE e.event_type IN (` + notificationQualifiedEventTypesSQL + `)
		ORDER BY e.rowid ASC`)
	if err != nil {
		return nil, fmt.Errorf("cistern: query qualifying events: %w", err)
	}
	var pending []qualifyingEvent
	for rows.Next() {
		var p qualifyingEvent
		if err := rows.Scan(&p.rowid, &p.dropletID, &p.eventType, &p.payload, &p.title); err != nil {
			rows.Close()
			return nil, fmt.Errorf("cistern: scan event row: %w", err)
		}
		pending = append(pending, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("cistern: iterate qualifying events: %w", err)
	}
	rows.Close()

	var created []Notification
	for _, p := range pending {
		if !NotificationEventTypes[p.eventType] {
			continue
		}
		label, detail := DisplayInfo(p.eventType, p.payload)
		name := p.title
		if name == "" {
			name = p.dropletID
		}
		notificationTitle := fmt.Sprintf("%s: %s", name, label)
		result, err := c.db.Exec(
			`INSERT OR IGNORE INTO notifications (event_row_id, droplet_id, event_type, title, detail, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			p.rowid, p.dropletID, p.eventType, notificationTitle, detail, time.Now().UTC(),
		)
		if err != nil {
			return created, fmt.Errorf("cistern: insert notification for event %d: %w", p.rowid, err)
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			id, err := result.LastInsertId()
			if err != nil {
				return created, fmt.Errorf("cistern: notification insert id: %w", err)
			}
			n, err := c.getNotification(id)
			if err != nil {
				return created, err
			}
			created = append(created, n)
		}
	}
	if len(created) > 0 {
		slog.Debug("notifications synced", "created", len(created))
	}
	return created, nil
}

// Precondition: the notifications row with the given id exists, or the caller
// treats wrapped sql.ErrNoRows as absent.
func (c *Client) getNotification(id int64) (Notification, error) {
	row := c.db.QueryRow(`SELECT `+notificationScanColumns+` FROM notifications WHERE id = ?`, id)
	n, err := scanNotification(row)
	if err != nil {
		return Notification{}, fmt.Errorf("cistern: get notification %d: %w", id, err)
	}
	return n, nil
}

// ListNotifications returns up to limit notifications, newest first. When
// onlyUnread is true the list is restricted to rows without a read_at value.
// limit < 1 defaults to 50.
func (c *Client) ListNotifications(onlyUnread bool, limit int) ([]Notification, error) {
	if limit < 1 {
		limit = 50
	}
	query := `SELECT ` + notificationScanColumns + ` FROM notifications`
	if onlyUnread {
		query += ` WHERE read_at IS NULL`
	}
	query += ` ORDER BY id DESC LIMIT ?`
	rows, err := c.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("cistern: list notifications: %w", err)
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("cistern: scan notification: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cistern: iterate notifications: %w", err)
	}
	return out, nil
}

// UnreadNotificationCount reports how many notifications have no read_at value.
func (c *Client) UnreadNotificationCount() (int64, error) {
	var count int64
	err := c.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE read_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cistern: unread notification count: %w", err)
	}
	return count, nil
}

// MarkNotificationRead stamps read_at on the notification with the given id.
// Marking an already-read notification is a no-op success. It returns the
// notification in its post-update state; unknown ids report not-found error text
// (recognized by dashboard handlers via isNotFoundError).
func (c *Client) MarkNotificationRead(id int64) (Notification, error) {
	result, err := c.db.Exec(
		`UPDATE notifications SET read_at = ? WHERE id = ? AND read_at IS NULL`,
		time.Now().UTC(), id,
	)
	if err != nil {
		return Notification{}, fmt.Errorf("cistern: mark notification %d read: %w", id, err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		// Either an unknown id or an already-read notification. Distinguish
		// by re-reading: absence is a real error, presence stays success.
		if _, err := c.getNotification(id); err != nil {
			return Notification{}, fmt.Errorf("cistern: mark notification %d read: not found", id)
		}
	}
	return c.getNotification(id)
}

// MarkAllNotificationsRead stamps read_at on every unread notification and
// returns the number of rows updated.
func (c *Client) MarkAllNotificationsRead() (int64, error) {
	result, err := c.db.Exec(
		`UPDATE notifications SET read_at = ? WHERE read_at IS NULL`,
		time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("cistern: mark all notifications read: %w", err)
	}
	return result.RowsAffected()
}
