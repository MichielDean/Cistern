package cistern

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	c, err := New(dbPath, "bf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestNew_CreatesDB(t *testing.T) {
	c := testClient(t)
	if c.db == nil {
		t.Fatal("expected non-nil db")
	}
	if c.prefix != "bf" {
		t.Errorf("prefix = %q, want %q", c.prefix, "bf")
	}
}

// TestNew_FreshDB_DefaultComplexityIsTwo verifies that a fresh database created
// from schema.sql defaults the complexity column to 2 (full), not 3 (critical).
func TestNew_FreshDB_DefaultComplexityIsTwo(t *testing.T) {
	c := testClient(t)
	// Add a droplet, then fetch it back directly to check the schema-level default
	// is not involved (Add validates and stores explicitly). Instead, verify the
	// schema default by inserting a row without specifying complexity.
	_, err := c.db.Exec(`INSERT INTO droplets (id, repo, title) VALUES ('bf-test1', 'repo', 'title')`)
	if err != nil {
		t.Fatal(err)
	}
	var got int
	if err := c.db.QueryRow(`SELECT complexity FROM droplets WHERE id = 'bf-test1'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("schema DEFAULT complexity = %d, want 2 (full)", got)
	}
}

// TestNew_ComplexityMigration_RemapsOldSchemeValues verifies that when New() is
// called on a DB containing old-scheme complexity values (1=trivial, 2=standard,
// 3=full, 4=critical), they are remapped to the new scheme (1=standard, 2=full,
// 3=critical).
func TestNew_ComplexityMigration_RemapsOldSchemeValues(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate.db")

	// Seed the DB with old-scheme complexity values using the raw driver,
	// bypassing New() so the migration has not yet run.
	seedDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = seedDB.Exec(`CREATE TABLE IF NOT EXISTS droplets (
		id TEXT PRIMARY KEY,
		repo TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		priority INTEGER DEFAULT 2,
		complexity INTEGER DEFAULT 3,
		status TEXT DEFAULT 'open',
		assignee TEXT DEFAULT '',
		current_cataractae TEXT DEFAULT '',
		outcome TEXT DEFAULT NULL,
		assigned_aqueduct TEXT DEFAULT '',
		last_reviewed_commit TEXT DEFAULT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatal(err)
	}
	// Insert rows with old-scheme values: trivial=1, standard=2, full=3, critical=4.
	for _, row := range []struct {
		id         string
		complexity int
	}{
		{"old-trivial", 1},
		{"old-standard", 2},
		{"old-full", 3},
		{"old-critical", 4},
	} {
		if _, err := seedDB.Exec(`INSERT INTO droplets (id, repo, title, complexity) VALUES (?, 'r', 't', ?)`, row.id, row.complexity); err != nil {
			t.Fatal(err)
		}
	}
	seedDB.Close()

	// Open with New() to trigger the migration.
	c, err := New(dbPath, "bf")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Given: old scheme 1=trivial → new scheme 1=standard (unchanged, trivial removed)
	// Given: old scheme 2=standard → new scheme 1=standard
	// Given: old scheme 3=full    → new scheme 2=full
	// Given: old scheme 4=critical → new scheme 3=critical
	cases := []struct {
		id      string
		wantNew int
	}{
		{"old-trivial", 1},
		{"old-standard", 1},
		{"old-full", 2},
		{"old-critical", 3},
	}
	for _, tc := range cases {
		var got int
		if err := c.db.QueryRow(`SELECT complexity FROM droplets WHERE id = ?`, tc.id).Scan(&got); err != nil {
			t.Fatalf("id=%s: %v", tc.id, err)
		}
		if got != tc.wantNew {
			t.Errorf("id=%s: complexity after migration = %d, want %d", tc.id, got, tc.wantNew)
		}
	}
}

func TestGenerateID(t *testing.T) {
	c := testClient(t)
	id, err := c.generateID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "bf-") {
		t.Errorf("id = %q, want prefix %q", id, "bf-")
	}
	if len(id) != 8 { // "bf-" (3) + 5 chars
		t.Errorf("id length = %d, want 8", len(id))
	}

	// IDs should be unique.
	ids := map[string]bool{}
	for range 100 {
		id, err := c.generateID()
		if err != nil {
			t.Fatal(err)
		}
		if ids[id] {
			t.Fatalf("duplicate id: %s", id)
		}
		ids[id] = true
	}
}

func TestAdd_And_Get(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("github.com/org/repo", "Fix bug", "Details here", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if item.Status != "open" {
		t.Errorf("status = %q, want %q", item.Status, "open")
	}
	if item.Priority != 1 {
		t.Errorf("priority = %d, want 1", item.Priority)
	}

	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Fix bug" {
		t.Errorf("title = %q, want %q", got.Title, "Fix bug")
	}
	if got.Description != "Details here" {
		t.Errorf("description = %q, want %q", got.Description, "Details here")
	}
}

func TestGetReady_PriorityOrdering(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Low priority", "", 3, 3)
	c.Add("myrepo", "High priority", "", 1, 3)
	c.Add("myrepo", "Medium priority", "", 2, 3)

	item, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if item == nil {
		t.Fatal("expected item")
	}
	if item.Title != "High priority" {
		t.Errorf("title = %q, want %q", item.Title, "High priority")
	}
}

func TestGetReady_RepoFilter(t *testing.T) {
	c := testClient(t)
	c.Add("repo-a", "A task", "", 1, 3)
	c.Add("repo-b", "B task", "", 1, 3)

	item, err := c.GetReady("repo-a")
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "A task" {
		t.Errorf("got %q from repo-a, want %q", item.Title, "A task")
	}

	item, err = c.GetReady("repo-b")
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "B task" {
		t.Errorf("got %q from repo-b, want %q", item.Title, "B task")
	}
}

func TestGetReady_OnlyOpen(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Task", "", 1, 3)

	// First GetReady atomically claims the item.
	got, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected item from first GetReady")
	}

	// Second GetReady returns nil — item is already in-progress.
	got, err = c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil (no open items)")
	}
}

func TestGetReady_NoWork(t *testing.T) {
	c := testClient(t)
	item, err := c.GetReady("empty-repo")
	if err != nil {
		t.Fatal(err)
	}
	if item != nil {
		t.Error("expected nil for empty repo")
	}
}

func TestAssign(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	// Claim the item via GetReady (atomically sets in_progress).
	c.GetReady("myrepo")

	if err := c.Assign(item.ID, "alice", "implement"); err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Assignee != "alice" {
		t.Errorf("assignee = %q, want %q", got.Assignee, "alice")
	}
	if got.CurrentCataractae != "implement" {
		t.Errorf("current_step = %q, want %q", got.CurrentCataractae, "implement")
	}
	if got.Status != "in_progress" {
		t.Errorf("status = %q, want %q", got.Status, "in_progress")
	}
}

func TestAssign_EmptyWorker_SetsOpen(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)
	c.GetReady("myrepo") // claim item (sets in_progress)
	c.Assign(item.ID, "alice", "implement")

	// Advance to next step with empty worker.
	if err := c.Assign(item.ID, "", "review"); err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Status != "open" {
		t.Errorf("status = %q, want %q", got.Status, "open")
	}
	if got.Assignee != "" {
		t.Errorf("assignee = %q, want empty", got.Assignee)
	}
	if got.CurrentCataractae != "review" {
		t.Errorf("current_step = %q, want %q", got.CurrentCataractae, "review")
	}
}

// TestAssign_EmptyWorker_ClearsAssignedAqueduct verifies that resetting a droplet
// to open (worker="") also clears the assigned_aqueduct field so the re-opened
// droplet is not locked to a stale aqueduct operator.
func TestAssign_EmptyWorker_ClearsAssignedAqueduct(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)
	c.GetReady("myrepo")
	c.Assign(item.ID, "alice", "implement")
	c.SetAssignedAqueduct(item.ID, "cistern-alice")

	pre, _ := c.Get(item.ID)
	if pre.AssignedAqueduct != "cistern-alice" {
		t.Fatal("precondition failed: SetAssignedAqueduct did not set the field")
	}

	if err := c.Assign(item.ID, "", "review"); err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.AssignedAqueduct != "" {
		t.Errorf("assigned_aqueduct after Assign(\"\",step) = %q, want empty", got.AssignedAqueduct)
	}
}

func TestUpdateStatus(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	if err := c.UpdateStatus(item.ID, "in_progress"); err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Status != "in_progress" {
		t.Errorf("status = %q, want %q", got.Status, "in_progress")
	}
}

func TestAddNote_And_GetNotes(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	if err := c.AddNote(item.ID, "implement", "wrote the code"); err != nil {
		t.Fatal(err)
	}
	if err := c.AddNote(item.ID, "review", "looks good"); err != nil {
		t.Fatal(err)
	}

	notes, err := c.GetNotes(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("got %d notes, want 2", len(notes))
	}
	// Notes are returned newest-first (DESC).
	if notes[0].CataractaeName != "review" || notes[0].Content != "looks good" {
		t.Errorf("note[0] = %+v", notes[0])
	}
	if notes[1].CataractaeName != "implement" || notes[1].Content != "wrote the code" {
		t.Errorf("note[1] = %+v", notes[1])
	}
}

func TestGetNotes_Empty(t *testing.T) {
	c := testClient(t)
	notes, err := c.GetNotes("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Errorf("got %d notes, want 0", len(notes))
	}
}

func TestPool(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	if err := c.Pool(item.ID, "stuck on flaky test"); err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Status != "pooled" {
		t.Errorf("status = %q, want %q", got.Status, "pooled")
	}
	if got.Outcome != "pool" {
		t.Errorf("outcome = %q, want %q", got.Outcome, "pool")
	}
	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundPoolEvent := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "pool") {
			foundPoolEvent = true
		}
	}
	if !foundPoolEvent {
		t.Error("expected pool event in droplet changes")
	}
}

