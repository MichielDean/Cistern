package cataractae

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// makeGitRepoWithCommit initialises a git repo with one commit on main.
func makeGitRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	steps := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "noreply@lobsterdog.dev"},
		{"git", "config", "user.name", "Lobsterdog Contributors"},
	}
	for _, args := range steps {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
		{"git", "branch", "-M", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// isWorktreeRegistered reports whether absPath appears in 'git worktree list' for primaryDir.
func isWorktreeRegistered(t *testing.T, primaryDir, absPath string) bool {
	t.Helper()
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = primaryDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git worktree list: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "worktree "+absPath {
			return true
		}
	}
	return false
}

func TestEnsureWorktree_CreatesWorktree(t *testing.T) {
	primary := makeGitRepoWithCommit(t)
	worktreeDir := filepath.Join(t.TempDir(), "wt")

	if err := EnsureWorktree(primary, worktreeDir); err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}

	if _, err := os.Stat(worktreeDir); err != nil {
		t.Fatalf("worktree dir not created: %v", err)
	}

	abs, _ := filepath.Abs(worktreeDir)
	if !isWorktreeRegistered(t, primary, abs) {
		t.Error("worktree not registered in primary clone")
	}
}

func TestEnsureWorktree_Idempotent(t *testing.T) {
	primary := makeGitRepoWithCommit(t)
	worktreeDir := filepath.Join(t.TempDir(), "wt")

	if err := EnsureWorktree(primary, worktreeDir); err != nil {
		t.Fatalf("first EnsureWorktree: %v", err)
	}
	if err := EnsureWorktree(primary, worktreeDir); err != nil {
		t.Fatalf("second EnsureWorktree (idempotent): %v", err)
	}

	// Still registered exactly once (not duplicated).
	abs, _ := filepath.Abs(worktreeDir)
	if !isWorktreeRegistered(t, primary, abs) {
		t.Error("worktree not registered after idempotent call")
	}
}

func TestEnsureWorktree_ReplacesLegacyClone(t *testing.T) {
	primary := makeGitRepoWithCommit(t)
	worktreeDir := filepath.Join(t.TempDir(), "legacy")

	// Simulate a legacy dedicated clone: a plain directory with files.
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(worktreeDir, "legacy_file.txt")
	if err := os.WriteFile(sentinel, []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureWorktree(primary, worktreeDir); err != nil {
		t.Fatalf("EnsureWorktree on legacy dir: %v", err)
	}

	// Legacy contents must be gone.
	if _, err := os.Stat(sentinel); err == nil {
		t.Error("legacy sentinel file should have been removed")
	}

	abs, _ := filepath.Abs(worktreeDir)
	if !isWorktreeRegistered(t, primary, abs) {
		t.Error("worktree not registered after legacy clone replacement")
	}
}

func TestEnsureWorktree_PrunesStaleRegistrations(t *testing.T) {
	primary := makeGitRepoWithCommit(t)
	worktreeDir := filepath.Join(t.TempDir(), "wt")

	// Register the worktree.
	if err := EnsureWorktree(primary, worktreeDir); err != nil {
		t.Fatalf("EnsureWorktree: %v", err)
	}

	// Simulate staleness: delete the worktree files without deregistering.
	if err := os.RemoveAll(worktreeDir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	// Re-add the same path. prune must clear the stale entry so add succeeds.
	if err := EnsureWorktree(primary, worktreeDir); err != nil {
		t.Fatalf("EnsureWorktree after stale deletion: %v", err)
	}

	abs, _ := filepath.Abs(worktreeDir)
	if !isWorktreeRegistered(t, primary, abs) {
		t.Error("worktree not registered after prune+re-add")
	}
}

// makeEmptyGitRepo creates a git repo with ZERO commits on main (unborn branch).
// This simulates a brand-new empty repo where HEAD points to refs/heads/main
// but no commit exists.
func makeEmptyGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Init creates a repo with an unborn default branch (no commits).
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "config", "user.email", "noreply@lobsterdog.dev")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.email: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "config", "user.name", "Lobsterdog Contributors")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.name: %v\n%s", err, out)
	}

	// Rename branch to main for consistency.
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = dir
	// This will fail on an empty repo with no commits — that's expected.
	// The branch doesn't exist yet, so -M can't rename it. That's fine;
	// we just need the .git directory to exist for worktree operations.
	_ = cmd.Run()

	return dir
}

// TestEnsureWorktree_EmptyRepo_UsesOrphanBranch verifies that on an empty repo
// (no commits), EnsureWorktree uses --orphan to create a worktree instead of
// --detach which would fail with "fatal: invalid reference: HEAD".
func TestEnsureWorktree_EmptyRepo_UsesOrphanBranch(t *testing.T) {
	primary := makeEmptyGitRepo(t)
	worktreeDir := filepath.Join(t.TempDir(), "wt")

	if err := EnsureWorktree(primary, worktreeDir); err != nil {
		t.Fatalf("EnsureWorktree on empty repo: %v", err)
	}

	// (a) no error returned (verified above)

	// (b) worktree directory exists
	if _, err := os.Stat(worktreeDir); err != nil {
		t.Fatalf("worktree dir not created: %v", err)
	}

	// (c) worktree is registered in git worktree list
	abs, _ := filepath.Abs(worktreeDir)
	if !isWorktreeRegistered(t, primary, abs) {
		t.Error("worktree not registered in primary clone")
	}
}

