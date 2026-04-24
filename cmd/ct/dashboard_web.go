package main

import (
	"bufio"
	"context"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/MichielDean/cistern/internal/aqueduct"
	"github.com/MichielDean/cistern/internal/castellarius"
	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/MichielDean/cistern/internal/skills"
	"github.com/MichielDean/cistern/internal/tracker"
)

// wsWriteTimeout is the per-frame write deadline set on the hijacked net.Conn
// before each wsSendText call. Without this, a client that disappears via a
// network partition (no TCP FIN) causes the goroutine to block indefinitely
// inside bufio.Writer.Flush.
const wsWriteTimeout = 10 * time.Second

// wsReadTimeout is the read deadline applied in WS handler frame-reader
// goroutines. It is reset after each received frame to keep active sessions
// alive. Without a deadline, a network partition leaks goroutines — neither
// gets an error, and cancel() is never called. Five minutes allows long
// idle-but-connected sessions while still reaping silently-partitioned ones.
const wsReadTimeout = 5 * time.Minute

// wsMaxClientPayload is the maximum payload size accepted from a client frame.
// Client→server frames carry only resize JSON (~40 bytes) or close frames,
// so 4 KiB is generous. This prevents a malicious client from triggering
// unbounded memory allocation via a forged frame length header.
const wsMaxClientPayload = 4096

// Field length limits for input validation.
const (
	maxTitleLen       = 256
	maxRepoLen        = 128
	maxDescriptionLen = 4096
	maxContentLen     = 65536
	maxDependsOnLen   = 128
	maxNotesLen       = 65536
	maxReasonLen      = 65536
	maxPerPage        = 500
	maxImportKeyLen   = 128
)

// apiMaxBodyBytes is the maximum request body size accepted by API endpoints.
// Prevents unbounded memory consumption from large payloads.
const apiMaxBodyBytes = 1 << 20 // 1 MiB

// maxDropletSSEConnections limits the number of simultaneous SSE connections
// for droplet detail streams, preventing resource exhaustion independent of
// log viewer connections.
const maxDropletSSEConnections = 64

// maxLogSSEConnections limits the number of simultaneous SSE connections
// for log viewer streams. Separate from droplet connections so log viewers
// cannot starve dashboard detail views.
const maxLogSSEConnections = 32

// currentDropletSSEConnections tracks the number of active droplet SSE connections.
var currentDropletSSEConnections int64

// currentLogSSEConnections tracks the number of active log SSE connections.
var currentLogSSEConnections int64


// WebSocket opcodes (RFC 6455 §5.2).
const (
	wsOpcodeText   = 0x1
	wsOpcodeBinary = 0x2
	wsOpcodeClose  = 0x8
)

// aqueductSessionInfo holds the tmux session name and droplet context for an
// active aqueduct worker.
type aqueductSessionInfo struct {
	sessionID string
	dropletID string
	title     string
	elapsed   time.Duration
}

// lookupAqueductSession returns session info for the named aqueduct worker, or
// false if the worker is not currently flowing.
func lookupAqueductSession(dbPath, name string) (aqueductSessionInfo, bool) {
	c, err := cistern.New(dbPath, "")
	if err != nil {
		return aqueductSessionInfo{}, false
	}
	defer c.Close()

	items, err := c.List("", "in_progress")
	if err != nil {
		return aqueductSessionInfo{}, false
	}
	for _, item := range items {
		if item.Assignee == name {
			return aqueductSessionInfo{
				sessionID: item.Repo + "-" + name,
				dropletID: item.ID,
				title:     item.Title,
				elapsed:   time.Since(item.UpdatedAt),
			}, true
		}
	}
	return aqueductSessionInfo{}, false
}

// parsePeekLines reads the optional ?lines= query parameter, falling back to
// defaultPeekLines.
func parsePeekLines(r *http.Request) int {
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultPeekLines
}

// isValidAqueductName validates that an aqueduct name contains only
// alphanumeric characters, hyphens, and underscores — characters safe for
// use in tmux session names. Rejects names containing tmux metacharacters
// (colon, dot, shell operators) that could enable tmux injection.
func isValidAqueductName(name string) bool {
	if name == "" {
		return false
	}
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// isValidTrackerKey validates that a tracker issue key contains only
// alphanumeric characters, hyphens, and underscores — the format expected
// by issue trackers (e.g., "PROJ-123"). Rejects keys containing slashes,
// dots, or other characters that could enable path traversal when the key
// is concatenated into a URL path segment.
func isValidTrackerKey(key string) bool {
	if key == "" {
		return false
	}
	for _, ch := range key {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// wsAcceptKey computes Sec-WebSocket-Accept per RFC 6455 §4.2.2.
func wsAcceptKey(clientKey string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(clientKey + magic))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// wsSendText writes a WebSocket text frame to the buffered writer and flushes.
// The server never masks frames (RFC 6455 §5.1).
func wsSendText(w *bufio.Writer, data string) error {
	return wsSendFrame(w, wsOpcodeText, []byte(data))
}

// wsSendBinary writes a WebSocket binary frame.
// Use for raw PTY output which may contain non-UTF-8 bytes — text frames
// with invalid UTF-8 cause browsers to close the connection immediately.
func wsSendBinary(w *bufio.Writer, data []byte) error {
	return wsSendFrame(w, wsOpcodeBinary, data)
}

// wsClosePayload builds a WebSocket close frame payload (status code + reason).
func wsClosePayload(code int, reason string) []byte {
	buf := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(buf[:2], uint16(code))
	copy(buf[2:], reason)
	return buf
}

// wsSendFrame writes a single unfragmented WebSocket frame (FIN=1) and flushes.
func wsSendFrame(w *bufio.Writer, opcode byte, payload []byte) error {
	n := len(payload)
	var header [10]byte
	header[0] = 0x80 | opcode // FIN + opcode
	var hLen int
	switch {
	case n < 126:
		header[1] = byte(n)
		hLen = 2
	case n < 65536:
		header[1] = 0x7E
		binary.BigEndian.PutUint16(header[2:4], uint16(n))
		hLen = 4
	default:
		header[1] = 0x7F
		binary.BigEndian.PutUint64(header[2:10], uint64(n))
		hLen = 10
	}
	if _, err := w.Write(header[:hLen]); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return w.Flush()
}



// wsReadClientFrame reads one WebSocket frame from a client (potentially masked).
// It returns the opcode, payload, and any read error. buf is reused across calls
// to avoid per-frame allocation; if the payload exceeds len(buf), a new slice is
// allocated and returned as the buf going forward.
func wsReadClientFrame(br *bufio.Reader, buf []byte) (opcode byte, payload []byte, newBuf []byte, err error) {
	var header [2]byte
	if _, err = io.ReadFull(br, header[:]); err != nil {
		return 0, nil, buf, err
	}
	opcode = header[0] & 0x0F
	masked := header[1]&0x80 != 0
	rawLen := int(header[1] & 0x7F)

	// RFC 6455 §5.1: clients MUST mask all frames to the server.
	if !masked {
		return 0, nil, buf, fmt.Errorf("unmasked client frame (RFC 6455 §5.1)")
	}

	var payloadLen int
	switch rawLen {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(br, ext[:]); err != nil {
			return 0, nil, buf, err
		}
		payloadLen = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(br, ext[:]); err != nil {
			return 0, nil, buf, err
		}
		extLen := binary.BigEndian.Uint64(ext[:])
		// Guard before int conversion: a value > wsMaxClientPayload but < math.MaxInt
		// would pass the int-typed check below, so reject it here first.
		if extLen > uint64(wsMaxClientPayload) {
			return 0, nil, buf, fmt.Errorf("client frame payload %d exceeds max %d", extLen, wsMaxClientPayload)
		}
		payloadLen = int(extLen)
	default:
		payloadLen = rawLen
	}

	if payloadLen > wsMaxClientPayload {
		return 0, nil, buf, fmt.Errorf("client frame payload %d exceeds max %d", payloadLen, wsMaxClientPayload)
	}

	var mask [4]byte
	if _, err = io.ReadFull(br, mask[:]); err != nil {
		return 0, nil, buf, err
	}

	if payloadLen > len(buf) {
		buf = make([]byte, payloadLen)
	}
	if _, err = io.ReadFull(br, buf[:payloadLen]); err != nil {
		return 0, nil, buf, err
	}
	for i := range buf[:payloadLen] {
		buf[i] ^= mask[i%4]
	}
	return opcode, buf[:payloadLen], buf, nil
}

// isAllowedWSOrigin returns true for localhost and private-network (RFC 1918)
// addresses. The dashboard is a local tool — LAN access is expected.
func isAllowedWSOrigin(host string) bool {
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "lobsterdog.local" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// wsUpgrade performs the RFC 6455 handshake. On success it returns the hijacked
// connection and its buffered read-writer. On failure it writes an HTTP error
// and returns a non-nil error.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	// Validate Origin header to prevent cross-origin WebSocket hijacking.
	// Browsers allow JS on any origin to connect to localhost WS endpoints, so
	// the localhost binding alone is not sufficient protection.
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil {
			http.Error(w, "invalid Origin header", http.StatusForbidden)
			return nil, nil, fmt.Errorf("invalid Origin header: %w", err)
		}
		h := u.Hostname()
		if !isAllowedWSOrigin(h) {
			http.Error(w, "cross-origin WebSocket request rejected", http.StatusForbidden)
			return nil, nil, fmt.Errorf("cross-origin WebSocket rejected: %s", origin)
		}
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return nil, nil, fmt.Errorf("not a websocket request")
	}
	key := r.Header.Get("Sec-Websocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, nil, fmt.Errorf("missing Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return nil, nil, fmt.Errorf("hijacking not supported")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAcceptKey(key) + "\r\n" +
		"\r\n"
	if _, err := brw.WriteString(resp); err != nil {
		conn.Close()
		return nil, nil, err
	}
	if err := brw.Flush(); err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, brw, nil
}

// newDashboardMux returns an http.Handler for the web dashboard.
func newDashboardMux(cfgPath, dbPath string) http.Handler {
	return newDashboardMuxInternal(cfgPath, dbPath)
}

// newDashboardMuxWith returns an http.Handler for the web dashboard with custom
// fetcher and refresh intervals. Exposed for testing.
func newDashboardMuxWith(cfgPath, dbPath string, fetcher func(cfg, db string) (*DashboardData, error), fastInterval, slowInterval time.Duration) http.Handler {
	return newDashboardMuxInternalWith(cfgPath, dbPath, fetcher, fastInterval, slowInterval)
}

// makeDashboardEventsHandler returns an http.HandlerFunc for the SSE dashboard events
// endpoint. Parameterised so tests can inject a custom fetcher and intervals.
func makeDashboardEventsHandler(cfgPath, dbPath string, fetcher func(string, string) (*DashboardData, error), fastInterval, slowInterval time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		sendEvent := func(d *DashboardData) {
			if b, err := json.Marshal(d); err == nil {
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
			}
		}

		// Initial send — establishes the hash baseline for adaptive rate.
		data, _ := fetcher(cfgPath, dbPath)
		sendEvent(data)
		lastHash := dashboardStateHash(data)

		ticker := time.NewTicker(fastInterval)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				data, _ = fetcher(cfgPath, dbPath)
				newHash := dashboardStateHash(data)
				sendEvent(data)
				// Adaptive backoff: slow down when Castellarius is idle.
				idle := newHash == lastHash && data.FlowingCount == 0
				lastHash = newHash
				next := fastInterval
				if idle {
					next = slowInterval
				}
				ticker.Reset(next)
			}
		}
	}
}

