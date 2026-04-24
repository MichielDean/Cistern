package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
)

func TestDashboardWebMux_RootRedirectsToApp(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("GET / status = %d, want 301", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/app/" {
		t.Errorf("Location = %q, want /app/", loc)
	}
}

func TestDashboardWebMux_NotFoundForUnknownPaths(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /nonexistent status = %d, want 404", w.Code)
	}
}

func TestDashboardWebMux_APIReturnsJSON(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/dashboard status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var data DashboardData
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if data.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set in JSON response")
	}
}

func TestDashboardWebMux_APIMethodNotAllowed(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/dashboard status = %d, want 405", w.Code)
	}
}

func TestDashboardWebMux_EventsSSEHeaders(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "data: ") {
		t.Errorf("SSE body should start with 'data: ', got %q", truncateStr(body, 60))
	}
	firstLine := strings.SplitN(body, "\n", 2)[0]
	payload := strings.TrimPrefix(firstLine, "data: ")
	var d DashboardData
	if err := json.Unmarshal([]byte(payload), &d); err != nil {
		t.Errorf("SSE payload is not valid DashboardData JSON: %v — payload: %q", err, truncateStr(payload, 80))
	}
}

func TestDashboardWebMux_APIReturnsCorrectCounts(t *testing.T) {
	cfgPath := tempCfg(t)
	dbPath := tempDB(t)

	c, err := cistern.New(dbPath, "mr")
	if err != nil {
		t.Fatal(err)
	}
	flowing, _ := c.Add("myrepo", "Feature A", "", 1)
	c.GetReady("myrepo")
	c.Assign(flowing.ID, "virgo", "implement")
	c.Add("myrepo", "Feature B", "", 2)
	c.Close()

	mux := newDashboardMux(cfgPath, dbPath)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var data DashboardData
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if data.FlowingCount != 1 {
		t.Errorf("FlowingCount = %d, want 1", data.FlowingCount)
	}
	if data.QueuedCount != 1 {
		t.Errorf("QueuedCount = %d, want 1", data.QueuedCount)
	}
}

func TestDashboardWebMux_NoteFieldsRoundTrip(t *testing.T) {
	cfgPath := tempCfg(t)
	dbPath := tempDB(t)

	c, err := cistern.New(dbPath, "mr")
	if err != nil {
		t.Fatal(err)
	}
	droplet, _ := c.Add("myrepo", "Note Test", "", 1)
	c.GetReady("myrepo")
	c.Assign(droplet.ID, "virgo", "implement")
	if err := c.AddNote(droplet.ID, "implementer", "hello world"); err != nil {
		t.Fatal(err)
	}
	c.Close()

	mux := newDashboardMux(cfgPath, dbPath)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var data DashboardData
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(data.FlowActivities) == 0 {
		t.Fatal("expected at least one flow activity")
	}
	notes := data.FlowActivities[0].RecentNotes
	if len(notes) == 0 {
		t.Fatal("expected at least one recent note")
	}
	if notes[0].CataractaeName != "implementer" {
		t.Errorf("CataractaeName = %q, want %q", notes[0].CataractaeName, "implementer")
	}
	if notes[0].Content != "hello world" {
		t.Errorf("Content = %q, want %q", notes[0].Content, "hello world")
	}
}

func TestDashboardWebMux_NoteFieldsSnakeCaseJSON(t *testing.T) {
	cfgPath := tempCfg(t)
	dbPath := tempDB(t)

	c, err := cistern.New(dbPath, "mr")
	if err != nil {
		t.Fatal(err)
	}
	droplet, _ := c.Add("myrepo", "Snake Case Test", "", 1)
	c.GetReady("myrepo")
	c.Assign(droplet.ID, "virgo", "implement")
	if err := c.AddNote(droplet.ID, "implementer", "snake test"); err != nil {
		t.Fatal(err)
	}
	c.Close()

	mux := newDashboardMux(cfgPath, dbPath)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal raw JSON: %v", err)
	}
	activities, _ := raw["flow_activities"].([]interface{})
	if len(activities) == 0 {
		t.Fatal("expected at least one flow_activity in raw JSON")
	}
	act, _ := activities[0].(map[string]interface{})
	notes, _ := act["recent_notes"].([]interface{})
	if len(notes) == 0 {
		t.Fatal("expected at least one recent_note in raw JSON")
	}
	note, _ := notes[0].(map[string]interface{})
	if _, ok := note["cataractae_name"]; !ok {
		t.Errorf("raw JSON note missing key %q (got %v)", "cataractae_name", note)
	}
	if _, ok := note["content"]; !ok {
		t.Errorf("raw JSON note missing key %q (got %v)", "content", note)
	}
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// --- peek HTTP/WS tests ---