// --- EnsurePrimaryClone fork-mode tests ---

// makeGitRepoWithRemoteURL creates a git repo with one commit on main and
// returns the local directory. Used as both origin and upstream in tests.
func makeGitRepoWithRemoteURL(t *testing.T) string {
	t.Helper()
	return makeGitRepoWithCommit(t)
}

// TestEnsurePrimaryClone_ForkMode_AddsUpstreamRemote verifies that when
// upstreamURL is provided, EnsurePrimaryClone adds an "upstream" remote
// pointing to that URL.
func TestEnsurePrimaryClone_ForkMode_AddsUpstreamRemote(t *testing.T) {
	primary := makeGitRepoWithRemoteURL(t)
	upstream := makeGitRepoWithRemoteURL(t)
	sandboxDir := filepath.Join(t.TempDir(), "primary")

	if err := EnsurePrimaryClone(sandboxDir, primary, upstream); err != nil {
		t.Fatalf("EnsurePrimaryClone: %v", err)
	}
	defer os.RemoveAll(sandboxDir)

	// Verify "upstream" remote exists and points to the correct URL.
	cmd := exec.Command("git", "-C", sandboxDir, "remote", "get-url", "upstream")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git remote get-url upstream: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != upstream {
		t.Errorf("upstream remote URL = %q, want %q", got, upstream)
	}
}

// TestEnsurePrimaryClone_DirectMode_NoUpstreamRemote verifies that when
// upstreamURL is empty, no "upstream" remote is added.
func TestEnsurePrimaryClone_DirectMode_NoUpstreamRemote(t *testing.T) {
	primary := makeGitRepoWithRemoteURL(t)
	sandboxDir := filepath.Join(t.TempDir(), "primary")

	if err := EnsurePrimaryClone(sandboxDir, primary, ""); err != nil {
		t.Fatalf("EnsurePrimaryClone: %v", err)
	}
	defer os.RemoveAll(sandboxDir)

	// Verify "upstream" remote does NOT exist.
	cmd := exec.Command("git", "-C", sandboxDir, "remote", "get-url", "upstream")
	_, err := cmd.Output()
	if err == nil {
		t.Error("upstream remote should not exist in direct mode")
	}
}

// TestEnsurePrimaryClone_FetchesUpstreamWhenPresent verifies that when
// upstreamURL is provided, the clone fetches from both origin and upstream.
func TestEnsurePrimaryClone_FetchesUpstreamWhenPresent(t *testing.T) {
	primary := makeGitRepoWithRemoteURL(t)
	upstream := makeGitRepoWithRemoteURL(t)
	sandboxDir := filepath.Join(t.TempDir(), "primary")

	if err := EnsurePrimaryClone(sandboxDir, primary, upstream); err != nil {
		t.Fatalf("EnsurePrimaryClone: %v", err)
	}
	defer os.RemoveAll(sandboxDir)

	// Verify upstream/main ref exists.
	cmd := exec.Command("git", "-C", sandboxDir, "branch", "-r", "--list", "upstream/main")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch -r --list upstream/main: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got == "" {
		t.Error("upstream/main ref not found after EnsurePrimaryClone with upstream URL")
	}
}

// TestEnsurePrimaryClone_UpdatesUpstreamRemoteURL verifies that calling
// EnsurePrimaryClone twice with different upstream URLs updates the remote.
func TestEnsurePrimaryClone_UpdatesUpstreamRemoteURL(t *testing.T) {
	primary := makeGitRepoWithRemoteURL(t)
	upstream1 := makeGitRepoWithRemoteURL(t)
	upstream2 := makeGitRepoWithRemoteURL(t)
	sandboxDir := filepath.Join(t.TempDir(), "primary")

	// First call with upstream1.
	if err := EnsurePrimaryClone(sandboxDir, primary, upstream1); err != nil {
		t.Fatalf("first EnsurePrimaryClone: %v", err)
	}
	defer os.RemoveAll(sandboxDir)

	// Second call with upstream2 — should update the remote.
	if err := EnsurePrimaryClone(sandboxDir, primary, upstream2); err != nil {
		t.Fatalf("second EnsurePrimaryClone: %v", err)
	}

	cmd := exec.Command("git", "-C", sandboxDir, "remote", "get-url", "upstream")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git remote get-url upstream: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != upstream2 {
		t.Errorf("upstream remote URL after update = %q, want %q", got, upstream2)
	}
}