// newDashboardMuxInternal returns an http.Handler for the web dashboard.
func newDashboardMuxInternal(cfgPath, dbPath string) http.Handler {
	return newDashboardMuxInternalWith(cfgPath, dbPath, fetchDashboardData, refreshInterval, idleRefreshInterval)
}

// newDashboardMuxInternalWith returns an http.Handler for the web dashboard with custom
// fetcher and refresh intervals. Exposed for testing.
func newDashboardMuxInternalWith(cfgPath, dbPath string, fetcher func(cfg, db string) (*DashboardData, error), fastInterval, slowInterval time.Duration) http.Handler {
	// Read dashboard config fresh at server start so a cistern.yaml edit
	// followed by restarting ct dashboard --web takes effect without recompiling.
	// This is the supported update path: edit cistern.yaml, restart the server.
	var cfg *aqueduct.AqueductConfig
	if parsedCfg, err := aqueduct.ParseAqueductConfig(cfgPath); err == nil {
		cfg = parsedCfg
	}

	// Resolve API key: env var takes precedence over config file.
	apiKey := os.Getenv("CISTERN_DASHBOARD_API_KEY")
	if apiKey == "" && cfg != nil {
		apiKey = cfg.DashboardAPIKey
	}
	if apiKey == "" {
		log.Println("warning: no dashboard_api_key configured; all endpoints are unauthenticated")
	}

	// Build allowed origins list for CORS.
	var allowedOrigins []string
	if cfg != nil {
		allowedOrigins = cfg.DashboardAllowedOrigins
	}
	if len(allowedOrigins) == 0 {
		allowedOrigins = defaultAllowedOrigins()
	}

	mux := http.NewServeMux()

	spa := newSPAHandler(apiKey)
	mux.Handle("/app/", spa)
	mux.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusMovedPermanently)
	})

	// Root path redirects to the SPA dashboard.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/app/", http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, _ := fetcher(cfgPath, dbPath)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data) //nolint:errcheck
	})

	mux.HandleFunc("/api/dashboard/events", makeDashboardEventsHandler(cfgPath, dbPath, fetcher, fastInterval, slowInterval))

	// GET /api/aqueducts/{name}/peek — snapshot of current tmux pane output.
	mux.HandleFunc("/api/aqueducts/{name}/peek", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := r.PathValue("name")
		if !isValidAqueductName(name) {
			http.Error(w, "invalid aqueduct name", http.StatusBadRequest)
			return
		}
		lines := parsePeekLines(r)
		sess, ok := lookupAqueductSession(dbPath, name)
		capturer := defaultCapturer
		if !ok || !capturer.HasSession(sess.sessionID) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprint(w, "session not active")
			return
		}
		content, err := capturer.Capture(sess.sessionID, lines)
		if err != nil {
			http.Error(w, fmt.Sprintf("capture error: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, stripANSI(content))
	})

	// WS /ws/aqueducts/{name}/peek — live streaming peek (poll every 500ms, send diffs).
	// Auth is handled in-band: the client sends {"type":"auth","token":"..."} as
	// the first WebSocket message after upgrade. The server validates before
	// starting the stream. When no API key is configured, auth is skipped.
	mux.HandleFunc("/ws/aqueducts/{name}/peek", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !isValidAqueductName(name) {
			http.Error(w, "invalid aqueduct name", http.StatusBadRequest)
			return
		}
		lines := parsePeekLines(r)

		conn, brw, err := wsUpgrade(w, r)
		if err != nil {
			return // wsUpgrade already wrote the HTTP error
		}
		defer conn.Close()

		// In-band WebSocket auth: if an API key is configured, read the first
		// text frame and expect {"type":"auth","token":"<key>"}. Close with
		// 4001 if auth fails; proceed immediately if no key is configured.
		if apiKey != "" {
			conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
			opcode, payload, _, readErr := wsReadClientFrame(brw.Reader, make([]byte, wsMaxClientPayload))
			if readErr != nil {
				closePayload := wsClosePayload(4001, "auth required")
				wsSendFrame(brw.Writer, wsOpcodeClose, closePayload) //nolint:errcheck
				return
			}
			if opcode != wsOpcodeText {
				closePayload := wsClosePayload(4001, "auth required")
				wsSendFrame(brw.Writer, wsOpcodeClose, closePayload) //nolint:errcheck
				return
			}
			var msg struct {
				Type  string `json:"type"`
				Token string `json:"token"`
			}
			if json.Unmarshal(payload, &msg) != nil || msg.Type != "auth" ||
				subtle.ConstantTimeCompare([]byte(msg.Token), []byte(apiKey)) != 1 {
				closePayload := wsClosePayload(4001, "invalid credentials")
				wsSendFrame(brw.Writer, wsOpcodeClose, closePayload) //nolint:errcheck
				return
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Reader goroutine: detects client close frames and network partitions.
		// Sets wsReadTimeout on the connection so a silently-partitioned client
		// (no TCP FIN, no frames) is reaped after 5 minutes. Without this, when
		// tmux output is stable (no diffs) the ticker loop never writes and never
		// sets a write deadline — the goroutine and TCP connection leak indefinitely.
		go func() {
			defer cancel()
			buf := make([]byte, wsMaxClientPayload)
			conn.SetReadDeadline(time.Now().Add(wsReadTimeout)) //nolint:errcheck
			for {
				opcode, _, nb, err := wsReadClientFrame(brw.Reader, buf)
				buf = nb
				if err != nil {
					return
				}
				conn.SetReadDeadline(time.Now().Add(wsReadTimeout)) //nolint:errcheck
				if opcode == wsOpcodeClose {
					return
				}
			}
		}()

		var prev string
		capturer := defaultCapturer
		ticker := time.NewTicker(peekInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				next := "session not active"
				if sess, ok := lookupAqueductSession(dbPath, name); ok && capturer.HasSession(sess.sessionID) {
					content, err := capturer.Capture(sess.sessionID, lines)
					if err != nil {
						continue
					}
					next = stripANSI(content)
				}
				if diff := computeDiff(prev, next); diff != "" {
					conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)) //nolint:errcheck
					if wsSendText(brw.Writer, diff) != nil {
						return
					}
					prev = next
				}
			}
		}
	})

	// ── REST API endpoints ──

	// apiMux is a sub-mux for the /api/ tree with CORS headers on every response.
	apiMux := http.NewServeMux()

	// Droplet CRUD
	apiMux.HandleFunc("GET /api/droplets", handleGetDroplets(dbPath))
	apiMux.HandleFunc("GET /api/droplets/search", handleSearchDroplets(dbPath))
	apiMux.HandleFunc("GET /api/droplets/export", handleExportDroplets(dbPath))
	apiMux.HandleFunc("POST /api/droplets", handleCreateDroplet(dbPath))
	apiMux.HandleFunc("GET /api/droplets/{id}", handleGetDroplet(dbPath))
	apiMux.HandleFunc("PATCH /api/droplets/{id}", handleEditDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/rename", handleRenameDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/purge", handlePurgeDroplets(dbPath))

	// Droplet state transitions
	apiMux.HandleFunc("POST /api/droplets/{id}/pass", handlePassDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/recirculate", handleRecirculateDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/pool", handlePoolDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/close", handleCloseDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/reopen", handleReopenDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/cancel", handleCancelDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/restart", handleRestartDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/approve", handleApproveDroplet(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/heartbeat", handleHeartbeatDroplet(dbPath))

	// Notes
	apiMux.HandleFunc("GET /api/droplets/{id}/notes", handleGetNotes(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/notes", handleAddNote(dbPath))

	// Issues
	apiMux.HandleFunc("GET /api/droplets/{id}/issues", handleListIssues(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/issues", handleAddIssue(dbPath))
	apiMux.HandleFunc("POST /api/issues/{id}/resolve", handleResolveIssue(dbPath))
	apiMux.HandleFunc("POST /api/issues/{id}/reject", handleRejectIssue(dbPath))

	// Dependencies
	apiMux.HandleFunc("GET /api/droplets/{id}/dependencies", handleGetDependencies(dbPath))
	apiMux.HandleFunc("POST /api/droplets/{id}/dependencies", handleAddDependency(dbPath))
	apiMux.HandleFunc("DELETE /api/droplets/{id}/dependencies/{dep_id}", handleRemoveDependency(dbPath))

	// History/Log
	apiMux.HandleFunc("GET /api/droplets/{id}/log", handleDropletLog(dbPath))
	apiMux.HandleFunc("GET /api/droplets/{id}/changes", handleDropletChanges(dbPath))

	// Stats
	apiMux.HandleFunc("GET /api/stats", handleGetStats(dbPath))

	// Castellarius
	apiMux.HandleFunc("GET /api/castellarius/status", handleCastellariusStatus(cfgPath, dbPath))
	apiMux.HandleFunc("POST /api/castellarius/start", handleCastellariusStart())
	apiMux.HandleFunc("POST /api/castellarius/stop", handleCastellariusStop())
	apiMux.HandleFunc("POST /api/castellarius/restart", handleCastellariusRestart())

	// Doctor
	apiMux.HandleFunc("GET /api/doctor", handleDoctor(cfgPath))

	// Repos & Skills
	apiMux.HandleFunc("GET /api/repos", handleGetRepos(cfgPath))
	apiMux.HandleFunc("GET /api/repos/{name}/steps", handleGetRepoSteps(cfgPath))
	apiMux.HandleFunc("GET /api/skills", handleGetSkills())

	// Logs
	apiMux.HandleFunc("GET /api/logs", handleGetLogs(cfgPath))
	apiMux.HandleFunc("GET /api/logs/events", handleLogEvents(cfgPath))
	apiMux.HandleFunc("GET /api/logs/sources", handleGetLogSources(cfgPath))

	// Filter/refine sessions — rate-limited (expensive outbound LLM calls)
	outboundLimiter := newOutboundRateLimiter()
	apiMux.HandleFunc("POST /api/filter/new", outboundLimiter.wrap(handleFilterNew(cfgPath, dbPath)))
	apiMux.HandleFunc("POST /api/filter/{session_id}/resume", outboundLimiter.wrap(handleFilterResume(cfgPath, dbPath)))
	apiMux.HandleFunc("GET /api/filter/sessions", handleFilterSessions(dbPath))
	apiMux.HandleFunc("GET /api/filter/{session_id}", handleFilterSession(dbPath))

	// Import — rate-limited (expensive outbound tracker HTTP calls)
	apiMux.HandleFunc("POST /api/import", outboundLimiter.wrap(handleImport(cfgPath, dbPath)))
	apiMux.HandleFunc("GET /api/import/preview", outboundLimiter.wrap(handleImportPreview(cfgPath)))

	// SSE for droplet detail
	apiMux.HandleFunc("GET /api/droplets/{id}/events", handleDropletEvents(cfgPath, dbPath))

	// Wrap the API mux with CORS and body-size middleware.
	// Auth is applied to the entire mux below so that pre-existing endpoints
	// (/api/dashboard, etc.) are also protected.
	var apiHandler http.Handler = apiMux
	apiHandler = corsMiddleware(apiHandler, allowedOrigins)
	apiHandler = apiBodyLimitMiddleware(apiHandler)
	mux.Handle("/api/", apiHandler)

	var handler http.Handler = mux
	if apiKey != "" {
		handler = apiAuthMiddleware(handler, apiKey)
	}

	return handler
}

// ── API handler helpers ──

// csvSanitizeCell prevents CSV formula injection by prefixing cells that
// start with dangerous characters (=, +, -, @, tab, carriage return) with
// a single-quote prefix, which Excel and Sheets interpret as a literal string.
func csvSanitizeCell(s string) string {
	if len(s) == 0 {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// apiClient opens a cistern.Client for the given dbPath and calls f.
// It writes the error response if the client cannot be opened or f returns an error.
// "Not found" errors from cistern.Client are mapped to 404; all others to 500.
// Internal error details are sanitized before being sent to the client.
func apiClient(dbPath string, w http.ResponseWriter, f func(*cistern.Client) error) {
	c, err := cistern.New(dbPath, "")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer c.Close()
	if err := f(c); err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, err.Error())
		} else {
			writeAPIError(w, http.StatusInternalServerError, "internal error")
		}
	}
}

// isNotFoundError returns true if the error is a "not found" error from cistern.Client.
func isNotFoundError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, " not found")
}