func TestWsAcceptKey(t *testing.T) {
	got := wsAcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Errorf("wsAcceptKey() = %q, want %q", got, want)
	}
}

func TestWsSendText_SmallPayload(t *testing.T) {
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := wsSendText(bw, "hello"); err != nil {
		t.Fatalf("wsSendText: %v", err)
	}
	b := buf.Bytes()
	if b[0] != 0x81 {
		t.Errorf("byte[0] = 0x%02x, want 0x81 (FIN+text)", b[0])
	}
	if b[1] != 5 {
		t.Errorf("byte[1] (len) = %d, want 5", b[1])
	}
	if string(b[2:]) != "hello" {
		t.Errorf("payload = %q, want %q", string(b[2:]), "hello")
	}
}

func TestWsSendText_MediumPayload(t *testing.T) {
	payload := strings.Repeat("x", 200)
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := wsSendText(bw, payload); err != nil {
		t.Fatalf("wsSendText: %v", err)
	}
	b := buf.Bytes()
	if b[0] != 0x81 {
		t.Errorf("byte[0] = 0x%02x, want 0x81", b[0])
	}
	if b[1] != 0x7E {
		t.Errorf("byte[1] = 0x%02x, want 0x7E (medium extended len)", b[1])
	}
	n := int(binary.BigEndian.Uint16(b[2:4]))
	if n != 200 {
		t.Errorf("encoded length = %d, want 200", n)
	}
	if string(b[4:]) != payload {
		t.Error("payload content mismatch")
	}
}

func TestWsSendText_LargePayload(t *testing.T) {
	payload := strings.Repeat("x", 65536)
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := wsSendText(bw, payload); err != nil {
		t.Fatalf("wsSendText: %v", err)
	}
	b := buf.Bytes()
	if b[0] != 0x81 {
		t.Errorf("byte[0] = 0x%02x, want 0x81 (FIN+text)", b[0])
	}
	if b[1] != 0x7F {
		t.Errorf("byte[1] = 0x%02x, want 0x7F (8-byte extended len)", b[1])
	}
	n := int(binary.BigEndian.Uint64(b[2:10]))
	if n != 65536 {
		t.Errorf("encoded length = %d, want 65536", n)
	}
	if string(b[10:]) != payload {
		t.Error("payload content mismatch for large frame")
	}
}

func TestWsFrameRoundtrip_LargePayload(t *testing.T) {
	payload := strings.Repeat("z", 65536)
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := wsSendText(bw, payload); err != nil {
		t.Fatalf("wsSendText: %v", err)
	}
	br := bufio.NewReader(&buf)
	got, err := readWSTextFrame(br)
	if err != nil {
		t.Fatalf("readWSTextFrame: %v", err)
	}
	if got != payload {
		t.Errorf("roundtrip length mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

func TestLookupAqueductSession_Empty(t *testing.T) {
	_, ok := lookupAqueductSession(tempDB(t), "virgo")
	if ok {
		t.Error("expected false for empty DB")
	}
}

func TestLookupAqueductSession_NoMatch(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	item, _ := c.Add("myrepo", "Some work", "", 1)
	c.GetReady("myrepo")
	c.Assign(item.ID, "other-aqueduct", "implement")
	c.Close()

	_, ok := lookupAqueductSession(db, "virgo")
	if ok {
		t.Error("expected false when no item assigned to 'virgo'")
	}
}

func TestLookupAqueductSession_Found(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	item, _ := c.Add("myrepo", "Peek target", "", 1)
	c.GetReady("myrepo")
	c.Assign(item.ID, "virgo", "implement")
	c.Close()

	info, ok := lookupAqueductSession(db, "virgo")
	if !ok {
		t.Fatal("expected session to be found")
	}
	if info.dropletID != item.ID {
		t.Errorf("dropletID = %q, want %q", info.dropletID, item.ID)
	}
	if !strings.Contains(info.sessionID, "virgo") {
		t.Errorf("sessionID %q should contain 'virgo'", info.sessionID)
	}
	if info.title != "Peek target" {
		t.Errorf("title = %q, want %q", info.title, "Peek target")
	}
}

func TestPeekHTTP_MethodNotAllowed(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodPost, "/api/aqueducts/virgo/peek", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestPeekHTTP_IdleAqueduct(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/aqueducts/virgo/peek", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "session not active") {
		t.Errorf("body = %q, want 'session not active'", w.Body.String())
	}
}

func TestPeekHTTP_ActiveWithMockCapturer(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	item, _ := c.Add("myrepo", "Peek work", "", 1)
	c.GetReady("myrepo")
	c.Assign(item.ID, "virgo", "implement")
	c.Close()

	orig := defaultCapturer
	t.Cleanup(func() { defaultCapturer = orig })
	defaultCapturer = mockCapturer{hasSession: true, content: "pane output line"}

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/aqueducts/virgo/peek", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "pane output line") {
		t.Errorf("body = %q, want 'pane output line'", w.Body.String())
	}
}

func TestPeekHTTP_ActiveButSessionGone(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	item, _ := c.Add("myrepo", "Gone session", "", 1)
	c.GetReady("myrepo")
	c.Assign(item.ID, "virgo", "implement")
	c.Close()

	orig := defaultCapturer
	t.Cleanup(func() { defaultCapturer = orig })
	defaultCapturer = mockCapturer{hasSession: false}

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/aqueducts/virgo/peek", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "session not active") {
		t.Errorf("body = %q, want 'session not active'", w.Body.String())
	}
}

func TestPeekHTTP_LinesQueryParam(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/aqueducts/virgo/peek?lines=50", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code >= 500 {
		t.Errorf("status = %d, want < 500", w.Code)
	}
}