func TestCloseItem(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	if err := c.CloseItem(item.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Status != "delivered" {
		t.Errorf("status = %q, want %q", got.Status, "delivered")
	}
}

func TestList_All(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Task 1", "", 1, 3)
	c.Add("myrepo", "Task 2", "", 2, 3)
	c.Add("other", "Task 3", "", 1, 3)

	items, err := c.List("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
}

func TestList_ByRepo(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Task 1", "", 1, 3)
	c.Add("other", "Task 2", "", 1, 3)

	items, err := c.List("myrepo", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Title != "Task 1" {
		t.Errorf("title = %q, want %q", items[0].Title, "Task 1")
	}
}

func TestList_ByStatus(t *testing.T) {
	c := testClient(t)
	item1, _ := c.Add("myrepo", "Open task", "", 1, 3)
	item2, _ := c.Add("myrepo", "Closed task", "", 1, 3)
	_ = item1
	c.CloseItem(item2.ID)

	items, err := c.List("", "delivered")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Title != "Closed task" {
		t.Errorf("title = %q, want %q", items[0].Title, "Closed task")
	}
}

func TestGet_NotFound(t *testing.T) {
	c := testClient(t)
	item, err := c.Get("nonexistent")
	if item != nil {
		t.Error("expected nil item")
	}
	if err == nil {
		t.Error("expected error for missing item")
	}
}

func TestAssign_NotFound(t *testing.T) {
	c := testClient(t)
	err := c.Assign("nonexistent", "worker", "step")
	if err == nil {
		t.Error("expected error for missing item")
	}
}

// TestAssign_SetsStageDispatchedAt verifies that Assign with a non-empty worker
// records StageDispatchedAt, and that Assign with an empty worker does not clear it
// (the field is preserved as-is when resetting to open).
func TestAssign_SetsStageDispatchedAt(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 2)
	c.GetReady("myrepo")

	before := time.Now()
	if err := c.Assign(item.ID, "alice", "implement"); err != nil {
		t.Fatal(err)
	}
	after := time.Now()

	got, _ := c.Get(item.ID)
	if got.StageDispatchedAt.IsZero() {
		t.Error("StageDispatchedAt must be set when worker is assigned, got zero")
	}
	if got.StageDispatchedAt.Before(before) || got.StageDispatchedAt.After(after) {
		t.Errorf("StageDispatchedAt = %v, want between %v and %v", got.StageDispatchedAt, before, after)
	}

	// Resetting to open (empty worker) must not clear StageDispatchedAt.
	if err := c.Assign(item.ID, "", "review"); err != nil {
		t.Fatal(err)
	}
	reset, _ := c.Get(item.ID)
	if reset.StageDispatchedAt.IsZero() {
		t.Error("StageDispatchedAt must not be cleared when resetting to open")
	}
}

func TestAdd_WithComplexity(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Trivial fix", "", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Complexity != 1 {
		t.Errorf("complexity = %d, want 1", item.Complexity)
	}

	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Complexity != 1 {
		t.Errorf("stored complexity = %d, want 1", got.Complexity)
	}
}

func TestAdd_ComplexityDefault(t *testing.T) {
	c := testClient(t)
	// Out-of-range complexity should default to 2 (full).
	item, err := c.Add("myrepo", "Bad cx", "", 2, 99)
	if err != nil {
		t.Fatal(err)
	}
	if item.Complexity != 2 {
		t.Errorf("complexity = %d, want 2 (default)", item.Complexity)
	}
}

func TestGetReady_ReturnsComplexity(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Critical task", "", 1, 3)

	item, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if item.Complexity != 3 {
		t.Errorf("complexity = %d, want 3", item.Complexity)
	}
}

func TestList_ReturnsComplexity(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Standard", "", 1, 1)
	c.Add("myrepo", "Critical", "", 1, 3)

	items, err := c.List("myrepo", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].Complexity != 1 {
		t.Errorf("items[0].Complexity = %d, want 1", items[0].Complexity)
	}
	if items[1].Complexity != 3 {
		t.Errorf("items[1].Complexity = %d, want 3", items[1].Complexity)
	}
}

func TestStats_EmptyDB(t *testing.T) {
	c := testClient(t)
	s, err := c.Stats()
	if err != nil {
		t.Fatalf("Stats on empty DB: %v", err)
	}
	if s.Flowing != 0 || s.Queued != 0 || s.Delivered != 0 || s.Pooled != 0 {
		t.Errorf("expected all zeros on empty DB, got %+v", s)
	}
}

func TestAdd_WithDeps(t *testing.T) {
	c := testClient(t)
	parent, _ := c.Add("myrepo", "Parent", "", 1, 3)
	child, err := c.Add("myrepo", "Child", "", 1, 3, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	deps, err := c.GetDependencies(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0] != parent.ID {
		t.Errorf("GetDependencies = %v, want [%s]", deps, parent.ID)
	}
}

func TestAdd_UnknownDep(t *testing.T) {
	c := testClient(t)
	_, err := c.Add("myrepo", "Child", "", 1, 3, "nonexistent")
	if err == nil {
		t.Error("expected error for unknown dep ID")
	}
}

func TestAddDependency_And_RemoveDependency(t *testing.T) {
	c := testClient(t)
	a, _ := c.Add("myrepo", "A", "", 1, 3)
	b, _ := c.Add("myrepo", "B", "", 1, 3)

	if err := c.AddDependency(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	deps, _ := c.GetDependencies(b.ID)
	if len(deps) != 1 || deps[0] != a.ID {
		t.Errorf("after add: GetDependencies = %v, want [%s]", deps, a.ID)
	}

	if err := c.RemoveDependency(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	deps, _ = c.GetDependencies(b.ID)
	if len(deps) != 0 {
		t.Errorf("after remove: GetDependencies = %v, want []", deps)
	}
}

func TestAddDependency_UnknownDroplet(t *testing.T) {
	c := testClient(t)
	a, _ := c.Add("myrepo", "A", "", 1, 3)
	if err := c.AddDependency("nonexistent", a.ID); err == nil {
		t.Error("expected error for unknown droplet")
	}
	if err := c.AddDependency(a.ID, "nonexistent"); err == nil {
		t.Error("expected error for unknown depends_on")
	}
}

func TestGetBlockedBy(t *testing.T) {
	c := testClient(t)
	parent, _ := c.Add("myrepo", "Parent", "", 1, 3)
	child, _ := c.Add("myrepo", "Child", "", 1, 3, parent.ID)

	// Parent not delivered — child is blocked.
	blocked, err := c.GetBlockedBy(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || blocked[0] != parent.ID {
		t.Errorf("GetBlockedBy = %v, want [%s]", blocked, parent.ID)
	}

	// Deliver parent — child should no longer be blocked.
	c.CloseItem(parent.ID)
	blocked, err = c.GetBlockedBy(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 0 {
		t.Errorf("GetBlockedBy after deliver = %v, want []", blocked)
	}
}

func TestGetDependents(t *testing.T) {
	c := testClient(t)
	parent, _ := c.Add("myrepo", "Parent", "", 1, 3)
	child, _ := c.Add("myrepo", "Child", "", 1, 3, parent.ID)

	dependents, err := c.GetDependents(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependents) != 1 || dependents[0] != child.ID {
		t.Errorf("GetDependents = %v, want [%s]", dependents, child.ID)
	}

	noDeps, err := c.GetDependents(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(noDeps) != 0 {
		t.Errorf("GetDependents for leaf = %v, want []", noDeps)
	}
}

func TestGetReady_SkipsBlocked(t *testing.T) {
	c := testClient(t)
	parent, _ := c.Add("myrepo", "Parent", "", 1, 3)
	_, _ = c.Add("myrepo", "Child", "", 1, 3, parent.ID)

	// GetReady should return parent (child is blocked).
	got, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected parent, got nil")
	}
	if got.ID != parent.ID {
		t.Errorf("got %s, want parent %s", got.ID, parent.ID)
	}

	// Deliver parent.
	c.CloseItem(parent.ID)
	// Reopen child to 'open' (it was still open, just couldn't be dispatched).
	// Child should now be ready.
	got, err = c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected child to be ready after parent delivered, got nil")
	}
}

func TestGetReady_SkipsBlocked_NothingAvailable(t *testing.T) {
	c := testClient(t)
	parent, _ := c.Add("myrepo", "Parent", "", 1, 3)
	_, _ = c.Add("myrepo", "Child", "", 1, 3, parent.ID)

	// Claim parent.
	c.GetReady("myrepo") // claims parent (in_progress)
	// Now only child is open but blocked — GetReady should return nil.
	got, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil (child blocked), got %s", got.ID)
	}
}

func TestSetAndGetLastReviewedCommit(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	// Initially empty.
	commit, err := c.GetLastReviewedCommit(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if commit != "" {
		t.Errorf("expected empty last_reviewed_commit, got %q", commit)
	}

	// Set a commit hash.
	hash := "abc1234def5678"
	if err := c.SetLastReviewedCommit(item.ID, hash); err != nil {
		t.Fatalf("SetLastReviewedCommit: %v", err)
	}

	// Read it back.
	got, err := c.GetLastReviewedCommit(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != hash {
		t.Errorf("GetLastReviewedCommit = %q, want %q", got, hash)
	}
}

func TestSetLastReviewedCommit_Overwrite(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	if err := c.SetLastReviewedCommit(item.ID, "hash-old"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetLastReviewedCommit(item.ID, "hash-new"); err != nil {
		t.Fatal(err)
	}

	got, err := c.GetLastReviewedCommit(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hash-new" {
		t.Errorf("expected overwritten hash 'hash-new', got %q", got)
	}
}

func TestGetLastReviewedCommit_PersistedInGet(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	hash := "deadbeef00"
	if err := c.SetLastReviewedCommit(item.ID, hash); err != nil {
		t.Fatal(err)
	}

	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastReviewedCommit != hash {
		t.Errorf("Droplet.LastReviewedCommit = %q, want %q", got.LastReviewedCommit, hash)
	}
}

func TestStats_WithData(t *testing.T) {
	c := testClient(t)

	// Add 2 open (queued), 1 in_progress (flowing), 3 delivered, 1 pooled.
	c.Add("repo", "q1", "", 1, 3)
	c.Add("repo", "q2", "", 1, 3)
	item3, _ := c.Add("repo", "ip1", "", 1, 3)
	item4, _ := c.Add("repo", "d1", "", 1, 3)
	item5, _ := c.Add("repo", "d2", "", 1, 3)
	item6, _ := c.Add("repo", "d3", "", 1, 3)
	item7, _ := c.Add("repo", "s1", "", 1, 3)

	c.UpdateStatus(item3.ID, "in_progress")
	c.CloseItem(item4.ID)
	c.CloseItem(item5.ID)
	c.CloseItem(item6.ID)
	c.Pool(item7.ID, "stuck")

	s, err := c.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if s.Queued != 2 {
		t.Errorf("Queued = %d, want 2", s.Queued)
	}
	if s.Flowing != 1 {
		t.Errorf("Flowing = %d, want 1", s.Flowing)
	}
	if s.Delivered != 3 {
		t.Errorf("Delivered = %d, want 3", s.Delivered)
	}
	if s.Pooled != 1 {
		t.Errorf("Pooled = %d, want 1", s.Pooled)
	}
}

func TestSearch(t *testing.T) {
	c := testClient(t)
	c.Add("repo", "Fix login bug", "", 1, 3)
	c.Add("repo", "Add dashboard feature", "", 2, 3)
	c.Add("repo", "Fix signup flow", "", 1, 2)
	ip, _ := c.Add("repo", "Refactor auth module", "", 3, 3)
	c.UpdateStatus(ip.ID, "in_progress")

	t.Run("empty query returns all", func(t *testing.T) {
		results, err := c.Search("", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 4 {
			t.Fatalf("expected 4 results, got %d", len(results))
		}
	})

	t.Run("query matches title substring case-insensitive", func(t *testing.T) {
		results, err := c.Search("fix", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		titles := map[string]bool{}
		for _, r := range results {
			titles[r.Title] = true
		}
		if !titles["Fix login bug"] {
			t.Error("expected 'Fix login bug' in results")
		}
		if !titles["Fix signup flow"] {
			t.Error("expected 'Fix signup flow' in results")
		}
	})

	t.Run("status filter", func(t *testing.T) {
		results, err := c.Search("", "in_progress", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Title != "Refactor auth module" {
			t.Errorf("expected 'Refactor auth module', got %q", results[0].Title)
		}
	})

	t.Run("priority filter", func(t *testing.T) {
		results, err := c.Search("", "", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("combined query and status", func(t *testing.T) {
		results, err := c.Search("fix", "open", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		results, err := c.Search("xyz-no-match", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 0 {
			t.Fatalf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("results ordered by priority then created_at", func(t *testing.T) {
		results, err := c.Search("", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		// Priority 1 items should come before priority 2 and 3.
		if results[0].Priority > results[len(results)-1].Priority {
			t.Errorf("results not ordered by priority: first=%d last=%d",
				results[0].Priority, results[len(results)-1].Priority)
		}
	})
}

func TestEditDroplet_Title_GuardInProgress(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Old title", "", 1, 3)
	c.UpdateStatus(item.ID, "in_progress")

	err := c.EditDroplet(item.ID, EditDropletFields{Title: ptr("New title")})
	if err == nil {
		t.Fatal("expected error for in_progress droplet title edit")
	}
	if !strings.Contains(err.Error(), "cannot edit a droplet that has been picked up") {
		t.Errorf("unexpected error: %v", err)
	}
	got, _ := c.Get(item.ID)
	if got.Title != "Old title" {
		t.Errorf("title = %q, want %q (should not have changed)", got.Title, "Old title")
	}
}

func TestSetOutcome(t *testing.T) {
	for _, outcome := range []string{"pass", "recirculate", "pool"} {
		t.Run(outcome, func(t *testing.T) {
			c := testClient(t)
			item, _ := c.Add("myrepo", "Task", "", 1, 3)
			if err := c.SetOutcome(item.ID, outcome); err != nil {
				t.Fatal(err)
			}
			got, _ := c.Get(item.ID)
			if got.Outcome != outcome {
				t.Errorf("outcome = %q, want %q", got.Outcome, outcome)
			}
		})
	}
}

func TestSetOutcome_Clear(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)
	c.SetOutcome(item.ID, "pass")

	if err := c.SetOutcome(item.ID, ""); err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Outcome != "" {
		t.Errorf("outcome = %q, want empty after clear", got.Outcome)
	}
}

func TestSetOutcome_NotFound(t *testing.T) {
	c := testClient(t)
	if err := c.SetOutcome("nonexistent", "pass"); err == nil {
		t.Error("expected error for missing item")
	}
}

func TestSetCataractae(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	if err := c.SetCataractae(item.ID, "review"); err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.CurrentCataractae != "review" {
		t.Errorf("current_cataractae = %q, want %q", got.CurrentCataractae, "review")
	}
}

func TestSetCataractae_NotFound(t *testing.T) {
	c := testClient(t)
	if err := c.SetCataractae("nonexistent", "review"); err == nil {
		t.Error("expected error for missing item")
	}
}

func TestPurge(t *testing.T) {
	c := testClient(t)
	delivered, _ := c.Add("myrepo", "Delivered", "", 1, 3)
	pooled, _ := c.Add("myrepo", "Pooled", "", 1, 3)
	inProgress, _ := c.Add("myrepo", "In progress", "", 1, 3)

	c.CloseItem(delivered.ID)  // status = delivered
	c.Pool(pooled.ID, "stuck") // status = pooled
	c.UpdateStatus(inProgress.ID, "in_progress")

	// Negative duration sets cutoff in the future, making all items eligible by age.
	n, err := c.Purge(-time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("purged %d, want 2 (delivered + pooled)", n)
	}

	// in_progress item must survive.
	if _, err := c.Get(inProgress.ID); err != nil {
		t.Errorf("in-progress item should not be purged: %v", err)
	}

	// delivered and pooled must be gone.
	if item, _ := c.Get(delivered.ID); item != nil {
		t.Error("delivered item should have been purged")
	}
	if item, _ := c.Get(pooled.ID); item != nil {
		t.Error("pooled item should have been purged")
	}
}

func TestPurge_DryRun(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)
	c.CloseItem(item.ID)

	n, err := c.Purge(-time.Hour, true) // dry run
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("dry-run count = %d, want 1", n)
	}
	// Item must still exist after a dry run.
	if _, err := c.Get(item.ID); err != nil {
		t.Errorf("dry run should not delete item: %v", err)
	}
}

func TestPurge_LeavesInProgress(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)
	c.UpdateStatus(item.ID, "in_progress")

	n, err := c.Purge(-time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("purged %d, want 0 (in-progress must not be purged)", n)
	}
	if _, err := c.Get(item.ID); err != nil {
		t.Error("in-progress item should not be purged")
	}
}

func TestListRecentEvents_Empty(t *testing.T) {
	c := testClient(t)
	events, err := c.ListRecentEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("got %d events, want 0", len(events))
	}
}

func TestListRecentEvents_WithEvents(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	c.AddNote(item.ID, "implement", "wrote the code")
	c.Pool(item.ID, "needs human review")

	events, err := c.ListRecentEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	// create event from Add, pool event from Pool = 2 (notes excluded)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	for _, e := range events {
		if e.Droplet != item.ID {
			t.Errorf("event droplet = %q, want %q", e.Droplet, item.ID)
		}
	}
}

func TestListRecentEvents_Limit(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	for i := range 5 {
		c.RecordEvent(item.ID, "pass", fmt.Sprintf(`{"cataractae":"step-%d"}`, i))
	}

	// create + 5 pass = 6 events; limit 3 returns 3 newest
	events, err := c.ListRecentEvents(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Errorf("got %d events, want 3 (limit enforced)", len(events))
	}
}

func ptr[T any](v T) *T { return &v }

func TestEditDroplet_Description(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "old desc", 2, 3)

	err := c.EditDroplet(item.ID, EditDropletFields{Description: ptr("new desc")})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Description != "new desc" {
		t.Errorf("description = %q, want %q", got.Description, "new desc")
	}
	// Other fields unchanged.
	if got.Priority != 2 {
		t.Errorf("priority = %d, want 2", got.Priority)
	}
	if got.Complexity != 3 {
		t.Errorf("complexity = %d, want 3", got.Complexity)
	}
}

func TestEditDroplet_Complexity(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "desc", 2, 3)

	err := c.EditDroplet(item.ID, EditDropletFields{Complexity: ptr(1)})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Complexity != 1 {
		t.Errorf("complexity = %d, want 1", got.Complexity)
	}
	if got.Description != "desc" {
		t.Errorf("description changed unexpectedly: %q", got.Description)
	}
}

func TestEditDroplet_Priority(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "", 2, 3)

	err := c.EditDroplet(item.ID, EditDropletFields{Priority: ptr(1)})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Priority != 1 {
		t.Errorf("priority = %d, want 1", got.Priority)
	}
}

func TestEditDroplet_Title(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Old Title", "desc", 2, 3)

	err := c.EditDroplet(item.ID, EditDropletFields{Title: ptr("New Title")})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Title != "New Title" {
		t.Errorf("title = %q, want %q", got.Title, "New Title")
	}
	if got.Description != "desc" {
		t.Errorf("description changed unexpectedly: %q", got.Description)
	}
}

func TestEditDroplet_EmptyTitle(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "desc", 2, 3)

	err := c.EditDroplet(item.ID, EditDropletFields{Title: ptr("")})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
	if !strings.Contains(err.Error(), "title must not be empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEditDroplet_InvalidPriority(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "", 2, 3)

	for _, bad := range []int{0, -1} {
		err := c.EditDroplet(item.ID, EditDropletFields{Priority: ptr(bad)})
		if err == nil {
			t.Errorf("expected error for priority=%d", bad)
		} else if !strings.Contains(err.Error(), "priority must be a positive integer") {
			t.Errorf("priority=%d: unexpected error: %v", bad, err)
		}
	}
}

func TestEditDroplet_AllFields(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "old", 3, 3)

	err := c.EditDroplet(item.ID, EditDropletFields{
		Title:       ptr("New Title"),
		Description: ptr("updated"),
		Complexity:  ptr(2),
		Priority:    ptr(1),
	})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Title != "New Title" {
		t.Errorf("title = %q, want %q", got.Title, "New Title")
	}
	if got.Description != "updated" {
		t.Errorf("description = %q, want %q", got.Description, "updated")
	}
	if got.Complexity != 2 {
		t.Errorf("complexity = %d, want 2", got.Complexity)
	}
	if got.Priority != 1 {
		t.Errorf("priority = %d, want 1", got.Priority)
	}
}

func TestEditDroplet_GuardInProgress(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "", 1, 3)
	c.UpdateStatus(item.ID, "in_progress")

	err := c.EditDroplet(item.ID, EditDropletFields{Description: ptr("new")})
	if err == nil {
		t.Fatal("expected error for in_progress droplet")
	}
	if !strings.Contains(err.Error(), "cannot edit a droplet that has been picked up") {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "in_progress") {
		t.Errorf("error should mention status, got: %v", err)
	}
}

func TestEditDroplet_GuardDelivered(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "", 1, 3)
	c.CloseItem(item.ID)

	err := c.EditDroplet(item.ID, EditDropletFields{Description: ptr("new")})
	if err == nil {
		t.Fatal("expected error for delivered droplet")
	}
	if !strings.Contains(err.Error(), "cannot edit a droplet that has been picked up") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEditDroplet_AllowPooled(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "old", 1, 3)
	c.Pool(item.ID, "stuck")

	err := c.EditDroplet(item.ID, EditDropletFields{Description: ptr("updated")})
	if err != nil {
		t.Fatalf("expected pooled droplet to be editable, got: %v", err)
	}

	got, _ := c.Get(item.ID)
	if got.Description != "updated" {
		t.Errorf("description = %q, want %q", got.Description, "updated")
	}
}

func TestEditDroplet_NoFields(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "desc", 2, 3)

	// No-op: no fields specified should be fine at the client layer.
	err := c.EditDroplet(item.ID, EditDropletFields{})
	if err != nil {
		t.Fatalf("unexpected error for no-op edit: %v", err)
	}
}

func TestEditDroplet_InvalidComplexity(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "desc", 2, 3)

	for _, bad := range []int{0, -1, 4, 5, 100} {
		err := c.EditDroplet(item.ID, EditDropletFields{Complexity: ptr(bad)})
		if err == nil {
			t.Errorf("expected error for complexity=%d", bad)
		} else if !strings.Contains(err.Error(), "complexity must be between 1 and 3") {
			t.Errorf("complexity=%d: unexpected error: %v", bad, err)
		}
	}

	// Valid boundary values should succeed.
	for _, ok := range []int{1, 3} {
		if err := c.EditDroplet(item.ID, EditDropletFields{Complexity: ptr(ok)}); err != nil {
			t.Errorf("complexity=%d should be valid, got: %v", ok, err)
		}
	}
}

func TestEditDroplet_NotFound(t *testing.T) {
	c := testClient(t)

	err := c.EditDroplet("bf-xxxxx", EditDropletFields{Description: ptr("x")})
	if err == nil {
		t.Fatal("expected error for unknown droplet")
	}
}

func TestEditDroplet_MultilineDescription(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "", 2, 3)

	multiline := "line one\nline two\nline three"
	err := c.EditDroplet(item.ID, EditDropletFields{Description: ptr(multiline)})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Description != multiline {
		t.Errorf("description = %q, want %q", got.Description, multiline)
	}
}

func TestEditDroplet_ClearDescription(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("repo", "Title", "has content", 2, 3)

	err := c.EditDroplet(item.ID, EditDropletFields{Description: ptr("")})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := c.Get(item.ID)
	if got.Description != "" {
		t.Errorf("description = %q, want empty", got.Description)
	}
}

func TestCancel_SetsStatusCancelled(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Superseded feature", "", 1, 3)

	if err := c.Cancel(item.ID, "superseded by newer approach"); err != nil {
		t.Fatal(err)
	}

	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "cancelled" {
		t.Errorf("status = %q, want %q", got.Status, "cancelled")
	}
}

func TestCancel_RecordsCancelEventWithReason(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Old feature", "", 1, 3)

	reason := "filed in error"
	if err := c.Cancel(item.ID, reason); err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "cancel") && strings.Contains(ch.Value, reason) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cancel event with reason %q not found in changes: %v", reason, changes)
	}
}

func TestCancel_RecordsCancelEventWithoutReason(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Old feature", "", 1, 3)

	if err := c.Cancel(item.ID, ""); err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "cancel") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cancel event not found in changes: %v", changes)
	}
}

