package castellarius

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/aqueduct"
	"github.com/MichielDean/cistern/internal/cistern"
)

// writeTestFile writes data to path, failing the test on error.
func writeTestFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// This file holds behavioral coverage tests for recently-shipped Cistern
// features that previously had only unit-level or smoke-test coverage:
//   - Recirculation routing with explicit target (recirculate:<step>)
//   - Fork-mode dispatch selecting baseRemote=upstream through the full tick
//   - Liveness exit-no-outcome detection triggering respawn
//
// Guidelines injection (extractGuidelines + CONTEXT.md) is covered by
// existing tests in internal/cataractae and is not duplicated here.
// Pool-reason dashboard display is covered in cmd/ct/dashboard_test.go.

// --- Recirculation routing with explicit target ---

// TestTick_RecirculateWithExplicitTarget_RoutesToTarget verifies that when an
// agent writes outcome "recirculate:<step>", the observe cycle routes the
// droplet to the explicitly named step, overriding the step's on_recirculate
// route.
//
// Given: a review step whose OnRecirculate points back to "implement", and an
//
//	agent that signals "recirculate:qa" (explicit target).
//
// When:  the observe tick processes the outcome.
// Then:  the droplet is assigned to "qa", NOT to "implement" (the on_recirculate
//
//	route is bypassed in favor of the explicit target).
func TestTick_RecirculateWithExplicitTarget_RoutesToTarget(t *testing.T) {
	wf := &aqueduct.Workflow{
		Name: "test",
		Cataractae: []aqueduct.WorkflowCataractae{
			{
				Name:   "implement",
				Type:   aqueduct.CataractaeTypeAgent,
				OnPass: "review",
				OnFail: "pooled",
			},
			{
				Name:          "review",
				Type:          aqueduct.CataractaeTypeAgent,
				OnPass:        "done",
				OnFail:        "implement",
				OnRecirculate: "implement", // default recirc route back to implement
			},
			{
				Name:   "qa",
				Type:   aqueduct.CataractaeTypeAgent,
				OnPass: "done",
				OnFail: "implement",
			},
		},
	}

	client := newMockClient()
	client.readyItems = []*cistern.Droplet{
		{ID: "rec-target", CurrentCataractae: "review"},
	}

	runner := newMockRunner(client)
	// Agent at review signals recirculate:qa — explicit target overrides on_recirculate.
	runner.outcomes["review"] = "recirculate:qa"

	config := testConfig()
	sched := NewFromParts(config,
		map[string]*aqueduct.Workflow{"test-repo": wf},
		map[string]CisternClient{"test-repo": client},
		runner)

	// Dispatch tick assigns the review step to a worker and writes the outcome.
	sched.Tick(context.Background())
	if !runner.waitCalls(1, time.Second) {
		t.Fatal("timed out waiting for dispatch spawn")
	}

	// Observe tick parses the outcome and routes to the explicit target.
	sched.Tick(context.Background())
	time.Sleep(20 * time.Millisecond)

	client.mu.Lock()
	defer client.mu.Unlock()

	if client.steps["rec-target"] != "qa" {
		t.Errorf("expected explicit target 'qa', got step %q (on_recirculate='implement' was incorrectly used)",
			client.steps["rec-target"])
	}
	if _, ok := client.pooled["rec-target"]; ok {
		t.Error("droplet should not be pooled when an explicit recirculate target is provided")
	}
}

