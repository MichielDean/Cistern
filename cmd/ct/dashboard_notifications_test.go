package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
)

// seedNotification creates a droplet + a qualifying event in the test db and
// promotes it, so REST/WS handlers have data to serve.
func seedNotification(t *testing.T, dbPath, repo, title string) cistern.Notification {
	t.Helper()
	c, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	id, err := c.Add(repo, title, "seed description", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RecordEvent(id.ID, cistern.EventStall, "{}"); err != nil {
		t.Fatal(err)
	}
	created, err := c.SyncNotifications()
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 seeded notification, got %d", len(created))
	}
	return created[0]
}

// TestNotificationsREST_ListAndMark covers the list endpoint, unread filter,
// limit validation, single mark-read and read-all.
func TestNotificationsREST_ListAndMark(t *testing.T) {
	dbPath := tempDB(t)
	seeded := seedNotification(t, dbPath, "repo-a", "REST check")

	mux := newDashboardMux(tempCfg(t), dbPath)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// List, newest first, includes seeded notification + unread count.
	resp, err := http.Get(srv.URL + "/api/notifications?limit=10")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var listResp notificationListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(listResp.Notifications))
	}
	if listResp.UnreadCount != 1 {
		t.Errorf("expected unread_count 1, got %d", listResp.UnreadCount)
	}
	if listResp.Notifications[0].ID != seeded.ID {
		t.Errorf("expected seeded notification %d, got %d", seeded.ID, listResp.Notifications[0].ID)
	}

	// Unread filter reflects it.
	resp2, err := http.Get(srv.URL + "/api/notifications?unread=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var unreadResp notificationListResponse
	if err := json.NewDecoder(resp2.Body).Decode(&unreadResp); err != nil {
		t.Fatal(err)
	}
	if len(unreadResp.Notifications) != 1 {
		t.Errorf("expected 1 unread, got %d", len(unreadResp.Notifications))
	}

	// Invalid limit is a 400.
	resp3, err := http.Get(srv.URL + "/api/notifications?limit=0")
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for limit=0, got %d", resp3.StatusCode)
	}

	// Mark read.
	resp4, err := http.Post(fmt.Sprintf(srv.URL+"/api/notifications/%d/read", seeded.ID), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	var marked cistern.Notification
	if err := json.NewDecoder(resp4.Body).Decode(&marked); err != nil {
		t.Fatal(err)
	}
	if marked.ReadAt == nil {
		t.Error("expected read_at to be set after marking read")
	}

	// Read-all on an empty set still succeeds.
	resp5, err := http.Post(srv.URL+"/api/notifications/read-all", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for read-all, got %d", resp5.StatusCode)
	}

	// Unknown id is 404 (isNotFoundError mapping).
	resp6, err := http.Post(srv.URL+"/api/notifications/999999/read", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp6.Body.Close()
	if resp6.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown id, got %d", resp6.StatusCode)
	}
}

// readServerFrame reads one unmasked server→client WebSocket frame.
func readServerFrame(br *bufio.Reader) (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(br, header[:]); err != nil {
		return 0, nil, err
	}
	opcode := header[0] & 0x0F
	length := int(header[1] & 0x7F)
	if header[1]&0x80 != 0 {
		return 0, nil, fmt.Errorf("server frame must not be masked")
	}
	switch header[1] & 0x7F {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint64(ext[:]))
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(br, payload); err != nil {
		return 0, nil, err
	}
	return opcode, payload, nil
}

// TestNotificationsWS_GreetingAndBroadcast verifies the connect greeting with
// the unread count, then a live broadcast after a hub sync tick.
func TestNotificationsWS_GreetingAndBroadcast(t *testing.T) {
	dbPath := tempDB(t)
	seedNotification(t, dbPath, "repo-a", "WS greeting check")

	mux := newDashboardMux(tempCfg(t), dbPath)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	fmt.Fprintf(conn, "GET /ws/notifications HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", addr, key)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}

	// Greeting frame: unread count for the seeded notification.
	opcode, payload, err := readServerFrame(br)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != 0x1 {
		t.Fatalf("expected text frame, got opcode %#x", opcode)
	}
	var greeting notificationWSFrame
	if err := json.Unmarshal(payload, &greeting); err != nil {
		t.Fatal(err)
	}
	if greeting.Type != "unread_count" || greeting.Count != 1 {
		t.Errorf("expected unread_count 1 greeting, got %+v", greeting)
	}

	// Record a qualifying event WITHOUT promoting it — that is the hub's job.
	// seedNotification pre-syncs, which would leave the hub nothing to see.
	c2, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	d2, err := c2.Add("repo-a", "WS broadcast check", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c2.RecordEvent(d2.ID, cistern.EventStall, "{}"); err != nil {
		t.Fatal(err)
	}
	c2.Close()

	conn.SetReadDeadline(time.Now().Add(2*notificationSyncInterval + 2*time.Second))
	for {
		opcode, payload, err = readServerFrame(br)
		if err != nil {
			t.Fatalf("waiting for broadcast: %v", err)
		}
		var frame notificationWSFrame
		if err := json.Unmarshal(payload, &frame); err != nil {
			t.Fatalf("decode ws frame: %v", err)
		}
		if frame.Type == "unread_count" {
			continue // mark-read refresh broadcast; keep waiting
		}
		if frame.Type != "notification" {
			t.Fatalf("unexpected frame type %q", frame.Type)
		}
		if frame.Notification == nil {
			t.Fatal("notification frame missing payload")
		}
		if got, want := frame.Notification.Title, "WS broadcast check: stall"; got != want {
			t.Fatalf("expected broadcast %q, got %q", want, got)
		}
		break
	}
}