func TestWsUpgrade_CrossOriginRejected(t *testing.T) {
	cases := []struct {
		name   string
		origin string
	}{
		{"evil_http", "http://evil.com"},
		{"evil_https", "https://evil.com"},
		{"localhost_subdomain", "http://localhost.evil.com"},
		{"127_lookalike", "http://127.0.0.1.evil.com"},
	}
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws/aqueducts/virgo/peek", nil)
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("Origin %q: status = %d, want 403 Forbidden", tc.origin, w.Code)
			}
		})
	}
}

func TestWsUpgrade_LocalhostOriginAllowed(t *testing.T) {
	cases := []struct {
		name   string
		origin string
	}{
		{"localhost", "http://localhost"},
		{"localhost_with_port", "http://localhost:5737"},
		{"loopback_ipv4", "http://127.0.0.1"},
		{"loopback_ipv4_with_port", "http://127.0.0.1:5737"},
		{"loopback_ipv6", "http://[::1]"},
		{"loopback_ipv6_with_port", "http://[::1]:5737"},
	}
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws/aqueducts/virgo/peek", nil)
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code == http.StatusForbidden {
				t.Errorf("Origin %q: got 403, want non-403 (localhost origin must be allowed)", tc.origin)
			}
		})
	}
}

func TestWsUpgrade_LANOriginAllowed(t *testing.T) {
	cases := []struct {
		name   string
		origin string
	}{
		{"192_168", "http://192.168.0.138:5737"},
		{"10_x", "http://10.0.0.1:5737"},
		{"172_16", "http://172.16.0.1:5737"},
	}
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws/aqueducts/virgo/peek", nil)
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code == http.StatusForbidden {
				t.Errorf("Origin %q: got 403, want non-403 (LAN origin must be allowed)", tc.origin)
			}
		})
	}
}

func TestWsUpgrade_MissingOriginAllowed(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/ws/aqueducts/virgo/peek", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusForbidden {
		t.Errorf("missing Origin: got 403, want non-403 (no Origin header should be allowed)")
	}
}

func TestWsPeek_NonWebSocketRejected(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/ws/aqueducts/virgo/peek", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUpgradeRequired {
		t.Errorf("status = %d, want 426", w.Code)
	}
}

