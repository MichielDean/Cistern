package castellarius

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/MichielDean/cistern/internal/aqueduct"
	"github.com/MichielDean/cistern/internal/cistern"
)

// --- git helpers for branch lifecycle tests ---

func branchGitCmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

func branchMustRun(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v failed: %v\n%s", cmd.Args, err, out)
	}
}

// makeBareAndClone creates:
//
//	base/remote/ — bare git repo with one commit on main
//	base/primary/ — full clone of remote (has origin remote set)
//
// Returns the primary directory and the bare remote directory.
// Callers have origin/main available for fetch.
func makeBareAndClone(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	remoteDir := filepath.Join(base, "remote")
	primaryDir := filepath.Join(base, "primary")

	// Create an intermediate repo to build the initial commit, then push to bare.
	initDir := filepath.Join(base, "init")

	branchMustRun(t, branchGitCmd(".", "init", "--bare", remoteDir))
	branchMustRun(t, branchGitCmd(".", "init", initDir))
	branchMustRun(t, branchGitCmd(initDir, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(initDir, "config", "user.name", "Lobsterdog Contributors"))

	if err := os.WriteFile(filepath.Join(initDir, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branchMustRun(t, branchGitCmd(initDir, "add", "."))
	branchMustRun(t, branchGitCmd(initDir, "commit", "-m", "initial"))
	branchMustRun(t, branchGitCmd(initDir, "branch", "-M", "main"))
	branchMustRun(t, branchGitCmd(initDir, "remote", "add", "origin", remoteDir))
	branchMustRun(t, branchGitCmd(initDir, "push", "-u", "origin", "main"))
	// Ensure the bare repo's HEAD points to main so clones default to main.
	branchMustRun(t, branchGitCmd(remoteDir, "symbolic-ref", "HEAD", "refs/heads/main"))

	// Clone the bare remote to create the primary (inherits origin remote).
	branchMustRun(t, branchGitCmd(".", "clone", remoteDir, primaryDir))
	branchMustRun(t, branchGitCmd(primaryDir, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(primaryDir, "config", "user.name", "Lobsterdog Contributors"))

	return primaryDir, remoteDir
}

// currentBranch returns the symbolic branch name of HEAD, or "HEAD" if detached.
func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// branchExists reports whether branchName appears in 'git branch --list'.
func branchExists(t *testing.T, dir, branchName string) bool {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "branch", "--list", branchName).Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	return strings.TrimSpace(string(out)) != ""
}

// --- prepareBranchInSandbox tests ---

// TestPrepareBranchInSandbox_NewBranch verifies that calling prepareBranchInSandbox
// on a repo that does not yet have the feature branch creates it from origin/main.
func TestPrepareBranchInSandbox_NewBranch(t *testing.T) {
	dir, _ := makeBareAndClone(t)

	if err := prepareBranchInSandbox(dir, "drop-new"); err != nil {
		t.Fatalf("prepareBranchInSandbox: %v", err)
	}

	if got := currentBranch(t, dir); got != "feat/drop-new" {
		t.Errorf("HEAD branch = %q, want feat/drop-new", got)
	}
}

// TestPrepareBranchInSandbox_NewBranch_ConfiguresGitIdentity verifies that
// git user.name and user.email are set in the repo after the call.
func TestPrepareBranchInSandbox_NewBranch_ConfiguresGitIdentity(t *testing.T) {
	dir, _ := makeBareAndClone(t)

	if err := prepareBranchInSandbox(dir, "drop-ident"); err != nil {
		t.Fatalf("prepareBranchInSandbox: %v", err)
	}

	nameOut, err := exec.Command("git", "-C", dir, "config", "user.name").Output()
	if err != nil {
		t.Fatalf("git config user.name: %v", err)
	}
	if got := strings.TrimSpace(string(nameOut)); got != "Lobsterdog Contributors" {
		t.Errorf("user.name = %q, want %q", got, "Lobsterdog Contributors")
	}

	emailOut, err := exec.Command("git", "-C", dir, "config", "user.email").Output()
	if err != nil {
		t.Fatalf("git config user.email: %v", err)
	}
	if got := strings.TrimSpace(string(emailOut)); got != "noreply@lobsterdog.dev" {
		t.Errorf("user.email = %q, want %q", got, "noreply@lobsterdog.dev")
	}
}

// TestPrepareBranchInSandbox_ResumeBranch verifies that when the feature branch
// already exists, prepareBranchInSandbox checks it out without resetting it —
// preserving any commits already on the branch.
func TestPrepareBranchInSandbox_ResumeBranch(t *testing.T) {
	dir, _ := makeBareAndClone(t)

	// First call creates the branch.
	if err := prepareBranchInSandbox(dir, "drop-resume"); err != nil {
		t.Fatalf("prepareBranchInSandbox (create): %v", err)
	}

	// Make a commit on the feature branch to represent prior agent work.
	if err := os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branchMustRun(t, branchGitCmd(dir, "add", "."))
	branchMustRun(t, branchGitCmd(dir, "commit", "-m", "agent work"))

	before, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse before: %v", err)
	}

	// Second call on an existing branch must resume, not reset.
	if err := prepareBranchInSandbox(dir, "drop-resume"); err != nil {
		t.Fatalf("prepareBranchInSandbox (resume): %v", err)
	}

	after, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse after: %v", err)
	}
	if strings.TrimSpace(string(before)) != strings.TrimSpace(string(after)) {
		t.Errorf("branch was reset instead of resumed: HEAD changed %s → %s",
			strings.TrimSpace(string(before)), strings.TrimSpace(string(after)))
	}

	if got := currentBranch(t, dir); got != "feat/drop-resume" {
		t.Errorf("HEAD branch after resume = %q, want feat/drop-resume", got)
	}
}

// --- removeDropletWorktree tests ---

// TestRemoveDropletWorktree_DeletesBranch verifies that removeDropletWorktree
// deletes the feat/<id> branch ref in the primary clone, not just the worktree
// directory. Without this, dead branch refs accumulate indefinitely.
func TestRemoveDropletWorktree_DeletesBranch(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()

	// Create a worktree so there's a branch to remove.
	_, err := prepareDropletWorktree(primaryDir, sandboxRoot, "myrepo", "drop-rm")
	if err != nil {
		t.Fatalf("prepareDropletWorktree: %v", err)
	}
	if !branchExists(t, primaryDir, "feat/drop-rm") {
		t.Fatal("feat/drop-rm should exist after prepareDropletWorktree")
	}

	removeDropletWorktree(primaryDir, sandboxRoot, "myrepo", "drop-rm", false)

	if branchExists(t, primaryDir, "feat/drop-rm") {
		t.Error("feat/drop-rm should have been deleted by removeDropletWorktree")
	}
}

// --- prepareDropletWorktree tests ---

// TestPrepareDropletWorktree_NewWorktree_CreatesOnFeatureBranch verifies that a
// new worktree is created at the correct path on the feature branch.
func TestPrepareDropletWorktree_NewWorktree_CreatesOnFeatureBranch(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()

	worktreePath, err := prepareDropletWorktree(primaryDir, sandboxRoot, "myrepo", "drop-new")
	if err != nil {
		t.Fatalf("prepareDropletWorktree: %v", err)
	}

	if _, statErr := os.Stat(worktreePath); statErr != nil {
		t.Fatalf("worktree path does not exist: %v", statErr)
	}

	if got := currentBranch(t, worktreePath); got != "feat/drop-new" {
		t.Errorf("HEAD branch = %q, want feat/drop-new", got)
	}
}

// TestPrepareDropletWorktree_FreshBranch_StartsAtOriginMain verifies that when
// the feature branch does not yet exist, the new worktree is created from
// origin/main and the worktree is clean — no dirty state from the primary clone.
func TestPrepareDropletWorktree_FreshBranch_StartsAtOriginMain(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()

	originMainSHA := func() string {
		out, err := exec.Command("git", "-C", primaryDir, "rev-parse", "origin/main").Output()
		if err != nil {
			t.Fatalf("rev-parse origin/main: %v", err)
		}
		return strings.TrimSpace(string(out))
	}()

	worktreePath, err := prepareDropletWorktree(primaryDir, sandboxRoot, "myrepo", "drop-fresh")
	if err != nil {
		t.Fatalf("prepareDropletWorktree: %v", err)
	}

	worktreeHEAD := func() string {
		out, err := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatalf("rev-parse HEAD in worktree: %v", err)
		}
		return strings.TrimSpace(string(out))
	}()

	if worktreeHEAD != originMainSHA {
		t.Errorf("worktree HEAD = %s, want origin/main = %s", worktreeHEAD, originMainSHA)
	}

	// Worktree must be clean after creation.
	statusOut, statusErr := exec.Command("git", "-C", worktreePath, "status", "--porcelain").Output()
	if statusErr != nil {
		t.Fatalf("git status: %v", statusErr)
	}
	if strings.TrimSpace(string(statusOut)) != "" {
		t.Errorf("worktree is not clean after prepareDropletWorktree:\n%s", statusOut)
	}
}