// writeAPIJSON marshals v to JSON and writes it with the given status code.
func writeAPIJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeAPIError writes a JSON error response.
// For 500 Internal Server Error responses, the message is sanitized to
// "internal error" to avoid leaking database paths, SQL statements, or
// other internal details. Other status codes pass the message through verbatim.
func writeAPIError(w http.ResponseWriter, code int, msg string) {
	if code == http.StatusInternalServerError {
		msg = "internal error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

// defaultAllowedOrigins returns the default CORS origin list used when
// dashboard_allowed_origins is not configured. Accepts localhost variants
// and the loopback address on the default dashboard port (5737).
func defaultAllowedOrigins() []string {
	return []string{
		"http://localhost:5737",
		"http://127.0.0.1:5737",
		"http://[::1]:5737",
		"http://lobsterdog.local:5737",
	}
}

// apiAuthMiddleware rejects requests that lack a valid Bearer token when
// an API key is configured. When no key is configured, all requests pass through.
// Uses constant-time comparison to prevent timing side-channel attacks.
// CORS preflight (OPTIONS) requests are always allowed through, since browsers
// send them without Authorization headers and the CORS middleware must respond.
// SPA static assets (/app/) are exempted so the login page can render without auth.
// SSE and WebSocket connections cannot set custom headers, so they may pass the
// token as a "token" query parameter instead of the Authorization header.
func apiAuthMiddleware(next http.Handler, apiKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt SPA static routes so the login page can load without auth.
		// Include exact "/app" path (no trailing slash) so the redirect to /app/ works without auth.
		if r.URL.Path == "/app" || strings.HasPrefix(r.URL.Path, "/app/") {
			next.ServeHTTP(w, r)
			return
		}
		// Exempt WS peek endpoints — authentication is handled in-band via the
		// first WebSocket message after upgrade, not via URL query parameters.
		// This prevents auth tokens from leaking into server access logs and
		// browser history. Only exempt /ws/aqueducts/{name}/peek, not all
		// /ws/aqueducts/ paths (defense-in-depth: other WS routes may need auth).
		if strings.HasPrefix(r.URL.Path, "/ws/aqueducts/") && strings.HasSuffix(r.URL.Path, "/peek") {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		token := ""
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		} else {
			token = r.URL.Query().Get("token")
		}
		if token == "" {
			writeAPIError(w, http.StatusUnauthorized, "authorization required")
			return
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
			writeAPIError(w, http.StatusUnauthorized, "invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// apiBodyLimitMiddleware wraps requests with an http.MaxBytesReader to prevent
// unbounded memory consumption from large request bodies.
func apiBodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, apiMaxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

type outboundRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*outboundBucket
	limit   int
	window  time.Duration
}

type outboundBucket struct {
	count   int
	resetAt time.Time
}

func newOutboundRateLimiter() *outboundRateLimiter {
	return &outboundRateLimiter{
		buckets: make(map[string]*outboundBucket),
		limit:   10,
		window:  time.Minute,
	}
}

func (l *outboundRateLimiter) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if !l.allow(ip) {
			w.Header().Set("Retry-After", strconv.Itoa(int(l.window.Seconds())))
			writeAPIError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (l *outboundRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for k, b := range l.buckets {
		if now.After(b.resetAt) {
			delete(l.buckets, k)
		}
	}
	b, ok := l.buckets[ip]
	if !ok {
		b = &outboundBucket{count: 0, resetAt: now.Add(l.window)}
		l.buckets[ip] = b
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

// corsMiddleware wraps an http.Handler with CORS headers.
// Only origins in allowedOrigins are permitted; others receive no
// Access-Control-Allow-Origin header, which browsers interpret as a rejection.
// Preflight OPTIONS requests are handled directly.
func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		matched := ""
		for _, ao := range allowedOrigins {
			if strings.EqualFold(origin, ao) {
				matched = ao
				break
			}
		}
		if matched != "" {
			w.Header().Set("Access-Control-Allow-Origin", matched)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// decodeJSON decodes a JSON request body into dst. Returns true on success.
// Writes 400 and returns false on failure. Error messages are sanitized to
// avoid leaking Go internal type information.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		msg := "invalid JSON"
		if err == io.EOF {
			msg = "request body is empty"
		} else if !strings.HasPrefix(err.Error(), "invalid character") && !strings.HasPrefix(err.Error(), "json:") {
			msg = "invalid JSON: " + err.Error()
		}
		writeAPIError(w, http.StatusBadRequest, msg)
		return false
	}
	return true
}

// decodeJSONOptional decodes a JSON request body into dst. Returns true on success
// or if the body is empty (EOF). Writes 400 and returns false on malformed JSON.
func decodeJSONOptional(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(r.Body).Decode(dst)
	if err != nil {
		if err == io.EOF {
			return true
		}
		msg := "invalid JSON"
		if !strings.HasPrefix(err.Error(), "invalid character") && !strings.HasPrefix(err.Error(), "json:") {
			msg = "invalid JSON: " + err.Error()
		}
		writeAPIError(w, http.StatusBadRequest, msg)
		return false
	}
	return true
}

// ── Droplet CRUD handlers ──

func sortDroplets(items []*cistern.Droplet, sort string) {
	switch sort {
	case "created_at":
		slices.SortFunc(items, func(a, b *cistern.Droplet) int { return a.CreatedAt.Compare(b.CreatedAt) })
	case "updated_at":
		slices.SortFunc(items, func(a, b *cistern.Droplet) int { return a.UpdatedAt.Compare(b.UpdatedAt) })
	case "title":
		slices.SortFunc(items, func(a, b *cistern.Droplet) int { return cmp.Compare(a.Title, b.Title) })
	default:
		slices.SortFunc(items, func(a, b *cistern.Droplet) int {
			if c := cmp.Compare(a.Priority, b.Priority); c != 0 {
				return c
			}
			return a.CreatedAt.Compare(b.CreatedAt)
		})
	}
}

func handleGetDroplets(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo := r.URL.Query().Get("repo")
		status := r.URL.Query().Get("status")
		sort := r.URL.Query().Get("sort")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		if page < 1 {
			page = 1
		}
		if perPage < 1 {
			perPage = 50
		}
		if perPage > maxPerPage {
			perPage = maxPerPage
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			items, err := c.List(repo, status)
			if err != nil {
				return err
			}
			if items == nil {
				items = []*cistern.Droplet{}
			}
			sortDroplets(items, sort)
			total := len(items)
			start := (page - 1) * perPage
			if start > total {
				start = total
			}
			end := start + perPage
			if end > total {
				end = total
			}
			writeAPIJSON(w, http.StatusOK, map[string]any{
				"droplets": items[start:end],
				"total":    total,
				"page":     page,
				"per_page": perPage,
			})
			return nil
		})
	}
}

func handleSearchDroplets(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		status := r.URL.Query().Get("status")
		priority, _ := strconv.Atoi(r.URL.Query().Get("priority"))
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		if page < 1 {
			page = 1
		}
		if perPage < 1 {
			perPage = 50
		}
		if perPage > maxPerPage {
			perPage = maxPerPage
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			items, err := c.Search(query, status, priority)
			if err != nil {
				return err
			}
			if items == nil {
				items = []*cistern.Droplet{}
			}
			total := len(items)
			start := (page - 1) * perPage
			if start > total {
				start = total
			}
			end := start + perPage
			if end > total {
				end = total
			}
			writeAPIJSON(w, http.StatusOK, map[string]any{
				"droplets": items[start:end],
				"total":    total,
				"page":     page,
				"per_page": perPage,
			})
			return nil
		})
	}
}

func handleGetDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			d, err := c.Get(id)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, d)
			return nil
		})
	}
}