func TestCancel_PreservesAssignee(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Obsolete task", "", 1, 2)
	c.Assign(item.ID, "worker-1", "implement")

	pre, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pre.Assignee == "" {
		t.Fatal("precondition failed: assignee not set before cancel")
	}

	if err := c.Cancel(item.ID, "no longer needed"); err != nil {
		t.Fatal(err)
	}

	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Assignee != "worker-1" {
		t.Errorf("Assignee after Cancel = %q, want %q (preserved for scheduler pool-release)", got.Assignee, "worker-1")
	}
}

func TestCancel_ClearsOutcome(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Obsolete task", "", 1, 2)
	c.Assign(item.ID, "worker-1", "implement")
	c.SetOutcome(item.ID, "pass")

	pre, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pre.Outcome == "" {
		t.Fatal("precondition failed: outcome not set before cancel")
	}

	if err := c.Cancel(item.ID, "no longer needed"); err != nil {
		t.Fatal(err)
	}

	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != "" {
		t.Errorf("Outcome after Cancel = %q, want empty string", got.Outcome)
	}
}

func TestCancel_ClearsAssignedAqueduct(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Obsolete task", "", 1, 2)
	c.SetAssignedAqueduct(item.ID, "cistern-gamma")
	pre, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pre.AssignedAqueduct != "cistern-gamma" {
		t.Fatal("precondition failed: SetAssignedAqueduct did not set the field")
	}

	if err := c.Cancel(item.ID, "no longer needed"); err != nil {
		t.Fatal(err)
	}

	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AssignedAqueduct != "" {
		t.Errorf("AssignedAqueduct after Cancel = %q, want empty string", got.AssignedAqueduct)
	}
}

func TestCancel_RecordsCancelEvent(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Obsolete task", "", 1, 2)

	reason := "superseded by ct-abc12"
	if err := c.Cancel(item.ID, reason); err != nil {
		t.Fatal(err)
	}

	events, err := c.ListRecentEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.Droplet == item.ID && e.Event == "cancel" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cancel event not found in recent events: %v", events)
	}
}