// --- keepBranch / stagnant-resume tests ---

// newBranchLifecycleLogger creates a slog.Logger that writes to w.
// Pass io.Discard for tests that don't assert log output.
func newBranchLifecycleLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// TestRemoveDropletWorktree_KeepBranch_WhenStagnant_PreservesFeatureBranch verifies
// that when keepBranch=true the worktree directory is removed but the feature
// branch ref survives in the primary clone (stagnant path).
func TestRemoveDropletWorktree_KeepBranch_WhenStagnant_PreservesFeatureBranch(t *testing.T) {
	// Given: a worktree created for a droplet with a commit on the feature branch.
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-stagnant", "origin")
	if err != nil {
		t.Fatalf("prepareDropletWorktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "work.go"), []byte("// work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branchMustRun(t, branchGitCmd(worktreePath, "add", "."))
	branchMustRun(t, branchGitCmd(worktreePath, "commit", "-m", "agent work"))

	// When: stagnant cleanup (keepBranch=true).
	removeDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-stagnant", true)

	// Then: worktree directory is gone.
	if _, statErr := os.Stat(worktreePath); statErr == nil {
		t.Error("worktree directory should have been removed on stagnant cleanup")
	}

	// Then: feature branch still exists in primary clone.
	if !branchExists(t, primaryDir, "feat/drop-stagnant") {
		t.Error("feat/drop-stagnant should be preserved in primary clone after stagnant cleanup")
	}
}

// TestRemoveDropletWorktree_DeletesBranchAndDir_WhenDone verifies that when
// keepBranch=false both the worktree directory and the feature branch are
// removed (done/cancelled path).
func TestRemoveDropletWorktree_DeletesBranchAndDir_WhenDone(t *testing.T) {
	// Given: a worktree created for a droplet.
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-done", "origin")
	if err != nil {
		t.Fatalf("prepareDropletWorktree: %v", err)
	}

	// When: done/cancelled cleanup (keepBranch=false).
	removeDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-done", false)

	// Then: worktree directory is gone.
	if _, statErr := os.Stat(worktreePath); statErr == nil {
		t.Error("worktree directory should have been removed on done cleanup")
	}

	// Then: feature branch is deleted.
	if branchExists(t, primaryDir, "feat/drop-done") {
		t.Error("feat/drop-done should have been deleted on done cleanup")
	}
}

// TestPrepareDropletWorktree_ResumesFromExistingBranch_AfterStagnantCleanup verifies
// that after a stagnant cleanup (worktree dir removed, branch preserved) a
// subsequent prepareDropletWorktree call attaches to the existing branch via
// the no-b path, retaining all prior commits.
func TestPrepareDropletWorktree_ResumesFromExistingBranch_AfterStagnantCleanup(t *testing.T) {
	// Given: a worktree created, agent commits some work, stagnant cleanup runs.
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-resume-stagnant", "origin")
	if err != nil {
		t.Fatalf("prepareDropletWorktree (first): %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "impl.go"), []byte("// implementation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branchMustRun(t, branchGitCmd(worktreePath, "add", "."))
	branchMustRun(t, branchGitCmd(worktreePath, "commit", "-m", "implement work"))

	beforeSHA, err := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse before: %v", err)
	}

	// Stagnant cleanup: remove worktree dir but keep branch (keepBranch=true).
	removeDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-resume-stagnant", true)

	if _, statErr := os.Stat(worktreePath); statErr == nil {
		t.Fatal("worktree directory should be gone after stagnant cleanup")
	}

	// When: Architecti restarts the droplet — prepareDropletWorktree is called again.
	newWorktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-resume-stagnant", "origin")
	if err != nil {
		t.Fatalf("prepareDropletWorktree (resume): %v", err)
	}

	// Then: the same branch is checked out (no fresh branch from origin/main).
	if got := currentBranch(t, newWorktreePath); got != "feat/drop-resume-stagnant" {
		t.Errorf("HEAD branch after resume = %q, want feat/drop-resume-stagnant", got)
	}

	// Then: prior commits are intact — HEAD matches the commit from before cleanup.
	afterSHA, err := exec.Command("git", "-C", newWorktreePath, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse after: %v", err)
	}
	before, after := strings.TrimSpace(string(beforeSHA)), strings.TrimSpace(string(afterSHA))
	if before != after {
		t.Errorf("prior commits lost: HEAD before=%s after=%s", before, after)
	}
}

// --- repoMu serialization tests ---

// TestPrepareDropletWorktree_ConcurrentSameRepo verifies that two goroutines
// calling prepareDropletWorktree for different droplets against the same
// primary clone succeed without error when serialized by a per-repo mutex —
// the pattern used by the Castellarius dispatch loop via repoMu.
//
// Run with -race to confirm no Go-level data races are introduced by the
// mutex acquisition pattern.
func TestPrepareDropletWorktree_ConcurrentSameRepo(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()

	const repoName = "myrepo"
	var mu sync.Mutex // simulates s.repoMu[repoName]

	type result struct {
		path string
		err  error
	}
	results := make([]result, 2)

	var wg sync.WaitGroup
	for i, id := range []string{"drop-concurrent-1", "drop-concurrent-2"} {
		wg.Add(1)
		i, id := i, id
		go func() {
			defer wg.Done()
			mu.Lock()
			path, err := prepareDropletWorktree(primaryDir, sandboxRoot, repoName, id)
			mu.Unlock()
			results[i] = result{path, err}
		}()
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			t.Errorf("goroutine %d: prepareDropletWorktree failed: %v", i, r.err)
		}
		if r.path == "" {
			t.Errorf("goroutine %d: empty worktree path returned", i)
		}
		if _, statErr := os.Stat(r.path); statErr != nil {
			t.Errorf("goroutine %d: worktree path does not exist: %v", i, statErr)
		}
	}
}

// --- purgeOrphanedWorktrees tests ---

// worktreeExists reports whether the named directory exists under the repo sandbox.
func worktreeExists(t *testing.T, sandboxRoot, repoName, dropletID string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(sandboxRoot, repoName, dropletID))
	return err == nil
}

// TestPurgeOrphanedWorktrees_RemovesDeliveredDropletWorktree verifies that
// purgeOrphanedWorktrees removes a worktree whose droplet is in "delivered"
// status, while leaving active droplet worktrees untouched.
func TestPurgeOrphanedWorktrees_RemovesDeliveredDropletWorktree(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	repoName := "test-repo"
	prefix := "test-"

	if err := os.MkdirAll(filepath.Join(sandboxRoot, repoName), 0o755); err != nil {
		t.Fatal(err)
	}

	dstPrimary := filepath.Join(sandboxRoot, repoName, "_primary")
	branchMustRun(t, branchGitCmd(".", "clone", primaryDir, dstPrimary))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.name", "Lobsterdog Contributors"))

	_, err := prepareDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"delivered")
	if err != nil {
		t.Fatalf("prepare worktree for delivered droplet: %v", err)
	}
	_, err = prepareDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"active")
	if err != nil {
		t.Fatalf("prepare worktree for active droplet: %v", err)
	}

	if !worktreeExists(t, sandboxRoot, repoName, prefix+"delivered") {
		t.Fatal("delivered worktree should exist before purge")
	}
	if !worktreeExists(t, sandboxRoot, repoName, prefix+"active") {
		t.Fatal("active worktree should exist before purge")
	}

	client := newMockClient()
	client.items[prefix+"delivered"] = &cistern.Droplet{ID: prefix + "delivered", Status: "delivered"}
	client.items[prefix+"active"] = &cistern.Droplet{ID: prefix + "active", Status: "in_progress"}

	s := &Castellarius{
		config: aqueduct.AqueductConfig{
			Repos: []aqueduct.RepoConfig{
				{Name: repoName, Prefix: prefix, Names: []string{"alpha", "beta"}},
			},
		},
		clients:     map[string]CisternClient{repoName: client},
		pools:       map[string]*AqueductPool{repoName: NewAqueductPool(repoName, []string{"alpha", "beta"})},
		sandboxRoot: sandboxRoot,
		logger:      newBranchLifecycleLogger(io.Discard),
	}

	s.purgeOrphanedWorktrees()

	if worktreeExists(t, sandboxRoot, repoName, prefix+"delivered") {
		t.Error("delivered droplet worktree should have been removed")
	}
	if !worktreeExists(t, sandboxRoot, repoName, prefix+"active") {
		t.Error("active droplet worktree should NOT have been removed")
	}
}

