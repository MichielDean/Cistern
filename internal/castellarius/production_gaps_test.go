package castellarius

// production_gaps_test.go — tests for failure modes that caused real incidents.
//
// These tests cover the interaction paths that were MISSING before 2026-03-25
// and whose absence allowed the self-kill bug, silent backoff, and liveness
// check race to go undetected.

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/aqueduct"
	"github.com/MichielDean/cistern/internal/cistern"
)

// --- Liveness check progress-monitoring tests ---

// TestLivenessCheck_StallDetected_WhenNoSignals verifies that the liveness check detects
// a stall and logs "stall detected" when all progress signals are absent
// (no notes, no worktree files, no session log).
func TestLivenessCheck_StallDetected_WhenNoSignals(t *testing.T) {
	// Mock tmux as alive so liveness check passes through to stall detector.
	orig := isTmuxAliveFn
	isTmuxAliveFn = func(_ string) bool { return true }
	t.Cleanup(func() { isTmuxAliveFn = orig })

	origMtime := sessionLogMtimeFn
	sessionLogMtimeFn = func(sessionID string) (time.Time, error) { return time.Time{}, nil }
	t.Cleanup(func() { sessionLogMtimeFn = origMtime })

	buf := &bytes.Buffer{}
	client := newMockClient()
	sched := newTestScheduler(buf, client)

	item := &cistern.Droplet{
		ID:                "stale-session",
		Repo:              "repo",
		Status:            "in_progress",
		Assignee:          "alpha",
		CurrentCataractae: "implement",
	}
	client.items["stale-session"] = item

	sched.livenessCheckRepo(context.Background(), aqueduct.RepoConfig{Name: "repo"})

	log := buf.String()
	if !strings.Contains(log, "stall detected") {
		t.Errorf("liveness check should log 'stall detected' when no signals present; log:\n%s", log)
	}
}

// TestLivenessCheck_NoStallNote_WhenRecentLogMtime verifies that the liveness check
// does not write a stall note when the agent's session log mtime is within the
// stall threshold.
func TestLivenessCheck_NoStallNote_WhenRecentLogMtime(t *testing.T) {
	// Mock tmux as alive so exit detection is bypassed and stall
	// detection runs on the session log mtime.
	orig := isTmuxAliveFn
	isTmuxAliveFn = func(_ string) bool { return true }
	t.Cleanup(func() { isTmuxAliveFn = orig })

	origMtime := sessionLogMtimeFn
	sessionLogMtimeFn = func(sessionID string) (time.Time, error) { return time.Now().Add(-5 * time.Second), nil }
	t.Cleanup(func() { sessionLogMtimeFn = origMtime })

	buf := &bytes.Buffer{}
	client := newMockClient()
	sched := newTestScheduler(buf, client)

	item := &cistern.Droplet{
		ID:                "fresh-dispatch",
		Repo:              "repo",
		Status:            "in_progress",
		Assignee:          "alpha",
		CurrentCataractae: "implement",
	}
	client.items["fresh-dispatch"] = item

	sched.livenessCheckRepo(context.Background(), aqueduct.RepoConfig{Name: "repo"})

	log := buf.String()
	if strings.Contains(log, "stall detected") {
		t.Errorf("liveness check flagged a recently-active droplet as stalled; log:\n%s", log)
	}
}

// TestLivenessCheck_SkipsItemsWithOutcome verifies that the liveness check never writes
// a stall note for a droplet that already has an outcome — the observe loop
// handles those and must not be interfered with.
func TestLivenessCheck_SkipsItemsWithOutcome(t *testing.T) {
	buf := &bytes.Buffer{}
	client := newMockClient()
	sched := newTestScheduler(buf, client)

	item := &cistern.Droplet{
		ID:                "has-outcome",
		Repo:              "repo",
		Status:            "in_progress",
		Assignee:          "alpha",
		CurrentCataractae: "implement",
		Outcome:           "pass",
	}
	client.items["has-outcome"] = item

	sched.livenessCheckRepo(context.Background(), aqueduct.RepoConfig{Name: "repo"})

	log := buf.String()
	if strings.Contains(log, "stall detected") {
		t.Errorf("liveness check flagged a droplet with an existing outcome; log:\n%s", log)
	}
}

// --- Dispatch error paths ---

// TestDispatch_GetReadyError_ReleasesWorker verifies that a DB error in GetReady
// releases the worker so subsequent ticks can still dispatch work.
func TestDispatch_GetReadyError_ReleasesWorker(t *testing.T) {
	buf := &bytes.Buffer{}
	client := newMockClient()
	client.getReadyErr = errors.New("db locked")
	sched := newTestScheduler(buf, client)

	sched.dispatchRepo(context.Background(), aqueduct.RepoConfig{Name: "repo"})

	log := buf.String()
	if !strings.Contains(log, "poll failed") {
		t.Errorf("expected 'poll failed' log; got:\n%s", log)
	}
	if !poolAllIdle(sched.pools["repo"]) {
		t.Error("worker not released after GetReady error — pool would deadlock on next tick")
	}
}

