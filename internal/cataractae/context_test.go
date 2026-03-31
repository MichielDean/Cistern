package cataractae

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/aqueduct"
	"github.com/MichielDean/cistern/internal/cistern"
)

// runGit runs a git command in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// initTestRepo creates a git repo with an initial commit and sets
// refs/remotes/origin/main to HEAD — simulating a fetched remote without
// needing a real one. Returns the temp directory path.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	runGit(t, dir, "update-ref", "refs/remotes/origin/main", runGit(t, dir, "rev-parse", "HEAD"))

	return dir
}

// TestGenerateDiff_NonEmptyWithChanges is an end-to-end regression test for
// ci-s5eg9: review got an empty diff.patch because generateDiff
// was called on the worker's own sandbox (on main) instead of the per-droplet
// worktree (on feat/<id> with committed changes).
//
// This test verifies that generateDiff produces a non-empty diff when the
// sandbox directory contains committed changes on a feature branch vs
// origin/main. It is the "closed loop" counterpart to
// TestDispatch_DiffOnlyStepGetsSandboxDir, which only verifies that the
// correct path is passed — not that the diff itself is non-empty.
func TestGenerateDiff_NonEmptyWithChanges(t *testing.T) {
	dir := initTestRepo(t)

	// Create feature branch and commit a new file — simulates an implementer pass.
	runGit(t, dir, "checkout", "-b", "feat/ci-s5eg9-test")
	if err := os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: add feature.go")

	// generateDiff must return a non-empty diff containing the new file.
	diff, err := generateDiff(dir)
	if err != nil {
		t.Fatalf("generateDiff: %v", err)
	}
	if len(diff) == 0 {
		t.Fatal("generateDiff returned empty diff — diff_only reviewer would see empty diff.patch (regression: ci-s5eg9)")
	}
	if !strings.Contains(string(diff), "feature.go") {
		t.Errorf("generateDiff output should contain 'feature.go'; got:\n%s", diff)
	}
}

// TestGenerateDiff_EmptyOnMain verifies that generateDiff returns an empty
// diff (not an error) when the sandbox is on the same commit as origin/main.
// This is a boundary test: no-changes produces empty bytes, not an error.
// The actual regression guard for ci-s5eg9 is TestGenerateDiff_NonEmptyWithChanges.
func TestGenerateDiff_EmptyOnMain(t *testing.T) {
	dir := initTestRepo(t)

	diff, err := generateDiff(dir)
	if err != nil {
		t.Fatalf("generateDiff: %v", err)
	}
	if len(diff) != 0 {
		t.Errorf("expected empty diff when HEAD == origin/main; got %d bytes:\n%s", len(diff), diff)
	}
}

// TestWriteContextFile_RecentStepNotes_ExcludesOtherCataractae verifies that the
// 'Recent Step Notes' section only contains notes from the current step's cataractae,
// not notes from other steps (ci-tgj96).
//
// Given: notes from multiple cataractae (implement + deliver)
// When:  writeContextFile is called with step "implement"
// Then:  'Recent Step Notes' contains only the implement notes, not deliver notes
func TestWriteContextFile_RecentStepNotes_ExcludesOtherCataractae(t *testing.T) {
	item := &cistern.Droplet{ID: "ci-test", Title: "Test item", Status: "in_progress"}
	step := &aqueduct.WorkflowCataractae{Name: "implement", Type: "agent"}

	// Notes ordered newest-first; "deliver" notes are foreign to the current step.
	notes := []cistern.CataractaeNote{
		{CataractaeName: "implement", Content: "implement note A"},
		{CataractaeName: "deliver", Content: "deliver note X"},
		{CataractaeName: "implement", Content: "implement note B"},
		{CataractaeName: "deliver", Content: "deliver note Y"},
	}

	p := ContextParams{
		Item:  item,
		Step:  step,
		Notes: notes,
	}

	ctxPath := filepath.Join(t.TempDir(), "CONTEXT.md")
	if err := writeContextFile(ctxPath, p); err != nil {
		t.Fatalf("writeContextFile: %v", err)
	}

	content, err := os.ReadFile(ctxPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)

	// Extract only the Recent Step Notes section to avoid false matches in other sections.
	sectionStart := strings.Index(got, "## Recent Step Notes")
	if sectionStart == -1 {
		t.Fatal("'## Recent Step Notes' section not found in CONTEXT.md")
	}
	// Trim everything before the section.
	section := got[sectionStart:]

	if !strings.Contains(section, "implement note A") {
		t.Error("expected own-cataractae note 'implement note A' in Recent Step Notes")
	}
	if !strings.Contains(section, "implement note B") {
		t.Error("expected own-cataractae note 'implement note B' in Recent Step Notes")
	}
	if strings.Contains(section, "deliver note X") {
		t.Error("cross-cataractae note 'deliver note X' must not appear in Recent Step Notes")
	}
	if strings.Contains(section, "deliver note Y") {
		t.Error("cross-cataractae note 'deliver note Y' must not appear in Recent Step Notes")
	}
}

// TestWriteContextFile_RecentStepNotes_EmptyWhenNoOwnNotes verifies that the
// 'Recent Step Notes' section is omitted entirely when the current cataractae
// has no prior notes (ci-tgj96).
//
// Given: notes exist only from a different cataractae ("deliver")
// When:  writeContextFile is called with step "implement"
// Then:  no 'Recent Step Notes' section appears at all
func TestWriteContextFile_RecentStepNotes_EmptyWhenNoOwnNotes(t *testing.T) {
	item := &cistern.Droplet{ID: "ci-test2", Title: "Test item 2", Status: "in_progress"}
	step := &aqueduct.WorkflowCataractae{Name: "implement", Type: "agent"}

	notes := []cistern.CataractaeNote{
		{CataractaeName: "deliver", Content: "deliver note only"},
	}

	p := ContextParams{
		Item:  item,
		Step:  step,
		Notes: notes,
	}

	ctxPath := filepath.Join(t.TempDir(), "CONTEXT.md")
	if err := writeContextFile(ctxPath, p); err != nil {
		t.Fatalf("writeContextFile: %v", err)
	}

	content, err := os.ReadFile(ctxPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)

	if strings.Contains(got, "## Recent Step Notes") {
		t.Error("'## Recent Step Notes' section must not appear when no own-cataractae notes exist")
	}
	if strings.Contains(got, "deliver note only") {
		t.Error("cross-cataractae note 'deliver note only' must not appear anywhere in Recent Step Notes")
	}
}