// TestTick_RecirculateWithExplicitTarget_UnknownStep verifies that when an
// agent writes "recirculate:<unknown>", the scheduler passes the target
// through verbatim — it does NOT validate target existence against the
// workflow. The droplet is assigned to the unknown step name; downstream
// handling (no matching workflow step on the next dispatch) treats it as
// a no-op reset.
//
// Given: a review step whose OnRecirculate points to "implement", and an agent
//
//	that signals "recirculate:nonexistent" (target not in workflow).
//
// When:  the observe tick processes the outcome.
// Then:  the droplet is assigned to "nonexistent" — the scheduler trusts the
//
//	agent's explicit routing instruction without validating it against
//	the workflow step list.
func TestTick_RecirculateWithExplicitTarget_UnknownStep(t *testing.T) {
	wf := &aqueduct.Workflow{
		Name: "test",
		Cataractae: []aqueduct.WorkflowCataractae{
			{
				Name:   "implement",
				Type:   aqueduct.CataractaeTypeAgent,
				OnPass: "review",
				OnFail: "pooled",
			},
			{
				Name:          "review",
				Type:          aqueduct.CataractaeTypeAgent,
				OnPass:        "done",
				OnFail:        "implement",
				OnRecirculate: "implement",
			},
		},
	}

	client := newMockClient()
	client.readyItems = []*cistern.Droplet{
		{ID: "rec-unknown", CurrentCataractae: "review"},
	}

	runner := newMockRunner(client)
	runner.outcomes["review"] = "recirculate:nonexistent"

	config := testConfig()
	sched := NewFromParts(config,
		map[string]*aqueduct.Workflow{"test-repo": wf},
		map[string]CisternClient{"test-repo": client},
		runner)

	sched.Tick(context.Background())
	if !runner.waitCalls(1, time.Second) {
		t.Fatal("timed out waiting for dispatch spawn")
	}
	sched.Tick(context.Background())
	time.Sleep(20 * time.Millisecond)

	client.mu.Lock()
	defer client.mu.Unlock()

	// The explicit target "nonexistent" is not a workflow step, so isTerminal
	// returns false and Assign is called with it. The mockClient records
	// whatever step string was passed. We assert that the droplet was NOT
	// pooled (a route was taken) and that the step recorded matches the
	// explicit (unknown) target — the scheduler does not validate target
	// existence, it trusts the agent's routing instruction.
	if _, ok := client.pooled["rec-unknown"]; ok {
		t.Error("droplet should not be pooled when an explicit recirculate target is provided, even if unknown")
	}
	// The scheduler assigns to the explicit target verbatim.
	if client.steps["rec-unknown"] != "nonexistent" {
		t.Errorf("expected explicit (unknown) target 'nonexistent' to be passed through, got %q",
			client.steps["rec-unknown"])
	}
}

// TestTick_RecirculateExplicitTarget_FromNonReviewStep verifies the explicit
// target path works from any step, not just review. An implement agent
// signaling "recirculate:security" routes to the security step directly.
func TestTick_RecirculateExplicitTarget_FromNonReviewStep(t *testing.T) {
	wf := &aqueduct.Workflow{
		Name: "test",
		Cataractae: []aqueduct.WorkflowCataractae{
			{
				Name:          "implement",
				Type:          aqueduct.CataractaeTypeAgent,
				OnPass:        "review",
				OnFail:        "pooled",
				OnRecirculate: "implement", // self-loop default
			},
			{
				Name:   "review",
				Type:   aqueduct.CataractaeTypeAgent,
				OnPass: "security",
				OnFail: "implement",
			},
			{
				Name:   "security",
				Type:   aqueduct.CataractaeTypeAgent,
				OnPass: "done",
				OnFail: "implement",
			},
		},
	}

	client := newMockClient()
	client.readyItems = []*cistern.Droplet{
		{ID: "rec-impl-to-sec", CurrentCataractae: "implement"},
	}

	runner := newMockRunner(client)
	// Implement agent bypasses review and routes directly to security.
	runner.outcomes["implement"] = "recirculate:security"

	config := testConfig()
	sched := NewFromParts(config,
		map[string]*aqueduct.Workflow{"test-repo": wf},
		map[string]CisternClient{"test-repo": client},
		runner)

	sched.Tick(context.Background())
	if !runner.waitCalls(1, time.Second) {
		t.Fatal("timed out waiting for dispatch spawn")
	}
	sched.Tick(context.Background())
	time.Sleep(20 * time.Millisecond)

	client.mu.Lock()
	defer client.mu.Unlock()

	if client.steps["rec-impl-to-sec"] != "security" {
		t.Errorf("expected explicit target 'security' from implement step, got %q",
			client.steps["rec-impl-to-sec"])
	}
}

// --- Fork-mode dispatch integration ---