func TestCancel_PreservesExistingNotes(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Obsolete task", "", 1, 2)

	c.AddNote(item.ID, "implement", "started work")
	c.AddNote(item.ID, "review", "found issues")

	if err := c.Cancel(item.ID, "no longer needed"); err != nil {
		t.Fatal(err)
	}

	notes, err := c.GetNotes(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	var implFound, reviewFound bool
	for _, n := range notes {
		if n.CataractaeName == "implement" && strings.Contains(n.Content, "started work") {
			implFound = true
		}
		if n.CataractaeName == "review" && strings.Contains(n.Content, "found issues") {
			reviewFound = true
		}
	}
	if !implFound {
		t.Error("existing implement note lost after cancel")
	}
	if !reviewFound {
		t.Error("existing review note lost after cancel")
	}
}

func TestCancel_NotFound(t *testing.T) {
	c := testClient(t)
	if err := c.Cancel("nonexistent", "reason"); err == nil {
		t.Error("expected error for missing droplet")
	}
}

func TestCancel_AlreadyCancelled_ReturnsTerminalError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Feature", "", 1, 3)
	if err := c.Cancel(item.ID, "first cancel"); err != nil {
		t.Fatal(err)
	}
	err := c.Cancel(item.ID, "second cancel")
	if err == nil {
		t.Fatal("expected error when re-cancelling already-cancelled droplet")
	}
	if !strings.Contains(err.Error(), "terminal status") {
		t.Errorf("error = %v, want terminal status message", err)
	}
}

func TestCancel_Delivered_ReturnsTerminalError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Feature", "", 1, 3)
	if err := c.CloseItem(item.ID); err != nil {
		t.Fatal(err)
	}
	err := c.Cancel(item.ID, "too late")
	if err == nil {
		t.Fatal("expected error when cancelling delivered droplet")
	}
	if !strings.Contains(err.Error(), "terminal status") {
		t.Errorf("error = %v, want terminal status message", err)
	}
}

func TestCancel_ExcludedFromGetReady(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Old feature", "", 1, 3)

	if err := c.Cancel(item.ID, "won't do"); err != nil {
		t.Fatal(err)
	}

	// GetReady must not return a cancelled droplet.
	got, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("GetReady returned cancelled droplet %s — cancelled droplets must never be dispatched", got.ID)
	}
}

func TestList_ExcludesCancelledByDefault(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Active", "", 1, 3)
	cancelled, _ := c.Add("myrepo", "Cancelled", "", 1, 3)
	c.Cancel(cancelled.ID, "not needed")

	// List with no status filter must not include cancelled items.
	items, err := c.List("myrepo", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == cancelled.ID {
			t.Errorf("List returned cancelled droplet %s — cancelled droplets must be hidden by default", cancelled.ID)
		}
	}
}

func TestList_CancelledStatus_ReturnsOnlyCancelled(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Active", "", 1, 3)
	cancelled, _ := c.Add("myrepo", "Cancelled", "", 1, 3)
	c.Cancel(cancelled.ID, "not needed")

	items, err := c.List("myrepo", "cancelled")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("List(cancelled) returned %d items, want 1", len(items))
	}
	if items[0].ID != cancelled.ID {
		t.Errorf("returned item %s, want %s", items[0].ID, cancelled.ID)
	}
}

// TestCancel_NotFound_NoOrphanData verifies that cancelling a nonexistent droplet
// does NOT create an orphan note or event row (UPDATE must happen before RecordEvent).
func TestCancel_NotFound_NoOrphanData(t *testing.T) {
	c := testClient(t)

	err := c.Cancel("nonexistent-id", "some reason")
	if err == nil {
		t.Fatal("expected error for nonexistent droplet, got nil")
	}

	notes, err2 := c.GetNotes("nonexistent-id")
	if err2 != nil {
		t.Fatal(err2)
	}
	if len(notes) != 0 {
		t.Errorf("Cancel on nonexistent droplet created %d orphan note(s); want 0", len(notes))
	}
}

// TestPurge_IncludesCancelled verifies that cancelled droplets are cleaned up by Purge.
func TestPurge_IncludesCancelled(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Cancelled task", "", 1, 3)
	c.Cancel(item.ID, "won't do")

	n, err := c.Purge(-time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("Purge returned %d, want 1 (cancelled item)", n)
	}
	if got, _ := c.Get(item.ID); got != nil {
		t.Error("cancelled item should have been purged")
	}
}

// TestGetReady_CancelledDependency_DoesNotBlock verifies that a droplet whose
// dependency was cancelled is still dispatched (cancelled != unresolved).
func TestGetReady_CancelledDependency_DoesNotBlock(t *testing.T) {
	c := testClient(t)
	dep, _ := c.Add("myrepo", "Dependency", "", 1, 3)
	child, _ := c.Add("myrepo", "Child", "", 2, 3, dep.ID)

	// Cancel the dependency instead of delivering it.
	if err := c.Cancel(dep.ID, "no longer needed"); err != nil {
		t.Fatal(err)
	}

	// Child should now be dispatchable — cancelled dep must not block it.
	got, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetReady returned nil — cancelled dependency should not block child")
	}
	if got.ID != child.ID {
		t.Errorf("GetReady returned %s, want child %s", got.ID, child.ID)
	}
}

// TestSearch_ExcludesCancelledByDefault verifies that Search omits cancelled
// droplets when no status filter is given (consistent with List behaviour).
func TestSearch_ExcludesCancelledByDefault(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Active task", "", 1, 3)
	cancelled, _ := c.Add("myrepo", "Cancelled task", "", 1, 3)
	c.Cancel(cancelled.ID, "not needed")

	results, err := c.Search("", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.ID == cancelled.ID {
			t.Errorf("Search returned cancelled droplet %s — must be hidden by default", cancelled.ID)
		}
	}
}

// TestGetReady_CaseInsensitiveRepo_ReturnsDropletStoredWithWrongCase verifies that
// GetReady("PortfolioWebsite") returns a droplet stored as "portfoliowebsite".
func TestGetReady_CaseInsensitiveRepo_ReturnsDropletStoredWithWrongCase(t *testing.T) {
	c := testClient(t)
	// Given: a droplet stored with lower-case repo name.
	_, err := c.Add("portfoliowebsite", "My task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	// When: GetReady is called with the canonical casing.
	got, err := c.GetReady("PortfolioWebsite")
	if err != nil {
		t.Fatal(err)
	}

	// Then: the droplet is returned.
	if got == nil {
		t.Fatal("GetReady(PortfolioWebsite): expected droplet, got nil")
	}
	if got.Title != "My task" {
		t.Errorf("GetReady(PortfolioWebsite): title = %q, want %q", got.Title, "My task")
	}
}

// TestExternalRef_IsNullByDefault verifies that a new droplet has an empty
// ExternalRef — the column defaults to NULL and is scanned as an empty string.
func TestExternalRef_IsNullByDefault(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalRef != "" {
		t.Errorf("ExternalRef = %q, want empty string", got.ExternalRef)
	}
}

// TestSetExternalRef_And_Get_RoundTrips verifies that SetExternalRef persists
// the external reference and Get returns it correctly.
func TestSetExternalRef_And_Get_RoundTrips(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Imported task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	// When: SetExternalRef is called with a valid provider:key value.
	if err := c.SetExternalRef(item.ID, "jira:DPF-456"); err != nil {
		t.Fatal(err)
	}

	// Then: Get returns the stored external_ref.
	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalRef != "jira:DPF-456" {
		t.Errorf("ExternalRef = %q, want %q", got.ExternalRef, "jira:DPF-456")
	}
}

// TestSetExternalRef_ClearsField_WhenEmptyStringPassed verifies that passing an
// empty string to SetExternalRef stores NULL (returned as empty string by Get).
func TestSetExternalRef_ClearsField_WhenEmptyStringPassed(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Imported task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetExternalRef(item.ID, "linear:LIN-789"); err != nil {
		t.Fatal(err)
	}

	// When: SetExternalRef is called with empty string.
	if err := c.SetExternalRef(item.ID, ""); err != nil {
		t.Fatal(err)
	}

	// Then: Get returns an empty ExternalRef.
	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalRef != "" {
		t.Errorf("ExternalRef after clear = %q, want empty string", got.ExternalRef)
	}
}

// TestExternalRef_RoundTrips_ThroughGetReady verifies that GetReady returns the
// external_ref stored on the droplet.
func TestExternalRef_RoundTrips_ThroughGetReady(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetExternalRef(item.ID, "jira:PROJ-123"); err != nil {
		t.Fatal(err)
	}

	// When: GetReady is called.
	got, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected droplet, got nil")
	}

	// Then: ExternalRef is populated.
	if got.ExternalRef != "jira:PROJ-123" {
		t.Errorf("GetReady ExternalRef = %q, want %q", got.ExternalRef, "jira:PROJ-123")
	}
}

// TestExternalRef_RoundTrips_ThroughList verifies that List returns the
// external_ref stored on each droplet.
func TestExternalRef_RoundTrips_ThroughList(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetExternalRef(item.ID, "linear:LIN-42"); err != nil {
		t.Fatal(err)
	}

	// When: List is called.
	items, err := c.List("myrepo", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("List returned %d items, want 1", len(items))
	}

	// Then: ExternalRef is populated.
	if items[0].ExternalRef != "linear:LIN-42" {
		t.Errorf("List ExternalRef = %q, want %q", items[0].ExternalRef, "linear:LIN-42")
	}
}

// TestExternalRef_RoundTrips_ThroughSearch verifies that Search returns the
// external_ref stored on each droplet.
func TestExternalRef_RoundTrips_ThroughSearch(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Imported feature", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetExternalRef(item.ID, "jira:FEAT-99"); err != nil {
		t.Fatal(err)
	}

	// When: Search is called.
	results, err := c.Search("Imported", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %d items, want 1", len(results))
	}

	// Then: ExternalRef is populated.
	if results[0].ExternalRef != "jira:FEAT-99" {
		t.Errorf("Search ExternalRef = %q, want %q", results[0].ExternalRef, "jira:FEAT-99")
	}
}

// --- Heartbeat round-trip tests ---

// TestHeartbeat_RoundTrips_ThroughGet verifies that Get returns the
// last_heartbeat_at written by Heartbeat(). A column scan order bug would
// leave LastHeartbeatAt zero and this test would fail.
func TestHeartbeat_RoundTrips_ThroughGet(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Add(-time.Second)
	if err := c.Heartbeat(item.ID); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC().Add(time.Second)

	// When: Get is called.
	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Then: LastHeartbeatAt is populated.
	if got.LastHeartbeatAt.IsZero() {
		t.Fatal("Get: LastHeartbeatAt is zero after Heartbeat()")
	}
	if got.LastHeartbeatAt.Before(before) || got.LastHeartbeatAt.After(after) {
		t.Errorf("Get: LastHeartbeatAt = %v, want between %v and %v", got.LastHeartbeatAt, before, after)
	}
}

// TestHeartbeat_RoundTrips_ThroughList verifies that List returns the
// last_heartbeat_at written by Heartbeat().
func TestHeartbeat_RoundTrips_ThroughList(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Add(-time.Second)
	if err := c.Heartbeat(item.ID); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC().Add(time.Second)

	// When: List is called.
	items, err := c.List("myrepo", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("List returned %d items, want 1", len(items))
	}

	// Then: LastHeartbeatAt is populated.
	if items[0].LastHeartbeatAt.IsZero() {
		t.Fatal("List: LastHeartbeatAt is zero after Heartbeat()")
	}
	if items[0].LastHeartbeatAt.Before(before) || items[0].LastHeartbeatAt.After(after) {
		t.Errorf("List: LastHeartbeatAt = %v, want between %v and %v", items[0].LastHeartbeatAt, before, after)
	}
}

// TestHeartbeat_RoundTrips_ThroughSearch verifies that Search returns the
// last_heartbeat_at written by Heartbeat().
func TestHeartbeat_RoundTrips_ThroughSearch(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Heartbeat task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Add(-time.Second)
	if err := c.Heartbeat(item.ID); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC().Add(time.Second)

	// When: Search is called.
	results, err := c.Search("Heartbeat task", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %d items, want 1", len(results))
	}

	// Then: LastHeartbeatAt is populated.
	if results[0].LastHeartbeatAt.IsZero() {
		t.Fatal("Search: LastHeartbeatAt is zero after Heartbeat()")
	}
	if results[0].LastHeartbeatAt.Before(before) || results[0].LastHeartbeatAt.After(after) {
		t.Errorf("Search: LastHeartbeatAt = %v, want between %v and %v", results[0].LastHeartbeatAt, before, after)
	}
}

