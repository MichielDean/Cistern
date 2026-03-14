package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MichielDean/citadel/internal/queue"
	"github.com/MichielDean/citadel/internal/workflow"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// --- Idle edge detection tests ---

func TestIdleHook_FiresOnIdleTransition(t *testing.T) {
	// When scheduler transitions from busy to idle (wasIdle false→true),
	// hooks should fire.
	client := newMockClient()
	client.readyItems = []*queue.WorkItem{{ID: "b1"}}

	runner := newMockRunner()
	runner.outcomes["implement"] = &Outcome{Result: ResultPass}

	sched := testScheduler(client, runner)

	// Create a temp file for shell hook to write to, proving it ran.
	tmpDir := t.TempDir()
	markerFile := filepath.Join(tmpDir, "idle-fired")
	sched.config.IdleHooks = []workflow.IdleHook{
		{
			Name:    "test-marker",
			Action:  "shell",
			Command: "touch " + markerFile,
			Timeout: 5,
		},
	}
	sched.logger = discardLogger()

	// First tick: picks up work (busy). wasIdle starts false.
	sched.Tick(context.Background())
	if !runner.waitCalls(1, time.Second) {
		t.Fatal("timed out waiting for runner")
	}
	// Allow routing to complete.
	time.Sleep(100 * time.Millisecond)

	// At this point the work is done and workers released.
	// Next tick: no work available → idle transition fires hooks.
	sched.Tick(context.Background())

	if _, err := os.Stat(markerFile); os.IsNotExist(err) {
		t.Error("idle hook did not fire on idle transition")
	}
}

func TestIdleHook_DoesNotFireWhenAlreadyIdle(t *testing.T) {
	// When scheduler is already idle (wasIdle true→true), hooks should NOT fire again.
	client := newMockClient()
	runner := newMockRunner()

	sched := testScheduler(client, runner)

	tmpDir := t.TempDir()
	counterFile := filepath.Join(tmpDir, "counter")
	sched.config.IdleHooks = []workflow.IdleHook{
		{
			Name:    "counter",
			Action:  "shell",
			Command: "echo x >> " + counterFile,
			Timeout: 5,
		},
	}
	sched.logger = discardLogger()

	// First tick: no work → enters idle, hooks fire.
	sched.Tick(context.Background())

	// Second tick: still no work → stays idle, hooks should NOT fire.
	sched.Tick(context.Background())

	// Third tick: same.
	sched.Tick(context.Background())

	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatal("counter file should exist:", err)
	}
	// Should have exactly one line (one "x\n") from the first idle transition.
	lines := 0
	for _, b := range data {
		if b == 'x' {
			lines++
		}
	}
	if lines != 1 {
		t.Errorf("expected hook to fire exactly once, got %d times", lines)
	}
}

func TestIdleHook_DoesNotFireWhileWorkInProgress(t *testing.T) {
	// When work is in progress, hooks should not fire.
	client := newMockClient()
	for i := range 3 {
		client.readyItems = append(client.readyItems, &queue.WorkItem{
			ID: fmt.Sprintf("b%d", i),
		})
	}

	blocker := newBlockingRunner()
	sched := testScheduler(client, blocker)

	tmpDir := t.TempDir()
	markerFile := filepath.Join(tmpDir, "should-not-exist")
	sched.config.IdleHooks = []workflow.IdleHook{
		{
			Name:    "test-marker",
			Action:  "shell",
			Command: "touch " + markerFile,
			Timeout: 5,
		},
	}
	sched.logger = discardLogger()

	// Tick: workers are busy (blocking runner).
	sched.Tick(context.Background())
	time.Sleep(50 * time.Millisecond)

	// Another tick while workers are still busy.
	sched.Tick(context.Background())

	if _, err := os.Stat(markerFile); !os.IsNotExist(err) {
		t.Error("idle hook should not fire while work is in progress")
	}

	close(blocker.ch)
}

// --- Built-in hook tests ---