// TestDispatch_SpawnFailure_ResetsDropletAndReleasesWorker verifies that when
// Spawn() returns an error, the droplet is reset to open (not left stuck in_progress)
// and the worker is released.
func TestDispatch_SpawnFailure_ResetsDropletAndReleasesWorker(t *testing.T) {
	buf := &bytes.Buffer{}
	client := newMockClient()

	droplet := &cistern.Droplet{
		ID:                "spawn-fail",
		Repo:              "repo",
		CurrentCataractae: "implement",
	}
	client.readyItems = []*cistern.Droplet{droplet}
	client.items["spawn-fail"] = droplet

	var spawnCalled int64
	runner := &funcRunner{fn: func(_ context.Context, _ CataractaeRequest) error {
		atomic.AddInt64(&spawnCalled, 1)
		return errors.New("tmux server dead")
	}}
	sched := newTestSchedulerWithRunner(buf, client, runner)

	sched.dispatchRepo(context.Background(), aqueduct.RepoConfig{Name: "repo"})

	// Wait for the goroutine to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if poolAllIdle(sched.pools["repo"]) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !poolAllIdle(sched.pools["repo"]) {
		t.Error("worker not released after spawn failure")
	}
	log := buf.String()
	if !strings.Contains(log, "spawn failed") {
		t.Errorf("expected 'spawn failed' log; got:\n%s", log)
	}

	// Droplet must have been reset to open (not pooled).
	client.mu.Lock()
	status := ""
	if it, ok := client.items["spawn-fail"]; ok {
		status = it.Status
	}
	client.mu.Unlock()
	if status != "open" {
		t.Errorf("droplet status = %q after spawn failure; want 'open'", status)
	}
}

// TestDispatch_SuccessfulSpawn_WorkerRemainsFlowing verifies the happy path:
// after a successful spawn the worker stays busy (observe loop releases it),
// not prematurely returned to idle.
func TestDispatch_SuccessfulSpawn_WorkerRemainsFlowing(t *testing.T) {
	buf := &bytes.Buffer{}
	client := newMockClient()

	droplet := &cistern.Droplet{
		ID:                "success",
		Repo:              "repo",
		CurrentCataractae: "implement",
	}
	client.readyItems = []*cistern.Droplet{droplet}
	client.items["success"] = droplet

	runner := &funcRunner{fn: func(_ context.Context, _ CataractaeRequest) error {
		return nil // success
	}}
	sched := newTestSchedulerWithRunner(buf, client, runner)

	sched.dispatchRepo(context.Background(), aqueduct.RepoConfig{Name: "repo"})

	// Give goroutine time to run.
	time.Sleep(50 * time.Millisecond)

	// Worker should still be flowing — the observe loop hasn't released it yet.
	if poolAllIdle(sched.pools["repo"]) {
		t.Error("worker returned to idle immediately after successful spawn — should stay flowing until observe loop releases it")
	}
}

// --- helpers ---