// TestHeartbeat_RoundTrips_ThroughGetReady verifies that GetReady returns the
// last_heartbeat_at written by Heartbeat().
func TestHeartbeat_RoundTrips_ThroughGetReady(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Add(-time.Second)
	if err := c.Heartbeat(item.ID); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC().Add(time.Second)

	// When: GetReady is called.
	got, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetReady returned nil")
	}

	// Then: LastHeartbeatAt is populated.
	if got.LastHeartbeatAt.IsZero() {
		t.Fatal("GetReady: LastHeartbeatAt is zero after Heartbeat()")
	}
	if got.LastHeartbeatAt.Before(before) || got.LastHeartbeatAt.After(after) {
		t.Errorf("GetReady: LastHeartbeatAt = %v, want between %v and %v", got.LastHeartbeatAt, before, after)
	}
}

// TestHeartbeat_RoundTrips_ThroughGetReadyForAqueduct verifies that
// GetReadyForAqueduct returns the last_heartbeat_at written by Heartbeat().
func TestHeartbeat_RoundTrips_ThroughGetReadyForAqueduct(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Add(-time.Second)
	if err := c.Heartbeat(item.ID); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC().Add(time.Second)

	// When: GetReadyForAqueduct is called.
	got, err := c.GetReadyForAqueduct("myrepo", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetReadyForAqueduct returned nil")
	}

	// Then: LastHeartbeatAt is populated.
	if got.LastHeartbeatAt.IsZero() {
		t.Fatal("GetReadyForAqueduct: LastHeartbeatAt is zero after Heartbeat()")
	}
	if got.LastHeartbeatAt.Before(before) || got.LastHeartbeatAt.After(after) {
		t.Errorf("GetReadyForAqueduct: LastHeartbeatAt = %v, want between %v and %v", got.LastHeartbeatAt, before, after)
	}
}

// TestSetExternalRef_ReturnsError_WhenDropletNotFound verifies that SetExternalRef
// returns an error when the given ID does not exist in the database.
func TestSetExternalRef_ReturnsError_WhenDropletNotFound(t *testing.T) {
	c := testClient(t)
	err := c.SetExternalRef("nonexistent-id", "jira:DPF-1")
	if err == nil {
		t.Error("expected error for non-existent droplet, got nil")
	}
}

// TestSetExternalRef_RejectsInvalidFormat verifies that SetExternalRef returns
// an error when the ref contains characters that are invalid in git branch
// names or that break the delivery shell awk extraction (spaces, ~, ^, etc.).
func TestSetExternalRef_RejectsInvalidFormat(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		ref  string
	}{
		{"space in key", "jira:DPF 456"},
		{"tilde in key", "jira:DPF~456"},
		{"caret in key", "jira:DPF^456"},
		{"colon in key", "jira:DPF:456"},
		{"question mark in key", "jira:DPF?456"},
		{"asterisk in key", "jira:DPF*456"},
		{"bracket in key", "jira:[DPF456"},
		{"backslash in key", `jira:\DPF456`},
		{"space in provider", "jir a:DPF-456"},
		{"no colon", "DPF-456"},
		{"empty key", "jira:"},
		{"empty provider", ":DPF-456"},
		{"space after colon", "jira: DPF-456"},
		{"consecutive dots in key", "jira:DPF..456"},
		{"trailing dot in key", "jira:DPF."},
		{"trailing .lock in key", "jira:DPF.lock"},
		{"leading dot in key", "jira:.DPF-456"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// When: SetExternalRef is called with an invalid format.
			err := c.SetExternalRef(item.ID, tc.ref)
			// Then: an error is returned and the field is not updated.
			if err == nil {
				t.Errorf("SetExternalRef(%q): expected error for invalid format, got nil", tc.ref)
			}
		})
	}
}

// TestSetExternalRef_AcceptsValidFormats verifies that SetExternalRef accepts
// well-formed 'provider:key' values with git-branch-safe characters.
func TestSetExternalRef_AcceptsValidFormats(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	cases := []string{
		"jira:DPF-456",
		"linear:LIN-789",
		"jira:FEAT.99",
		"jira:FEAT_99",
		"my-provider:ABC-123",
		"github:issue-42",
	}

	for _, ref := range cases {
		t.Run(ref, func(t *testing.T) {
			// When: SetExternalRef is called with a valid format.
			if err := c.SetExternalRef(item.ID, ref); err != nil {
				t.Errorf("SetExternalRef(%q): unexpected error: %v", ref, err)
			}
		})
	}
}

