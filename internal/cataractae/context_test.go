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

// TestWriteContextFile_ManualNotes_ShownSeparately verifies that notes with
// CataractaeName=="manual" appear in a dedicated "## Manual Notes" section
// and are NOT silently dropped by the step-name filter (ci-tgj96).
//
// Given: notes from implement, deliver, and manual sources
// When:  writeContextFile is called with step "implement"
// Then:  manual notes appear in "## Manual Notes", implement notes appear in
//
//	"## Recent Step Notes", and deliver notes appear in neither section.
func TestWriteContextFile_ManualNotes_ShownSeparately(t *testing.T) {
	item := &cistern.Droplet{ID: "ci-test3", Title: "Test manual notes", Status: "in_progress"}
	step := &aqueduct.WorkflowCataractae{Name: "implement", Type: "agent"}

	notes := []cistern.CataractaeNote{
		{CataractaeName: "manual", Content: "operator annotation: critical refinement needed"},
		{CataractaeName: "implement", Content: "implement note A"},
		{CataractaeName: "deliver", Content: "deliver note X"},
		{CataractaeName: "manual", Content: "operator annotation: second note"},
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

	// Manual notes must appear in the dedicated Manual Notes section.
	manualIdx := strings.Index(got, "## Manual Notes")
	if manualIdx == -1 {
		t.Fatal("'## Manual Notes' section not found — manual notes would be invisible (regression: ci-tgj96)")
	}
	manualSection := got[manualIdx:]
	if !strings.Contains(manualSection, "operator annotation: critical refinement needed") {
		t.Error("expected first manual note in '## Manual Notes' section")
	}
	if !strings.Contains(manualSection, "operator annotation: second note") {
		t.Error("expected second manual note in '## Manual Notes' section")
	}

	// Own-step notes must still appear in Recent Step Notes.
	recentIdx := strings.Index(got, "## Recent Step Notes")
	if recentIdx == -1 {
		t.Fatal("'## Recent Step Notes' section not found")
	}
	if !strings.Contains(got[recentIdx:], "implement note A") {
		t.Error("expected own-cataractae note 'implement note A' in '## Recent Step Notes'")
	}

	// Cross-cataractae step notes must not appear anywhere.
	if strings.Contains(got, "deliver note X") {
		t.Error("cross-cataractae note 'deliver note X' must not appear anywhere in CONTEXT.md")
	}
}

// TestWriteContextFile_SchedulerNotes_ShownSeparately verifies that notes with
// CataractaeName=="scheduler" appear in a dedicated "## Scheduler Notes" section
// and are NOT silently dropped by the step-name filter (ci-tgj96).
//
// Given: notes from implement, deliver, manual, and scheduler sources
// When:  writeContextFile is called with step "implement"
// Then:  scheduler notes appear in "## Scheduler Notes", implement notes appear
//
//	in "## Recent Step Notes", manual notes in "## Manual Notes", and
//	deliver notes appear in none of these sections.
func TestWriteContextFile_SchedulerNotes_ShownSeparately(t *testing.T) {
	item := &cistern.Droplet{ID: "ci-test4", Title: "Test scheduler notes", Status: "in_progress"}
	step := &aqueduct.WorkflowCataractae{Name: "implement", Type: "agent"}

	notes := []cistern.CataractaeNote{
		{CataractaeName: "scheduler", Content: "scheduler: zombie detected, recirculating"},
		{CataractaeName: "implement", Content: "implement note A"},
		{CataractaeName: "deliver", Content: "deliver note X"},
		{CataractaeName: "scheduler", Content: "scheduler: timeout notice"},
		{CataractaeName: "manual", Content: "operator annotation"},
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

	// Scheduler notes must appear in the dedicated Scheduler Notes section.
	schedulerIdx := strings.Index(got, "## Scheduler Notes")
	if schedulerIdx == -1 {
		t.Fatal("'## Scheduler Notes' section not found — scheduler notes would be invisible (regression: ci-tgj96)")
	}
	schedulerSection := got[schedulerIdx:]
	if !strings.Contains(schedulerSection, "scheduler: zombie detected, recirculating") {
		t.Error("expected first scheduler note in '## Scheduler Notes' section")
	}
	if !strings.Contains(schedulerSection, "scheduler: timeout notice") {
		t.Error("expected second scheduler note in '## Scheduler Notes' section")
	}

	// Own-step notes must still appear in Recent Step Notes.
	recentIdx := strings.Index(got, "## Recent Step Notes")
	if recentIdx == -1 {
		t.Fatal("'## Recent Step Notes' section not found")
	}
	if !strings.Contains(got[recentIdx:], "implement note A") {
		t.Error("expected own-cataractae note 'implement note A' in '## Recent Step Notes'")
	}

	// Manual notes must still appear in Manual Notes section.
	if !strings.Contains(got, "## Manual Notes") {
		t.Error("'## Manual Notes' section not found")
	}
	if !strings.Contains(got, "operator annotation") {
		t.Error("expected manual note 'operator annotation' in CONTEXT.md")
	}

	// Cross-cataractae step notes must not appear anywhere.
	if strings.Contains(got, "deliver note X") {
		t.Error("cross-cataractae note 'deliver note X' must not appear anywhere in CONTEXT.md")
	}
}