func TestRolesGenerate_NoOpWhenYAMLOlder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a workflow YAML file.
	wfPath := filepath.Join(tmpDir, "workflow.yaml")
	wfContent := `name: test
steps:
  - name: impl
    type: agent
    on_pass: done
roles:
  implementer:
    name: Implementer
    description: test
    instructions: old instructions
`
	os.WriteFile(wfPath, []byte(wfContent), 0o644)

	// Create a roles dir with a CLAUDE.md that is newer than the workflow YAML.
	rolesDir := filepath.Join(tmpDir, "roles")
	implDir := filepath.Join(rolesDir, "implementer")
	os.MkdirAll(implDir, 0o755)
	claudePath := filepath.Join(implDir, "CLAUDE.md")
	os.WriteFile(claudePath, []byte("existing role content"), 0o644)

	// Make the CLAUDE.md newer than the workflow YAML by touching it.
	future := time.Now().Add(time.Hour)
	os.Chtimes(claudePath, future, future)

	cfg := &workflow.FarmConfig{
		Repos: []workflow.RepoConfig{
			{Name: "test", WorkflowPath: wfPath, Workers: 1, Prefix: "t"},
		},
		MaxTotalWorkers: 1,
	}

	logger := discardLogger()
	err := hookRolesGenerate(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The role file should NOT have been regenerated (content unchanged).
	data, _ := os.ReadFile(claudePath)
	if string(data) != "existing role content" {
		t.Error("roles_generate should have been a no-op but content changed")
	}
}

func TestRolesGenerate_RegeneratesWhenYAMLNewer(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a workflow YAML file.
	wfPath := filepath.Join(tmpDir, "workflow.yaml")
	wfContent := `name: test
steps:
  - name: impl
    type: agent
    on_pass: done
roles:
  implementer:
    name: Implementer
    description: test
    instructions: new instructions
`
	os.WriteFile(wfPath, []byte(wfContent), 0o644)

	// Make the YAML file newer than roles.
	future := time.Now().Add(time.Hour)
	os.Chtimes(wfPath, future, future)

	// Create a roles dir with an older CLAUDE.md.
	rolesDir := filepath.Join(tmpDir, "roles")
	implDir := filepath.Join(rolesDir, "implementer")
	os.MkdirAll(implDir, 0o755)
	claudePath := filepath.Join(implDir, "CLAUDE.md")
	os.WriteFile(claudePath, []byte("old content"), 0o644)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(claudePath, past, past)

	// Override home dir to use tmpDir for roles.
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create ~/.citadel/roles structure.
	citadelRoles := filepath.Join(tmpDir, ".citadel", "roles", "implementer")
	os.MkdirAll(citadelRoles, 0o755)
	citadelClaude := filepath.Join(citadelRoles, "CLAUDE.md")
	os.WriteFile(citadelClaude, []byte("old"), 0o644)
	os.Chtimes(citadelClaude, past, past)

	cfg := &workflow.FarmConfig{
		Repos: []workflow.RepoConfig{
			{Name: "test", WorkflowPath: wfPath, Workers: 1, Prefix: "t"},
		},
		MaxTotalWorkers: 1,
	}

	logger := discardLogger()
	err := hookRolesGenerate(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The role file should have been regenerated.
	data, _ := os.ReadFile(citadelClaude)
	content := string(data)
	if content == "old" {
		t.Error("roles_generate should have regenerated but didn't")
	}
	if len(content) == 0 {
		t.Error("regenerated file is empty")
	}
}

func TestWorktreePrune_HandlesErrorGracefully(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a repo dir that is NOT a git repo — git worktree prune will fail.
	repoDir := filepath.Join(tmpDir, "fakerepo")
	os.MkdirAll(repoDir, 0o755)

	cfg := &workflow.FarmConfig{
		Repos: []workflow.RepoConfig{
			{Name: "fakerepo", Workers: 1, Prefix: "f"},
		},
		MaxTotalWorkers: 1,
	}

	logger := discardLogger()
	// Should not panic or return error — errors are logged.
	err := hookWorktreePrune(cfg, tmpDir, logger)
	if err != nil {
		t.Fatalf("worktree_prune should not return error, got: %v", err)
	}
}

func TestShellHook_RunsCommand(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "output.txt")

	hook := workflow.IdleHook{
		Name:    "test-shell",
		Action:  "shell",
		Command: "echo hello > " + outFile,
		Timeout: 5,
	}

	logger := discardLogger()
	err := hookShell(hook, logger)
	if err != nil {
		t.Fatalf("shell hook failed: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if got := string(data); got != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", got)
	}
}

func TestShellHook_NonZeroExitIsWarning(t *testing.T) {
	hook := workflow.IdleHook{
		Name:    "failing-hook",
		Action:  "shell",
		Command: "exit 1",
		Timeout: 5,
	}

	logger := discardLogger()
	err := hookShell(hook, logger)
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	// Error should be returned (logged as warning by RunIdleHooks), not panic.
}

func TestShellHook_EmptyCommandErrors(t *testing.T) {
	hook := workflow.IdleHook{
		Name:   "empty",
		Action: "shell",
	}

	logger := discardLogger()
	err := hookShell(hook, logger)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}