type createDropletRequest struct {
	Repo        string   `json:"repo"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    int      `json:"priority"`
	DependsOn   []string `json:"depends_on"`
}

func handleCreateDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createDropletRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Title == "" {
			writeAPIError(w, http.StatusBadRequest, "title is required")
			return
		}
		if len(req.Title) > maxTitleLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("title exceeds maximum length (%d)", maxTitleLen))
			return
		}
		if req.Repo == "" {
			writeAPIError(w, http.StatusBadRequest, "repo is required")
			return
		}
		if len(req.Repo) > maxRepoLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("repo exceeds maximum length (%d)", maxRepoLen))
			return
		}
		if len(req.Description) > maxDescriptionLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("description exceeds maximum length (%d)", maxDescriptionLen))
			return
		}
		if req.Priority < 1 {
			req.Priority = 2
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			d, err := c.Add(req.Repo, req.Title, req.Description, req.Priority, req.DependsOn...)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusCreated, d)
			return nil
		})
	}
}

type editDropletRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
}

func handleEditDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req editDropletRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Title != nil && len(*req.Title) > maxTitleLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("title exceeds maximum length (%d)", maxTitleLen))
			return
		}
		if req.Description != nil && len(*req.Description) > maxDescriptionLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("description exceeds maximum length (%d)", maxDescriptionLen))
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.EditDroplet(id, cistern.EditDropletFields{
				Title:       req.Title,
				Description: req.Description,
				Priority:    req.Priority,
			}); err != nil {
				return err
			}
			d, err := c.Get(id)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, d)
			return nil
		})
	}
}

type renameDropletRequest struct {
	Title string `json:"title"`
}

func handleRenameDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req renameDropletRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Title == "" {
			writeAPIError(w, http.StatusBadRequest, "title is required")
			return
		}
		if len(req.Title) > maxTitleLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("title exceeds maximum length (%d)", maxTitleLen))
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.EditDroplet(id, cistern.EditDropletFields{Title: &req.Title}); err != nil {
				return err
			}
			d, err := c.Get(id)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, d)
			return nil
		})
	}
}

func handleExportDroplets(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "json"
		}
		repo := r.URL.Query().Get("repo")
		query := r.URL.Query().Get("query")
		status := r.URL.Query().Get("status")
		priority, _ := strconv.Atoi(r.URL.Query().Get("priority"))
		apiClient(dbPath, w, func(c *cistern.Client) error {
			var items []*cistern.Droplet
			var err error
			if repo != "" && query == "" && status == "" && priority == 0 {
				items, err = c.List(repo, "")
			} else if repo != "" {
				items, err = c.Search(query, status, priority)
				if err != nil {
					return err
				}
				filtered := make([]*cistern.Droplet, 0, len(items))
				for _, d := range items {
					if strings.EqualFold(d.Repo, repo) {
						filtered = append(filtered, d)
					}
				}
				items = filtered
			} else {
				items, err = c.Search(query, status, priority)
			}
			if err != nil {
				return err
			}
			if items == nil {
				items = []*cistern.Droplet{}
			}
			switch format {
			case "csv":
				w.Header().Set("Content-Type", "text/csv")
				cw := csv.NewWriter(w)
				if err := cw.Write([]string{"id", "repo", "title", "description", "priority", "status", "assignee", "current_cataractae", "outcome", "created_at", "updated_at"}); err != nil {
					return err
				}
				for _, item := range items {
					if err := cw.Write([]string{
						csvSanitizeCell(item.ID), csvSanitizeCell(item.Repo), csvSanitizeCell(item.Title), csvSanitizeCell(item.Description),
						strconv.Itoa(item.Priority),
						csvSanitizeCell(item.Status), csvSanitizeCell(item.Assignee), csvSanitizeCell(item.CurrentCataractae), csvSanitizeCell(item.Outcome),
						item.CreatedAt.Format(time.RFC3339), item.UpdatedAt.Format(time.RFC3339),
					}); err != nil {
						return err
					}
				}
				cw.Flush()
				return cw.Error()
			default:
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(items) //nolint:errcheck
				return nil
			}
		})
	}
}

type purgeRequest struct {
	OlderThan string `json:"older_than"`
	DryRun    bool   `json:"dry_run"`
}

func handlePurgeDroplets(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req purgeRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		d, err := parseDuration(req.OlderThan)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid older_than: "+err.Error())
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			n, err := c.Purge(d, req.DryRun)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]any{"purged": n, "dry_run": req.DryRun})
			return nil
		})
	}
}

// ── Droplet state transition handlers ──

type signalRequest struct {
	Notes string `json:"notes"`
	To    string `json:"to"`
}

func handlePassDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req signalRequest
		if !decodeJSONOptional(w, r, &req) {
			return
		}
		if len(req.Notes) > maxNotesLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("notes exceeds maximum length (%d)", maxNotesLen))
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if req.Notes != "" {
				if err := c.AddNote(id, "manual", req.Notes); err != nil {
					return err
				}
			}
			if err := c.Pass(id, "manual", req.Notes); err != nil {
				return err
			}
			notifyCastellarius()
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "outcome": "pass"})
			return nil
		})
	}
}

func handleRecirculateDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req signalRequest
		if !decodeJSONOptional(w, r, &req) {
			return
		}
		if len(req.Notes) > maxNotesLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("notes exceeds maximum length (%d)", maxNotesLen))
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if req.Notes != "" {
				if err := c.AddNote(id, "manual", "♻ "+req.Notes); err != nil {
					return err
				}
			}
			if err := c.Recirculate(id, "manual", req.To, req.Notes); err != nil {
				return err
			}
			outcome := "recirculate"
			if req.To != "" {
				outcome = "recirculate:" + req.To
			}
			notifyCastellarius()
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "outcome": outcome})
			return nil
		})
	}
}

func handlePoolDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req signalRequest
		if !decodeJSONOptional(w, r, &req) {
			return
		}
		if len(req.Notes) > maxNotesLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("notes exceeds maximum length (%d)", maxNotesLen))
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if req.Notes != "" {
				if err := c.AddNote(id, "manual", req.Notes); err != nil {
					return err
				}
			}
			if err := c.Pool(id, req.Notes); err != nil {
				return err
			}
			notifyCastellarius()
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "status": "pooled"})
			return nil
		})
	}
}

func handleCloseDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.CloseItem(id); err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "status": "delivered"})
			return nil
		})
	}
}

func handleReopenDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.UpdateStatus(id, "open"); err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "status": "open"})
			return nil
		})
	}
}

type cancelRequest struct {
	Reason string `json:"reason"`
}

func handleCancelDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req cancelRequest
		if !decodeJSONOptional(w, r, &req) {
			return
		}
		if len(req.Reason) > maxReasonLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("reason exceeds maximum length (%d)", maxReasonLen))
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.Cancel(id, req.Reason); err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "status": "cancelled"})
			return nil
		})
	}
}

type restartRequest struct {
	Cataractae string `json:"cataractae"`
}

func handleRestartDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req restartRequest
		if !decodeJSONOptional(w, r, &req) {
			return
		}
		if req.Cataractae == "" {
			req.Cataractae = "implement"
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			d, err := c.Restart(id, req.Cataractae)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, d)
			return nil
		})
	}
}

func handleApproveDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.Approve(id, "manual"); err != nil {
				if strings.Contains(err.Error(), "not awaiting human approval") {
					writeAPIError(w, http.StatusBadRequest, "droplet is not awaiting human approval")
					return nil
				}
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "status": "approved"})
			return nil
		})
	}
}

func handleHeartbeatDroplet(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.Heartbeat(id); err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "heartbeat": "recorded"})
			return nil
		})
	}
}

// ── Notes handlers ──

func handleGetNotes(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			notes, err := c.GetNotes(id)
			if err != nil {
				return err
			}
			if notes == nil {
				notes = []cistern.CataractaeNote{}
			}
			writeAPIJSON(w, http.StatusOK, notes)
			return nil
		})
	}
}

type addNoteRequest struct {
	Cataractae string `json:"cataractae"`
	Content    string `json:"content"`
}

func handleAddNote(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req addNoteRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Content == "" {
			writeAPIError(w, http.StatusBadRequest, "content is required")
			return
		}
		if len(req.Content) > maxContentLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("content exceeds maximum length (%d)", maxContentLen))
			return
		}
		name := req.Cataractae
		if name == "" {
			name = "manual"
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if _, err := c.Get(id); err != nil {
				return err
			}
			if err := c.AddNote(id, name, req.Content); err != nil {
				return err
			}
			notes, err := c.GetNotes(id)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusCreated, notes)
			return nil
		})
	}
}

// ── Issues handlers ──

func handleListIssues(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		openOnly := r.URL.Query().Get("open") == "true"
		flaggedBy := r.URL.Query().Get("flagged_by")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			issues, err := c.ListIssues(id, openOnly, flaggedBy)
			if err != nil {
				return err
			}
			if issues == nil {
				issues = []cistern.DropletIssue{}
			}
			writeAPIJSON(w, http.StatusOK, issues)
			return nil
		})
	}
}

type addIssueRequest struct {
	FlaggedBy   string `json:"flagged_by"`
	Description string `json:"description"`
}

func handleAddIssue(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req addIssueRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Description == "" {
			writeAPIError(w, http.StatusBadRequest, "description is required")
			return
		}
		if len(req.Description) > maxContentLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("description exceeds maximum length (%d)", maxContentLen))
			return
		}
		flaggedBy := req.FlaggedBy
		if flaggedBy == "" {
			flaggedBy = "manual"
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if _, err := c.Get(id); err != nil {
				return err
			}
			iss, err := c.AddIssue(id, flaggedBy, req.Description)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusCreated, iss)
			return nil
		})
	}
}

type evidenceRequest struct {
	Evidence string `json:"evidence"`
}

func handleResolveIssue(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req evidenceRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.ResolveIssue(id, req.Evidence); err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "status": "resolved"})
			return nil
		})
	}
}

func handleRejectIssue(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req evidenceRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.RejectIssue(id, req.Evidence); err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]string{"id": id, "status": "unresolved"})
			return nil
		})
	}
}

// ── Dependencies handlers ──

func handleGetDependencies(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			deps, err := c.GetDependencies(id)
			if err != nil {
				return err
			}
			undelivered, err := c.GetBlockedBy(id)
			if err != nil {
				return err
			}
			dependents, err := c.GetDependents(id)
			if err != nil {
				return err
			}
			result := make([]map[string]string, 0, len(deps)+len(dependents))
			for _, d := range deps {
				ty := "resolves"
				for _, u := range undelivered {
					if u == d {
						ty = "blocked_by"
						break
					}
				}
				result = append(result, map[string]string{"depends_on": d, "type": ty})
			}
			for _, d := range dependents {
				result = append(result, map[string]string{"depends_on": d, "type": "blocks"})
			}
			writeAPIJSON(w, http.StatusOK, result)
			return nil
		})
	}
}

type addDepRequest struct {
	DependsOn string `json:"depends_on"`
}

func handleAddDependency(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req addDepRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.DependsOn == "" {
			writeAPIError(w, http.StatusBadRequest, "depends_on is required")
			return
		}
		if len(req.DependsOn) > maxDependsOnLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("depends_on exceeds maximum length (%d)", maxDependsOnLen))
			return
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.AddDependency(id, req.DependsOn); err != nil {
				return err
			}
			deps, err := c.GetDependencies(id)
			if err != nil {
				return err
			}
			undelivered, err := c.GetBlockedBy(id)
			if err != nil {
				return err
			}
			dependents, err := c.GetDependents(id)
			if err != nil {
				return err
			}
			result := make([]map[string]string, 0, len(deps)+len(dependents))
			for _, d := range deps {
				ty := "resolves"
				for _, u := range undelivered {
					if u == d {
						ty = "blocked_by"
						break
					}
				}
				result = append(result, map[string]string{"depends_on": d, "type": ty})
			}
			for _, d := range dependents {
				result = append(result, map[string]string{"depends_on": d, "type": "blocks"})
			}
			writeAPIJSON(w, http.StatusCreated, result)
			return nil
		})
	}
}

func handleRemoveDependency(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		depID := r.PathValue("dep_id")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			if err := c.RemoveDependency(id, depID); err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, map[string]string{"removed": depID})
			return nil
		})
	}
}

// ── History/Log handlers ──

func handleDropletLog(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
				if limit > 1000 {
					limit = 1000
				}
			}
		}
		format := r.URL.Query().Get("format")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			switch format {
			case "notes":
				notes, err := c.GetNotes(id)
				if err != nil {
					return err
				}
				if notes == nil {
					notes = []cistern.CataractaeNote{}
				}
				writeAPIJSON(w, http.StatusOK, notes)
				return nil
			default:
				changes, err := c.GetDropletChanges(id, limit)
				if err != nil {
					return err
				}
				if changes == nil {
					changes = []cistern.DropletChange{}
				}
				writeAPIJSON(w, http.StatusOK, changes)
				return nil
			}
		})
	}
}

func handleDropletChanges(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		limit := 20
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
				if limit > 1000 {
					limit = 1000
				}
			}
		}
		apiClient(dbPath, w, func(c *cistern.Client) error {
			changes, err := c.GetDropletChanges(id, limit)
			if err != nil {
				return err
			}
			if changes == nil {
				changes = []cistern.DropletChange{}
			}
			writeAPIJSON(w, http.StatusOK, changes)
			return nil
		})
	}
}

// ── Stats handler ──

func handleGetStats(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiClient(dbPath, w, func(c *cistern.Client) error {
			stats, err := c.Stats()
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, stats)
			return nil
		})
	}
}

// ── Castellarius handlers ──

func handleCastellariusStatus(cfgPath, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type aqueductStatus struct {
			Name          string  `json:"name"`
			Status        string  `json:"status"`
			DropletID     *string `json:"droplet_id"`
			DropletTitle  *string `json:"droplet_title"`
			CurrentStep   *string `json:"current_step"`
			Elapsed       int64   `json:"elapsed"`
		}
		type castellariusStatusResp struct {
			Running             bool             `json:"running"`
			PID                 *int             `json:"pid"`
			UptimeSeconds       *float64         `json:"uptime_seconds"`
			Aqueducts           []aqueductStatus `json:"aqueducts"`
			CastellariusRunning bool             `json:"castellarius_running"`
		}

		data, _ := fetchDashboardData(cfgPath, dbPath)

		hf, _ := castellarius.ReadHealthFile(filepath.Dir(dbPath))
		running := hf != nil && hf.PID > 0
		var pid *int
		var uptime *float64
		if running {
			pid = &hf.PID
			if !hf.LastTickAt.IsZero() {
				elapsed := time.Since(hf.LastTickAt).Seconds()
				uptime = &elapsed
			}
		}

		aqueducts := make([]aqueductStatus, len(data.Cataractae))
		for i, cat := range data.Cataractae {
			aq := aqueductStatus{
				Name:    cat.Name,
				Elapsed: int64(cat.Elapsed),
			}
			if cat.DropletID != "" {
				aq.Status = "flowing"
				aq.DropletID = &cat.DropletID
				aq.DropletTitle = &cat.Title
				aq.CurrentStep = &cat.Step
			} else {
				aq.Status = "idle"
			}
			aqueducts[i] = aq
		}

		resp := castellariusStatusResp{
			Running:             running,
			PID:                 pid,
			UptimeSeconds:       uptime,
			Aqueducts:           aqueducts,
			CastellariusRunning: data.CastellariusRunning,
		}
		writeAPIJSON(w, http.StatusOK, resp)
	}
}

func handleCastellariusStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusNotImplemented, "castellarius start via API is not yet supported; use 'ct castellarius start'")
	}
}

func handleCastellariusStop() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusNotImplemented, "castellarius stop via API is not yet supported; use 'ct castellarius stop'")
	}
}

func handleCastellariusRestart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusNotImplemented, "castellarius restart via API is not yet supported; use 'ct castellarius restart'")
	}
}

// ── Doctor handler ──

func handleDoctor(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.URL.Query().Get("fix") // fix param accepted but not applied via API
		cfg, err := aqueduct.ParseAqueductConfig(cfgPath)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal error")
			return
		}
		type repoInfo struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		var repos []repoInfo
		for _, r := range cfg.Repos {
			repos = append(repos, repoInfo{Name: r.Name, URL: r.URL})
		}
		writeAPIJSON(w, http.StatusOK, map[string]any{
			"config_ok": true,
			"repos":     repos,
		})
	}
}

// ── Repos handler ──

func handleGetRepos(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := aqueduct.ParseAqueductConfig(cfgPath)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal error")
			return
		}
		type repoInfo struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		var repos []repoInfo
		for _, r := range cfg.Repos {
			repos = append(repos, repoInfo{Name: r.Name, URL: r.URL})
		}
		writeAPIJSON(w, http.StatusOK, repos)
	}
}

func handleGetRepoSteps(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoName := r.PathValue("name")
		cfg, err := aqueduct.ParseAqueductConfig(cfgPath)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal error")
			return
		}
		for _, repo := range cfg.Repos {
			if repo.Name == repoName {
				wfPath := repo.WorkflowPath
				if !filepath.IsAbs(wfPath) {
					wfPath = filepath.Join(filepath.Dir(cfgPath), wfPath)
				}
				wf, wfErr := aqueduct.ParseWorkflow(wfPath)
				if wfErr != nil || wf == nil {
					writeAPIJSON(w, http.StatusOK, []string{})
					return
				}
				steps := make([]string, len(wf.Cataractae))
				for i, step := range wf.Cataractae {
					steps[i] = step.Name
				}
				writeAPIJSON(w, http.StatusOK, steps)
				return
			}
		}
		writeAPIError(w, http.StatusNotFound, "repo not found")
	}
}

// ── Skills handler ──

func handleGetSkills() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := skills.ListInstalled()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeAPIJSON(w, http.StatusOK, entries)
	}
}

// ── Log handlers ──

const maxLogLines = 5000

const sseInitialTail int64 = 32 * 1024 // Send last ~32KB of existing log on connect

const maxScanTokenSize = 1024 * 1024 // 1MB — accommodate long log lines

// isValidLogSource checks that source is a safe, non-traversal log file name.
// Only alphanumeric, hyphens, and underscores are allowed. No dots, slashes,
// or other characters that could enable path traversal.
func isValidLogSource(source string) bool {
	if source == "" || source == "castellarius" {
		return true
	}
	if len(source) > 64 {
		return false
	}
	for _, ch := range source {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

func logFilePath(cfgPath, source string) (string, error) {
	if !isValidLogSource(source) {
		return "", fmt.Errorf("invalid log source name")
	}
	home, _ := os.UserHomeDir()
	if source == "" || source == "castellarius" {
		return filepath.Join(home, ".cistern", "castellarius.log"), nil
	}
	return filepath.Join(home, ".cistern", source+".log"), nil
}

// sanitizeSSEData escapes newlines in data for safe SSE transmission.
// Per the SSE spec, data fields must not contain raw newlines.
func sanitizeSSEData(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\\n")
}

// countLinesUpTo counts newlines in the file up to (but not including) the
// given byte offset. Used by the SSE handler to compute line numbers when
// starting mid-file.
func countLinesUpTo(path string, offset int64) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	var count int64
	buf := make([]byte, 32*1024)
	remaining := offset
	for remaining > 0 {
		chunk := int64(len(buf))
		if remaining < chunk {
			chunk = remaining
		}
		n, err := f.Read(buf[:chunk])
		if err != nil || n == 0 {
			break
		}
		for i := 0; i < n; i++ {
			if buf[i] == '\n' {
				count++
			}
		}
		remaining -= int64(n)
	}
	return count
}

// handleGetLogs returns the last N lines of the log file with absolute line numbers.
// Query params: ?lines=500&source=castellarius
// Returns: [{line: <int>, text: "<string>"}] with line numbers matching the file position.
func handleGetLogs(cfgPath string) http.HandlerFunc {
	type logLine struct {
		Line int64  `json:"line"`
		Text string `json:"text"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		linesStr := r.URL.Query().Get("lines")
		lines := 500
		if n, err := strconv.Atoi(linesStr); err == nil && n > 0 {
			lines = n
		}
		if lines > maxLogLines {
			lines = maxLogLines
		}

		source := r.URL.Query().Get("source")
		if source == "" {
			source = "castellarius"
		}

		path, err := logFilePath(cfgPath, source)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}

		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				writeAPIJSON(w, http.StatusOK, []logLine{})
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "internal error")
			return
		}
		defer f.Close()

		// Ring buffer approach: only keep the last `lines` entries in memory,
		// tracking absolute line numbers.
		type ringEntry struct {
			Line int64
			Text string
		}
		ring := make([]ringEntry, 0, lines)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, maxScanTokenSize), maxScanTokenSize)
		lineNum := int64(0)
		for scanner.Scan() {
			lineNum++
			entry := ringEntry{Line: lineNum, Text: scanner.Text()}
			if len(ring) < lines {
				ring = append(ring, entry)
			} else {
				ring[int((lineNum-1)%int64(lines))] = entry
			}
		}
		if err := scanner.Err(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal error")
			return
		}

		var result []logLine
		if lineNum <= int64(lines) {
			result = make([]logLine, lineNum)
			for i, e := range ring {
				result[i] = logLine{Line: e.Line, Text: e.Text}
			}
		} else {
			// Ring buffer: elements need to be reordered from oldest to newest.
			start := int(lineNum % int64(lines))
			result = make([]logLine, lines)
			for i := 0; i < lines; i++ {
				e := ring[(start+i)%lines]
				result[i] = logLine{Line: e.Line, Text: e.Text}
			}
		}
		writeAPIJSON(w, http.StatusOK, result)
	}
}