// TestTick_ForkModeDispatch_CreatesWorktreeFromUpstreamMain verifies that
// when a droplet is dispatched for a fork-mode repo, the per-droplet worktree
// is created from upstream/main (not origin/main). This exercises the full
// dispatch path: dispatchRepo → DeliveryModeFork check → prepareDropletWorktree
// with baseRemote="upstream".
//
// This is the behavioral integration test that complements the unit-level
// prepareDropletWorktree fork-mode tests in branch_lifecycle_test.go.
func TestTick_ForkModeDispatch_CreatesWorktreeFromUpstreamMain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Build a fork-mode primary clone: origin is the fork, upstream is the
	// source-of-truth repo. makeBareAndCloneWithUpstream gives us a primary
	// clone with both remotes and matching main branches.
	primaryDir, upstreamDir := makeBareAndCloneWithUpstream(t)

	// Diverge origin/main from upstream/main so we can tell which remote the
	// worktree was based on. Add a fork-only commit to origin (the fork).
	branchMustRun(t, branchGitCmd(primaryDir, "checkout", "main"))
	if err := exec.Command("git", "-C", primaryDir, "config", "user.email", "noreply@lobsterdog.dev").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", primaryDir, "config", "user.name", "Lobsterdog Contributors").Run(); err != nil {
		t.Fatal(err)
	}
	forkOnlyPath := filepath.Join(primaryDir, "FORK_ONLY.md")
	if err := writeTestFile(forkOnlyPath, []byte("fork-only change\n")); err != nil {
		t.Fatal(err)
	}
	branchMustRun(t, branchGitCmd(primaryDir, "add", "FORK_ONLY.md"))
	branchMustRun(t, branchGitCmd(primaryDir, "commit", "-m", "fork-only commit"))
	branchMustRun(t, branchGitCmd(primaryDir, "push", "origin", "main"))

	// Place the primary clone at sandboxRoot/<repo>/_primary (the location
	// dispatchRepo expects). Clone the diverged primary so the sandbox copy
	// has origin/main (with fork-only commit) and upstream/main (without).
	sandboxRoot := t.TempDir()
	repoName := "fork-dispatch-repo"
	primaryTarget := filepath.Join(sandboxRoot, repoName, "_primary")
	branchMustRun(t, branchGitCmd(".", "clone", primaryDir, primaryTarget))
	branchMustRun(t, branchGitCmd(primaryTarget, "remote", "add", "upstream", upstreamDir))
	branchMustRun(t, branchGitCmd(primaryTarget, "fetch", "upstream"))
	branchMustRun(t, branchGitCmd(primaryTarget, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(primaryTarget, "config", "user.name", "Lobsterdog Contributors"))

	const itemID = "fork-dispatch-1"
	client := newMockClient()
	client.readyItems = []*cistern.Droplet{
		{ID: itemID, CurrentCataractae: "implement", Status: "open"},
	}

	runner := newMockRunner(client)

	config := aqueduct.AqueductConfig{
		Repos: []aqueduct.RepoConfig{
			{
				Name:           repoName,
				Cataractae:     1,
				Names:          []string{"alpha"},
				Prefix:         "fd",
				DeliveryMode:   aqueduct.DeliveryModeFork,
				UpstreamRemote: upstreamDir,
			},
		},
	}
	wf := &aqueduct.Workflow{
		Name: "test",
		Cataractae: []aqueduct.WorkflowCataractae{
			{Name: "implement", Type: aqueduct.CataractaeTypeAgent, OnPass: "done", OnFail: "pooled"},
		},
	}
	sched := NewFromParts(config,
		map[string]*aqueduct.Workflow{repoName: wf},
		map[string]CisternClient{repoName: client},
		runner,
		WithSandboxRoot(sandboxRoot))

	sched.Tick(context.Background())
	if !runner.waitCalls(1, 3*time.Second) {
		t.Fatal("timed out waiting for fork-mode dispatch spawn")
	}

	// The per-droplet worktree should be created at sandboxRoot/<repo>/<itemID>
	// and its HEAD should match upstream/main, NOT origin/main.
	worktreePath := filepath.Join(sandboxRoot, repoName, itemID)
	upstreamMainSHA, err := exec.Command("git", "-C", primaryTarget, "rev-parse", "upstream/main").Output()
	if err != nil {
		t.Fatalf("git rev-parse upstream/main: %v", err)
	}
	worktreeHEAD, err := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in worktree: %v", err)
	}

	if strings.TrimSpace(string(worktreeHEAD)) != strings.TrimSpace(string(upstreamMainSHA)) {
		originMainSHA, _ := exec.Command("git", "-C", primaryTarget, "rev-parse", "origin/main").Output()
		t.Errorf("fork-mode worktree HEAD = %s, want upstream/main = %s (origin/main = %s — fork dispatch used origin instead of upstream)",
			strings.TrimSpace(string(worktreeHEAD)),
			strings.TrimSpace(string(upstreamMainSHA)),
			strings.TrimSpace(string(originMainSHA)))
	}

	// The fork-only file should NOT be present (it only exists on origin/main).
	if err := exec.Command("git", "-C", worktreePath, "cat-file", "-e", "HEAD:FORK_ONLY.md").Run(); err == nil {
		t.Error("fork-only commit is in the worktree — fork dispatch based the worktree on origin/main, not upstream/main")
	}
}

