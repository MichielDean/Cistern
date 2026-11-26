package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/MichielDean/cistern/internal/cistern"
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
	handler := makeDashboardEventsHandler(context.Background(), "cfg", "db", fetcher, 20*time.Millisecond, time.Hour)
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

// ── web-diag RSS threshold alerting (ci-ii5cz) ──

func TestParseByteSize(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{"megabytes", "512M", 512 << 20, false},
		{"gigabytes", "1G", 1 << 30, false},
		{"plain bytes", "536870912", 536870912, false},
		{"lowercase megabytes", "256m", 256 << 20, false},
		{"lowercase gigabytes", "2g", 2 << 30, false},
		{"empty string", "", 0, true},
		{"non-numeric", "abc", 0, true},
		{"zero", "0", 0, true},
		{"zero with unit", "0M", 0, true},
		{"negative", "-5", 0, true},
		{"unknown suffix", "10B", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseByteSize(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseByteSize(%q) = %d, want error", tc.in, got)
				}
				if !strings.HasPrefix(err.Error(), "ct: parseByteSize") {
					t.Errorf("error %q must start with \"ct: parseByteSize\"", err)
				}
				if got != 0 {
					t.Errorf("parseByteSize(%q) error case returned %d, want 0", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseByteSize(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("parseByteSize(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestRSSAlertTracker_EvaluateDefaultThreshold(t *testing.T) {
	tr := newRSSAlertTracker(defaultDiagRSSThreshold, func(string) error { return nil })
	if tr.threshold != 268435456 {
		t.Fatalf("defaultDiagRSSThreshold = %d, want 268435456 (256 MiB)", tr.threshold)
	}
	alert, _ := tr.evaluate(268435456)
	if !alert {
		t.Error("rss exactly at threshold must alert (crossing is >=)")
	}
}

func TestRSSAlertTracker_Crossing(t *testing.T) {
	const threshold = int64(100)
	cases := []struct {
		name     string
		rss      int64
		primeRSS int64
		primed   bool
		want     bool
	}{
		{"below threshold", 50, 0, false, false},
		{"above while already above (no re-alert)", 150, 150, true, false},
		{"crossing from below", 150, 50, true, true},
		{"at exact threshold from below", 100, 50, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newRSSAlertTracker(threshold, func(string) error { return nil })
			if tc.primed {
				tr.evaluate(tc.primeRSS)
			}
			got, _ := tr.evaluate(tc.rss)
			if got != tc.want {
				t.Errorf("evaluate(%d) alert = %v, want %v", tc.rss, got, tc.want)
			}
		})
	}
}

func TestResolveDiagRSSThreshold(t *testing.T) {
	defaultVal := int64(268435456)

	if v, err := resolveDiagRSSThreshold(func(string) string { return "" }); err != nil || v != defaultVal {
		t.Errorf("unset env = (%d, %v), want (%d, nil)", v, err, defaultVal)
	}
	if v, err := resolveDiagRSSThreshold(func(string) string { return " " }); err != nil || v != defaultVal {
		t.Errorf("whitespace env = (%d, %v), want (%d, nil)", v, err, defaultVal)
	}
	if v, err := resolveDiagRSSThreshold(func(string) string { return "128M" }); err != nil || v != 134217728 {
		t.Errorf("\"128M\" = (%d, %v), want (134217728, nil)", v, err)
	}
	for _, bad := range []string{"abc", "  abc ", "0"} {
		v, err := resolveDiagRSSThreshold(func(string) string { return bad })
		if err == nil {
			t.Errorf("invalid env %q: want error, got nil", bad)
		}
		if v != defaultVal {
			t.Errorf("invalid env %q: value = %d, want default %d (caller Warns and still constructs a working tracker)", bad, v, defaultVal)
		}
	}
}

func TestRunDiagTick_ThresholdCrossingPersistsEventsRow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	client, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	tracker := newRSSAlertTracker(1000, func(payload string) error {
		return client.RecordEvent(diagEventDropletID, cistern.EventMemWarn, payload)
	})

	rssSeq := []int64{500, 1000, 1500}
	for i, rss := range rssSeq {
		runDiagTick(tracker, func() dashboardMemStats {
			return dashboardMemStats{RSS: rss, HeapAlloc: 1, HeapSys: 1, Sys: 1, NumGC: 1, NumGoroutine: 1, FetchedAt: "x"}
		})
		client2, err := cistern.New(dbPath, "")
		if err != nil {
			t.Fatal(err)
		}
		events, err := client2.ListRecentEvents(100)
		client2.Close()
		if err != nil {
			t.Fatal(err)
		}
		var count int
		for _, e := range events {
			if e.Event == cistern.EventMemWarn && e.Droplet == diagEventDropletID {
				count++
			}
		}
		want := 0
		if i >= 1 {
			want = 1
		}
		if count != want {
			t.Fatalf("after tick %d (rss=%d): mem_warn rows = %d, want %d (edge-triggered)", i+1, rss, count, want)
		}
	}

	// Payload must exist and carry rss_bytes.
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var payload string
	if err := db.QueryRow(`SELECT payload FROM events WHERE event_type = ? LIMIT 1`, cistern.EventMemWarn).Scan(&payload); err != nil {
		t.Fatalf("read mem_warn payload: %v", err)
	}
	if !strings.Contains(payload, "rss_bytes") {
		t.Errorf("payload %q must contain rss_bytes", payload)
	}
}

func TestRunDiagTick_RecordEventFailureDoesNotChangeState(t *testing.T) {
	var calls int
	fail := true
	tracker := newRSSAlertTracker(100, func(payload string) error {
		calls++
		if fail {
			return fmt.Errorf("ct: test: record failure")
		}
		return nil
	})
	collect := func(rss int64) func() dashboardMemStats {
		return func() dashboardMemStats { return dashboardMemStats{RSS: rss} }
	}

	// First tick crosses with a failing recorder: warns, must not mute future alerts.
	runDiagTick(tracker, collect(150))
	if tracker.above {
		t.Error("a failed record must revert the crossing so the next tick can still alert")
	}

	// Second tick with a working recorder: crossing records successfully.
	fail = false
	runDiagTick(tracker, collect(150))
	if calls != 2 {
		t.Errorf("record calls = %d, want 2 (transient failure must not permanently mute alerting)", calls)
	}
	if !tracker.above {
		t.Error("after a successful crossing record the tracker must be above")
	}

	// Third tick stays above: still exactly one successful record (edge-trigger).
	runDiagTick(tracker, collect(160))
	if calls != 2 {
		t.Errorf("record calls = %d, want 2 (level-triggered logging would add a third)", calls)
	}
}

// TestDefaultDiagRSSThresholdMatchesMemoryMax pins the "half of MemoryMax"
// derivation: if either cistern-web.service MemoryMax or
// defaultDiagRSSThreshold changes independently, this test fails and forces
// the pair to be reconciled intentionally.
// ── diag sampler cgroup/child RSS (ci-y7pvo) ──

// The sampler must see the whole systemd cgroup: the Bubble Tea child spawned
// by /ws/tui counts against MemoryMax=512M in cistern-web.service but is
// invisible to self-RSS sampling. A fake proc reader provides pids+RSS for
// several processes, including this one.
func TestDiagMemStats_CapturesChildAndCgroupRSS(t *testing.T) {
	selfRSS := int64(55 << 20)
	childRSS := int64(21 << 20)
	procs := func() ([]procSample, error) {
		return []procSample{
			{pid: 1847, rss: selfRSS, self: true},
			{pid: 1875, rss: childRSS},
		}, nil
	}

	ms := collectDashboardMemStatsProcs(procs)

	if ms.CgroupRSS != selfRSS+childRSS {
		t.Errorf("cgroup_rss_bytes = %d, want %d (self + child summed)", ms.CgroupRSS, selfRSS+childRSS)
	}
	if ms.ChildRSS != childRSS {
		t.Errorf("child_rss_bytes = %d, want %d (non-self processes only)", ms.ChildRSS, childRSS)
	}
	// Existing self-RSS field stays self-only: byte-compatible consumers rely on it.
	if ms.RSS == 0 {
		t.Error("rss_bytes must remain populated")
	}
	if ms.FetchedAt == "" {
		t.Error("fetched_at must be set")
	}

	// Cgroup read failure falls back to self-only RSS with the existing field
	// set — no error path may kill sampling.
	ms = collectDashboardMemStatsProcs(func() ([]procSample, error) {
		return nil, fmt.Errorf("ct: test: cgroup unreadable")
	})
	if ms.CgroupRSS != 0 || ms.ChildRSS != 0 {
		t.Errorf("cgroup read failure: cgroup=%d child=%d, want 0/0", ms.CgroupRSS, ms.ChildRSS)
	}
	if ms.FetchedAt == "" || ms.NumGoroutine <= 0 {
		t.Error("cgroup read failure must leave the rest of the sample intact")
	}
}

// ── mem_warn pprof snapshot capture (ci-y7pvo) ──

// A threshold crossing must leave evidence that survives the restart:
// a heap-<timestamp>.pb + goroutine-<timestamp>.txt pair under the snapshot
// dir, and the event payload naming the directory.
func TestRSSAlertWritesPprofSnapshotOnThreshold(t *testing.T) {
	t.Run("crossing writes timestamped heap.pb and goroutine.txt and payload includes dir", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "web-pprof")
		tracker := newRSSAlertTrackerWithSnapshot(1000, func(string) error { return nil }, dir, captureDiagPprofSnapshot)

		runDiagTick(tracker, func() dashboardMemStats {
			return dashboardMemStats{RSS: 2000, HeapAlloc: 1, Sys: 1, NumGC: 1, NumGoroutine: 1, FetchedAt: "x"}
		})

		heaps, err := filepath.Glob(filepath.Join(dir, "heap-*.pb"))
		if err != nil || len(heaps) != 1 {
			t.Fatalf("glob heap-*.pb: %v, matches %v — want exactly one timestamped heap profile", err, heaps)
		}
		goroutines, err := filepath.Glob(filepath.Join(dir, "goroutine-*.txt"))
		if err != nil || len(goroutines) != 1 {
			t.Fatalf("glob goroutine-*.txt: %v, matches %v — want exactly one timestamped goroutine dump", err, goroutines)
		}

		// Payload carries the snapshot dir: wire record through a capture.
		var payload string
		tracker2 := newRSSAlertTrackerWithSnapshot(1000, func(p string) error { payload = p; return nil }, dir, captureDiagPprofSnapshot)
		runDiagTick(tracker2, func() dashboardMemStats { return dashboardMemStats{RSS: 2000, FetchedAt: "x"} })
		if !strings.Contains(payload, dir) {
			t.Errorf("payload %q must contain the pprof dir %q", payload, dir)
		}
	})

	t.Run("two crossings keep two snapshot sets — no overwriting earlier evidence", func(t *testing.T) {
		// Fixed filenames would replace the first crossing's snapshots when a
		// later crossing fires — destroying the before/after pprof diff the
		// root-cause analysis depends on.
		dir := filepath.Join(t.TempDir(), "web-pprof")
		tracker := newRSSAlertTrackerWithSnapshot(1000, func(string) error { return nil }, dir, captureDiagPprofSnapshot)
		sample := func() dashboardMemStats { return dashboardMemStats{RSS: 2000, FetchedAt: "x"} }
		runDiagTick(tracker, sample)                                                                          // crossing 1
		runDiagTick(tracker, func() dashboardMemStats { return dashboardMemStats{RSS: 100, FetchedAt: "x"} }) // back under
		runDiagTick(tracker, sample)                                                                          // crossing 2

		heaps, err := filepath.Glob(filepath.Join(dir, "heap-*.pb"))
		if err != nil {
			t.Fatal(err)
		}
		if len(heaps) != 2 {
			t.Errorf("heap snapshots after two crossings = %d (%v), want 2 for the before/after diff", len(heaps), heaps)
		}
		goroutines, err := filepath.Glob(filepath.Join(dir, "goroutine-*.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if len(goroutines) != 2 {
			t.Errorf("goroutine snapshots after two crossings = %d (%v), want 2", len(goroutines), goroutines)
		}
	})

	t.Run("missing dir logs a warn and does not kill the ticker", func(t *testing.T) {
		// A directory that cannot be created (procfs is non-writable).
		dir := "/proc/nonexistent/web-pprof"
		calls := 0
		tracker := newRSSAlertTrackerWithSnapshot(1000, func(string) error { calls++; return nil }, dir, captureDiagPprofSnapshot)

		runDiagTick(tracker, func() dashboardMemStats { return dashboardMemStats{RSS: 2000, FetchedAt: "x"} })
		runDiagTick(tracker, func() dashboardMemStats { return dashboardMemStats{RSS: 2000, FetchedAt: "x"} })

		if calls != 1 {
			t.Errorf("record calls = %d, want 1 (snapshot failure must not block or mute alerting)", calls)
		}
		if !tracker.above {
			t.Error("tracker must stay above after a successful record despite the snapshot failure")
		}
	})
}

func TestDefaultDiagRSSThresholdMatchesMemoryMax(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "cistern-web.service"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^MemoryMax=(\S+)\s*$`)
	m := re.FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("MemoryMax not found in cistern-web.service:\n%s", data)
	}
	memMax, err := parseByteSize(m[1])
	if err != nil {
		t.Fatalf("parse MemoryMax %q: %v", m[1], err)
	}
	if defaultDiagRSSThreshold != memMax/2 {
		t.Errorf("defaultDiagRSSThreshold = %d, want MemoryMax/2 = %d (half of %s from cistern-web.service)",
			defaultDiagRSSThreshold, memMax/2, m[1])
	}
}