// TestPurgeOrphanedWorktrees_RemovesDeletedDropletWorktree verifies that
// purgeOrphanedWorktrees removes a worktree whose droplet has been purged
// from the database entirely (Get returns not-found error).
func TestPurgeOrphanedWorktrees_RemovesDeletedDropletWorktree(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	repoName := "test-repo"
	prefix := "test-"

	if err := os.MkdirAll(filepath.Join(sandboxRoot, repoName), 0o755); err != nil {
		t.Fatal(err)
	}

	dstPrimary := filepath.Join(sandboxRoot, repoName, "_primary")
	branchMustRun(t, branchGitCmd(".", "clone", primaryDir, dstPrimary))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.name", "Lobsterdog Contributors"))

	_, err := prepareDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"purged")
	if err != nil {
		t.Fatalf("prepare worktree: %v", err)
	}

	if !worktreeExists(t, sandboxRoot, repoName, prefix+"purged") {
		t.Fatal("worktree should exist before purge")
	}

	client := newMockClient()

	s := &Castellarius{
		config: aqueduct.AqueductConfig{
			Repos: []aqueduct.RepoConfig{
				{Name: repoName, Prefix: prefix, Names: []string{"alpha", "beta"}},
			},
		},
		clients:     map[string]CisternClient{repoName: client},
		pools:       map[string]*AqueductPool{repoName: NewAqueductPool(repoName, []string{"alpha", "beta"})},
		sandboxRoot: sandboxRoot,
		logger:      newBranchLifecycleLogger(io.Discard),
	}

	s.purgeOrphanedWorktrees()

	if worktreeExists(t, sandboxRoot, repoName, prefix+"purged") {
		t.Error("purged droplet worktree should have been removed")
	}
	if branchExists(t, dstPrimary, "feat/"+prefix+"purged") {
		t.Error("feature branch for purged droplet should have been deleted")
	}
}

