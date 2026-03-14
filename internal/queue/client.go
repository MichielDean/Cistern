// Package queue provides a SQLite-backed work queue for Citadel.
//
// Each work item flows through a workflow pipeline. The queue stores items,
// step notes, and events. No external dependencies — just SQLite.
package queue

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"fmt"
	"math/big"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

// WorkItem represents a unit of work in the queue.
type WorkItem struct {
	ID           string    `json:"id"`
	Repo         string    `json:"repo"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Priority     int       `json:"priority"`
	Complexity   int       `json:"complexity"`
	Status       string    `json:"status"`
	Assignee     string    `json:"assignee"`      // empty string when unassigned
	CurrentStep  string    `json:"current_step"`

	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// StepNote is a note attached by a workflow step.
type StepNote struct {
	ID        int       `json:"id"`
	ItemID    string    `json:"item_id"`
	StepName  string    `json:"step_name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Client is a SQLite-backed work queue client.
type Client struct {
	db     *sql.DB
	prefix string
}

// New opens (or creates) a SQLite database at dbPath, runs the schema, and
// returns a Client. The prefix is used when generating work item IDs.
func New(dbPath, prefix string) (*Client, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("queue: open %s: %w", dbPath, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("queue: schema: %w", err)
	}
	// Migration: add complexity column for existing DBs. Ignore error — fires
	// on first run when column already exists from schema.
	db.Exec(`ALTER TABLE work_items ADD COLUMN complexity INTEGER DEFAULT 3`)
	return &Client{db: db, prefix: prefix}, nil
}

// Close closes the underlying database connection.
func (c *Client) Close() error {
	return c.db.Close()
}

func (c *Client) generateID() (string, error) {
	b := make([]byte, 5)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return c.prefix + "-" + string(b), nil
}

// Add creates a new work item and returns it.
func (c *Client) Add(repo, title, description string, priority, complexity int) (*WorkItem, error) {
	if complexity < 1 || complexity > 4 {
		complexity = 3
	}
	id, err := c.generateID()
	if err != nil {
		return nil, fmt.Errorf("queue: generate id: %w", err)
	}

	now := time.Now().UTC()
	_, err = c.db.Exec(
		`INSERT INTO work_items (id, repo, title, description, priority, complexity, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?)`,
		id, repo, title, description, priority, complexity, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("queue: add: %w", err)
	}

	return &WorkItem{
		ID:          id,
		Repo:        repo,
		Title:       title,
		Description: description,
		Priority:    priority,
		Complexity:  complexity,
		Status:      "open",
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetReady atomically selects the next open work item for a repo and marks it
// in-progress within a single transaction. Ordered by priority (lower number =
// higher priority) then FIFO within the same priority. Returns nil if no work
// is available.
func (c *Client) GetReady(repo string) (*WorkItem, error) {
	tx, err := c.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("queue: begin tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRow(
		`SELECT id, repo, title, description, priority, complexity, status, assignee, current_step, created_at, updated_at
		 FROM work_items
		 WHERE repo = ? AND status = 'open'
		 ORDER BY priority ASC, created_at ASC
		 LIMIT 1`,
		repo,
	)

	var item WorkItem
	var assignee, currentStep sql.NullString
	err = row.Scan(
		&item.ID, &item.Repo, &item.Title, &item.Description,
		&item.Priority, &item.Complexity, &item.Status, &assignee, &currentStep,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("queue: scan ready item: %w", err)
	}
	item.Assignee = assignee.String
	item.CurrentStep = currentStep.String

	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE work_items SET status = 'in_progress', updated_at = ? WHERE id = ?`,
		now, item.ID,
	); err != nil {
		return nil, fmt.Errorf("queue: mark in_progress %s: %w", item.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("queue: commit: %w", err)
	}

	item.Status = "in_progress"
	item.UpdatedAt = now
	return &item, nil
}

// Assign records the worker and step on a work item. When worker is non-empty
// it only updates the assignee and step (status is already in-progress from
// GetReady). When worker is empty the item is set back to "open" (used when
// advancing to the next step without a specific worker assignment).
func (c *Client) Assign(id, worker, step string) error {
	now := time.Now().UTC()
	var res sql.Result
	var err error
	if worker == "" {
		res, err = c.db.Exec(
			`UPDATE work_items SET assignee = ?, current_step = ?, status = 'open',
			 updated_at = ? WHERE id = ?`,
			worker, step, now, id,
		)
	} else {
		res, err = c.db.Exec(
			`UPDATE work_items SET assignee = ?, current_step = ?,
			 updated_at = ? WHERE id = ?`,
			worker, step, now, id,
		)
	}
	if err != nil {
		return fmt.Errorf("queue: assign %s: %w", id, err)
	}
	return checkRowsAffected(res, id)
}

// UpdateStatus sets the status field on a work item.
func (c *Client) UpdateStatus(id, status string) error {
	res, err := c.db.Exec(
		`UPDATE work_items SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("queue: update status %s: %w", id, err)
	}
	return checkRowsAffected(res, id)
}

// AddNote attaches a step note to a work item.
func (c *Client) AddNote(id, step, content string) error {
	_, err := c.db.Exec(
		`INSERT INTO step_notes (item_id, step_name, content, created_at) VALUES (?, ?, ?, ?)`,
		id, step, content, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("queue: add note %s: %w", id, err)
	}
	return nil
}

// GetNotes returns all step notes for a work item, ordered by creation time.
func (c *Client) GetNotes(id string) ([]StepNote, error) {
	rows, err := c.db.Query(
		`SELECT id, item_id, step_name, content, created_at
		 FROM step_notes
		 WHERE item_id = ?
		 ORDER BY created_at ASC`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("queue: get notes %s: %w", id, err)
	}
	defer rows.Close()

	var notes []StepNote
	for rows.Next() {
		var n StepNote
		if err := rows.Scan(&n.ID, &n.ItemID, &n.StepName, &n.Content, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("queue: scan note: %w", err)
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// Escalate marks a work item as needing human attention and records the reason.
func (c *Client) Escalate(id, reason string) error {
	res, err := c.db.Exec(
		`UPDATE work_items SET status = 'escalated', updated_at = ? WHERE id = ?`,
		time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("queue: escalate %s: %w", id, err)
	}
	if err := checkRowsAffected(res, id); err != nil {
		return err
	}

	_, err = c.db.Exec(
		`INSERT INTO events (item_id, event_type, payload, created_at) VALUES (?, 'escalate', ?, ?)`,
		id, reason, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("queue: escalate event %s: %w", id, err)
	}
	return nil
}

// Close marks a work item as closed.
func (c *Client) CloseItem(id string) error {
	res, err := c.db.Exec(
		`UPDATE work_items SET status = 'closed', updated_at = ? WHERE id = ?`,
		time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("queue: close %s: %w", id, err)
	}
	return checkRowsAffected(res, id)
}

// Get retrieves a single work item by ID. Returns an error if not found.
func (c *Client) Get(id string) (*WorkItem, error) {
	row := c.db.QueryRow(
		`SELECT id, repo, title, description, priority, complexity, status, assignee, current_step, created_at, updated_at
		 FROM work_items WHERE id = ?`,
		id,
	)
	item, err := scanWorkItem(row)
	if err != nil {
		return nil, fmt.Errorf("queue: get %s: %w", id, err)
	}
	if item == nil {
		return nil, fmt.Errorf("queue: item %s not found", id)
	}
	return item, nil
}

// List returns work items filtered by repo and/or status. Empty strings mean no filter.
func (c *Client) List(repo, status string) ([]*WorkItem, error) {
	query := `SELECT id, repo, title, description, priority, complexity, status, assignee, current_step, created_at, updated_at
		 FROM work_items WHERE 1=1`
	var args []any
	if repo != "" {
		query += ` AND repo = ?`
		args = append(args, repo)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY priority ASC, created_at ASC`

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("queue: list: %w", err)
	}
	defer rows.Close()

	var items []*WorkItem
	for rows.Next() {
		var item WorkItem
		var assignee, currentStep sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Repo, &item.Title, &item.Description,
			&item.Priority, &item.Complexity, &item.Status, &assignee, &currentStep,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("queue: scan item: %w", err)
		}
		item.Assignee = assignee.String
		item.CurrentStep = currentStep.String
		items = append(items, &item)
	}
	return items, rows.Err()
}

// scanWorkItem scans a single row into a WorkItem. Returns nil, nil for sql.ErrNoRows.
func scanWorkItem(row *sql.Row) (*WorkItem, error) {
	var item WorkItem
	var assignee, currentStep sql.NullString
	err := row.Scan(
		&item.ID, &item.Repo, &item.Title, &item.Description,
		&item.Priority, &item.Complexity, &item.Status, &assignee, &currentStep,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.Assignee = assignee.String
	item.CurrentStep = currentStep.String
	return &item, nil
}

// Purge deletes closed/escalated items older than olderThan, cascading to
// step_notes and events. Returns the count of items deleted (or that would be
// deleted in dry-run mode).
func (c *Client) Purge(olderThan time.Duration, dryRun bool) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan)

	if dryRun {
		var count int
		err := c.db.QueryRow(
			`SELECT COUNT(*) FROM work_items WHERE status IN ('closed', 'escalated') AND updated_at < ?`,
			cutoff,
		).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("queue: purge dry-run: %w", err)
		}
		return count, nil
	}

	tx, err := c.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("queue: purge begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM step_notes WHERE item_id IN (
			SELECT id FROM work_items WHERE status IN ('closed', 'escalated') AND updated_at < ?
		)`, cutoff,
	); err != nil {
		return 0, fmt.Errorf("queue: purge step_notes: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM events WHERE item_id IN (
			SELECT id FROM work_items WHERE status IN ('closed', 'escalated') AND updated_at < ?
		)`, cutoff,
	); err != nil {
		return 0, fmt.Errorf("queue: purge events: %w", err)
	}

	res, err := tx.Exec(
		`DELETE FROM work_items WHERE status IN ('closed', 'escalated') AND updated_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("queue: purge delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("queue: purge rows affected: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("queue: purge commit: %w", err)
	}
	return int(n), nil
}

// RecentEvent is a summary entry from the events or step_notes table.
type RecentEvent struct {
	Time  time.Time `json:"time"`
	Drop  string    `json:"drop"`
	Event string    `json:"event"`
}

// ListRecentEvents returns up to limit recent entries from the events and
// step_notes tables, ordered newest-first.
func (c *Client) ListRecentEvents(limit int) ([]RecentEvent, error) {
	rows, err := c.db.Query(`
		SELECT item_id, event_type, created_at FROM events
		UNION ALL
		SELECT item_id, step_name, created_at FROM step_notes
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("queue: list recent events: %w", err)
	}
	defer rows.Close()

	var events []RecentEvent
	for rows.Next() {
		var e RecentEvent
		if err := rows.Scan(&e.Drop, &e.Event, &e.Time); err != nil {
			return nil, fmt.Errorf("queue: scan recent event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func checkRowsAffected(res sql.Result, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("queue: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("queue: item %s not found", id)
	}
	return nil
}