// --- Liveness: dead session with no outcome triggers respawn ---

// TestLiveness_DeadSession_NoOutcome_ResetsForRedispatch verifies that when a
// dispatched agent's tmux session dies without writing an outcome, the
// liveness check records an exit_no_outcome event, releases the worker, and
// resets the droplet to open so it gets re-dispatched.
//
// This complements the existing TestLiveness_ExitDetection_DeadSession_NoOutcome
// in liveness_regression_test.go by asserting the pool slot is released and
// the event payload carries the worker name.
func TestLiveness_DeadSession_NoOutcome_ResetsForRedispatch(t *testing.T) {
	client := newMockClient()
	item := &cistern.Droplet{
		ID:                "dead-session-1",
		CurrentCataractae: "implement",
		Status:            "in_progress",
		Assignee:          "alpha",
		StageDispatchedAt: time.Now().Add(-10 * time.Minute), // well past exit guard
	}
	client.items[item.ID] = item

	// Force isTmuxAlive to report the session as dead (agent has exited).
	origIsTmuxAlive := isTmuxAliveFn
	isTmuxAliveFn = func(sessionID string) bool { return false }
	t.Cleanup(func() { isTmuxAliveFn = origIsTmuxAlive })

	// No session log mtime — forces stall path to skip straight to exit check.
	origSessionLogMtime := sessionLogMtimeFn
	sessionLogMtimeFn = func(sessionID string) (time.Time, error) { return time.Time{}, nil }
	t.Cleanup(func() { sessionLogMtimeFn = origSessionLogMtime })

	config := testConfig()
	// Short liveness interval so exit guard (4×) is satisfied quickly.
	sched := NewFromParts(config,
		map[string]*aqueduct.Workflow{"test-repo": testWorkflow()},
		map[string]CisternClient{"test-repo": client},
		newMockRunner(client),
		WithLivenessInterval(time.Second))

	sched.livenessCheckRepo(context.Background(), config.Repos[0])

	client.mu.Lock()
	defer client.mu.Unlock()

	// Droplet should be reset to open (empty assignee) for re-dispatch.
	if got := client.items[item.ID]; got != nil {
		if got.Assignee != "" {
			t.Errorf("assignee = %q, want empty (reset for redispatch)", got.Assignee)
		}
		if got.Status != "open" {
			t.Errorf("status = %q, want 'open' (reset for redispatch)", got.Status)
		}
	}

	// exit_no_outcome event must be recorded.
	var exitEvent *recordedEvent
	for i := range client.events {
		if client.events[i].eventType == cistern.EventExitNoOutcome {
			exitEvent = &client.events[i]
			break
		}
	}
	if exitEvent == nil {
		t.Fatal("expected exit_no_outcome event, got none")
	}
	if !strings.Contains(exitEvent.payload, `"worker":"alpha"`) {
		t.Errorf("exit_no_outcome payload should contain worker name, got %q", exitEvent.payload)
	}
	if !strings.Contains(exitEvent.payload, `"cataractae":"implement"`) {
		t.Errorf("exit_no_outcome payload should contain cataractae name, got %q", exitEvent.payload)
	}

	// Pool slot should be released — the worker is no longer flowing.
	pool := sched.pools["test-repo"]
	if pool == nil {
		t.Fatal("no pool for test-repo")
	}
	if pool.FlowingCount() != 0 {
		t.Errorf("pool flowing count = %d, want 0 (worker released)", pool.FlowingCount())
	}
	if w := pool.FindByName("alpha"); w != nil && w.Status == AqueductFlowing {
		t.Error("worker alpha should not be flowing after exit_no_outcome reset")
	}
}

