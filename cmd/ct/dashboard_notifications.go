package main

// Dashboard notifications: a persistent, unread-tracked feed of qualifying
// pipeline events, pushed over WebSocket and served over REST.
//
// Architecture:
//   - notificationHub owns the sync loop. Every interval it promotes new
//     qualifying event rows (stall, circuit_breaker, no_route, recovery,
//     delivered, recirculate) into the notifications table via
//     cistern.Client.SyncNotifications and broadcasts each promoted
//     notification to every connected WebSocket subscriber.
//   - /ws/notifications pushes {"type":"notification",...} frames; on connect
//     the client first receives the current unread count. Reconnecting
//     clients replay missed notifications through the REST list endpoint,
//     so no per-device delivery acknowledgement is needed.
//   - REST endpoints provide the history list, unread count, single-read and
//     mark-all-read actions. Marking read also broadcasts a fresh unread
//     count so other connected clients stay accurate.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
)

// notificationSyncInterval is how often the hub promotes new qualifying
// events into notifications. Sub-second latency is not required for
// pipeline-level events; 3s keeps write load negligible while feeling live.
const notificationSyncInterval = 3 * time.Second

// notificationHistoryLimit is the default page size for the REST list
// endpoint. Notifications accumulate slowly (qualifying events only), so a
// fixed window is sufficient; clients paginate by unread flag, not cursor.
const notificationHistoryLimit = 50

// notificationWSFrame is the JSON frame pushed to WebSocket clients.
type notificationWSFrame struct {
	Type         string                `json:"type"`
	Count        int64                 `json:"count,omitempty"`
	Notification *cistern.Notification `json:"notification,omitempty"`
}

// notificationHub fans notifications out to WebSocket subscribers. All
// mutable state lives in struct fields guarded by mu — no package-level
// variables.
type notificationHub struct {
	dbPath   string
	interval time.Duration

	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

// newNotificationHub constructs a stopped hub. Precondition: dbPath is the
// writable Cistern database path.
func newNotificationHub(dbPath string) *notificationHub {
	return &notificationHub{
		dbPath:   dbPath,
		interval: notificationSyncInterval,
		clients:  make(map[chan []byte]struct{}),
	}
}

// Start runs the sync-and-broadcast loop until ctx is cancelled. It performs
// one immediate sync so notifications exist without waiting for the first
// tick. Precondition: called at most once per hub instance.
func (h *notificationHub) Start(ctx context.Context) {
	go h.loop(ctx)
}

// Subscribe registers a subscriber and returns its delivery channel. The
// channel is buffered; senders never block (see broadcast). Precondition: the
// returned channel must be passed to Unsubscribe exactly once when no longer
// needed.
func (h *notificationHub) Subscribe() chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a previously returned subscriber channel and closes it,
// which also terminates the handler loop reading from it.
func (h *notificationHub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *notificationHub) loop(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncErr := h.syncOnce()
			if syncErr != nil {
				slog.Warn("notification sync failed", "error", syncErr)
			}
		}
	}
}

// syncOnce promotes new qualifying events and broadcasts each created
// notification. Errors are returned for the loop to log; a failed sync is
// retried on the next tick because promotion is idempotent.
func (h *notificationHub) syncOnce() error {
	c, err := cistern.New(h.dbPath, "")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer c.Close()

	created, err := c.SyncNotifications()
	if err != nil {
		return err
	}
	for _, n := range created {
		payload, err := json.Marshal(notificationWSFrame{Type: "notification", Notification: &n})
		if err != nil {
			return fmt.Errorf("marshal notification %d: %w", n.ID, err)
		}
		h.broadcast(payload)
	}
	if len(created) > 0 {
		h.broadcastUnreadCount(c)
	}
	return nil
}

// BroadcastUnreadCount refreshes every connected client's unread badge after
// a mark-read action. This method owns the db connection lifecycle.
func (h *notificationHub) BroadcastUnreadCount() {
	if h == nil {
		return
	}
	c, err := cistern.New(h.dbPath, "")
	if err != nil {
		slog.Warn("notification unread broadcast: open db failed", "error", err)
		return
	}
	defer c.Close()
	h.broadcastUnreadCount(c)
}