func TestWsPeek_MissingKeyRejected(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/ws/aqueducts/virgo/peek", nil)
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func wsDialPeek(t *testing.T, srv *httptest.Server, aqName string) (*bufio.Reader, net.Conn) {
	t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	fmt.Fprintf(conn, "GET /ws/aqueducts/%s/peek HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", aqName, key)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	return br, conn
}

func TestWsPeek_SuccessfulStreamIdle(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	br, conn := wsDialPeek(t, srv, "virgo")
	defer conn.Close()

	payload, err := readWSTextFrame(br)
	if err != nil {
		t.Fatalf("read WS frame: %v", err)
	}
	if payload != "session not active" {
		t.Errorf("payload = %q, want %q", payload, "session not active")
	}
}

func TestWsPeek_SuccessfulStreamActive(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	item, _ := c.Add("myrepo", "Peek work", "", 1)
	c.GetReady("myrepo")
	c.Assign(item.ID, "virgo", "implement")
	c.Close()

	orig := defaultCapturer
	t.Cleanup(func() { defaultCapturer = orig })
	defaultCapturer = mockCapturer{hasSession: true, content: "live pane output"}

	mux := newDashboardMux(tempCfg(t), db)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	br, conn := wsDialPeek(t, srv, "virgo")
	defer conn.Close()

	payload, err := readWSTextFrame(br)
	if err != nil {
		t.Fatalf("read WS frame: %v", err)
	}
	if !strings.Contains(payload, "live pane output") {
		t.Errorf("payload = %q, want it to contain %q", payload, "live pane output")
	}
}

func TestWsPeek_InBandAuth_RejectsNoAuth(t *testing.T) {
	mux := newDashboardMux(tempCfgWithAPIKey(t, "test-secret"), tempDB(t))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	fmt.Fprintf(conn, "GET /ws/aqueducts/virgo/peek HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", key)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}

	binaryFrame := maskedTextFrame([]byte("garbage"))
	binaryFrame[0] = 0x82
	if _, err := conn.Write(binaryFrame); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	header := make([]byte, 2)
	_, err = io.ReadFull(br, header)
	if err != nil {
		t.Fatalf("expected close frame from server, got error: %v", err)
	}
	if header[0]&0x0F != wsOpcodeClose {
		t.Errorf("expected close opcode 0x8, got 0x%x", header[0]&0x0F)
	}
}

func TestWsPeek_InBandAuth_RejectsBadCredentials(t *testing.T) {
	mux := newDashboardMux(tempCfgWithAPIKey(t, "test-secret"), tempDB(t))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	fmt.Fprintf(conn, "GET /ws/aqueducts/virgo/peek HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", key)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}

	authMsg := `{"type":"auth","token":"wrong-key"}`
	maskedFrame := maskedTextFrame([]byte(authMsg))
	if _, err := conn.Write(maskedFrame); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	header := make([]byte, 2)
	_, err = io.ReadFull(br, header)
	if err != nil {
		t.Fatalf("expected close frame from server, got error: %v", err)
	}
	if header[0]&0x0F != wsOpcodeClose {
		t.Errorf("expected close opcode 0x8, got 0x%x", header[0]&0x0F)
	}
}

func TestWsPeek_InBandAuth_AcceptsValidToken(t *testing.T) {
	mux := newDashboardMux(tempCfgWithAPIKey(t, "test-secret"), tempDB(t))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	fmt.Fprintf(conn, "GET /ws/aqueducts/virgo/peek HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", key)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}

	authMsg := `{"type":"auth","token":"test-secret"}`
	maskedFrame := maskedTextFrame([]byte(authMsg))
	if _, err := conn.Write(maskedFrame); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	payload, err := readWSTextFrame(br)
	if err != nil {
		t.Fatalf("expected data frame after auth, got error: %v", err)
	}
	if payload != "session not active" {
		t.Errorf("payload = %q, want %q", payload, "session not active")
	}
}

func TestWsPeek_NoAuthRequired(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	br, conn := wsDialPeek(t, srv, "virgo")
	defer conn.Close()

	payload, err := readWSTextFrame(br)
	if err != nil {
		t.Fatalf("read WS frame: %v", err)
	}
	if payload != "session not active" {
		t.Errorf("payload = %q, want %q", payload, "session not active")
	}
}

func maskedTextFrame(payload []byte) []byte {
	n := len(payload)
	var frame []byte
	frame = append(frame, 0x81)
	var maskKey [4]byte
	switch {
	case n < 126:
		frame = append(frame, 0x80|byte(n))
	case n < 65536:
		frame = append(frame, 0x80|0x7E)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		frame = append(frame, ext[:]...)
	default:
		frame = append(frame, 0x80|0x7F)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		frame = append(frame, ext[:]...)
	}
	frame = append(frame, maskKey[:]...)
	frame = append(frame, payload...)
	return frame
}

func TestWsPeek_ReaderGoroutine_ExitsOnConnClose(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	br := bufio.NewReader(server)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		buf := make([]byte, wsMaxClientPayload)
		for {
			opcode, _, nb, err := wsReadClientFrame(br, buf)
			buf = nb
			if err != nil {
				return
			}
			if opcode == wsOpcodeClose {
				return
			}
		}
	}()

	client.Close()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("peek reader goroutine did not exit after connection close")
	}
	select {
	case <-ctx.Done():
	default:
		t.Error("cancel() was not called by peek reader goroutine on connection close")
	}
}