// maxLogSSERetries is the maximum number of consecutive poll cycles where the
// log file is missing before the SSE handler terminates the connection.
const maxLogSSERetries = 20 // 20 × 500ms = 10 seconds

// handleLogEvents streams new log lines via SSE.
func handleLogEvents(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&currentLogSSEConnections, 1) > maxLogSSEConnections {
			atomic.AddInt64(&currentLogSSEConnections, -1)
			writeAPIError(w, http.StatusServiceUnavailable, "too many log SSE connections")
			return
		}
		defer atomic.AddInt64(&currentLogSSEConnections, -1)

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeAPIError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}

		source := r.URL.Query().Get("source")
		if source == "" {
			source = "castellarius"
		}
		path, err := logFilePath(cfgPath, source)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher.Flush()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		var offset int64
		lineNum := int64(1)
		if info, err := os.Stat(path); err == nil {
			if info.Size() > sseInitialTail {
				offset = info.Size() - sseInitialTail
				lineNum = countLinesUpTo(path, offset) + 1
			}
		}

		missCount := 0
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				info, err := os.Stat(path)
				if err != nil {
					missCount++
					if missCount >= maxLogSSERetries {
						return
					}
					continue
				}
				missCount = 0
				if info.Size() < offset {
					offset = 0
					lineNum = 1
				} else if info.Size() == offset {
					continue
				}

				f, err := os.Open(path)
				if err != nil {
					continue
				}
				func() {
					defer f.Close()
					seekOffset, seekErr := f.Seek(offset, io.SeekStart)
					if seekErr != nil {
						return
					}
					_ = seekOffset
					scanner := bufio.NewScanner(f)
					scanner.Buffer(make([]byte, 0, maxScanTokenSize), maxScanTokenSize)
					var newOffset int64
					for scanner.Scan() {
						line := scanner.Text()
						type sseLine struct {
							Line int64  `json:"line"`
							Text string `json:"text"`
						}
						payload, _ := json.Marshal(sseLine{Line: lineNum, Text: line})
						_, err := fmt.Fprintf(w, "data: %s\n\n", payload)
						if err != nil {
							return
						}
						lineNum++
						newOffset, _ = f.Seek(0, io.SeekCurrent)
					}
					if err := scanner.Err(); err != nil {
						if newOffset > offset {
							offset = newOffset
						}
						flusher.Flush()
						return
					}
					if newOffset > offset {
						offset = newOffset
					} else {
						offset, _ = f.Seek(0, io.SeekCurrent)
					}
					flusher.Flush()
				}()
			}
		}
	}
}