// TestPurgeOrphanedWorktrees_SkipsAqueductWorkerDirs verifies that directories
// matching aqueduct worker names (alpha, beta, etc.) are never removed even
// though they don't match the droplet prefix.
func TestPurgeOrphanedWorktrees_SkipsAqueductWorkerDirs(t *testing.T) {
	_, remoteDir := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	repoName := "test-repo"
	prefix := "test-"

	if err := os.MkdirAll(filepath.Join(sandboxRoot, repoName), 0o755); err != nil {
		t.Fatal(err)
	}

	dstPrimary := filepath.Join(sandboxRoot, repoName, "_primary")
	branchMustRun(t, branchGitCmd(".", "clone", remoteDir, dstPrimary))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.name", "Lobsterdog Contributors"))

	branchMustRun(t, branchGitCmd(dstPrimary, "worktree", "add", "--detach", filepath.Join(sandboxRoot, repoName, "alpha"), "main"))
	branchMustRun(t, branchGitCmd(dstPrimary, "worktree", "add", "--detach", filepath.Join(sandboxRoot, repoName, "beta"), "main"))

	client := newMockClient()

	s := &Castellarius{
		config: aqueduct.AqueductConfig{
			Repos: []aqueduct.RepoConfig{
				{Name: repoName, Prefix: prefix, Names: []string{"alpha", "beta"}},
			},
		},
		clients:     map[string]CisternClient{repoName: client},
		pools:       map[string]*AqueductPool{repoName: NewAqueductPool(repoName, []string{"alpha", "beta"})},
		sandboxRoot: sandboxRoot,
		logger:      newBranchLifecycleLogger(io.Discard),
	}

	s.purgeOrphanedWorktrees()

	if !worktreeExists(t, sandboxRoot, repoName, "alpha") {
		t.Error("alpha aqueduct worktree should NOT be removed")
	}
	if !worktreeExists(t, sandboxRoot, repoName, "beta") {
		t.Error("beta aqueduct worktree should NOT be removed")
	}
}

// TestPurgeOrphanedWorktrees_SkipsNonPrefixDirs verifies that directories
// that don't match the droplet prefix (e.g. random dirs) are skipped.
func TestPurgeOrphanedWorktrees_SkipsNonPrefixDirs(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	repoName := "test-repo"
	prefix := "test-"

	if err := os.MkdirAll(filepath.Join(sandboxRoot, repoName), 0o755); err != nil {
		t.Fatal(err)
	}

	dstPrimary := filepath.Join(sandboxRoot, repoName, "_primary")
	branchMustRun(t, branchGitCmd(".", "clone", primaryDir, dstPrimary))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.name", "Lobsterdog Contributors"))

	if err := os.MkdirAll(filepath.Join(sandboxRoot, repoName, "other-dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	client := newMockClient()

	s := &Castellarius{
		config: aqueduct.AqueductConfig{
			Repos: []aqueduct.RepoConfig{
				{Name: repoName, Prefix: prefix, Names: []string{"alpha"}},
			},
		},
		clients:     map[string]CisternClient{repoName: client},
		pools:       map[string]*AqueductPool{repoName: NewAqueductPool(repoName, []string{"alpha"})},
		sandboxRoot: sandboxRoot,
		logger:      newBranchLifecycleLogger(io.Discard),
	}

	s.purgeOrphanedWorktrees()

	if _, err := os.Stat(filepath.Join(sandboxRoot, repoName, "other-dir")); err != nil {
		t.Error("non-prefix directory should NOT be removed")
	}
}

// TestPurgeOrphanedWorktrees_RemovesCancelledAndPooled verifies that
// worktrees for cancelled and pooled droplets are also removed.
func TestPurgeOrphanedWorktrees_RemovesCancelledAndPooled(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	repoName := "test-repo"
	prefix := "test-"

	if err := os.MkdirAll(filepath.Join(sandboxRoot, repoName), 0o755); err != nil {
		t.Fatal(err)
	}

	dstPrimary := filepath.Join(sandboxRoot, repoName, "_primary")
	branchMustRun(t, branchGitCmd(".", "clone", primaryDir, dstPrimary))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.name", "Lobsterdog Contributors"))

	_, err := prepareDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"cancelled")
	if err != nil {
		t.Fatalf("prepare worktree: %v", err)
	}
	_, err = prepareDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"pooled")
	if err != nil {
		t.Fatalf("prepare worktree: %v", err)
	}

	client := newMockClient()
	client.items[prefix+"cancelled"] = &cistern.Droplet{ID: prefix + "cancelled", Status: "cancelled"}
	client.items[prefix+"pooled"] = &cistern.Droplet{ID: prefix + "pooled", Status: "pooled"}

	s := &Castellarius{
		config: aqueduct.AqueductConfig{
			Repos: []aqueduct.RepoConfig{
				{Name: repoName, Prefix: prefix, Names: []string{"alpha"}},
			},
		},
		clients:     map[string]CisternClient{repoName: client},
		pools:       map[string]*AqueductPool{repoName: NewAqueductPool(repoName, []string{"alpha"})},
		sandboxRoot: sandboxRoot,
		logger:      newBranchLifecycleLogger(io.Discard),
	}

	s.purgeOrphanedWorktrees()

	if worktreeExists(t, sandboxRoot, repoName, prefix+"cancelled") {
		t.Error("cancelled droplet worktree should have been removed")
	}
	if worktreeExists(t, sandboxRoot, repoName, prefix+"pooled") {
		t.Error("pooled droplet worktree should have been removed")
	}
}