// TestLiveness_DeadSession_WithOutcome_SkipsReset verifies that when a dead
// tmux session is detected but the agent DID write an outcome before exiting,
// the liveness check does NOT reset the droplet — the observe cycle handles
// the outcome. This is the guard against the heartbeat-observer race.
//
// This complements liveness_regression_test.go by asserting no exit_no_outcome
// event is recorded and the pool slot is NOT released (observe owns that).
func TestLiveness_DeadSession_WithOutcome_SkipsReset(t *testing.T) {
	client := newMockClient()
	item := &cistern.Droplet{
		ID:                "dead-session-2",
		CurrentCataractae: "implement",
		Status:            "in_progress",
		Assignee:          "alpha",
		Outcome:           "pass", // agent wrote an outcome before exiting
		StageDispatchedAt: time.Now().Add(-10 * time.Minute),
	}
	client.items[item.ID] = item

	origIsTmuxAlive := isTmuxAliveFn
	isTmuxAliveFn = func(sessionID string) bool { return false }
	t.Cleanup(func() { isTmuxAliveFn = origIsTmuxAlive })

	origSessionLogMtime := sessionLogMtimeFn
	sessionLogMtimeFn = func(sessionID string) (time.Time, error) { return time.Time{}, nil }
	t.Cleanup(func() { sessionLogMtimeFn = origSessionLogMtime })

	// Mark the worker as flowing to assert it stays that way.
	pool := NewAqueductPool("test-repo", []string{"alpha"})
	if w := pool.FindByName("alpha"); w != nil {
		pool.Assign(w, item.ID, "implement")
	}

	config := testConfig()
	sched := NewFromParts(config,
		map[string]*aqueduct.Workflow{"test-repo": testWorkflow()},
		map[string]CisternClient{"test-repo": client},
		newMockRunner(client),
		WithLivenessInterval(time.Second))
	sched.pools["test-repo"] = pool

	sched.livenessCheckRepo(context.Background(), config.Repos[0])

	client.mu.Lock()
	defer client.mu.Unlock()

	// No exit_no_outcome event — the agent produced an outcome.
	for _, e := range client.events {
		if e.eventType == cistern.EventExitNoOutcome {
			t.Errorf("exit_no_outcome event recorded when outcome was present: %+v", e)
		}
	}

	// Worker should still be flowing — observe will release it.
	if pool.FlowingCount() != 1 {
		t.Errorf("pool flowing count = %d, want 1 (observe owns release)", pool.FlowingCount())
	}
}

// --- Fork-mode recirculation: outcome written by fork-delivery cataractae ---

// TestTick_ForkModeDeliveryOutcome_RoutesToTerminal verifies that when a
// fork-mode repo's fork-delivery step writes "pass", the droplet routes to
// the terminal (done) state just like direct-mode delivery. The routing
// logic is delivery-mode-agnostic; this test confirms no fork-specific
// branching in the observe path breaks terminal routing.
func TestTick_ForkModeDeliveryOutcome_RoutesToTerminal(t *testing.T) {
	wf := &aqueduct.Workflow{
		Name: "test",
		Cataractae: []aqueduct.WorkflowCataractae{
			{
				Name:   "implement",
				Type:   aqueduct.CataractaeTypeAgent,
				OnPass: "fork-delivery",
				OnFail: "pooled",
			},
			{
				Name:   "fork-delivery",
				Type:   aqueduct.CataractaeTypeAgent,
				OnPass: "done",
				OnFail: "implement",
			},
		},
	}

	client := newMockClient()
	client.readyItems = []*cistern.Droplet{
		{ID: "fork-deliver-1", CurrentCataractae: "fork-delivery"},
	}

	runner := newMockRunner(client)
	runner.outcomes["fork-delivery"] = "pass"

	config := aqueduct.AqueductConfig{
		Repos: []aqueduct.RepoConfig{
			{
				Name:           "test-repo",
				Cataractae:     1,
				Names:          []string{"alpha"},
				Prefix:         "fd",
				DeliveryMode:   aqueduct.DeliveryModeFork,
				UpstreamRemote: "https://example.com/upstream.git",
			},
		},
	}
	sched := NewFromParts(config,
		map[string]*aqueduct.Workflow{"test-repo": wf},
		map[string]CisternClient{"test-repo": client},
		runner)

	sched.Tick(context.Background())
	if !runner.waitCalls(1, time.Second) {
		t.Fatal("timed out waiting for fork-delivery dispatch")
	}
	sched.Tick(context.Background())
	time.Sleep(20 * time.Millisecond)

	client.mu.Lock()
	defer client.mu.Unlock()

	if !client.closed["fork-deliver-1"] {
		t.Error("fork-mode delivery pass should close (deliver) the droplet, got not-closed")
	}
}