// poolAllIdle returns true when every aqueduct in the pool is idle.
func poolAllIdle(pool *AqueductPool) bool {
	if pool == nil {
		return true
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for _, a := range pool.aqueducts {
		if a.Status != AqueductIdle {
			return false
		}
	}
	return true
}

// newTestScheduler builds a minimal Castellarius for unit testing.
func newTestScheduler(buf *bytes.Buffer, client *mockClient) *Castellarius {
	return newTestSchedulerWithRunner(buf, client, &funcRunner{fn: func(_ context.Context, _ CataractaeRequest) error {
		return nil
	}})
}

func newTestSchedulerWithRunner(buf *bytes.Buffer, client *mockClient, runner CataractaeRunner) *Castellarius {
	wf := &aqueduct.Workflow{
		Name: "feature",
		Cataractae: []aqueduct.WorkflowCataractae{
			{Name: "implement", Type: aqueduct.CataractaeTypeAgent, OnPass: "done"},
		},
	}
	cfg := aqueduct.AqueductConfig{
		Aqueducts: []aqueduct.Workflow{*wf},
		Repos: []aqueduct.RepoConfig{
			{Name: "repo", Prefix: "r", Aqueduct: wf.Name, Names: []string{"alpha"}},
		},
	}
	return NewFromParts(cfg,
		map[string]*aqueduct.Workflow{"repo": wf},
		map[string]CisternClient{"repo": client},
		runner,
		WithLogger(newTestLogger(buf)),
		WithPollInterval(10*time.Second),
	)
}

// funcRunner is a CataractaeRunner backed by an arbitrary function.
type funcRunner struct {
	fn func(ctx context.Context, req CataractaeRequest) error
}

func (r *funcRunner) Spawn(ctx context.Context, req CataractaeRequest) error {
	return r.fn(ctx, req)
}

// Extend mockClient with getReadyErrOnce for error-path tests.
// (Other mockClient fields/methods are defined in scheduler_test.go.)
func init() {
	// Verify mockClient still satisfies the interface after our additions.
	var _ CisternClient = (*mockClient)(nil)
}

// --- Liveness check DB integration tests ---
//
// These tests use a real cistern.Client backed by SQLite to catch column scan
// ordering bugs that mock-client tests cannot detect. If List() has a bug that
// leaves session log mtime always zero, the stall detector falls back to
// UpdatedAt. Because UpdatedAt is artificially aged in these tests, such a
// scan bug would cause a false stall in TestLivenessCheck_DB_NotStalled and
// the test would fail.

// TestLivenessCheck_DB_NotStalled_WhenRecentLogMtime uses a real DB to verify
// that the stall detector skips agents whose session log mtime is recent, even
// when updated_at is old. Detects scan ordering bugs in List().
func TestLivenessCheck_DB_NotStalled_WhenRecentLogMtime(t *testing.T) {
	origTmux := isTmuxAliveFn
	isTmuxAliveFn = func(_ string) bool { return true }
	t.Cleanup(func() { isTmuxAliveFn = origTmux })

	dbPath := filepath.Join(t.TempDir(), "test.db")
	c, err := cistern.New(dbPath, "ts")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	item, err := c.Add("test-repo", "DB integration task", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateStatus(item.ID, "in_progress"); err != nil {
		t.Fatal(err)
	}
	if err := c.Assign(item.ID, "alpha", "implement"); err != nil {
		t.Fatal(err)
	}

	// Age updated_at so the stall detector fires on the fallback path if
	// session_log mtime is not available.
	rawDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-2 * time.Hour)
	if _, err := rawDB.Exec(`UPDATE droplets SET updated_at = ?, stage_dispatched_at = ? WHERE id = ?`, past, past, item.ID); err != nil {
		rawDB.Close()
		t.Fatal(err)
	}
	rawDB.Close()

	// Mock session log mtime to return a recent time — agent is alive and working.
	origMtime := sessionLogMtimeFn
	sessionLogMtimeFn = func(sessionID string) (time.Time, error) { return time.Now().Add(-5 * time.Second), nil }
	t.Cleanup(func() { sessionLogMtimeFn = origMtime })

	cfg := testConfig()
	cfg.StallThresholdMinutes = 1
	workflows := map[string]*aqueduct.Workflow{"test-repo": testWorkflow()}
	clients := map[string]CisternClient{"test-repo": c}
	sched := NewFromParts(cfg, workflows, clients, newMockRunner(nil))

	sched.livenessCheckRepo(context.Background(), cfg.Repos[0])

	// Recent session log mtime → no stall event should be recorded.
	stallCount, err := c.CountEventsByType(item.ID, cistern.EventStall, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if stallCount != 0 {
		t.Errorf("DB integration: stall event written for recently-active agent (count=%d)", stallCount)
	}
}

// TestLivenessCheck_DB_Stalled_WhenNoLogMtime uses a real DB to verify that an
// agent with no session log mtime and an aged updated_at is detected as stalled
// and an escalation note is written.
func TestLivenessCheck_DB_Stalled_WhenNoLogMtime(t *testing.T) {
	origTmux := isTmuxAliveFn
	isTmuxAliveFn = func(_ string) bool { return true }
	t.Cleanup(func() { isTmuxAliveFn = origTmux })

	dbPath := filepath.Join(t.TempDir(), "test.db")
	c, err := cistern.New(dbPath, "ts")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	item, err := c.Add("test-repo", "DB stall task", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateStatus(item.ID, "in_progress"); err != nil {
		t.Fatal(err)
	}
	if err := c.Assign(item.ID, "alpha", "implement"); err != nil {
		t.Fatal(err)
	}

	// Age updated_at and stage_dispatched_at — no session log mtime; fallback triggers stall.
	rawDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-2 * time.Hour)
	if _, err := rawDB.Exec(`UPDATE droplets SET updated_at = ?, stage_dispatched_at = ? WHERE id = ?`, past, past, item.ID); err != nil {
		rawDB.Close()
		t.Fatal(err)
	}
	rawDB.Close()

	// Mock session log mtime to return zero time — agent has no session log.
	origMtime := sessionLogMtimeFn
	sessionLogMtimeFn = func(sessionID string) (time.Time, error) { return time.Time{}, nil }
	t.Cleanup(func() { sessionLogMtimeFn = origMtime })

	cfg := testConfig()
	cfg.StallThresholdMinutes = 1
	workflows := map[string]*aqueduct.Workflow{"test-repo": testWorkflow()}
	clients := map[string]CisternClient{"test-repo": c}
	sched := NewFromParts(cfg, workflows, clients, newMockRunner(nil))

	sched.livenessCheckRepo(context.Background(), cfg.Repos[0])

	// No session log → stall detected → stall event recorded.
	stallCount, err := c.CountEventsByType(item.ID, cistern.EventStall, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if stallCount == 0 {
		t.Error("DB integration: expected stall event for no-liveness agent, got none")
		return
	}
}