func (h *notificationHub) broadcastUnreadCount(c *cistern.Client) {
	count, err := c.UnreadNotificationCount()
	if err != nil {
		slog.Warn("notification unread broadcast: count failed", "error", err)
		return
	}
	payload, err := json.Marshal(notificationWSFrame{Type: "unread_count", Count: count})
	if err != nil {
		slog.Warn("notification unread broadcast: marshal failed", "error", err)
		return
	}
	h.broadcast(payload)
}

// broadcast delivers payload to every subscriber. A subscriber whose buffer
// is full skips this frame; its unread badge and history refresh through the
// REST endpoints on focus/visibility, so nothing is lost permanently.
func (h *notificationHub) broadcast(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
		}
	}
}

// makeNotificationsWSHandler returns the GET /ws/notifications handler.
// Authentication is enforced by the outer mux middleware (Bearer header or
// token query parameter, see apiAuthMiddleware), exactly like the dashboard
// SSE endpoints.
func makeNotificationsWSHandler(hub *notificationHub, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := wsUpgrade(w, r)
		if err != nil {
			return // wsUpgrade already wrote the HTTP error
		}
		defer conn.Close()

		// Greeting: current unread count so a fresh connection renders the
		// badge without a companion REST round-trip.
		var greeting []byte
		if c, err := cistern.New(dbPath, ""); err == nil {
			if count, err := c.UnreadNotificationCount(); err == nil {
				greeting, _ = json.Marshal(notificationWSFrame{Type: "unread_count", Count: count})
			}
			c.Close()
		}
		if greeting != nil {
			conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)) //nolint:errcheck
			if wsSendText(brw.Writer, string(greeting)) != nil {
				return
			}
		}

		sub := hub.Subscribe()
		defer hub.Unsubscribe(sub)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Reader goroutine: detects close frames and silently-partitioned
		// clients. The notification client sends no application frames, so
		// only transport liveness matters — use wsTuiReadTimeout.
		go func() {
			defer cancel()
			buf := make([]byte, wsMaxClientPayload)
			conn.SetReadDeadline(time.Now().Add(wsTuiReadTimeout)) //nolint:errcheck
			for {
				opcode, _, nb, err := wsReadClientFrame(brw.Reader, buf)
				buf = nb
				if err != nil {
					return
				}
				conn.SetReadDeadline(time.Now().Add(wsTuiReadTimeout)) //nolint:errcheck
				if opcode == wsOpcodeClose {
					return
				}
				// Pings and app frames are ignored; browsers rely on pongs,
				// but a stalled pinger is indistinguishable from a dead peer.
				// The 5-minute read deadline reaps both.
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case payload := <-sub:
				conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)) //nolint:errcheck
				if wsSendText(brw.Writer, string(payload)) != nil {
					return
				}
			}
		}
	}
}

// notificationListResponse is the GET /api/notifications body.
type notificationListResponse struct {
	Notifications []cistern.Notification `json:"notifications"`
	UnreadCount   int64                  `json:"unread_count"`
}

// handleNotificationsList serves GET /api/notifications with optional
// unread=1 filtering and an explicit limit (default 50, max maxPerPage).
func handleNotificationsList(hub *notificationHub, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		onlyUnread := r.URL.Query().Get("unread") == "1"
		limit := notificationHistoryLimit
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > maxPerPage {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			limit = n
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			list, err := c.ListNotifications(onlyUnread, limit)
			if err != nil {
				return err
			}
			if list == nil {
				list = []cistern.Notification{}
			}
			unread, err := c.UnreadNotificationCount()
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, notificationListResponse{
				Notifications: list,
				UnreadCount:   unread,
			})
			return nil
		})
	}
}

// handleNotificationMarkRead serves POST /api/notifications/{id}/read.
// Marking a read notification again is a success; an unknown id is a 404.
func handleNotificationMarkRead(hub *notificationHub, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			http.Error(w, "invalid notification id", http.StatusBadRequest)
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			n, err := c.MarkNotificationRead(id)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, n)
			go hub.BroadcastUnreadCount()
			return nil
		})
	}
}

// handleNotificationsMarkAllRead serves POST /api/notifications/read-all and
// returns the number of notifications marked read.
func handleNotificationsMarkAllRead(hub *notificationHub, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiClient(dbPath, w, func(c *cistern.Client) error {
			marked, err := c.MarkAllNotificationsRead()
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]int64{"marked": marked})
			go hub.BroadcastUnreadCount()
			return nil
		})
	}
}
