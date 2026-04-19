package cistern

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrationEntry is a numbered SQL migration loaded from the embedded filesystem.
type migrationEntry struct {
	Number int
	Name   string
	SQL    string
}

// loadMigrations reads all numbered SQL files from the embedded migrations/
// directory and returns them sorted by number.
func loadMigrations() ([]migrationEntry, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var migrations []migrationEntry
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		numAndName := strings.SplitN(name, "_", 2)
		if len(numAndName) != 2 {
			continue
		}
		num, err := strconv.Atoi(numAndName[0])
		if err != nil {
			continue
		}
		data, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migrationEntry{
			Number: num,
			Name:   strings.TrimSuffix(numAndName[1], ".sql"),
			SQL:    string(data),
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Number < migrations[j].Number
	})
	return migrations, nil
}

// legacyMigrationAliases maps old ad-hoc migration IDs to their numbered
// equivalents. Databases that ran the old inline migrations will have these
// IDs recorded, and we honour them so the numbered migrations don't re-run.
var legacyMigrationAliases = map[string]string{
	"complexity_renumber": "003_complexity_renumber",
	"repo_case_normalize": "004_repo_case_normalize",
}

// runMigrations executes all migrations that have not yet been applied.
// It uses the _schema_migrations table to track which migrations have run.
// Migrations are applied in order. DDL-heavy migrations tolerate errors
// (columns/tables may already exist). DML migrations are wrapped in
// transactions for atomicity.
//
// For backward compatibility, legacy migration IDs (e.g. "complexity_renumber")
// are treated as aliases for the corresponding numbered migration. If the
// legacy ID exists, the numbered equivalent is marked as done automatically.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS "_schema_migrations" (
		"id" TEXT PRIMARY KEY
	)`); err != nil {
		return err
	}

	// Backfill: if a legacy alias exists, insert the numbered equivalent.
	for legacyID, numberedID := range legacyMigrationAliases {
		var count int
		db.QueryRow(`SELECT COUNT(*) FROM "_schema_migrations" WHERE "id" = ?`, legacyID).Scan(&count)
		if count > 0 {
			db.Exec(`INSERT OR IGNORE INTO "_schema_migrations" ("id") VALUES (?)`, numberedID)
		}
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		var count int
		db.QueryRow(`SELECT COUNT(*) FROM "_schema_migrations" WHERE "id" = ?`, m.migrationID()).Scan(&count)
		if count > 0 {
			continue
		}

		if err := applyMigration(db, m); err != nil {
			return err
		}
	}
	return nil
}

func (m migrationEntry) migrationID() string {
	return fmt.Sprintf("%03d_%s", m.Number, m.Name)
}

// applyMigration executes a single migration. DDL-heavy migrations tolerate
// errors (columns/tables may already exist). DML migrations are run in
// transactions for atomicity.
func applyMigration(db *sql.DB, m migrationEntry) error {
	statements := splitStatements(m.SQL)

	hasDML := false
	for _, s := range statements {
		upper := strings.TrimSpace(strings.ToUpper(s))
		if strings.HasPrefix(upper, "UPDATE ") || strings.HasPrefix(upper, "INSERT ") {
			hasDML = true
			break
		}
	}

	if hasDML {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("cistern: migration %s: begin tx: %w", m.migrationID(), err)
		}
		for _, s := range statements {
			if _, err := tx.Exec(s); err != nil {
				tx.Rollback()
				return fmt.Errorf("cistern: migration %s: %w", m.migrationID(), err)
			}
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO "_schema_migrations" ("id") VALUES (?)`, m.migrationID()); err != nil {
			tx.Rollback()
			return fmt.Errorf("cistern: migration %s: record migration: %w", m.migrationID(), err)
		}
		return tx.Commit()
	}

	// DDL-only: tolerate errors for idempotent operations (already-exists
	// columns/tables), but log them so broken migrations don't go silently undetected.
	for _, s := range statements {
		if _, err := db.Exec(s); err != nil {
			slog.Debug("migration DDL tolerance", "migration", m.migrationID(), "error", err)
		}
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO "_schema_migrations" ("id") VALUES (?)`, m.migrationID())
	return err
}

// splitStatements splits a migration SQL file into individual statements,
// preserving quoted strings (including ” escapes) and ignoring comments.
func splitStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inQuote := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		switch {
		case ch == '\'' && !inQuote:
			inQuote = true
			current.WriteByte(ch)
		case ch == '\'' && inQuote:
			// Check for escaped quote ('' — two consecutive single quotes).
			if i+1 < len(sql) && sql[i+1] == '\'' {
				// Escaped quote: write both and skip the next one.
				current.WriteByte(ch)
				current.WriteByte(sql[i+1])
				i++ // skip the second quote
				// Stay in quote — the string continues.
			} else {
				// End of string.
				inQuote = false
				current.WriteByte(ch)
			}
		case ch == ';' && !inQuote:
			s := strings.TrimSpace(current.String())
			if s != "" && s != ";" {
				statements = append(statements, stripComments(s))
			}
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}

	// Handle last statement without trailing semicolon.
	s := strings.TrimSpace(current.String())
	if s != "" {
		statements = append(statements, stripComments(s))
	}

	return filterEmpty(statements)
}

// stripComments removes SQL line comments (-- ...) from a statement.
func stripComments(s string) string {
	var result strings.Builder
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		result.WriteString(line)
		result.WriteRune('\n')
	}
	return strings.TrimSpace(result.String())
}

func filterEmpty(stmts []string) []string {
	var filtered []string
	for _, s := range stmts {
		s = strings.TrimSpace(s)
		if s != "" && s != ";" {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