// handleGetLogSources returns metadata about available log files.
func handleGetLogSources(cfgPath string) http.HandlerFunc {
	type logSourceInfo struct {
		Name         string `json:"name"`
		SizeBytes    int64  `json:"size_bytes"`
		LastModified string `json:"last_modified"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		home, _ := os.UserHomeDir()
		logDir := filepath.Join(home, ".cistern")
		var sources []logSourceInfo

		entries, err := os.ReadDir(logDir)
		if err != nil {
			writeAPIJSON(w, http.StatusOK, sources)
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".log")
			if !isValidLogSource(name) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			sources = append(sources, logSourceInfo{
				Name:         name,
				SizeBytes:    info.Size(),
				LastModified: info.ModTime().UTC().Format(time.RFC3339),
			})
		}
		writeAPIJSON(w, http.StatusOK, sources)
	}
}

// ── Droplet events SSE handler ──

func handleDropletEvents(cfgPath, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		// Limit concurrent SSE connections to prevent resource exhaustion.
		if atomic.AddInt64(&currentDropletSSEConnections, 1) > maxDropletSSEConnections {
			atomic.AddInt64(&currentDropletSSEConnections, -1)
			writeAPIError(w, http.StatusServiceUnavailable, "too many SSE connections")
			return
		}
		defer atomic.AddInt64(&currentDropletSSEConnections, -1)

		// Validate droplet existence and flusher support before setting SSE headers,
		// so error responses use clean Content-Type instead of text/event-stream.
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeAPIError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}

		c, err := cistern.New(dbPath, "")
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal error")
			return
		}
		defer c.Close()

		d, err := c.Get(id)
		if err != nil {
			if isNotFoundError(err) {
				writeAPIError(w, http.StatusNotFound, err.Error())
			} else {
				writeAPIError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}

		// All validation passed — now set SSE headers and begin streaming.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		b, _ := json.Marshal(d)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				d, _ := c.Get(id)
				if d != nil {
					b, _ := json.Marshal(d)
					fmt.Fprintf(w, "data: %s\n\n", b)
					flusher.Flush()
				}
			}
		}
	}
}

// RunDashboardWeb starts the HTTP web dashboard on addr and blocks until
// SIGINT/SIGTERM is received or the server fails.
func RunDashboardWeb(cfgPath, dbPath, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           newDashboardMuxInternal(cfgPath, dbPath),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      0, // SSE streams are long-lived
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "Cistern web dashboard listening on http://localhost%s\n", addr)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// ── Filter session handlers ──

type filterNewRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type filterResumeRequest struct {
	Message string `json:"message"`
}

func handleFilterNew(cfgPath, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req filterNewRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Title == "" {
			writeAPIError(w, http.StatusBadRequest, "title is required")
			return
		}
		if len(req.Title) > maxTitleLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("title exceeds maximum length (%d)", maxTitleLen))
			return
		}
		if len(req.Description) > maxDescriptionLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("description exceeds maximum length (%d)", maxDescriptionLen))
			return
		}

		apiClient(dbPath, w, func(c *cistern.Client) error {
			session, err := c.CreateFilterSession(req.Title, req.Description)
			if err != nil {
				return err
			}

			preset := resolveFilterPreset("")
			contextBlock := gatherFilterContext(filterContextConfig{
				DBPath: dbPath,
				Title:  req.Title,
				Desc:   req.Description,
			})

			userPrompt := "Title: " + req.Title
			if req.Description != "" {
				userPrompt += "\nDescription: " + req.Description
			}

			result, err := invokeFilterNew(preset, req.Title, req.Description, contextBlock)
			if err != nil {
				c.DeleteFilterSession(session.ID) //nolint:errcheck
				return err
			}

			var messages []cistern.FilterMessage
			messages = append(messages, cistern.FilterMessage{Role: "user", Content: userPrompt})
			messages = append(messages, cistern.FilterMessage{Role: "assistant", Content: result.Text})

			msgJSON, _ := json.Marshal(messages)
			if err := c.UpdateFilterSessionMessages(session.ID, string(msgJSON), result.Text, result.SessionID); err != nil {
				return err
			}

			session.Messages = string(msgJSON)
			session.SpecSnapshot = result.Text
			session.LLMSessionID = result.SessionID

			w.Header().Set("Content-Type", "application/json")
			writeAPIJSON(w, http.StatusCreated, map[string]any{
				"session_id":        session.ID,
				"llm_session_id":    result.SessionID,
				"session":           session,
				"assistant_message": result.Text,
			})
			return nil
		})
	}
}

func handleFilterResume(cfgPath, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("session_id")
		var req filterResumeRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Message == "" {
			writeAPIError(w, http.StatusBadRequest, "message is required")
			return
		}
		if len(req.Message) > maxNotesLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("message exceeds maximum length (%d)", maxNotesLen))
			return
		}

		apiClient(dbPath, w, func(c *cistern.Client) error {
			session, err := c.GetFilterSession(sessionID)
			if err != nil {
				return err
			}

			preset := resolveFilterPreset("")
			var existingMessages []cistern.FilterMessage
			if session.Messages != "" && session.Messages != "[]" {
				json.Unmarshal([]byte(session.Messages), &existingMessages) //nolint:errcheck
			}

			llmSessionID := session.LLMSessionID

			var result filterSessionResult
			if llmSessionID != "" {
				result, err = invokeFilterResume(preset, llmSessionID, req.Message)
				if err != nil {
					return err
				}
			} else {
				contextBlock := gatherFilterContext(filterContextConfig{
					DBPath: dbPath,
					Title:  session.Title,
					Desc:   session.Description,
				})
				fullPrompt := buildFilterPrompt(contextBlock, session.Title)
				if session.Description != "" {
					fullPrompt += "\nDescription: " + session.Description
				}
				for _, msg := range existingMessages {
					if msg.Role == "user" {
						fullPrompt += "\n\nUser: " + msg.Content
					} else if msg.Role == "assistant" {
						fullPrompt += "\n\nAssistant: " + msg.Content
					}
				}
				fullPrompt += "\n\nUser: " + req.Message
				result, err = callFilterAgent(preset, nil, fullPrompt)
				if err != nil {
					return err
				}
			}

			existingMessages = append(existingMessages, cistern.FilterMessage{Role: "user", Content: req.Message})
			existingMessages = append(existingMessages, cistern.FilterMessage{Role: "assistant", Content: result.Text})

			msgJSON, _ := json.Marshal(existingMessages)
			if err := c.UpdateFilterSessionMessages(sessionID, string(msgJSON), result.Text, result.SessionID); err != nil {
				return err
			}

			w.Header().Set("Content-Type", "application/json")
			writeAPIJSON(w, http.StatusOK, map[string]any{
				"session_id":        sessionID,
				"llm_session_id":    result.SessionID,
				"assistant_message": result.Text,
				"spec_snapshot":     result.Text,
			})
			return nil
		})
	}
}

func handleFilterSessions(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiClient(dbPath, w, func(c *cistern.Client) error {
			sessions, err := c.ListFilterSessions()
			if err != nil {
				return err
			}
			if sessions == nil {
				sessions = []cistern.FilterSession{}
			}
			writeAPIJSON(w, http.StatusOK, sessions)
			return nil
		})
	}
}

func handleFilterSession(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("session_id")
		apiClient(dbPath, w, func(c *cistern.Client) error {
			session, err := c.GetFilterSession(sessionID)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusOK, session)
			return nil
		})
	}
}

// ── Import handler ──

type importRequest struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
	Repo     string `json:"repo"`
	Priority int    `json:"priority"`
}

func handleImport(cfgPath, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req importRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Provider == "" {
			writeAPIError(w, http.StatusBadRequest, "provider is required")
			return
		}
		if req.Key == "" {
			writeAPIError(w, http.StatusBadRequest, "key is required")
			return
		}
		if len(req.Key) > maxImportKeyLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("key exceeds maximum length (%d)", maxImportKeyLen))
			return
		}
		if !isValidTrackerKey(req.Key) {
			writeAPIError(w, http.StatusBadRequest, "key contains invalid characters")
			return
		}
		if req.Repo == "" {
			writeAPIError(w, http.StatusBadRequest, "repo is required")
			return
		}

		repo, err := resolveCanonicalRepo(req.Repo)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "unknown repo")
			return
		}

		trackerCfg, err := loadTrackerConfig(req.Provider)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "tracker configuration error")
			return
		}

		constructor, ok := tracker.Resolve(req.Provider)
		if !ok {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("unknown tracker provider %q", req.Provider))
			return
		}
		tp, err := constructor(trackerCfg)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "tracker initialization error")
			return
		}

		issue, err := tp.FetchIssue(req.Key)
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, "failed to fetch issue")
			return
		}

		priority := issue.Priority
		if req.Priority > 0 {
			priority = req.Priority
		}

		externalRef := req.Provider + ":" + req.Key

		apiClient(dbPath, w, func(c *cistern.Client) error {
			d, err := c.AddDroplet(repo, issue.Title, issue.Description, externalRef, priority)
			if err != nil {
				return err
			}
			writeAPIJSON(w, http.StatusCreated, d)
			return nil
		})
	}
}

func handleImportPreview(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerName := r.URL.Query().Get("provider")
		key := r.URL.Query().Get("key")
		if providerName == "" {
			writeAPIError(w, http.StatusBadRequest, "provider is required")
			return
		}
		if key == "" {
			writeAPIError(w, http.StatusBadRequest, "key is required")
			return
		}
		if len(key) > maxImportKeyLen {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("key exceeds maximum length (%d)", maxImportKeyLen))
			return
		}
		if !isValidTrackerKey(key) {
			writeAPIError(w, http.StatusBadRequest, "key contains invalid characters")
			return
		}

		trackerCfg, err := loadTrackerConfig(providerName)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "tracker configuration error")
			return
		}

		constructor, ok := tracker.Resolve(providerName)
		if !ok {
			writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("unknown tracker provider %q", providerName))
			return
		}
		tp, err := constructor(trackerCfg)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "tracker initialization error")
			return
		}

		issue, err := tp.FetchIssue(key)
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, "failed to fetch issue")
			return
		}

		writeAPIJSON(w, http.StatusOK, map[string]any{
			"key":         issue.Key,
			"title":       issue.Title,
			"description": issue.Description,
			"priority":    issue.Priority,
			"labels":      issue.Labels,
			"source_url":  issue.SourceURL,
		})
	}
}

// end of dashboard_web.go