// TestGetReadyForAqueduct_CaseInsensitiveRepo_ReturnsDroplet verifies that
// GetReadyForAqueduct respects case-insensitive repo matching.
func TestGetReadyForAqueduct_CaseInsensitiveRepo_ReturnsDroplet(t *testing.T) {
	c := testClient(t)
	// Given: a droplet stored with upper-case repo name.
	_, err := c.Add("CISTERN", "Cistern task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	// When: GetReadyForAqueduct is called with the canonical lower-case name.
	got, err := c.GetReadyForAqueduct("cistern", "default")
	if err != nil {
		t.Fatal(err)
	}

	// Then: the droplet is returned.
	if got == nil {
		t.Fatal("GetReadyForAqueduct(cistern): expected droplet, got nil")
	}
	if got.Title != "Cistern task" {
		t.Errorf("GetReadyForAqueduct(cistern): title = %q, want %q", got.Title, "Cistern task")
	}
}

// TestList_CaseInsensitiveRepo_ReturnsDroplets verifies that List filters by repo
// case-insensitively.
func TestList_CaseInsensitiveRepo_ReturnsDroplets(t *testing.T) {
	c := testClient(t)
	// Given: droplets stored with mixed-case repo names.
	c.Add("scaledtest", "Task A", "", 1, 2)
	c.Add("SCALEDTEST", "Task B", "", 1, 2)
	c.Add("other", "Task C", "", 1, 2)

	// When: List is called with canonical casing.
	items, err := c.List("ScaledTest", "")
	if err != nil {
		t.Fatal(err)
	}

	// Then: both ScaledTest-repo droplets are returned, "other" is excluded.
	if len(items) != 2 {
		t.Fatalf("List(ScaledTest): got %d items, want 2", len(items))
	}
}

// TestNew_RepoCaseMigration_NormalizesCanonicalRepos verifies that when New() is
// called on a DB containing wrong-cased canonical repo values, they are normalized
// to canonical casing (cistern, ScaledTest, PortfolioWebsite).
func TestNew_RepoCaseMigration_NormalizesCanonicalRepos(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate_repo.db")

	// Seed the DB with wrong-cased repo values, bypassing New() so the migration
	// has not yet run.
	seedDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = seedDB.Exec(`CREATE TABLE IF NOT EXISTS droplets (
		id TEXT PRIMARY KEY,
		repo TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		priority INTEGER DEFAULT 2,
		complexity INTEGER DEFAULT 2,
		status TEXT DEFAULT 'open',
		assignee TEXT DEFAULT '',
		current_cataractae TEXT DEFAULT '',
		outcome TEXT DEFAULT NULL,
		assigned_aqueduct TEXT DEFAULT '',
		last_reviewed_commit TEXT DEFAULT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id   string
		repo string
	}{
		{"id-1", "CISTERN"},
		{"id-2", "Cistern"},
		{"id-3", "SCALEDTEST"},
		{"id-4", "scaledtest"},
		{"id-5", "portfoliowebsite"},
		{"id-6", "PORTFOLIOWEBSITE"},
		{"id-7", "unrelated"},         // should not be touched
		{"id-8", "cistern"},           // already canonical — should not change
		{"id-9", "ScaledTest"},        // already canonical — should not change
		{"id-10", "PortfolioWebsite"}, // already canonical — should not change
	} {
		if _, err := seedDB.Exec(`INSERT INTO droplets (id, repo, title) VALUES (?, ?, 't')`, row.id, row.repo); err != nil {
			t.Fatal(err)
		}
	}
	seedDB.Close()

	// Open with New() to trigger the migration.
	c, err := New(dbPath, "bf")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	cases := []struct {
		id       string
		wantRepo string
	}{
		{"id-1", "cistern"},
		{"id-2", "cistern"},
		{"id-3", "ScaledTest"},
		{"id-4", "ScaledTest"},
		{"id-5", "PortfolioWebsite"},
		{"id-6", "PortfolioWebsite"},
		{"id-7", "unrelated"},
		{"id-8", "cistern"},
		{"id-9", "ScaledTest"},
		{"id-10", "PortfolioWebsite"},
	}
	for _, tc := range cases {
		var got string
		if err := c.db.QueryRow(`SELECT repo FROM droplets WHERE id = ?`, tc.id).Scan(&got); err != nil {
			t.Fatalf("id=%s: %v", tc.id, err)
		}
		if got != tc.wantRepo {
			t.Errorf("id=%s: repo after migration = %q, want %q", tc.id, got, tc.wantRepo)
		}
	}
}

// TestNew_RepoCaseMigration_IsIdempotent verifies that running New() on an already-
// migrated database does not alter canonical repo values a second time.
func TestNew_RepoCaseMigration_IsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "idempotent_repo.db")

	// First open: migration runs.
	c1, err := New(dbPath, "bf")
	if err != nil {
		t.Fatal(err)
	}
	c1.Add("cistern", "Task", "", 1, 2)
	c1.Close()

	// Second open: migration must be a no-op.
	c2, err := New(dbPath, "bf")
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	// Verify the migration sentinel exists exactly once (under the numbered ID).
	var count int
	if err := c2.db.QueryRow(`SELECT COUNT(*) FROM _schema_migrations WHERE id = '004_repo_case_normalize'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("_schema_migrations sentinel count = %d, want 1", count)
	}
}

// TestSearch_CancelledStatus_ReturnsCancelled verifies that Search with an explicit
// status="cancelled" filter returns cancelled droplets.
func TestSearch_CancelledStatus_ReturnsCancelled(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Active task", "", 1, 3)
	cancelled, _ := c.Add("myrepo", "Cancelled task", "", 1, 3)
	c.Cancel(cancelled.ID, "not needed")

	results, err := c.Search("", "cancelled", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("Search(cancelled) returned %d items, want 1", len(results))
	}
	if results[0].ID != cancelled.ID {
		t.Errorf("Search(cancelled) returned %s, want %s", results[0].ID, cancelled.ID)
	}
}

// TestCloseItem_ClearsAssignedAqueduct verifies that delivering a droplet removes
// the assigned_aqueduct so no ghost assignments linger after terminal state.
func TestCloseItem_ClearsAssignedAqueduct(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Feature", "", 1, 2)
	c.SetAssignedAqueduct(item.ID, "cistern-alpha")
	pre, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pre.AssignedAqueduct != "cistern-alpha" {
		t.Fatal("precondition failed: SetAssignedAqueduct did not set the field")
	}

	if err := c.CloseItem(item.ID); err != nil {
		t.Fatal(err)
	}

	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AssignedAqueduct != "" {
		t.Errorf("AssignedAqueduct after CloseItem = %q, want empty string", got.AssignedAqueduct)
	}
}

// TestPool_ClearsAssignedAqueduct verifies that pooling a droplet
// removes assigned_aqueduct so no ghost assignments linger.
func TestPool_ClearsAssignedAqueduct(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Stuck task", "", 1, 2)
	c.SetAssignedAqueduct(item.ID, "cistern-beta")
	pre, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pre.AssignedAqueduct != "cistern-beta" {
		t.Fatal("precondition failed: SetAssignedAqueduct did not set the field")
	}

	if err := c.Pool(item.ID, "timeout"); err != nil {
		t.Fatal(err)
	}

	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AssignedAqueduct != "" {
		t.Errorf("AssignedAqueduct after Pool = %q, want empty string", got.AssignedAqueduct)
	}
}

// TestSetAssignedAqueduct_WhenAlreadySet_DoesNotOverwrite verifies that the
// conditional WHERE clause prevents a second SetAssignedAqueduct call from
// overwriting an existing assignment.
func TestSetAssignedAqueduct_WhenAlreadySet_DoesNotOverwrite(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Contested task", "", 1, 2)

	// First assignment should succeed.
	if err := c.SetAssignedAqueduct(item.ID, "cistern-alpha"); err != nil {
		t.Fatal(err)
	}
	pre, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pre.AssignedAqueduct != "cistern-alpha" {
		t.Fatalf("precondition failed: want AssignedAqueduct = %q, got %q", "cistern-alpha", pre.AssignedAqueduct)
	}

	// Second assignment with a different value must not overwrite.
	if err := c.SetAssignedAqueduct(item.ID, "cistern-beta"); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AssignedAqueduct != "cistern-alpha" {
		t.Errorf("AssignedAqueduct after second SetAssignedAqueduct = %q, want %q (original must not be overwritten)", got.AssignedAqueduct, "cistern-alpha")
	}
}

// TestNew_LastHeartbeatAtMigration_AddsColumnToExistingDB verifies that when
// New() is called on a DB that predates the last_heartbeat_at column, the ALTER
// TABLE migration adds the column so that Heartbeat() and subsequent reads work
// without error.
//
// Given: a DB created without last_heartbeat_at (simulating a pre-migration install)
// When:  New() is called to open that DB
// Then:  Heartbeat() succeeds and Get returns a non-zero LastHeartbeatAt
func TestNew_LastHeartbeatAtMigration_AddsColumnToExistingDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	// Seed a DB that has stage_dispatched_at but not last_heartbeat_at,
	// bypassing New() so the migration has not yet run.
	seedDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seedDB.Exec(`CREATE TABLE IF NOT EXISTS droplets (
		id TEXT PRIMARY KEY,
		repo TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		priority INTEGER DEFAULT 2,
		complexity INTEGER DEFAULT 1,
		status TEXT DEFAULT 'open',
		assignee TEXT DEFAULT '',
		current_cataractae TEXT DEFAULT '',
		outcome TEXT DEFAULT NULL,
		assigned_aqueduct TEXT DEFAULT '',
		last_reviewed_commit TEXT DEFAULT NULL,
		external_ref TEXT DEFAULT NULL,
		stage_dispatched_at DATETIME DEFAULT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := seedDB.Exec(`INSERT INTO droplets (id, repo, title) VALUES ('migrate-test', 'myrepo', 'heartbeat migration test')`); err != nil {
		t.Fatal(err)
	}
	seedDB.Close()

	// When: New() is called — the ALTER TABLE migration should add last_heartbeat_at.
	c, err := New(dbPath, "bf")
	if err != nil {
		t.Fatalf("New() on legacy DB: %v", err)
	}
	defer c.Close()

	// Then: Heartbeat must succeed (would fail with "no such column" without the migration).
	if err := c.Heartbeat("migrate-test"); err != nil {
		t.Fatalf("Heartbeat() after migration: %v", err)
	}

	// And: Get must return a populated LastHeartbeatAt.
	got, err := c.Get("migrate-test")
	if err != nil {
		t.Fatalf("Get() after migration: %v", err)
	}
	if got.LastHeartbeatAt.IsZero() {
		t.Error("Get: LastHeartbeatAt is zero after migration + Heartbeat(); expected non-zero")
	}
}

// TestExternalRef_RoundTrips_ThroughGetReadyForAqueduct verifies that
// GetReadyForAqueduct — the primary dispatch path used by the castellarius —
// returns the external_ref stored on the droplet.
//
// Given: a droplet with ExternalRef "jira:DPF-456" in the ready state
// When:  GetReadyForAqueduct is called for the droplet's repo and aqueduct
// Then:  the returned droplet has ExternalRef == "jira:DPF-456"
func TestExternalRef_RoundTrips_ThroughGetReadyForAqueduct(t *testing.T) {
	c := testClient(t)
	// Given: a droplet with an external_ref set.
	item, err := c.Add("myrepo", "Imported task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetExternalRef(item.ID, "jira:DPF-456"); err != nil {
		t.Fatal(err)
	}

	// When: GetReadyForAqueduct is called (droplet has no assigned aqueduct, so it
	// is eligible for any aqueduct).
	got, err := c.GetReadyForAqueduct("myrepo", "default")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetReadyForAqueduct: expected droplet, got nil")
	}

	// Then: ExternalRef is populated.
	if got.ExternalRef != "jira:DPF-456" {
		t.Errorf("GetReadyForAqueduct ExternalRef = %q, want %q", got.ExternalRef, "jira:DPF-456")
	}
}

// --- GetDropletChanges tests ---

func TestGetDropletChanges_Empty(t *testing.T) {
	c := testClient(t)

	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Errorf("got %d changes, want 1 (create event) for new droplet", len(changes))
	}
	if changes[0].Kind != "event" {
		t.Errorf("Kind = %q, want event", changes[0].Kind)
	}
}

func TestGetDropletChanges_ReturnsOnlyEvents(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.AddNote(item.ID, "implement", "wrote the code"); err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("got %d changes, want 1 (create event only; notes excluded)", len(changes))
	}
	for _, ch := range changes {
		if ch.Kind != "event" {
			t.Errorf("Kind = %q, want 'event' for all changes", ch.Kind)
		}
	}
}

func TestGetDropletChanges_ReturnsEvents(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	c.Pool(item.ID, "needs review")

	changes, err := c.GetDropletChanges(item.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	// create event + pool event = 2
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want 2", len(changes))
	}
	poolFound := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "pool") {
			poolFound = true
			break
		}
	}
	if !poolFound {
		t.Errorf("pool event not found in changes: %v", changes)
	}
}

func TestGetDropletChanges_MixedNotesAndEvents(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	c.AddNote(item.ID, "implement", "wrote code")
	c.Pool(item.ID, "stuck")
	c.AddNote(item.ID, "review", "found issues")

	changes, err := c.GetDropletChanges(item.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	// create event + pool event = 2 (notes excluded)
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want 2 (events only)", len(changes))
	}
	for _, ch := range changes {
		if ch.Kind != "event" {
			t.Errorf("Kind = %q, want 'event' for all changes", ch.Kind)
		}
	}
}

func TestGetDropletChanges_RespectsLimit_ReturnsNewest(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	for i := range 5 {
		c.RecordEvent(item.ID, "pass", fmt.Sprintf(`{"cataractae":"step-%d"}`, i))
	}

	// create event + 5 pass events = 6 total; limit 3 returns the 3 newest, ordered oldest-first
	changes, err := c.GetDropletChanges(item.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("got %d changes, want 3 (limit applied)", len(changes))
	}
	// Verify we got the newest 3 events (pass-2, pass-3, pass-4), not the oldest (create, pass-0, pass-1)
	for _, ch := range changes {
		if ch.Kind != "event" {
			t.Errorf("Kind = %q, want 'event'", ch.Kind)
		}
	}
	if changes[0].Value != "pass: {\"cataractae\":\"step-2\"}" {
		t.Errorf("first change = %q, want step-2 (newest slice starts at step-2)", changes[0].Value)
	}
	if changes[2].Value != "pass: {\"cataractae\":\"step-4\"}" {
		t.Errorf("last change = %q, want step-4 (newest slice ends at step-4)", changes[2].Value)
	}
}

func TestGetDropletChanges_OrderedOldestFirst(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	c.Pool(item.ID, "first event")
	time.Sleep(10 * time.Millisecond)
	c.RecordEvent(item.ID, "pass", `{"cataractae":"reviewer"}`)

	// create event + pool event + pass event = 3 total
	changes, err := c.GetDropletChanges(item.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("got %d changes, want 3", len(changes))
	}
	if !changes[0].Time.Before(changes[1].Time) {
		t.Errorf("changes not in chronological order: %v >= %v", changes[0].Time, changes[1].Time)
	}
}

func TestGetDropletChanges_NotFoundReturnEmpty(t *testing.T) {
	c := testClient(t)

	changes, err := c.GetDropletChanges("nonexistent", 20)
	if err != nil {
		t.Fatalf("expected no error for nonexistent droplet, got: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("got %d changes, want 0 for nonexistent droplet", len(changes))
	}
}

// --- GetDropletTimeline tests ---

func TestGetDropletTimeline_ReturnsEvents(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Timeline task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	c.Pool(item.ID, "needs review")

	entries, err := c.GetDropletTimeline(item.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (create + pool)", len(entries))
	}
	if entries[0].EventType != "create" {
		t.Errorf("first entry EventType = %q, want 'create'", entries[0].EventType)
	}
	if entries[1].EventType != "pool" {
		t.Errorf("second entry EventType = %q, want 'pool'", entries[1].EventType)
	}
}

func TestGetDropletTimeline_OrderedOldestFirst(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Timeline order", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.Pool(item.ID, "first event")
	time.Sleep(10 * time.Millisecond)
	c.RecordEvent(item.ID, "pass", `{"cataractae":"reviewer"}`)

	entries, err := c.GetDropletTimeline(item.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	if !entries[0].Time.Before(entries[1].Time) {
		t.Errorf("entries not in chronological order: %v >= %v", entries[0].Time, entries[1].Time)
	}
}

func TestGetDropletTimeline_NotFoundReturnEmpty(t *testing.T) {
	c := testClient(t)

	entries, err := c.GetDropletTimeline("nonexistent", 0)
	if err != nil {
		t.Fatalf("expected no error for nonexistent droplet, got: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 for nonexistent droplet", len(entries))
	}
}

func TestGetDropletTimeline_IncludesStructuredPayload(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Payload task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := c.GetDropletTimeline(item.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Payload == "" {
		t.Error("create event should have a non-empty payload")
	}
}

func TestGetDropletTimeline_RespectsLimit_ReturnsNewest(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Limit task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.Pool(item.ID, "first")
	time.Sleep(10 * time.Millisecond)
	c.RecordEvent(item.ID, "pass", `{"cataractae":"reviewer"}`)
	time.Sleep(10 * time.Millisecond)
	c.RecordEvent(item.ID, "dispatch", `{"aqueduct":"default","cataractae":"implement"}`)

	entries, err := c.GetDropletTimeline(item.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries with limit=2, want 2", len(entries))
	}
	if entries[0].EventType != "pass" {
		t.Errorf("first entry EventType = %q, want 'pass' (newest entries with oldest first display)", entries[0].EventType)
	}
	if entries[1].EventType != "dispatch" {
		t.Errorf("second entry EventType = %q, want 'dispatch'", entries[1].EventType)
	}
}

func TestGetDropletTimeline_LimitZero_ReturnsAll(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "No-limit task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.Pool(item.ID, "first")
	c.RecordEvent(item.ID, "pass", `{"cataractae":"reviewer"}`)

	entries, err := c.GetDropletTimeline(item.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries with limit=0, want 3 (all events)", len(entries))
	}
}

// --- DisplayInfo tests ---

func TestDisplayInfo_MapsEventTypesToHumanReadableLabels(t *testing.T) {
	tests := []struct {
		eventType string
		payload   string
		wantLabel string
		wantSub   string
	}{
		{"create", `{"repo":"myrepo","title":"My task","priority":1,"complexity":2}`, "created", "repo: myrepo"},
		{"dispatch", `{"aqueduct":"default","cataractae":"implement"}`, "dispatched", "step: implement"},
		{"pass", `{"cataractae":"reviewer","notes":"all good"}`, "pass", "by: reviewer"},
		{"recirculate", `{"cataractae":"reviewer","target":"implement"}`, "recirculate", "to: implement"},
		{"delivered", `{}`, "delivered", ""},
		{"restart", `{"cataractae":"implement"}`, "restart", "by: implement"},
		{"approve", `{"cataractae":"manual"}`, "approved", "by: manual"},
		{"edit", `{"fields":["title"]}`, "edit", "fields:"},
		{"pool", `{"reason":"blocked"}`, "pooled", "reason: blocked"},
		{"cancel", `{"reason":"not needed"}`, "cancelled", "reason: not needed"},
		{"exit_no_outcome", `{"session":"s1","worker":"w1"}`, "exit_no_outcome", "session: s1"},
		{"stall", `{"cataractae":"implement","elapsed":"5m"}`, "stall", "step: implement"},
		{"recovery", `{"cataractae":"implement"}`, "recovery", "by: implement"},
		{"circuit_breaker", `{"death_count":3,"window":"10m"}`, "circuit_breaker", "dead sessions: 3"},
		{"loop_recovery", `{"from":"impl","to":"impl"}`, "loop_recovery", "from: impl"},
		{"auto_promote", `{"cataractae":"impl","routed_to":"review"}`, "auto_promote", "routed to: review"},
		{"no_route", `{"cataractae":"impl"}`, "no_route", "by: impl"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			gotLabel, gotDetail := DisplayInfo(tt.eventType, tt.payload)
			if gotLabel != tt.wantLabel {
				t.Errorf("label = %q, want %q", gotLabel, tt.wantLabel)
			}
			if tt.wantSub != "" && !strings.Contains(gotDetail, tt.wantSub) {
				t.Errorf("detail = %q, want substring %q", gotDetail, tt.wantSub)
			}
			if tt.wantSub == "" && gotDetail != "" {
				t.Errorf("detail = %q, want empty", gotDetail)
			}
		})
	}
}

func TestDisplayInfo_UnknownEventType(t *testing.T) {
	label, detail := DisplayInfo("unknown_event", `{"data":"test"}`)
	if label != "unknown_event" {
		t.Errorf("label = %q, want 'unknown_event'", label)
	}
	if detail != `{"data":"test"}` {
		t.Errorf("detail = %q, want original payload", detail)
	}
}

// --- Restart tests ---

func TestRestart_WithCataractae(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Stuck feature", "", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	c.GetReady("myrepo")
	c.Assign(item.ID, "alice", "implement")
	c.SetOutcome(item.ID, "pass")

	got, err := c.Restart(item.ID, "review")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "open" {
		t.Errorf("Status = %q, want %q", got.Status, "open")
	}
	if got.Assignee != "" {
		t.Errorf("Assignee = %q, want empty", got.Assignee)
	}
	if got.Outcome != "" {
		t.Errorf("Outcome = %q, want empty", got.Outcome)
	}
	if got.CurrentCataractae != "review" {
		t.Errorf("CurrentCataractae = %q, want %q", got.CurrentCataractae, "review")
	}
	if got.AssignedAqueduct != "" {
		t.Errorf("AssignedAqueduct = %q, want empty", got.AssignedAqueduct)
	}
}

func TestRestart_WithSameCataractae(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	c.GetReady("myrepo")
	c.Assign(item.ID, "bob", "implement")

	got, err := c.Restart(item.ID, "implement")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "open" {
		t.Errorf("Status = %q, want %q", got.Status, "open")
	}
	if got.CurrentCataractae != "implement" {
		t.Errorf("CurrentCataractae = %q, want %q", got.CurrentCataractae, "implement")
	}
	if got.Assignee != "" {
		t.Errorf("Assignee = %q, want empty", got.Assignee)
	}
}

func TestRestart_RecordsRestartEvent(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 3)
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Restart(item.ID, "delivery")
	if err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "restart") && strings.Contains(ch.Value, "delivery") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("restart event with cataractae 'delivery' not found in changes: %v", changes)
	}
}

func TestRestart_PreservesExistingNotes(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	c.AddNote(item.ID, "implement", "first implementation note")
	c.AddNote(item.ID, "review", "first review note")

	_, err = c.Restart(item.ID, "implement")
	if err != nil {
		t.Fatal(err)
	}

	notes, err := c.GetNotes(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	existingCount := 0
	for _, n := range notes {
		if n.CataractaeName != "scheduler" {
			existingCount++
		}
	}
	if existingCount != 2 {
		t.Errorf("got %d existing notes, want 2", existingCount)
	}
}

func TestRestart_ClearsOutcomeAndAssignee(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	c.GetReady("myrepo")
	c.Assign(item.ID, "worker-1", "review")
	c.SetOutcome(item.ID, "pass")
	c.SetAssignedAqueduct(item.ID, "cistern-alpha")

	got, err := c.Restart(item.ID, "implement")
	if err != nil {
		t.Fatal(err)
	}
	if got.Assignee != "" {
		t.Errorf("Assignee = %q, want empty after restart", got.Assignee)
	}
	if got.Outcome != "" {
		t.Errorf("Outcome = %q, want empty after restart", got.Outcome)
	}
	if got.AssignedAqueduct != "" {
		t.Errorf("AssignedAqueduct = %q, want empty after restart", got.AssignedAqueduct)
	}
}

func TestRestart_NotFound(t *testing.T) {
	c := testClient(t)
	_, err := c.Restart("nonexistent", "implement")
	if err == nil {
		t.Error("expected error for nonexistent droplet")
	}
}

func TestRestart_UpdatesTimestamp(t *testing.T) {
	c := testClient(t)
	item, err := c.Add("myrepo", "Task", "", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	before := item.UpdatedAt

	time.Sleep(10 * time.Millisecond)
	got, err := c.Restart(item.ID, "review")
	if err != nil {
		t.Fatal(err)
	}
	if !got.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt = %v, want after %v", got.UpdatedAt, before)
	}
}

func TestRestart_ClearsStageDispatchedAt(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)
	c.GetReady("myrepo")
	c.Assign(item.ID, "worker-1", "implement")

	dispatched, _ := c.Get(item.ID)
	if dispatched.StageDispatchedAt.IsZero() {
		t.Fatal("precondition: StageDispatchedAt must be set after Assign with worker")
	}

	got, err := c.Restart(item.ID, "review")
	if err != nil {
		t.Fatal(err)
	}
	if !got.StageDispatchedAt.IsZero() {
		t.Errorf("StageDispatchedAt = %v, want zero after restart", got.StageDispatchedAt)
	}

	reloaded, _ := c.Get(item.ID)
	if !reloaded.StageDispatchedAt.IsZero() {
		t.Errorf("reloaded StageDispatchedAt = %v, want zero after restart", reloaded.StageDispatchedAt)
	}
}

func TestRestart_ClearsLastHeartbeatAt(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Task", "", 1, 3)

	if err := c.Heartbeat(item.ID); err != nil {
		t.Fatal(err)
	}
	pre, _ := c.Get(item.ID)
	if pre.LastHeartbeatAt.IsZero() {
		t.Fatal("precondition: LastHeartbeatAt must be set after Heartbeat()")
	}

	got, err := c.Restart(item.ID, "implement")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastHeartbeatAt.IsZero() {
		t.Errorf("LastHeartbeatAt = %v, want zero after restart", got.LastHeartbeatAt)
	}

	reloaded, _ := c.Get(item.ID)
	if !reloaded.LastHeartbeatAt.IsZero() {
		t.Errorf("reloaded LastHeartbeatAt = %v, want zero after restart", reloaded.LastHeartbeatAt)
	}
}

// ── FilterSession CRUD ──

func TestClient_CreateFilterSession(t *testing.T) {
	c := testClient(t)
	s, err := c.CreateFilterSession("My idea", "A description")
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "My idea" {
		t.Errorf("title = %q, want %q", s.Title, "My idea")
	}
	if s.Description != "A description" {
		t.Errorf("description = %q, want %q", s.Description, "A description")
	}
	if s.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if s.Messages != "[]" {
		t.Errorf("messages = %q, want %q", s.Messages, "[]")
	}
	if s.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestClient_GetFilterSession_NotFound(t *testing.T) {
	c := testClient(t)
	_, err := c.GetFilterSession("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestClient_GetFilterSession(t *testing.T) {
	c := testClient(t)
	created, err := c.CreateFilterSession("Test session", "Desc")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.GetFilterSession(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Test session" {
		t.Errorf("title = %q, want %q", got.Title, "Test session")
	}
	if got.Description != "Desc" {
		t.Errorf("description = %q, want %q", got.Description, "Desc")
	}
}

func TestClient_GetFilterSession_LLMSessionID(t *testing.T) {
	c := testClient(t)
	s, err := c.CreateFilterSession("LLM test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateFilterSessionMessages(s.ID, "[]", "", "llm-persist-99"); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetFilterSession(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LLMSessionID != "llm-persist-99" {
		t.Errorf("llm_session_id = %q, want %q", got.LLMSessionID, "llm-persist-99")
	}
}

func TestClient_ListFilterSessions(t *testing.T) {
	c := testClient(t)
	_, err := c.CreateFilterSession("Session 1", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateFilterSession("Session 2", "")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := c.ListFilterSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestClient_UpdateFilterSessionMessages(t *testing.T) {
	c := testClient(t)
	s, err := c.CreateFilterSession("Update test", "")
	if err != nil {
		t.Fatal(err)
	}
	msgs := `[{"role":"user","content":"hello"},{"role":"assistant","content":"hi there"}]`
	if err := c.UpdateFilterSessionMessages(s.ID, msgs, "spec v1", "llm-sess-42"); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetFilterSession(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Messages != msgs {
		t.Errorf("messages = %q, want %q", got.Messages, msgs)
	}
	if got.SpecSnapshot != "spec v1" {
		t.Errorf("spec_snapshot = %q, want %q", got.SpecSnapshot, "spec v1")
	}
	if got.LLMSessionID != "llm-sess-42" {
		t.Errorf("llm_session_id = %q, want %q", got.LLMSessionID, "llm-sess-42")
	}
}

func TestClient_UpdateFilterSessionMessages_NotFound(t *testing.T) {
	c := testClient(t)
	err := c.UpdateFilterSessionMessages("nonexistent", "[]", "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestClient_DeleteFilterSession(t *testing.T) {
	c := testClient(t)
	s, err := c.CreateFilterSession("Delete me", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteFilterSession(s.ID); err != nil {
		t.Fatalf("DeleteFilterSession: %v", err)
	}
	_, err = c.GetFilterSession(s.ID)
	if err == nil {
		t.Fatal("expected error getting deleted session")
	}
}

func TestClient_DeleteFilterSession_NotFound(t *testing.T) {
	c := testClient(t)
	err := c.DeleteFilterSession("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestRecordEvent_ValidType(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Event test", "", 1, 2)

	err := c.RecordEvent(item.ID, EventPass, `{"cataractae":"implementer","notes":"all good"}`)
	if err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "pass") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("pass event not found in changes: %v", changes)
	}
}

func TestRecordEvent_UnknownType_ReturnsError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Event test", "", 1, 2)

	err := c.RecordEvent(item.ID, "invalid_type", "{}")
	if err == nil {
		t.Fatal("expected error for unknown event type")
	}
	if !strings.Contains(err.Error(), "unknown event type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRecordEvent_InvalidJSON_ReturnsError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Event test", "", 1, 2)

	err := c.RecordEvent(item.ID, EventPass, "not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON payload")
	}
	if !strings.Contains(err.Error(), "valid JSON") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRecordEvent_EmptyPayload(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Event test", "", 1, 2)

	err := c.RecordEvent(item.ID, EventDelivered, "{}")
	if err != nil {
		t.Fatalf("RecordEvent with empty payload: %v", err)
	}
}

func TestAdd_RecordsCreateEvent(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Create event test", "", 1, 2)

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "create: ") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("create event not found in changes: %v", changes)
	}
}

func TestGetReady_RecordsDispatchEvent(t *testing.T) {
	c := testClient(t)
	c.Add("myrepo", "Dispatch test", "", 1, 2)

	_, err := c.GetReady("myrepo")
	if err != nil {
		t.Fatal(err)
	}

	items, err := c.List("myrepo", "in_progress")
	if err != nil {
		t.Fatal(err)
	}
	changes, err := c.GetDropletChanges(items[0].ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "dispatch: ") && strings.Contains(ch.Value, "cataractae") && strings.Contains(ch.Value, "assignee") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("dispatch event with cataractae/assignee not found in changes: %v", changes)
	}
}

func TestGetReadyForAqueduct_RecordsDispatchEvent(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Dispatch aqueduct test", "", 1, 2)

	_, err := c.GetReadyForAqueduct("myrepo", "my-aqueduct")
	if err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "dispatch: ") && strings.Contains(ch.Value, "my-aqueduct") && strings.Contains(ch.Value, "cataractae") && strings.Contains(ch.Value, "assignee") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("dispatch event with aqueduct, cataractae, and assignee not found in changes: %v", changes)
	}
}

func TestCloseItem_RecordsDeliveredEvent(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Delivered event test", "", 1, 2)

	if err := c.CloseItem(item.ID); err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "delivered") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("delivered event not found in changes: %v", changes)
	}
}

func TestPool_RecordsPoolEventWithJSONPayload(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Pool event test", "", 1, 2)

	if err := c.Pool(item.ID, "stuck on flaky test"); err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "pool") && strings.Contains(ch.Value, "stuck on flaky test") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("pool event with reason not found in changes: %v", changes)
	}
}

func TestCancel_RecordsCancelEventWithJSONPayload(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Cancel event test", "", 1, 2)

	reason := "superseded by new approach"
	if err := c.Cancel(item.ID, reason); err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "cancel") && strings.Contains(ch.Value, reason) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cancel event with reason not found in changes: %v", changes)
	}
}

func TestCancel_NoLongerWritesSchedulerNote(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Cancel no note test", "", 1, 2)

	if err := c.Cancel(item.ID, "reason"); err != nil {
		t.Fatal(err)
	}

	notes, err := c.GetNotes(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if n.CataractaeName == "scheduler" {
			t.Errorf("cancel should no longer write scheduler notes, found: %s", n.Content)
		}
	}
}

func TestRestart_NoLongerWritesSchedulerNote(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Restart no note test", "", 1, 2)

	_, err := c.Restart(item.ID, "implement")
	if err != nil {
		t.Fatal(err)
	}

	notes, err := c.GetNotes(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if n.CataractaeName == "scheduler" {
			t.Errorf("restart should no longer write scheduler notes, found: %s", n.Content)
		}
	}
}

func TestEditDroplet_RecordsEditEvent(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Edit event test", "", 1, 2)

	err := c.EditDroplet(item.ID, EditDropletFields{Title: ptr("New Title"), Priority: ptr(3)})
	if err != nil {
		t.Fatal(err)
	}

	changes, err := c.GetDropletChanges(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "edit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("edit event not found in changes: %v", changes)
	}
}

func TestValidEventTypes_ContainsAllConstants(t *testing.T) {
	expected := []string{
		EventCreate, EventDispatch, EventPass, EventRecirculate,
		EventDelivered, EventRestart, EventApprove, EventEdit,
		EventPool, EventCancel,
		EventExitNoOutcome, EventStall, EventRecovery,
		EventCircuitBreaker, EventLoopRecovery,
		EventAutoPromote, EventNoRoute,
	}
	for _, e := range expected {
		if !ValidEventTypes[e] {
			t.Errorf("ValidEventTypes missing %q", e)
		}
	}
	if len(ValidEventTypes) != len(expected) {
		t.Errorf("ValidEventTypes has %d entries, want %d", len(ValidEventTypes), len(expected))
	}
}

func TestPass_SetsOutcomeAndRecordsEvent(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Pass test", "", 1, 2)
	c.UpdateStatus(item.ID, "in_progress")

	if err := c.Pass(item.ID, "implementer", "all good"); err != nil {
		t.Fatalf("Pass: %v", err)
	}

	got, _ := c.Get(item.ID)
	if got.Outcome != "pass" {
		t.Errorf("outcome = %q, want pass", got.Outcome)
	}

	changes, _ := c.GetDropletChanges(item.ID, 100)
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "pass") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("pass event not found in changes: %v", changes)
	}
}

func TestPass_WhenTerminal_ReturnsError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Pass terminal test", "", 1, 2)
	c.CloseItem(item.ID)

	err := c.Pass(item.ID, "reviewer", "")
	if err == nil {
		t.Fatal("expected error for terminal status")
	}
	if !strings.Contains(err.Error(), "terminal status") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPass_NotFound(t *testing.T) {
	c := testClient(t)
	if err := c.Pass("nonexistent", "reviewer", ""); err == nil {
		t.Fatal("expected error for nonexistent droplet")
	}
}

func TestRecirculate_InProgress_SetsOutcomeAndRecordsEvent(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Recirculate test", "", 1, 2)
	c.UpdateStatus(item.ID, "in_progress")

	if err := c.Recirculate(item.ID, "reviewer", "implement", "needs fixes"); err != nil {
		t.Fatalf("Recirculate: %v", err)
	}

	got, _ := c.Get(item.ID)
	if got.Outcome != "recirculate:implement" {
		t.Errorf("outcome = %q, want recirculate:implement", got.Outcome)
	}

	changes, _ := c.GetDropletChanges(item.ID, 100)
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "recirculate") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("recirculate event not found in changes: %v", changes)
	}
}

func TestRecirculate_NonInProgress_AssignsAndRecordsEvent(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Recirculate non-in-progress", "", 1, 2)
	c.Pool(item.ID, "stuck")

	if err := c.Recirculate(item.ID, "reviewer", "implement", ""); err != nil {
		t.Fatalf("Recirculate on pooled: %v", err)
	}

	got, _ := c.Get(item.ID)
	if got.Status != "open" {
		t.Errorf("status = %q, want open", got.Status)
	}
	if got.CurrentCataractae != "implement" {
		t.Errorf("current_cataractae = %q, want implement", got.CurrentCataractae)
	}
}

func TestRecirculate_NonInProgress_DefaultsToCurrentCataractae(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Recirculate default target", "", 1, 2)
	c.Pool(item.ID, "stuck")
	c.SetCataractae(item.ID, "review")

	if err := c.Recirculate(item.ID, "reviewer", "", ""); err != nil {
		t.Fatalf("Recirculate: %v", err)
	}

	got, _ := c.Get(item.ID)
	if got.CurrentCataractae != "review" {
		t.Errorf("current_cataractae = %q, want review (defaults to current)", got.CurrentCataractae)
	}
}

func TestRecirculate_WhenTerminal_ReturnsError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Recirculate terminal test", "", 1, 2)
	c.Cancel(item.ID, "not needed")

	if err := c.Recirculate(item.ID, "reviewer", "", ""); err == nil {
		t.Fatal("expected error for terminal status")
	}
}

func TestRecirculate_NotFound(t *testing.T) {
	c := testClient(t)
	if err := c.Recirculate("nonexistent", "reviewer", "", ""); err == nil {
		t.Fatal("expected error for nonexistent droplet")
	}
}

func TestApprove_SetsDeliveryStepAndRecordsEvent(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Approve test", "", 1, 2)
	c.SetCataractae(item.ID, "human")

	if err := c.Approve(item.ID, "manual"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got, _ := c.Get(item.ID)
	if got.CurrentCataractae != "delivery" {
		t.Errorf("current_cataractae = %q, want delivery", got.CurrentCataractae)
	}
	if got.Status != "open" {
		t.Errorf("status = %q, want open", got.Status)
	}

	changes, _ := c.GetDropletChanges(item.ID, 100)
	found := false
	for _, ch := range changes {
		if ch.Kind == "event" && strings.Contains(ch.Value, "approve") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("approve event not found in changes: %v", changes)
	}
}

func TestApprove_NotHumanGated(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Approve not human test", "", 1, 2)

	if err := c.Approve(item.ID, "manual"); err == nil {
		t.Fatal("expected error for non-human gated droplet")
	}
}

func TestApprove_WhenTerminal_ReturnsError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Approve terminal test", "", 1, 2)
	c.SetCataractae(item.ID, "human")
	c.Cancel(item.ID, "not needed")

	if err := c.Approve(item.ID, "manual"); err == nil {
		t.Fatal("expected error for terminal status")
	}
}

func TestApprove_NotFound(t *testing.T) {
	c := testClient(t)
	if err := c.Approve("nonexistent", "manual"); err == nil {
		t.Fatal("expected error for nonexistent droplet")
	}
}

func TestPass_DeliveredGuardInWhere(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Pass guard test", "", 1, 2)
	c.UpdateStatus(item.ID, "in_progress")

	c.CloseItem(item.ID)

	err := c.Pass(item.ID, "reviewer", "")
	if err == nil {
		t.Fatal("expected error when droplet becomes delivered before Pass")
	}
}

func TestRecirculate_CancelledGuardInWhere(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Recirculate guard test", "", 1, 2)
	c.UpdateStatus(item.ID, "in_progress")

	c.Cancel(item.ID, "obsolete")

	err := c.Recirculate(item.ID, "reviewer", "implement", "fix it")
	if err == nil {
		t.Fatal("expected error when droplet becomes cancelled before Recirculate")
	}
}

func TestApprove_CataractaeGuardInWhere(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Approve guard test", "", 1, 2)
	c.SetCataractae(item.ID, "human")

	c.SetCataractae(item.ID, "delivery")

	err := c.Approve(item.ID, "manual")
	if err == nil {
		t.Fatal("expected error when cataractae changed from human before Approve")
	}
}

func TestPool_Delivered_ReturnsTerminalError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Pool delivered guard test", "", 1, 2)
	c.CloseItem(item.ID)

	err := c.Pool(item.ID, "should fail")
	if err == nil {
		t.Fatal("expected error for delivered droplet")
	}
	if !strings.Contains(err.Error(), "terminal status") {
		t.Errorf("unexpected error: %v", err)
	}

	got, _ := c.Get(item.ID)
	if got.Status != "delivered" {
		t.Errorf("status = %q, want delivered (should not resurrect)", got.Status)
	}
}

func TestPool_Cancelled_ReturnsTerminalError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Pool cancelled guard test", "", 1, 2)
	c.Cancel(item.ID, "no longer needed")

	err := c.Pool(item.ID, "should fail")
	if err == nil {
		t.Fatal("expected error for cancelled droplet")
	}
	if !strings.Contains(err.Error(), "terminal status") {
		t.Errorf("unexpected error: %v", err)
	}

	got, _ := c.Get(item.ID)
	if got.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled (should not resurrect)", got.Status)
	}
}

func TestCloseItem_Cancelled_ReturnsTerminalError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Close cancelled guard test", "", 1, 2)
	c.Cancel(item.ID, "superseded")

	err := c.CloseItem(item.ID)
	if err == nil {
		t.Fatal("expected error for cancelled droplet")
	}
	if !strings.Contains(err.Error(), "terminal status") {
		t.Errorf("unexpected error: %v", err)
	}

	got, _ := c.Get(item.ID)
	if got.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled (should not resurrect)", got.Status)
	}
}

func TestCloseItem_Delivered_ReturnsTerminalError(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Close delivered guard test", "", 1, 2)
	c.CloseItem(item.ID)

	err := c.CloseItem(item.ID)
	if err == nil {
		t.Fatal("expected error for already-delivered droplet")
	}
	if !strings.Contains(err.Error(), "terminal status") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPool_DeliveredGuardInWhere(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Pool guard where test", "", 1, 2)
	c.UpdateStatus(item.ID, "in_progress")

	c.CloseItem(item.ID)

	err := c.Pool(item.ID, "should fail")
	if err == nil {
		t.Fatal("expected error when droplet becomes delivered before Pool")
	}
}

func TestCloseItem_CancelledGuardInWhere(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Close guard where test", "", 1, 2)
	c.UpdateStatus(item.ID, "in_progress")

	c.Cancel(item.ID, "obsolete")

	err := c.CloseItem(item.ID)
	if err == nil {
		t.Fatal("expected error when droplet becomes cancelled before CloseItem")
	}
}

func TestCountEventsByType_CountsMatchingEvents(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Count test", "", 1, 2)
	payload := `{"session":"s1","worker":"w1","cataractae":"implement"}`
	c.RecordEvent(item.ID, EventExitNoOutcome, payload)
	c.RecordEvent(item.ID, EventExitNoOutcome, payload)
	c.RecordEvent(item.ID, EventExitNoOutcome, payload)
	c.RecordEvent(item.ID, EventStall, `{"cataractae":"implement"}`)

	count, err := c.CountEventsByType(item.ID, EventExitNoOutcome, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("CountEventsByType = %d, want 3", count)
	}
}

func TestCountEventsByType_RespectsCutoff(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Cutoff test", "", 1, 2)

	c.RecordEvent(item.ID, EventExitNoOutcome, `{}`)
	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	c.RecordEvent(item.ID, EventExitNoOutcome, `{}`)

	count, err := c.CountEventsByType(item.ID, EventExitNoOutcome, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("CountEventsByType = %d, want 1 (only recent event)", count)
	}
}

func TestCountEventsByType_ZeroWhenNone(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Zero test", "", 1, 2)

	count, err := c.CountEventsByType(item.ID, EventExitNoOutcome, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("CountEventsByType = %d, want 0 when no events", count)
	}
}

func TestCountEventsByType_ZeroWhenWrongType(t *testing.T) {
	c := testClient(t)
	item, _ := c.Add("myrepo", "Wrong type test", "", 1, 2)
	c.RecordEvent(item.ID, EventStall, `{"cataractae":"implement"}`)

	count, err := c.CountEventsByType(item.ID, EventExitNoOutcome, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("CountEventsByType = %d, want 0 when only stall events exist", count)
	}
}
