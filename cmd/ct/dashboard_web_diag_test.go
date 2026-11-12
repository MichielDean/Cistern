package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── sseConnectionLimiter ──

func TestSSEConnectionLimiter_AcquireRelease(t *testing.T) {
	l := newSSEConnectionLimiter(2)

	if !l.acquire() {
		t.Fatal("first acquire should succeed")
	}
	if !l.acquire() {
		t.Fatal("second acquire should succeed")
	}
	if l.acquire() {
		t.Error("third acquire at max should fail")
	}
	l.release()
	if !l.acquire() {
		t.Error("acquire after release should succeed")
	}
}

// Concurrent acquire/release must never exceed max concurrent holders.
func TestSSEConnectionLimiter_ConcurrentNeverExceedsMax(t *testing.T) {
	const max = 4
	const goroutines = 40
	l := newSSEConnectionLimiter(max)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if l.acquire() {
					cur := l.currentCount()
					if cur > max {
						t.Errorf("current %d exceededs max %d", cur, max)
					}
					l.release()
				}
			}
		}()
	}
	wg.Wait()
	if got := l.currentCount(); got != 0 {
		t.Errorf("final current = %d, want 0", got)
	}
}

// ── setSSEHeaders ──

func TestSetSSEHeaders_SetsExactlyFourHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	setSSEHeaders(w)

	checks := map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	}
	for k, want := range checks {
		if got := w.Header().Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
}

// ── DashboardTUI pending/lastFrame fixes ──

// TestDashboardTUI_FlushPendingFrameClonesAndResets verifies that
// flushPendingFrame commits an independent clone to lastFrame and resets
// pending, so later appends cannot mutate the committed snapshot.
// repaintMarkerStr is string(repaintMarker) — pending frames carry the
// marker as their leading bytes.
var repaintMarkerStr = string(repaintMarker)

func TestDashboardTUI_FlushPendingFrameClonesAndResets(t *testing.T) {
	d := newDashboardTUI("", "", "")
	// No Start(): no run loop needed; we poke frame state directly.

	d.mu.Lock()
	// Feed a repaint marker so a frame is open, then a chunk with no closing marker.
	d.frameAccumulate(repaintMarker)
	d.frameAccumulate([]byte("hello"))
	d.mu.Unlock()

	d.flushPendingFrame(d.flushGen)

	d.mu.Lock()
	got := string(d.lastFrame)
	pendingLen := len(d.pending)
	d.mu.Unlock()

	if got != repaintMarkerStr+"hello" {
		t.Errorf("lastFrame = %q, want %q", got, repaintMarkerStr+"hello")
	}
	if pendingLen != 0 {
		t.Errorf("len(pending) = %d, want 0", pendingLen)
	}

	// Appending to pending must NOT mutate lastFrame (no aliasing).
	d.mu.Lock()
	d.pending = append(d.pending, []byte("X")...)
	appended := string(d.lastFrame)
	d.mu.Unlock()
	if appended != repaintMarkerStr+"hello" {
		t.Errorf("lastFrame after pending append = %q, want %q (aliasing bug)", appended, repaintMarkerStr+"hello")
	}

	// A fresh repaint cycle commits its own frame as an independent snapshot.
	d.mu.Lock()
	d.frameAccumulate(repaintMarker)
	d.frameAccumulate([]byte("world"))
	d.mu.Unlock()
	d.flushPendingFrame(d.flushGen)
	d.mu.Lock()
	got2 := string(d.lastFrame)
	pendingLen2 := len(d.pending)
	d.mu.Unlock()
	if got2 != "\x1b[2J\x1b[Hworld" { // pending holds the marker as its first bytes
		t.Errorf("lastFrame after repaint-cycle flush = %q, want %q", got2, "world")
	}
	if pendingLen2 != 0 {
		t.Errorf("len(pending) after flush = %d, want 0", pendingLen2)
	}
}

// TestDashboardTUI_PendingBoundedWithoutMarker feeds marker-free output far
// beyond maxPendingFrameBytes and asserts pending never exceeds the cap and
// the frame state is reset (inFrame==false) once discarded.
func TestDashboardTUI_PendingBoundedWithoutMarker(t *testing.T) {
	d := newDashboardTUI("", "", "")
	// No Start(): no run loop needed; we poke frame state directly.

	// 10 MiB of marker-free chunks.
	chunk := bytes.Repeat([]byte("x"), 4096)
	total := 10 << 20
	for sent := 0; sent < total; sent += len(chunk) {
		d.broadcast(chunk)
	}

	d.mu.Lock()
	pendingLen := len(d.pending)
	inFrame := d.inFrame
	d.mu.Unlock()

	if pendingLen > maxPendingFrameBytes {
		t.Errorf("len(pending) = %d exceeds cap %d", pendingLen, maxPendingFrameBytes)
	}
	if inFrame {
		t.Error("inFrame should be false after cap discard")
	}
}

// ── dashboard events SSE cap + error frames ──