func TestWsPeek_ReaderGoroutine_ExitsOnReadDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	br := bufio.NewReader(server)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		buf := make([]byte, wsMaxClientPayload)
		for {
			server.SetReadDeadline(time.Now().Add(50 * time.Millisecond)) //nolint:errcheck
			opcode, _, nb, err := wsReadClientFrame(br, buf)
			buf = nb
			if err != nil {
				return
			}
			server.SetReadDeadline(time.Now().Add(50 * time.Millisecond)) //nolint:errcheck
			if opcode == wsOpcodeClose {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("peek reader goroutine did not exit after read deadline (network partition case)")
	}
	select {
	case <-ctx.Done():
	default:
		t.Error("cancel() was not called by peek reader goroutine on read deadline")
	}
	runtime.KeepAlive(client)
}

func TestWsReadClientFrame_PayloadSizeLimit(t *testing.T) {
	buildFrame := func(extLen uint16) []byte {
		var frame []byte
		frame = append(frame, 0x81)
		frame = append(frame, 0x80|0x7E)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], extLen)
		frame = append(frame, ext[:]...)
		frame = append(frame, 0, 0, 0, 0)
		frame = append(frame, make([]byte, extLen)...)
		return frame
	}

	t.Run("rejects_payload_exceeding_max", func(t *testing.T) {
		frame := buildFrame(5000)
		br := bufio.NewReader(bytes.NewReader(frame))
		_, _, _, err := wsReadClientFrame(br, make([]byte, 128))
		if err == nil {
			t.Fatal("expected error for payload > wsMaxClientPayload, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds max") {
			t.Errorf("error = %q, want it to mention 'exceeds max'", err)
		}
	})

	t.Run("accepts_payload_at_max", func(t *testing.T) {
		frame := buildFrame(4096)
		br := bufio.NewReader(bytes.NewReader(frame))
		_, payload, _, err := wsReadClientFrame(br, make([]byte, 128))
		if err != nil {
			t.Fatalf("unexpected error for payload == wsMaxClientPayload: %v", err)
		}
		if len(payload) != 4096 {
			t.Errorf("payload length = %d, want 4096", len(payload))
		}
	})
}

func TestWsReadClientFrame_RejectsUnmaskedFrame(t *testing.T) {
	frame := []byte{0x81, 0x05, 'h', 'e', 'l', 'l', 'o'}
	br := bufio.NewReader(bytes.NewReader(frame))
	_, _, _, err := wsReadClientFrame(br, make([]byte, 128))
	if err == nil {
		t.Fatal("expected error for unmasked client frame, got nil")
	}
	if !strings.Contains(err.Error(), "unmasked") {
		t.Errorf("error = %q, want it to mention 'unmasked'", err)
	}
}

func readWSTextFrame(br *bufio.Reader) (string, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(br, header); err != nil {
		return "", err
	}
	if header[0] != 0x81 {
		return "", fmt.Errorf("unexpected frame byte[0]: 0x%02x, want 0x81", header[0])
	}
	rawLen := int(header[1] & 0x7F)
	var length int
	switch rawLen {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(br, ext); err != nil {
			return "", err
		}
		length = int(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(br, ext); err != nil {
			return "", err
		}
		length = int(binary.BigEndian.Uint64(ext))
	default:
		length = rawLen
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(br, payload); err != nil {
		return "", err
	}
	return string(payload), nil
}

// --- TestDashboardWebMux_EventsSSE_AdaptiveBackoff ---

func TestDashboardWebMux_EventsSSE_AdaptiveBackoff_PollCountDropsWhenIdle(t *testing.T) {
	var callCount int32
	idleFetcher := func(cfg, db string) (*DashboardData, error) {
		atomic.AddInt32(&callCount, 1)
		return &DashboardData{FlowingCount: 0, FetchedAt: time.Now()}, nil
	}

	mux := newDashboardMuxWith(tempCfg(t), tempDB(t), idleFetcher, 50*time.Millisecond, 250*time.Millisecond)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	const window = 600 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), window)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/dashboard/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil && ctx.Err() == nil {
		t.Fatalf("GET /api/dashboard/events: %v", err)
	}
	if resp != nil {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
	}

	n := int(atomic.LoadInt32(&callCount))
	maxFastPolls := int(window/(50*time.Millisecond)) + 1
	halfMax := maxFastPolls / 2
	if n >= halfMax {
		t.Errorf("SSE fetch count = %d, want < %d (SSE adaptive backoff not working)", n, halfMax)
	}
}