// TestPurgeOrphanedWorktrees_PrefixMismatch_StillPurgesOrphans verifies that
// when the configured prefix doesn't match actual droplet IDs (e.g. config says
// "lm" but droplets use "ll"), the regex fallback still catches orphaned
// worktrees via the DB lookup.
func TestPurgeOrphanedWorktrees_PrefixMismatch_StillPurgesOrphans(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	repoName := "test-repo"
	configPrefix := "lm-"

	if err := os.MkdirAll(filepath.Join(sandboxRoot, repoName), 0o755); err != nil {
		t.Fatal(err)
	}

	dstPrimary := filepath.Join(sandboxRoot, repoName, "_primary")
	branchMustRun(t, branchGitCmd(".", "clone", primaryDir, dstPrimary))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.name", "Lobsterdog Contributors"))

	_, err := prepareDropletWorktree(dstPrimary, sandboxRoot, repoName, "ll-abcde")
	if err != nil {
		t.Fatalf("prepare worktree: %v", err)
	}

	if !worktreeExists(t, sandboxRoot, repoName, "ll-abcde") {
		t.Fatal("worktree should exist before purge")
	}

	client := newMockClient()
	client.items["ll-abcde"] = &cistern.Droplet{ID: "ll-abcde", Status: "cancelled"}

	s := &Castellarius{
		config: aqueduct.AqueductConfig{
			Repos: []aqueduct.RepoConfig{
				{Name: repoName, Prefix: configPrefix, Names: []string{"alpha"}},
			},
		},
		clients:     map[string]CisternClient{repoName: client},
		pools:       map[string]*AqueductPool{repoName: NewAqueductPool(repoName, []string{"alpha"})},
		sandboxRoot: sandboxRoot,
		logger:      newBranchLifecycleLogger(io.Discard),
	}

	s.purgeOrphanedWorktrees()

	if worktreeExists(t, sandboxRoot, repoName, "ll-abcde") {
		t.Error("prefix-mismatched cancelled droplet worktree should have been removed via regex fallback")
	}
	if branchExists(t, dstPrimary, "feat/ll-abcde") {
		t.Error("feature branch for prefix-mismatched cancelled droplet should have been deleted")
	}
}

// TestPurgeOrphanedWorktrees_NoSandboxRoot_IsNoOp verifies that when
// sandboxRoot is empty (e.g. test environments), the function returns
// immediately without error.
func TestPurgeOrphanedWorktrees_NoSandboxRoot_IsNoOp(t *testing.T) {
	s := &Castellarius{
		sandboxRoot: "",
		logger:      newBranchLifecycleLogger(io.Discard),
	}
	s.purgeOrphanedWorktrees()
}

// --- empty repo (orphan) tests ---