// TestDashboardEventsSSE_ConnectionCapRejects opens maxDashboardSSEConnections
// SSE streams, asserts the next is rejected with 503, then that closing one
// frees a slot.
func TestDashboardEventsSSE_ConnectionCapRejects(t *testing.T) {
	fetcherCalls := 0
	fetcher := func(cfg, db string) (*DashboardData, error) {
		fetcherCalls++
		return &DashboardData{}, nil
	}
	ts := httptest.NewServer(newDashboardMuxWith("unused-cfg", "unused-db", fetcher, time.Hour, time.Hour))
	defer ts.Close()

	clients := make([]*http.Client, 0, maxDashboardSSEConnections)
	resps := make([]*http.Response, 0, maxDashboardSSEConnections)
	defer func() {
		for _, r := range resps {
			r.Body.Close()
		}
	}()

	for i := 0; i < maxDashboardSSEConnections; i++ {
		c := &http.Client{Timeout: 10 * time.Second}
		resp, err := c.Get(ts.URL + "/api/dashboard/events")
		if err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("connection %d: status = %d, want 200", i, resp.StatusCode)
		}
		clients = append(clients, c)
		resps = append(resps, resp)
	}

	// 33rd connection must be rejected.
	resp, err := http.Get(ts.URL + "/api/dashboard/events")
	if err != nil {
		t.Fatalf("over-cap request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("over-cap status = %d, want 503", resp.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)
	if !strings.Contains(body, "too many dashboard SSE connections") {
		t.Errorf("over-cap body = %q, want it to mention cap", body)
	}

	// Close one connection; the next must succeed.
	resps[0].Body.Close()
	<-time.After(100 * time.Millisecond)
	resp2, err := http.Get(ts.URL + "/api/dashboard/events")
	if err != nil {
		t.Fatalf("post-release request: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("post-release status = %d, want 200", resp2.StatusCode)
	}
	resp2.Body.Close()
}

// TestDashboardEventsSSE_FetcherErrorSendsErrorEvent injects a fetcher that
// fails after the initial tick; the stream must carry `event: error` and
// never `data: null`.
func TestDashboardEventsSSE_FetcherErrorSendsErrorEvent(t *testing.T) {
	calls := 0
	var mu sync.Mutex
	fetcher := func(cfg, db string) (*DashboardData, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			return &DashboardData{FlowingCount: 3}, nil
		}
		// Newlines must be escaped: a raw newline would break SSE framing by
		// starting multiple data lines/frames.
		return nil, fmt.Errorf("db unavailable\nsecond line")
	}
	handler := makeDashboardEventsHandler("cfg", "db", fetcher, 20*time.Millisecond, time.Hour)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Read a bounded amount of the stream.
	buf := make([]byte, 4096)
	var collected []byte
	deadline := time.After(3 * time.Second)
	for len(collected) < 512 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for frames; got %q", collected)
		default:
		}
		n, err := resp.Body.Read(buf)
		collected = append(collected, buf[:n]...)
		if err != nil {
			break
		}
	}
	body := string(collected)
	if !strings.Contains(body, "event: error") {
		t.Errorf("stream does not contain 'event: error'; got %q", body)
	}
	if strings.Contains(body, "data: null") {
		t.Errorf("stream contains 'data: null'; got %q", body)
	}
	// The embedded newline must be escaped so the error stays a single SSE
	// data line — a raw second line would break SSE framing.
	if strings.Contains(body, "\ndata: second line") {
		t.Errorf("error newline not escaped: second line emitted as its own data line; got %q", body)
	}
}

// ── debug endpoints ──

func TestDebugMemstatsEndpoint(t *testing.T) {
	mux := newDashboardMuxWith("unused-cfg", "unused-db", func(cfg, db string) (*DashboardData, error) {
		return &DashboardData{}, nil
	}, time.Hour, time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/debug/memstats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var ms struct {
		RSS       int64  `json:"rss_bytes"`
		HeapAlloc uint64 `json:"heap_alloc_bytes"`
		HeapSys   uint64 `json:"heap_sys_bytes"`
		Sys       uint64 `json:"sys_bytes"`
		NumGC     uint32 `json:"num_gc"`
		NumGoro   int    `json:"num_goroutine"`
		FetchedAt string `json:"fetched_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ms); err != nil {
		t.Fatalf("decode memstats JSON: %v — body: %s", err, w.Body.String())
	}
	if ms.FetchedAt == "" {
		t.Error("fetched_at must be set")
	}
	if _, err := time.Parse(time.RFC3339, ms.FetchedAt); err != nil {
		t.Errorf("fetched_at is not RFC3339: %v (%q)", err, ms.FetchedAt)
	}
	if ms.HeapAlloc == 0 {
		t.Error("heap_alloc_bytes should be non-zero in a live process")
	}
	if ms.NumGoro <= 0 {
		t.Errorf("num_goroutine = %d, want > 0", ms.NumGoro)
	}
}

func TestDebugGoroutineDump(t *testing.T) {
	mux := newDashboardMuxWith("unused-cfg", "unused-db", func(cfg, db string) (*DashboardData, error) {
		return &DashboardData{}, nil
	}, time.Hour, time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil) // debug=1 → plain text
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "goroutine") {
		t.Errorf("goroutine profile body should contain 'goroutine'; got %q", truncateStr(body, 120))
	}
}

func TestDebugHeapProfile(t *testing.T) {
	mux := newDashboardMuxWith("unused-cfg", "unused-db", func(cfg, db string) (*DashboardData, error) {
		return &DashboardData{}, nil
	}, time.Hour, time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	// pprof profiles are gzipped protobufs; a magic-byte sniff is enough to
	// confirm we did not get an HTML error page.
	if !bytes.HasPrefix(w.Body.Bytes(), []byte{0x1f, 0x8b}) {
		n := w.Body.Len()
		if n > 8 {
			n = 8
		}
		t.Errorf("heap profile should be gzip-prefixed pprof data, got %x", w.Body.Bytes()[:n])
	}
}
