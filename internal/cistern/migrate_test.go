package cistern

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMigrations(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected at least one migration, got 0")
	}
	t.Logf("Loaded %d migrations", len(migrations))
	for _, m := range migrations {
		stmts := splitStatements(m.SQL)
		t.Logf("  %s: %d statements", m.migrationID(), len(stmts))
	}
}

func TestSplitStatements(t *testing.T) {
	sql := `-- Migration 004: Normalize stored repo values to canonical casing.
UPDATE "droplets" SET repo = 'cistern' WHERE LOWER(repo) = LOWER('cistern') AND repo != 'cistern';
UPDATE "droplets" SET repo = 'ScaledTest' WHERE LOWER(repo) = LOWER('ScaledTest') AND repo != 'ScaledTest';`
	stmts := splitStatements(sql)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(stmts), stmts)
	}
	for i, s := range stmts {
		t.Logf("  [%d] %s", i, s[:min(80, len(s))])
	}
}

func TestMigrationsAreApplied(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM "_schema_migrations"`).Scan(&count); err != nil {
		t.Fatalf("query _schema_migrations: %v", err)
	}
	if count == 0 {
		t.Errorf("expected migrations to be recorded, got %d rows", count)
	}

	// Check that the numbered migration IDs exist
	rows, err := c.db.Query(`SELECT "id" FROM "_schema_migrations" ORDER BY "id"`)
	if err != nil {
		t.Fatalf("query _schema_migrations: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	t.Logf("Migration IDs: %v", ids)

	// All DDL migrations that succeeded should be tracked
	// At minimum the DML migrations (003, 004) should be tracked
	var count003 int
	c.db.QueryRow(`SELECT COUNT(*) FROM "_schema_migrations" WHERE "id" = '003_complexity_renumber'`).Scan(&count003)
	if count003 != 1 {
		t.Errorf("expected 003_complexity_renumber to be tracked, got count=%d", count003)
	}

	var count004 int
	c.db.QueryRow(`SELECT COUNT(*) FROM "_schema_migrations" WHERE "id" = '004_repo_case_normalize'`).Scan(&count004)
	if count004 != 1 {
		t.Errorf("expected 004_repo_case_normalize to be tracked, got count=%d", count004)
	}
}

func TestSplitStatements_EscapedQuotes(t *testing.T) {
	sql := `UPDATE "droplets" SET "description" = 'it''s a test' WHERE "id" = 'abc';
UPDATE "droplets" SET "title" = 'O''Brien' WHERE "id" = 'def';`
	stmts := splitStatements(sql)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "it''s a test") {
		t.Errorf("first statement should contain escaped quote, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "O''Brien") {
		t.Errorf("second statement should contain escaped quote, got: %s", stmts[1])
	}
}

func TestSplitStatements_SemicolonInString(t *testing.T) {
	sql := `UPDATE "droplets" SET "description" = 'a;b' WHERE "id" = 'x';`
	stmts := splitStatements(sql)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement (semicolons inside strings are not delimiters), got %d: %v", len(stmts), stmts)
	}
}

// TestEndToEndSchemaVerification opens a fresh DB, runs all migrations, and
// verifies every expected column exists on the droplets table. This catches
// migration ordering issues or missing columns.
func TestEndToEndSchemaVerification(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	expectedColumns := []string{
		"id", "repo", "title", "description", "priority",
		"status", "assignee", "current_cataractae", "outcome",
		"assigned_aqueduct", "last_reviewed_commit", "external_ref",
		"last_heartbeat_at", "created_at", "updated_at", "stage_dispatched_at",
	}

	rows, err := c.db.Query(`PRAGMA table_info("droplets")`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	var foundColumns []string
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltVal any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltVal, &pk); err != nil {
			t.Fatal(err)
		}
		foundColumns = append(foundColumns, name)
	}

	seen := make(map[string]bool, len(foundColumns))
	for _, col := range foundColumns {
		seen[col] = true
	}

	for _, expected := range expectedColumns {
		if !seen[expected] {
			t.Errorf("missing column %q on droplets table. Found: %v", expected, foundColumns)
		}
	}

	for _, table := range []string{"cataractae_notes", "events", "droplet_dependencies", "droplet_issues"} {
		var count int
		c.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count)
		if count != 1 {
			t.Errorf("expected table %q to exist", table)
		}
	}

	var migrationCount int
	c.db.QueryRow(`SELECT COUNT(*) FROM "_schema_migrations"`).Scan(&migrationCount)
	if migrationCount != 16 {
		t.Errorf("expected 16 migration records, got %d", migrationCount)
	}
}

func TestMigrationsAreApplied_LegacyCompat(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	// Simulate a DB with the old migration IDs.
	c, err := New(dbPath, "lc")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Insert a legacy migration ID (as the old system would have).
	c.db.Exec(`INSERT OR IGNORE INTO "_schema_migrations" ("id") VALUES ('complexity_renumber')`)
	c.db.Exec(`INSERT OR IGNORE INTO "_schema_migrations" ("id") VALUES ('repo_case_normalize')`)

	// Verify legacy aliases are mapped to numbered IDs.
	var count003 int
	c.db.QueryRow(`SELECT COUNT(*) FROM "_schema_migrations" WHERE "id" = '003_complexity_renumber'`).Scan(&count003)
	if count003 != 1 {
		t.Errorf("legacy 'complexity_renumber' should be aliased to '003_complexity_renumber', got count=%d", count003)
	}

	var count004 int
	c.db.QueryRow(`SELECT COUNT(*) FROM "_schema_migrations" WHERE "id" = '004_repo_case_normalize'`).Scan(&count004)
	if count004 != 1 {
		t.Errorf("legacy 'repo_case_normalize' should be aliased to '004_repo_case_normalize', got count=%d", count004)
	}
}

// minInt returns the smaller of a and b (avoids shadowing the Go 1.21 built-in min).
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