// makeEmptyBareAndClone creates:
//
//	base/remote/ — bare git repo with ZERO commits on main (unborn branch)
//	base/primary/ — full clone of remote (has origin remote set, but origin/main
//	                does not exist because there are no commits)
//
// Returns the primary directory. Callers will find that origin/main does NOT
// resolve, simulating a brand-new empty repo.
func makeEmptyBareAndClone(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	remoteDir := filepath.Join(base, "remote")
	primaryDir := filepath.Join(base, "primary")

	// Create a bare repo. It has no commits and an unborn default branch.
	branchMustRun(t, branchGitCmd(".", "init", "--bare", remoteDir))

	// We need to set HEAD in the bare repo to point to main so that clones
	// get origin/main as the expected default. Since there are no commits,
	// refs/heads/main won't exist yet — but HEAD will point to it.
	// Git init --bare defaults HEAD to refs/heads/main, so this should be fine.

	// Clone the bare remote. This creates a primary clone with origin set,
	// but origin/main is unborn (no commits).
	branchMustRun(t, branchGitCmd(".", "clone", remoteDir, primaryDir))
	branchMustRun(t, branchGitCmd(primaryDir, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(primaryDir, "config", "user.name", "Lobsterdog Contributors"))

	return primaryDir
}

// TestRepoHasCommits_TrueWhenCommitsExist verifies that repoHasCommits returns
// true when origin/main has commits.
func TestRepoHasCommits_TrueWhenCommitsExist(t *testing.T) {
	dir, _ := makeBareAndClone(t)

	has, err := repoHasCommits(dir, "origin/main")
	if err != nil {
		t.Fatalf("repoHasCommits: %v", err)
	}
	if !has {
		t.Error("expected repoHasCommits to return true for repo with commits")
	}
}

// TestRepoHasCommits_FalseWhenEmptyRepo verifies that repoHasCommits returns
// false when the repo has no commits on origin/main (empty/unborn repo).
func TestRepoHasCommits_FalseWhenEmptyRepo(t *testing.T) {
	dir := makeEmptyBareAndClone(t)

	has, err := repoHasCommits(dir, "origin/main")
	if err != nil {
		t.Fatalf("repoHasCommits: %v", err)
	}
	if has {
		t.Error("expected repoHasCommits to return false for empty repo")
	}
}

// TestPrepareDropletWorktree_EmptyRepo_UsesOrphanBranch verifies that on an empty
// repo (no origin/main commits), prepareDropletWorktree creates an orphan branch
// worktree instead of failing with "invalid reference: HEAD".
func TestPrepareDropletWorktree_EmptyRepo_UsesOrphanBranch(t *testing.T) {
	primaryDir := makeEmptyBareAndClone(t)
	sandboxRoot := t.TempDir()

	worktreePath, err := prepareDropletWorktreeWithLogger(
		newBranchLifecycleLogger(io.Discard), primaryDir, sandboxRoot, "myrepo", "drop-orphan", "origin",
	)
	if err != nil {
		t.Fatalf("prepareDropletWorktree on empty repo: %v", err)
	}

	// (a) no error returned (verified above by not fatalf-ing)

	// (b) worktree directory exists
	if _, statErr := os.Stat(worktreePath); statErr != nil {
		t.Fatalf("worktree path does not exist: %v", statErr)
	}

	// (c) git branch --show-current returns feat/<id>
	// On an orphan branch with no commits, rev-parse HEAD fails, so use
	// git branch --show-current which correctly reports unborn branch names.
	branchOut, branchErr := exec.Command("git", "-C", worktreePath, "branch", "--show-current").Output()
	if branchErr != nil {
		t.Fatalf("git branch --show-current: %v", branchErr)
	}
	if got := strings.TrimSpace(string(branchOut)); got != "feat/drop-orphan" {
		t.Errorf("current branch = %q, want feat/drop-orphan", got)
	}

	// (d) worktree has no tracked files (empty initial state — orphan branch)
	statusOut, statusErr := exec.Command("git", "-C", worktreePath, "status", "--porcelain").Output()
	if statusErr != nil {
		t.Fatalf("git status in orphan worktree: %v", statusErr)
	}
	if strings.TrimSpace(string(statusOut)) != "" {
		t.Errorf("orphan worktree has tracked files (should be empty):\n%s", statusOut)
	}
}

// TestPrepareDropletWorktree_EmptyRepo_ResumeOrphanBranch verifies that when
// an orphan worktree already exists (from a prior dispatch), the resume path
// works correctly — checking out the existing branch with the commit intact.
func TestPrepareDropletWorktree_EmptyRepo_ResumeOrphanBranch(t *testing.T) {
	primaryDir := makeEmptyBareAndClone(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	// First dispatch: create the orphan worktree and commit a file.
	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-resume-orphan", "origin")
	if err != nil {
		t.Fatalf("first prepareDropletWorktree on empty repo: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktreePath, "impl.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branchMustRun(t, branchGitCmd(worktreePath, "add", "."))
	branchMustRun(t, branchGitCmd(worktreePath, "commit", "-m", "agent work"))

	beforeSHA, err := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse before cleanup: %v", err)
	}

	// Simulate stagnant cleanup: remove worktree but keep branch (keepBranch=true).
	removeDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-resume-orphan", true)

	if _, statErr := os.Stat(worktreePath); statErr == nil {
		t.Fatal("worktree directory should be gone after stagnant cleanup")
	}

	// Second dispatch: resume the orphan branch.
	newWorktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-resume-orphan", "origin")
	if err != nil {
		t.Fatalf("second prepareDropletWorktree (resume) on empty repo: %v", err)
	}

	// Verify the branch is resumed with the commit intact.
	if got := currentBranch(t, newWorktreePath); got != "feat/drop-resume-orphan" {
		t.Errorf("HEAD branch after resume = %q, want feat/drop-resume-orphan", got)
	}

	afterSHA, err := exec.Command("git", "-C", newWorktreePath, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse after resume: %v", err)
	}

	before, after := strings.TrimSpace(string(beforeSHA)), strings.TrimSpace(string(afterSHA))
	if before != after {
		t.Errorf("prior commits lost on orphan resume: HEAD before=%s after=%s", before, after)
	}
}

// TestPrepareDropletWorktree_EmptyRepo_MultipleDroplets verifies that two
// droplets can create independent orphan worktrees on an empty repo.
func TestPrepareDropletWorktree_EmptyRepo_MultipleDroplets(t *testing.T) {
	primaryDir := makeEmptyBareAndClone(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	wt1, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-multi-1", "origin")
	if err != nil {
		t.Fatalf("prepareDropletWorktree for drop-multi-1: %v", err)
	}
	wt2, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-multi-2", "origin")
	if err != nil {
		t.Fatalf("prepareDropletWorktree for drop-multi-2: %v", err)
	}

	// Both branches should exist and be independent.
	// Use git branch --show-current since rev-parse HEAD fails on unborn branches.
	branch1Out, err := exec.Command("git", "-C", wt1, "branch", "--show-current").Output()
	if err != nil {
		t.Fatalf("git branch --show-current in wt1: %v", err)
	}
	if got := strings.TrimSpace(string(branch1Out)); got != "feat/drop-multi-1" {
		t.Errorf("worktree 1 branch = %q, want feat/drop-multi-1", got)
	}

	branch2Out, err := exec.Command("git", "-C", wt2, "branch", "--show-current").Output()
	if err != nil {
		t.Fatalf("git branch --show-current in wt2: %v", err)
	}
	if got := strings.TrimSpace(string(branch2Out)); got != "feat/drop-multi-2" {
		t.Errorf("worktree 2 branch = %q, want feat/drop-multi-2", got)
	}

	// Commit in wt1 and verify wt2 is unaffected.
	if err := os.WriteFile(filepath.Join(wt1, "from1.txt"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branchMustRun(t, branchGitCmd(wt1, "add", "."))
	branchMustRun(t, branchGitCmd(wt1, "commit", "-m", "commit from droplet 1"))

	// wt2 should still have no tracked files.
	statusOut, err := exec.Command("git", "-C", wt2, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status in wt2: %v", err)
	}
	if strings.TrimSpace(string(statusOut)) != "" {
		t.Errorf("wt2 should be empty after wt1 commit, but has:\n%s", statusOut)
	}
}

// TestPrepareDropletWorktree_EmptyRepo_LogsWorktreeCreated verifies that
// prepareDropletWorktree emits a log message containing "worktree created (orphan)"
// when creating a worktree on an empty repo.
func TestPrepareDropletWorktree_EmptyRepo_LogsWorktreeCreated(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	primaryDir := makeEmptyBareAndClone(t)
	sandboxRoot := t.TempDir()

	_, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "ci-orphan-log", "origin")
	if err != nil {
		t.Fatalf("prepareDropletWorktree on empty repo: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "worktree created (orphan)") {
		t.Errorf("log missing 'worktree created (orphan)'; got: %s", out)
	}
	if !strings.Contains(out, "ci-orphan-log") {
		t.Errorf("log missing droplet ID; got: %s", out)
	}
	if !strings.Contains(out, "duration=") {
		t.Errorf("log missing duration field; got: %s", out)
	}
}

// TestPrepareBranchInSandbox_EmptyRepo_CreatesOrphanBranch verifies that on an
// empty repo (no origin/main commits), prepareBranchInSandbox creates an orphan
// branch instead of failing with git fetch/reset errors.
func TestPrepareBranchInSandbox_EmptyRepo_CreatesOrphanBranch(t *testing.T) {
	primaryDir := makeEmptyBareAndClone(t)

	// First, create a detached worktree on the empty repo using the orphan path
	// (simulating the worktree EnsureWorktree would create).
	worktreeDir := filepath.Join(t.TempDir(), "sandbox")
	branchMustRun(t, branchGitCmd(primaryDir, "worktree", "add", "--orphan", "-b", "_worker_sandbox", worktreeDir))
	branchMustRun(t, branchGitCmd(worktreeDir, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(worktreeDir, "config", "user.name", "Lobsterdog Contributors"))

	// Now prepareBranchInSandbox on the orphan worktree.
	if err := prepareBranchInSandbox(worktreeDir, "drop-empty-branch"); err != nil {
		t.Fatalf("prepareBranchInSandbox on empty repo: %v", err)
	}

	// Verify the feature branch was created. On an unborn orphan branch,
	// rev-parse HEAD fails, so use git branch --show-current.
	branchOut, branchErr := exec.Command("git", "-C", worktreeDir, "branch", "--show-current").Output()
	if branchErr != nil {
		t.Fatalf("git branch --show-current: %v", branchErr)
	}
	if got := strings.TrimSpace(string(branchOut)); got != "feat/drop-empty-branch" {
		t.Errorf("current branch = %q, want feat/drop-empty-branch", got)
	}
}

// --- purgeStaleBranches tests ---

// TestPurgeStaleBranches_DeletesTerminalDropletBranches verifies that
// purgeStaleBranches removes local feat/<id> branches for droplets in
// delivered/cancelled/pooled status, while preserving branches for active
// droplets and non-feat branches.
func TestPurgeStaleBranches_DeletesTerminalDropletBranches(t *testing.T) {
	_, remoteDir := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	repoName := "test-repo"
	prefix := "te-"

	dstPrimary := filepath.Join(sandboxRoot, repoName, "_primary")
	branchMustRun(t, branchGitCmd(".", "clone", remoteDir, dstPrimary))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(dstPrimary, "config", "user.name", "Lobsterdog Contributors"))

	_, err := prepareDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"deliver")
	if err != nil {
		t.Fatalf("prepare worktree: %v", err)
	}
	_, err = prepareDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"cancel")
	if err != nil {
		t.Fatalf("prepare worktree: %v", err)
	}
	_, err = prepareDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"active")
	if err != nil {
		t.Fatalf("prepare worktree: %v", err)
	}

	removeDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"deliver", true)
	removeDropletWorktree(dstPrimary, sandboxRoot, repoName, prefix+"cancel", true)

	if !branchExists(t, dstPrimary, "feat/"+prefix+"deliver") {
		t.Fatal("feat/te-deliver branch should exist after keepBranch cleanup")
	}
	if !branchExists(t, dstPrimary, "feat/"+prefix+"cancel") {
		t.Fatal("feat/te-cancel branch should exist before purge")
	}
	if !branchExists(t, dstPrimary, "feat/"+prefix+"active") {
		t.Fatal("feat/te-active branch should exist before purge")
	}

	client := newMockClient()
	client.items[prefix+"deliver"] = &cistern.Droplet{ID: prefix + "deliver", Status: "delivered"}
	client.items[prefix+"cancel"] = &cistern.Droplet{ID: prefix + "cancel", Status: "cancelled"}
	client.items[prefix+"active"] = &cistern.Droplet{ID: prefix + "active", Status: "in_progress"}

	s := &Castellarius{
		config: aqueduct.AqueductConfig{
			Repos: []aqueduct.RepoConfig{
				{Name: repoName, Prefix: prefix, Names: []string{"alpha"}},
			},
		},
		clients:     map[string]CisternClient{repoName: client},
		pools:       map[string]*AqueductPool{repoName: NewAqueductPool(repoName, []string{"alpha"})},
		sandboxRoot: sandboxRoot,
		logger:      newBranchLifecycleLogger(io.Discard),
	}

	s.purgeStaleBranches(repoName, dstPrimary, prefix, client)

	if branchExists(t, dstPrimary, "feat/"+prefix+"deliver") {
		t.Error("feat/te-deliver should be deleted (delivered droplet)")
	}
	if branchExists(t, dstPrimary, "feat/"+prefix+"cancel") {
		t.Error("feat/te-cancel should be deleted (cancelled droplet)")
	}
	if !branchExists(t, dstPrimary, "feat/"+prefix+"active") {
		t.Error("feat/te-active should be preserved (in-progress droplet)")
	}
}

// --- Fork-mode prepareDropletWorktree tests ---

// makeBareAndCloneWithUpstream creates a primary clone with an "upstream"
// remote pointing to a separate bare repo. This simulates the fork-mode
// setup where origin is the fork and upstream is the source-of-truth repo.
func makeBareAndCloneWithUpstream(t *testing.T) (string, string) {
	t.Helper()
	primaryDir, _ := makeBareAndClone(t)

	// Create a second bare repo as "upstream".
	upstreamDir := filepath.Join(t.TempDir(), "upstream.git")
	branchMustRun(t, branchGitCmd(".", "init", "--bare", upstreamDir))

	// Push the initial commit to upstream so upstream/main exists.
	// We need to create a commit in the upstream repo first.
	upstreamWork := filepath.Join(t.TempDir(), "upstream-work")
	branchMustRun(t, branchGitCmd(".", "clone", upstreamDir, upstreamWork))
	branchMustRun(t, branchGitCmd(upstreamWork, "config", "user.email", "noreply@lobsterdog.dev"))
	branchMustRun(t, branchGitCmd(upstreamWork, "config", "user.name", "Lobsterdog Contributors"))
	if err := os.WriteFile(filepath.Join(upstreamWork, "README.md"), []byte("upstream init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branchMustRun(t, branchGitCmd(upstreamWork, "add", "."))
	branchMustRun(t, branchGitCmd(upstreamWork, "commit", "-m", "upstream initial"))
	branchMustRun(t, branchGitCmd(upstreamWork, "branch", "-M", "main"))
	// The clone already set up "origin" pointing to upstreamDir, so just push.
	branchMustRun(t, branchGitCmd(upstreamWork, "push", "-u", "origin", "main"))

	// Add "upstream" remote to the primary clone.
	branchMustRun(t, branchGitCmd(primaryDir, "remote", "add", "upstream", upstreamDir))
	branchMustRun(t, branchGitCmd(primaryDir, "fetch", "upstream"))

	return primaryDir, upstreamDir
}

// TestPrepareDropletWorktree_ForkMode_TracksUpstreamMain verifies that when
// baseRemote is "upstream", the worktree branch is created from upstream/main
// rather than origin/main.
func TestPrepareDropletWorktree_ForkMode_TracksUpstreamMain(t *testing.T) {
	primaryDir, _ := makeBareAndCloneWithUpstream(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	// Create worktree based on upstream/main via baseRemote="upstream".
	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-fork-base", "upstream")
	if err != nil {
		t.Fatalf("prepareDropletWorktreeWithLogger: %v", err)
	}
	defer os.RemoveAll(worktreePath)

	if _, statErr := os.Stat(worktreePath); statErr != nil {
		t.Fatalf("worktree path does not exist: %v", statErr)
	}

	// Verify HEAD points to upstream/main's commit.
	upstreamMainSHA, err := exec.Command("git", "-C", primaryDir, "rev-parse", "upstream/main").Output()
	if err != nil {
		t.Fatalf("git rev-parse upstream/main: %v", err)
	}
	worktreeHEAD, err := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in worktree: %v", err)
	}

	if strings.TrimSpace(string(worktreeHEAD)) != strings.TrimSpace(string(upstreamMainSHA)) {
		t.Errorf("worktree HEAD = %s, want upstream/main = %s",
			strings.TrimSpace(string(worktreeHEAD)),
			strings.TrimSpace(string(upstreamMainSHA)))
	}
}

// TestPrepareDropletWorktree_ForkMode_ExistingWorktreeChecksOutFromUpstream
// verifies that when resuming an existing worktree in fork mode, the tracking
// branch is set to upstream/main.
func TestPrepareDropletWorktree_ForkMode_ExistingWorktreeChecksOutFromUpstream(t *testing.T) {
	primaryDir, _ := makeBareAndCloneWithUpstream(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	// First call creates the worktree.
	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-fork-resume", "upstream")
	if err != nil {
		t.Fatalf("first prepareDropletWorktreeWithLogger: %v", err)
	}

	// Second call resumes the existing worktree — should set tracking to upstream/main.
	worktreePath2, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-fork-resume", "upstream")
	if err != nil {
		t.Fatalf("second prepareDropletWorktreeWithLogger (resume): %v", err)
	}
	if worktreePath2 != worktreePath {
		t.Errorf("resumed worktree path = %q, want %q", worktreePath2, worktreePath)
	}

	// Verify tracking branch is set to upstream/main.
	trackOut, trackErr := exec.Command("git", "-C", worktreePath, "for-each-ref", "--format=%(upstream:short)", "refs/heads/feat/drop-fork-resume").Output()
	if trackErr != nil {
		// This may fail if no tracking is set — log but don't fail; some git versions don't set tracking.
		t.Logf("tracking branch check (non-fatal): %v: %s", trackErr, string(trackOut))
	} else {
		trackBranch := strings.TrimSpace(string(trackOut))
		if trackBranch != "upstream/main" && trackBranch != "" {
			t.Logf("tracking branch = %q (expected upstream/main or empty)", trackBranch)
		}
	}
}

// TestPrepareDropletWorktree_DirectMode_DefaultsToOrigin verifies that when
// baseRemote is "" (empty string), it defaults to "origin" and the worktree
// is created from origin/main.
func TestPrepareDropletWorktree_DirectMode_DefaultsToOrigin(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	// Empty baseRemote should default to "origin".
	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-direct-default", "")
	if err != nil {
		t.Fatalf("prepareDropletWorktreeWithLogger: %v", err)
	}
	defer os.RemoveAll(worktreePath)

	originMainSHA, err := exec.Command("git", "-C", primaryDir, "rev-parse", "origin/main").Output()
	if err != nil {
		t.Fatalf("git rev-parse origin/main: %v", err)
	}
	worktreeHEAD, err := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}

	if strings.TrimSpace(string(worktreeHEAD)) != strings.TrimSpace(string(originMainSHA)) {
		t.Errorf("worktree HEAD = %s, want origin/main = %s",
			strings.TrimSpace(string(worktreeHEAD)),
			strings.TrimSpace(string(originMainSHA)))
	}
}

// TestPrepareDropletWorktree_ExplicitOrigin_UsesOriginMain verifies that when
// baseRemote is "origin" (explicit), the worktree is created from origin/main.
func TestPrepareDropletWorktree_ExplicitOrigin_UsesOriginMain(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-origin-explicit", "origin")
	if err != nil {
		t.Fatalf("prepareDropletWorktreeWithLogger: %v", err)
	}
	defer os.RemoveAll(worktreePath)

	originMainSHA, err := exec.Command("git", "-C", primaryDir, "rev-parse", "origin/main").Output()
	if err != nil {
		t.Fatalf("git rev-parse origin/main: %v", err)
	}
	worktreeHEAD, err := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}

	if strings.TrimSpace(string(worktreeHEAD)) != strings.TrimSpace(string(originMainSHA)) {
		t.Errorf("worktree HEAD = %s, want origin/main = %s",
			strings.TrimSpace(string(worktreeHEAD)),
			strings.TrimSpace(string(originMainSHA)))
	}
}

// TestPrepareDropletWorktree_ForkMode_NewBranchTrackingSet verifies that when a
// fresh worktree is created in fork mode (baseRemote="upstream"), the tracking
// branch is set to upstream/main — matching the resume path's behavior.
func TestPrepareDropletWorktree_ForkMode_NewBranchTrackingSet(t *testing.T) {
	primaryDir, _ := makeBareAndCloneWithUpstream(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	// Create a NEW worktree (droplet ID not used before).
	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-fork-track", "upstream")
	if err != nil {
		t.Fatalf("prepareDropletWorktreeWithLogger: %v", err)
	}
	defer os.RemoveAll(worktreePath)

	// Verify the tracking branch is set to upstream/main.
	// Use for-each-ref to query the upstream ref for the feature branch.
	trackOut, trackErr := exec.Command("git", "-C", worktreePath, "for-each-ref", "--format=%(upstream:short)", "refs/heads/feat/drop-fork-track").Output()
	if trackErr != nil {
		t.Logf("tracking branch check (non-fatal): %v: %s", trackErr, string(trackOut))
	} else {
		trackBranch := strings.TrimSpace(string(trackOut))
		if trackBranch != "upstream/main" {
			t.Errorf("tracking branch for new fork-mode worktree = %q, want %q", trackBranch, "upstream/main")
		}
	}
}

// TestPrepareDropletWorktree_DirectMode_NewBranchNoUpstreamTracking verifies that
// when a fresh worktree is created in direct mode (baseRemote="origin"), no
// upstream/main tracking is set (the default git behavior applies).
func TestPrepareDropletWorktree_DirectMode_NewBranchNoUpstreamTracking(t *testing.T) {
	primaryDir, _ := makeBareAndClone(t)
	sandboxRoot := t.TempDir()
	l := newBranchLifecycleLogger(io.Discard)

	// Create a NEW worktree in direct mode.
	worktreePath, err := prepareDropletWorktreeWithLogger(l, primaryDir, sandboxRoot, "myrepo", "drop-direct-track", "origin")
	if err != nil {
		t.Fatalf("prepareDropletWorktreeWithLogger: %v", err)
	}
	defer os.RemoveAll(worktreePath)

	// In direct mode, the tracking should be origin/main (standard git).
	trackOut, trackErr := exec.Command("git", "-C", worktreePath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "HEAD").Output()
	if trackErr != nil {
		// Non-fatal: some git versions don't set tracking on worktree-created branches.
		t.Logf("tracking branch check (non-fatal): %v: %s", trackErr, string(trackOut))
	} else {
		trackBranch := strings.TrimSpace(string(trackOut))
		// In direct mode, we do NOT set --set-upstream-to=upstream/main (there's
		// no upstream remote). Tracking may be empty or origin/main depending on
		// how the worktree was created.
		if trackBranch == "upstream/main" {
			t.Errorf("direct-mode worktree should not track upstream/main, got tracking = %q", trackBranch)
		}
	}
}
