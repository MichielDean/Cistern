package cataractae

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EnsurePrimaryClone guarantees a full git clone exists at primaryDir.
// This is the shared object store for all worktrees in the same repo.
//
// On first call: clones the repo.
// On subsequent calls: fetches latest remote refs.
func EnsurePrimaryClone(primaryDir, repoURL string) error {
	gitDir := filepath.Join(primaryDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := os.RemoveAll(primaryDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale dir %s: %w", primaryDir, err)
		}
		t0 := time.Now()
		if err := cloneSandbox(primaryDir, repoURL); err != nil {
			slog.Default().Error("git clone failed",
				"dir", primaryDir, "duration", time.Since(t0).Round(time.Millisecond).String(), "error", err)
			return err
		}
		slog.Default().Info("git clone completed",
			"dir", primaryDir, "duration", time.Since(t0).Round(time.Millisecond).String())
		return nil
	}
	t0 := time.Now()
	if err := fetchSandbox(primaryDir); err != nil {
		slog.Default().Error("git fetch failed",
			"dir", primaryDir, "duration", time.Since(t0).Round(time.Millisecond).String(), "error", err)
		return err
	}
	slog.Default().Info("git fetch completed",
		"dir", primaryDir, "duration", time.Since(t0).Round(time.Millisecond).String())
	return nil
}

// EnsureWorktree ensures a git worktree exists at worktreeDir backed by primaryDir.
// It prunes stale worktree registrations first to prevent "already in use" errors.
// If worktreeDir exists as a legacy dedicated clone (not a registered worktree), it
// is removed so a fresh worktree can be created.
func EnsureWorktree(primaryDir, worktreeDir string) error {
	// Prune stale registrations so dead paths don't block re-registration.
	prune := exec.Command("git", "worktree", "prune", "--expire=0")
	prune.Dir = primaryDir
	if out, err := prune.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree prune in %s: %w: %s", primaryDir, err, out)
	}

	// Check if this worktree is already registered.
	list := exec.Command("git", "worktree", "list", "--porcelain")
	list.Dir = primaryDir
	out, err := list.Output()
	if err != nil {
		return fmt.Errorf("git worktree list in %s: %w", primaryDir, err)
	}

	absPath, err := filepath.Abs(worktreeDir)
	if err != nil {
		return fmt.Errorf("abs path %s: %w", worktreeDir, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "worktree "+absPath {
			return nil // already registered
		}
	}

	// If the directory exists but is not a registered worktree (e.g., a legacy
	// dedicated clone), remove it so git worktree add can create a fresh worktree.
	if _, err := os.Stat(worktreeDir); err == nil {
		if err := os.RemoveAll(worktreeDir); err != nil {
			return fmt.Errorf("remove legacy clone at %s: %w", worktreeDir, err)
		}
	}

	t0 := time.Now()
	add := exec.Command("git", "worktree", "add", "--detach", worktreeDir)
	add.Dir = primaryDir
	if addOut, err := add.CombinedOutput(); err != nil {
		slog.Default().Error("git worktree add failed",
			"dir", worktreeDir, "duration", time.Since(t0).Round(time.Millisecond).String(), "error", err)
		return fmt.Errorf("git worktree add --detach %s: %w: %s", worktreeDir, err, addOut)
	}
	slog.Default().Info("git worktree created",
		"dir", worktreeDir, "duration", time.Since(t0).Round(time.Millisecond).String())
	return nil
}

// cloneSandbox performs a fresh git clone into dir.
func cloneSandbox(dir, repoURL string) error {
	if repoURL == "" {
		return fmt.Errorf("repo URL is required for initial clone")
	}
	cmd := exec.Command("git", "clone", repoURL, dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", repoURL, err)
	}
	return nil
}

// fetchSandbox fetches latest remote refs without touching the working tree.
func fetchSandbox(dir string) error {
	fetch := exec.Command("git", "fetch", "origin")
	fetch.Dir = dir
	if out, err := fetch.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch in %s: %w: %s", dir, err, out)
	}
	return nil
}

// currentHead returns the current HEAD commit hash in the given directory.
func currentHead(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}
