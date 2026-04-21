// Package queue provides a SQLite-backed work queue for Cistern.
//
// Each droplet flows through an aqueduct. The queue stores droplets,
// cataractae notes, and events. No external dependencies — just SQLite.
package cistern

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	EventCreate         = "create"
	EventDispatch       = "dispatch"
	EventPass           = "pass"
	EventRecirculate    = "recirculate"
	EventDelivered      = "delivered"
	EventRestart        = "restart"
	EventApprove        = "approve"
	EventEdit           = "edit"
	EventPool           = "pool"
	EventCancel         = "cancel"
	EventExitNoOutcome  = "exit_no_outcome"
	EventStall          = "stall"
	EventRecovery       = "recovery"
	EventCircuitBreaker = "circuit_breaker"
	EventLoopRecovery   = "loop_recovery"
	EventAutoPromote    = "auto_promote"
	EventNoRoute        = "no_route"
)

var validEventTypes = map[string]bool{
	EventCreate:         true,
	EventDispatch:       true,
	EventPass:           true,
	EventRecirculate:    true,
	EventDelivered:      true,
	EventRestart:        true,
	EventApprove:        true,
	EventEdit:           true,
	EventPool:           true,
	EventCancel:         true,
	EventExitNoOutcome:  true,
	EventStall:          true,
	EventRecovery:       true,
	EventCircuitBreaker: true,
	EventLoopRecovery:   true,
	EventAutoPromote:    true,
	EventNoRoute:        true,
}

type executor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// externalRefRE validates the 'provider:key' format for external_ref values.
// Both parts must consist solely of characters safe for use in git branch names
// and shell awk extraction: letters, digits, hyphens, underscores, and (key only)
// dots. Spaces and git-invalid characters (~, ^, :, ?, *, [, \) are rejected.
var externalRefRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+:[a-zA-Z0-9._-]+$`)

//go:embed schema.sql
var schema string

const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

// Droplet represents a unit of work flowing through the cistern.
type Droplet struct {
	ID                string `json:"id"`
	Repo              string `json:"repo"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	Priority          int    `json:"priority"`
	Complexity        int    `json:"complexity"`
	Status            string `json:"status"`
	Assignee          string `json:"assignee"` // empty string when unassigned
	CurrentCataractae string `json:"current_cataractae"`
	// Outcome is set by agents via `ct droplet pass/recirculate/pool`.
	// Empty string means no outcome yet (NULL in DB).
	Outcome string `json:"outcome,omitempty"`
	// AssignedAqueduct records which aqueduct operator is currently holding this
	// droplet. Set when first dispatched; cleared on terminal states (delivered,
	// pooled, cancelled) so no ghost assignments linger.
	AssignedAqueduct string `json:"assigned_aqueduct,omitempty"`
	// LastReviewedCommit is the HEAD commit hash at the time the last review
	// diff was generated. Used to detect phantom commits (implement pass without
	// any new commits since the last review).
	LastReviewedCommit string `json:"last_reviewed_commit,omitempty"`
	// ExternalRef is the external issue reference for imported issues.
	// Format: 'provider:key' (e.g. 'jira:DPF-456', 'linear:LIN-789').
	// Empty string means no external reference (NULL in DB).
	ExternalRef string `json:"external_ref,omitempty"`
	// LastHeartbeatAt is the most recent time the agent called `ct droplet heartbeat`.
	// Zero value means no heartbeat has been emitted yet. Used by the stall detector
	// to distinguish alive-but-slow agents from genuinely stuck or dead ones.
	LastHeartbeatAt time.Time `json:"last_heartbeat_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// StageDispatchedAt is set only when a worker is assigned to this droplet
	// (Assign(id, worker, step) with non-empty worker). Unlike UpdatedAt, it is
	// not bumped by notes, outcome signals, or other state changes — making it
	// the reliable anchor for the exit detection age guard.
	StageDispatchedAt time.Time `json:"stage_dispatched_at,omitempty"`
}

// CataractaeNote is a note attached by a workflow cataractae.
type CataractaeNote struct {
	ID             int       `json:"id"`
	DropletID      string    `json:"droplet_id"`
	CataractaeName string    `json:"cataractae_name"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

// Client is a SQLite-backed work queue client.
type Client struct {
	db     *sql.DB
	prefix string
}

// New opens (or creates) a SQLite database at dbPath, applies the schema and
// all numbered migrations, and returns a Client ready for use.
// The prefix is used when generating droplet IDs (e.g., "bf" → "bf-a3k9x").
//
// IMPORTANT: This is the only way to create a valid *Client. The db field is
// unexported. Do not construct Client manually — migrations and schema must
// run exactly once, and New guarantees that.
func New(dbPath, prefix string) (*Client, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("cistern: open %s: %w", dbPath, err)
	}
	// SQLite only supports one concurrent writer. Limit the connection pool to
	// a single connection so concurrent goroutines (dispatch, heartbeat, observe)
	// queue behind the same connection rather than racing across independent
	// *sql.DB pools, which causes "database is locked" errors even with WAL mode.
	db.SetMaxOpenConns(1)
	// Apply schema first — CREATE TABLE IF NOT EXISTS creates the base tables.
	// Migrations then add columns, rename things, and update data.
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("cistern: schema: %w", err)
	}
	// Run numbered migrations after schema is in place.
	// Each migration is idempotent and tracked in _schema_migrations.
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("cistern: migrations: %w", err)
	}
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

// Add creates a new droplet and returns it. Optional deps are dependency IDs
// that must be delivered before this droplet can be dispatched.
func (c *Client) Add(repo, title, description string, priority, complexity int, deps ...string) (*Droplet, error) {
	if complexity < 1 || complexity > 3 {
		complexity = 2
	}
	id, err := c.generateID()
	if err != nil {
		return nil, fmt.Errorf("cistern: generate id: %w", err)
	}

	// Validate dep IDs before inserting.
	for _, dep := range deps {
		var exists int
		if err := c.db.QueryRow(`SELECT COUNT(*) FROM droplets WHERE id = ?`, dep).Scan(&exists); err != nil {
			return nil, fmt.Errorf("cistern: validate dep %s: %w", dep, err)
		}
		if exists == 0 {
			return nil, fmt.Errorf("cistern: dependency %s not found", dep)
		}
	}

	tx, err := c.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	if _, err = tx.Exec(
		`INSERT INTO droplets (id, repo, title, description, priority, complexity, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?)`,
		id, repo, title, description, priority, complexity, now, now,
	); err != nil {
		return nil, fmt.Errorf("cistern: add: %w", err)
	}

	for _, dep := range deps {
		if _, err := tx.Exec(
			`INSERT INTO droplet_dependencies (droplet_id, depends_on) VALUES (?, ?)`,
			id, dep,
		); err != nil {
			return nil, fmt.Errorf("cistern: add dep %s: %w", dep, err)
		}
	}

	createPayload, _ := json.Marshal(map[string]any{
		"repo":       repo,
		"title":      title,
		"priority":   priority,
		"complexity": complexity,
	})
	if err := c.recordEvent(tx, id, EventCreate, string(createPayload)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cistern: commit: %w", err)
	}

	return &Droplet{
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

// AddDroplet is a convenience method that adds a droplet and sets its external reference.
func (c *Client) AddDroplet(repo, title, description, externalRef string, priority, complexity int) (*Droplet, error) {
	droplet, err := c.Add(repo, title, description, priority, complexity)
	if err != nil {
		return nil, err
	}
	if err := c.SetExternalRef(droplet.ID, externalRef); err != nil {
		return nil, err
	}
	droplet.ExternalRef = externalRef
	return droplet, nil
}

// GetReady atomically selects the next open droplet for a repo and marks it
// in-progress within a single transaction. Ordered by priority (lower number =
// higher priority) then FIFO within the same priority. Returns nil if no work
// is available.
func (c *Client) GetReady(repo string) (*Droplet, error) {
	tx, err := c.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRow(
		`SELECT id, repo, title, description, priority, complexity, status, assignee, current_cataractae, outcome, assigned_aqueduct, last_reviewed_commit, external_ref, last_heartbeat_at, created_at, updated_at, stage_dispatched_at
		 FROM droplets d
		 WHERE d.repo = ? COLLATE NOCASE AND d.status = 'open'
		   AND NOT EXISTS (
		     SELECT 1 FROM droplet_dependencies dep
		     JOIN droplets dep_d ON dep_d.id = dep.depends_on
		     WHERE dep.droplet_id = d.id AND dep_d.status NOT IN ('delivered', 'cancelled')
		   )
		 ORDER BY d.priority ASC, d.created_at ASC
		 LIMIT 1`,
		repo,
	)

	var droplet Droplet
	var assignee, currentCataracta, outcome, assignedAqueduct, lastReviewedCommit, externalRef sql.NullString
	var lastHeartbeatAt, stageDispatchedAt sql.NullTime
	err = row.Scan(
		&droplet.ID, &droplet.Repo, &droplet.Title, &droplet.Description,
		&droplet.Priority, &droplet.Complexity, &droplet.Status, &assignee, &currentCataracta, &outcome, &assignedAqueduct, &lastReviewedCommit, &externalRef,
		&lastHeartbeatAt, &droplet.CreatedAt, &droplet.UpdatedAt, &stageDispatchedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cistern: scan ready droplet: %w", err)
	}
	fillDropletFromNullable(&droplet, assignee, currentCataracta, outcome, assignedAqueduct, lastReviewedCommit, externalRef, lastHeartbeatAt, stageDispatchedAt)

	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE "droplets" SET "status" = 'in_progress', "updated_at" = ? WHERE "id" = ?`,
		now, droplet.ID,
	); err != nil {
		return nil, fmt.Errorf("cistern: mark in_progress %s: %w", droplet.ID, err)
	}

	dispatchPayload, _ := json.Marshal(map[string]any{
		"cataractae": droplet.CurrentCataractae,
		"assignee":   droplet.Assignee,
	})
	if err := c.recordEvent(tx, droplet.ID, EventDispatch, string(dispatchPayload)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cistern: commit: %w", err)
	}

	droplet.Status = "in_progress"
	droplet.UpdatedAt = now
	return &droplet, nil
}

// GetReadyForAqueduct is like GetReady but only returns droplets that are either
// unassigned (assigned_aqueduct = ”) or already assigned to aqueductName.
// This enforces sticky aqueduct assignment: once a droplet enters an aqueduct
// it stays there for its entire lifecycle.
func (c *Client) GetReadyForAqueduct(repo, aqueductName string) (*Droplet, error) {
	tx, err := c.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRow(
		`SELECT id, repo, title, description, priority, complexity, status, assignee, current_cataractae, outcome, assigned_aqueduct, last_reviewed_commit, external_ref, last_heartbeat_at, created_at, updated_at, stage_dispatched_at
		 FROM droplets d
		 WHERE d.repo = ? COLLATE NOCASE AND d.status = 'open'
		   AND (d.assigned_aqueduct = '' OR d.assigned_aqueduct IS NULL OR d.assigned_aqueduct = ?)
		   AND NOT EXISTS (
		     SELECT 1 FROM droplet_dependencies dep
		     JOIN droplets dep_d ON dep_d.id = dep.depends_on
		     WHERE dep.droplet_id = d.id AND dep_d.status NOT IN ('delivered', 'cancelled')
		   )
		 ORDER BY d.priority ASC, d.created_at ASC
		 LIMIT 1`,
		repo, aqueductName,
	)

	var droplet Droplet
	var assignee, currentCataracta, outcome, assignedAqueduct, lastReviewedCommit, externalRef sql.NullString
	var lastHeartbeatAt, stageDispatchedAt sql.NullTime
	now := time.Now().UTC()
	err = row.Scan(
		&droplet.ID, &droplet.Repo, &droplet.Title, &droplet.Description,
		&droplet.Priority, &droplet.Complexity, &droplet.Status, &assignee, &currentCataracta, &outcome, &assignedAqueduct, &lastReviewedCommit, &externalRef,
		&lastHeartbeatAt, &droplet.CreatedAt, &droplet.UpdatedAt, &stageDispatchedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cistern: scan ready droplet: %w", err)
	}
	fillDropletFromNullable(&droplet, assignee, currentCataracta, outcome, assignedAqueduct, lastReviewedCommit, externalRef, lastHeartbeatAt, stageDispatchedAt)

	if _, err := tx.Exec(
		`UPDATE "droplets" SET "status" = 'in_progress', "updated_at" = ? WHERE "id" = ?`,
		now, droplet.ID,
	); err != nil {
		return nil, fmt.Errorf("cistern: mark in_progress %s: %w", droplet.ID, err)
	}

	dispatchPayload, _ := json.Marshal(map[string]any{
		"aqueduct":   aqueductName,
		"cataractae": droplet.CurrentCataractae,
		"assignee":   droplet.Assignee,
	})
	if err := c.recordEvent(tx, droplet.ID, EventDispatch, string(dispatchPayload)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cistern: commit: %w", err)
	}
	droplet.Status = "in_progress"
	droplet.UpdatedAt = now
	return &droplet, nil
}

// Assign records the worker and cataractae on a droplet. When worker is non-empty
// it only updates the assignee and cataractae (status is already in-progress from
// GetReady). When worker is empty the droplet is set back to "open" (used when
// advancing to the next cataractae without a specific worker assignment).
func (c *Client) Assign(id, worker, step string) error {
	now := time.Now().UTC()
	var res sql.Result
	var err error
	if worker == "" {
		res, err = c.db.Exec(
			`UPDATE droplets SET assignee = ?, current_cataractae = ?, outcome = NULL, status = 'open',
			 assigned_aqueduct = '', updated_at = ? WHERE id = ?`,
			worker, step, now, id,
		)
	} else {
		res, err = c.db.Exec(
			`UPDATE droplets SET assignee = ?, current_cataractae = ?, outcome = NULL,
			 updated_at = ?, stage_dispatched_at = ? WHERE id = ?`,
			worker, step, now, now, id,
		)
	}
	if err != nil {
		return fmt.Errorf("cistern: assign %s: %w", id, err)
	}
	return checkRowsAffected(res, id)
}

// SetAssignedAqueduct records the aqueduct operator currently holding this
// droplet. Only updates when the field is currently empty; CloseItem, Cancel,
// and Pool clear it as part of their terminal-state transitions.
func (c *Client) SetAssignedAqueduct(id, aqueductName string) error {
	_, err := c.db.Exec(
		`UPDATE droplets SET assigned_aqueduct = ? WHERE id = ? AND (assigned_aqueduct = '' OR assigned_aqueduct IS NULL)`,
		aqueductName, id,
	)
	if err != nil {
		return fmt.Errorf("cistern: set assigned_aqueduct %s: %w", id, err)
	}
	return nil
}

// SetLastReviewedCommit records the HEAD commit hash at the time the review diff
// was generated. Called by the runner when preparing a diff_only context.
func (c *Client) SetLastReviewedCommit(id, commitHash string) error {
	_, err := c.db.Exec(
		`UPDATE droplets SET last_reviewed_commit = ? WHERE id = ?`,
		commitHash, id,
	)
	if err != nil {
		return fmt.Errorf("cistern: set last_reviewed_commit %s: %w", id, err)
	}
	return nil
}

// GetLastReviewedCommit returns the HEAD commit hash from the last time a review
// diff was generated for this droplet. Returns an empty string if not yet set.
func (c *Client) GetLastReviewedCommit(id string) (string, error) {
	var commit sql.NullString
	err := c.db.QueryRow(
		`SELECT last_reviewed_commit FROM droplets WHERE id = ?`, id,
	).Scan(&commit)
	if err != nil {
		return "", fmt.Errorf("cistern: get last_reviewed_commit %s: %w", id, err)
	}
	return commit.String, nil
}

// SetExternalRef sets the external_ref field on a droplet. Pass an empty string
// to clear the field (stores NULL). Format should be 'provider:key'
// (e.g. 'jira:DPF-456', 'linear:LIN-789').
func (c *Client) SetExternalRef(id, ref string) error {
	if ref != "" {
		if !externalRefRE.MatchString(ref) {
			return fmt.Errorf("cistern: invalid external_ref %q: must match provider:key with git-safe characters", ref)
		}
		_, key, _ := strings.Cut(ref, ":")
		if strings.Contains(key, "..") || strings.HasSuffix(key, ".") || strings.HasSuffix(key, ".lock") || strings.HasPrefix(key, ".") {
			return fmt.Errorf("cistern: invalid external_ref %q: key produces git-invalid branch name", ref)
		}
	}
	var val any
	if ref != "" {
		val = ref
	}
	now := time.Now().UTC()
	res, err := c.db.Exec(
		`UPDATE droplets SET external_ref = ?, updated_at = ? WHERE id = ?`,
		val, now, id,
	)
	if err != nil {
		return fmt.Errorf("cistern: set external_ref %s: %w", id, err)
	}
	return checkRowsAffected(res, id)
}

// Heartbeat records the current time as the agent's most recent activity
// timestamp. Called by agents via `ct droplet heartbeat <id>` every 60 seconds
// while working. The stall detector uses this timestamp to distinguish alive
// (heartbeating) agents from genuinely stuck or dead ones.
func (c *Client) Heartbeat(id string) error {
	now := time.Now().UTC()
	res, err := c.db.Exec(
		`UPDATE droplets SET last_heartbeat_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("cistern: heartbeat %s: %w", id, err)
	}
	return checkRowsAffected(res, id)
}

// EditDropletFields holds the optional fields for EditDroplet.
// A nil pointer means "do not update this field".
type EditDropletFields struct {
	Title       *string
	Description *string
	Complexity  *int
	Priority    *int
}

func (f EditDropletFields) Empty() bool {
	return f.Title == nil && f.Description == nil && f.Complexity == nil && f.Priority == nil
}

// EditDroplet updates mutable fields on a droplet that has not yet been picked
// up. Allowed statuses: open, pooled. Returns an error if the droplet is
// in_progress or delivered.
func (c *Client) EditDroplet(id string, fields EditDropletFields) error {
	if fields.Empty() {
		return nil
	}

	if fields.Title != nil && *fields.Title == "" {
		return fmt.Errorf("cistern: title must not be empty")
	}

	if fields.Complexity != nil && (*fields.Complexity < 1 || *fields.Complexity > 3) {
		return fmt.Errorf("cistern: complexity must be between 1 and 3, got %d", *fields.Complexity)
	}

	if fields.Priority != nil && *fields.Priority < 1 {
		return fmt.Errorf("cistern: priority must be a positive integer, got %d", *fields.Priority)
	}

	var changedFields []string
	if fields.Title != nil {
		changedFields = append(changedFields, "title")
	}
	if fields.Description != nil {
		changedFields = append(changedFields, "description")
	}
	if fields.Complexity != nil {
		changedFields = append(changedFields, "complexity")
	}
	if fields.Priority != nil {
		changedFields = append(changedFields, "priority")
	}

	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	var setClauses []string
	var args []any
	if fields.Title != nil {
		setClauses = append(setClauses, "title = ?")
		args = append(args, *fields.Title)
	}
	if fields.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *fields.Description)
	}
	if fields.Complexity != nil {
		setClauses = append(setClauses, "complexity = ?")
		args = append(args, *fields.Complexity)
	}
	if fields.Priority != nil {
		setClauses = append(setClauses, "priority = ?")
		args = append(args, *fields.Priority)
	}
	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, time.Now().UTC())
	args = append(args, id)

	query := "UPDATE droplets SET " + strings.Join(setClauses, ", ") + " WHERE id = ? AND status IN ('open', 'pooled')"
	res, err := tx.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("cistern: edit %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cistern: rows affected: %w", err)
	}
	if n == 0 {
		var status string
		if err := tx.QueryRow(`SELECT status FROM droplets WHERE id = ?`, id).Scan(&status); err != nil {
			return fmt.Errorf("cistern: droplet %s not found", id)
		}
		return fmt.Errorf("droplet %s is %s — cannot edit a droplet that has been picked up", id, status)
	}

	editPayload, _ := json.Marshal(map[string]any{"fields": changedFields})
	if err := c.recordEvent(tx, id, EventEdit, string(editPayload)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cistern: commit: %w", err)
	}
	return nil
}

// UpdateStatus sets the status field on a droplet.
func (c *Client) UpdateStatus(id, status string) error {
	res, err := c.db.Exec(
		`UPDATE droplets SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("cistern: update status %s: %w", id, err)
	}
	return checkRowsAffected(res, id)
}

// AddNote attaches a cataractae note to a droplet.
func (c *Client) AddNote(id, step, content string) error {
	_, err := c.db.Exec(
		`INSERT INTO cataractae_notes (droplet_id, cataractae_name, content, created_at) VALUES (?, ?, ?, ?)`,
		id, step, content, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("cistern: add note %s: %w", id, err)
	}
	return nil
}

// RecordEvent inserts a typed event row into the events table. eventType must
// be one of the Event* constants; payload must be valid JSON (use "{}" for
// empty). Returns an error for unknown event types or invalid JSON.
func (c *Client) RecordEvent(id, eventType, payload string) error {
	return c.recordEvent(c.db, id, eventType, payload)
}

func (c *Client) recordEvent(exec executor, id, eventType, payload string) error {
	if !validEventTypes[eventType] {
		return fmt.Errorf("cistern: unknown event type %q", eventType)
	}
	if !json.Valid([]byte(payload)) {
		return fmt.Errorf("cistern: event payload must be valid JSON, got %q", payload)
	}
	_, err := exec.Exec(
		`INSERT INTO events (droplet_id, event_type, payload, created_at) VALUES (?, ?, ?, ?)`,
		id, eventType, payload, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("cistern: record %s event %s: %w", eventType, id, err)
	}
	return nil
}

// Pass sets outcome to "pass" on a droplet and records a pass event, all within
// a single transaction. This ensures the outcome and event are always consistent.
// Returns an error if the droplet has a terminal status (delivered or cancelled).
func (c *Client) Pass(id, cataractaeName, notes string) error {
	item, err := c.Get(id)
	if err != nil {
		return fmt.Errorf("cistern: pass %s: %w", id, err)
	}
	if item.Status == "delivered" || item.Status == "cancelled" {
		return fmt.Errorf("cistern: cannot pass %s: droplet has terminal status %q", id, item.Status)
	}

	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.Exec(
		`UPDATE droplets SET outcome = ?, updated_at = ? WHERE id = ? AND status NOT IN ('delivered', 'cancelled')`,
		"pass", now, id,
	)
	if err != nil {
		return fmt.Errorf("cistern: pass %s: %w", id, err)
	}
	if err := checkRowsAffected(res, id); err != nil {
		return err
	}

	passPayload, _ := json.Marshal(map[string]any{"cataractae": cataractaeName, "notes": notes})
	if err := c.recordEvent(tx, id, EventPass, string(passPayload)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cistern: commit: %w", err)
	}
	return nil
}

// Recirculate sets the outcome for a recirculate signal and records a recirculate event.
// When status is not in_progress, it also calls Assign to re-open the droplet at the
// target cataractae, all within a single transaction.
func (c *Client) Recirculate(id, cataractaeName, target, notes string) error {
	item, err := c.Get(id)
	if err != nil {
		return fmt.Errorf("cistern: recirculate %s: %w", id, err)
	}

	outcome := "recirculate"
	if target != "" {
		outcome = "recirculate:" + target
	}

	if item.Status == "delivered" || item.Status == "cancelled" {
		return fmt.Errorf("cistern: cannot recirculate %s: droplet has terminal status %q", id, item.Status)
	}

	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	if item.Status != "in_progress" {
		effectiveTarget := target
		if effectiveTarget == "" {
			effectiveTarget = item.CurrentCataractae
		}
		var res sql.Result
		res, err = tx.Exec(
			`UPDATE droplets SET assignee = ?, current_cataractae = ?, outcome = NULL, status = 'open',
			 assigned_aqueduct = '', updated_at = ? WHERE id = ? AND status NOT IN ('delivered', 'cancelled')`,
			"", effectiveTarget, now, id,
		)
		if err != nil {
			return fmt.Errorf("cistern: recirculate assign %s: %w", id, err)
		}
		if err := checkRowsAffected(res, id); err != nil {
			return err
		}
	} else {
		res, err := tx.Exec(
			`UPDATE droplets SET outcome = ?, updated_at = ? WHERE id = ? AND status NOT IN ('delivered', 'cancelled')`,
			outcome, now, id,
		)
		if err != nil {
			return fmt.Errorf("cistern: recirculate %s: %w", id, err)
		}
		if err := checkRowsAffected(res, id); err != nil {
			return err
		}
	}

	recircPayload, _ := json.Marshal(map[string]any{"cataractae": cataractaeName, "target": target, "notes": notes})
	if err := c.recordEvent(tx, id, EventRecirculate, string(recircPayload)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cistern: commit: %w", err)
	}
	return nil
}

// Approve approves a human-gated droplet for delivery. It assigns the droplet to
// the delivery cataractae and records an approve event within a single transaction.
// Returns an error if the droplet has a terminal status or is not at the human gate.
func (c *Client) Approve(id, cataractaeName string) error {
	item, err := c.Get(id)
	if err != nil {
		return fmt.Errorf("cistern: approve %s: %w", id, err)
	}
	if item.Status == "delivered" || item.Status == "cancelled" {
		return fmt.Errorf("cistern: cannot approve %s: droplet has terminal status %q", id, item.Status)
	}
	if item.CurrentCataractae != "human" {
		return fmt.Errorf("cistern: %s is not awaiting human approval (cataractae: %s)", id, item.CurrentCataractae)
	}

	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.Exec(
		`UPDATE droplets SET assignee = ?, current_cataractae = ?, outcome = NULL, status = 'open',
		 assigned_aqueduct = '', updated_at = ? WHERE id = ? AND status NOT IN ('delivered', 'cancelled') AND current_cataractae = 'human'`,
		"", "delivery", now, id,
	)
	if err != nil {
		return fmt.Errorf("cistern: approve assign %s: %w", id, err)
	}
	if err := checkRowsAffected(res, id); err != nil {
		return err
	}

	approvePayload, _ := json.Marshal(map[string]any{"cataractae": cataractaeName})
	if err := c.recordEvent(tx, id, EventApprove, string(approvePayload)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cistern: commit: %w", err)
	}
	return nil
}

// GetNotes returns all cataractae notes for a droplet, newest first.
func (c *Client) GetNotes(id string) ([]CataractaeNote, error) {
	rows, err := c.db.Query(
		`SELECT id, droplet_id, cataractae_name, content, created_at
		 FROM cataractae_notes
		 WHERE droplet_id = ?
		 ORDER BY created_at DESC`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("cistern: get notes %s: %w", id, err)
	}
	defer rows.Close()

	var notes []CataractaeNote
	for rows.Next() {
		var n CataractaeNote
		if err := rows.Scan(&n.ID, &n.DropletID, &n.CataractaeName, &n.Content, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("cistern: scan note: %w", err)
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// Pool marks a droplet as pooled — cannot currently flow forward — and records the reason.
// assigned_aqueduct and outcome are cleared/set atomically so no ghost assignments linger.
func (c *Client) Pool(id, reason string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.Exec(
		`UPDATE droplets SET status = 'pooled', outcome = 'pool', assigned_aqueduct = '', updated_at = ? WHERE id = ? AND status NOT IN ('delivered', 'cancelled')`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("cistern: pool %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cistern: pool %s: rows affected: %w", id, err)
	}
	if n == 0 {
		var status string
		if err := tx.QueryRow(`SELECT status FROM droplets WHERE id = ?`, id).Scan(&status); err != nil {
			return fmt.Errorf("cistern: droplet %s not found", id)
		}
		return fmt.Errorf("cistern: pool %s: droplet has terminal status %q", id, status)
	}

	poolPayload, _ := json.Marshal(map[string]any{"reason": reason})
	if err := c.recordEvent(tx, id, EventPool, string(poolPayload)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cistern: commit: %w", err)
	}
	return nil
}

// Cancel marks a droplet as cancelled. Cancelled droplets are excluded from the
// dispatch queue and from default list views. They can still be retrieved with
// List(repo, "cancelled"). It clears outcome and assigned_aqueduct atomically
// so no ghost assignments linger. The assignee is preserved so the scheduler's
// external-cancel pool-release path can find and free the slot.
// The cancel is recorded as an event in the events table for the audit trail.
func (c *Client) Cancel(id, reason string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.Exec(
		`UPDATE droplets SET status = 'cancelled', outcome = NULL, assigned_aqueduct = '', updated_at = ? WHERE id = ? AND status NOT IN ('delivered', 'cancelled')`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("cistern: cancel %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cistern: cancel %s: rows affected: %w", id, err)
	}
	if n == 0 {
		var status string
		if err := tx.QueryRow(`SELECT status FROM droplets WHERE id = ?`, id).Scan(&status); err != nil {
			return fmt.Errorf("cistern: droplet %s not found", id)
		}
		return fmt.Errorf("cistern: cancel %s: droplet has terminal status %q", id, status)
	}

	cancelPayload, _ := json.Marshal(map[string]any{"reason": reason})
	if err := c.recordEvent(tx, id, EventCancel, string(cancelPayload)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cistern: commit: %w", err)
	}
	return nil
}

// FileDroplet creates a new droplet in the given repo. It is a convenience
// wrapper around Add used by the Architecti to file structural fix work items.
func (c *Client) FileDroplet(repo, title, description string, priority, complexity int) (*Droplet, error) {
	return c.Add(repo, title, description, priority, complexity)
}

// CloseItem marks a droplet as delivered.
// assigned_aqueduct is cleared atomically so no ghost assignments linger.
// Returns an error if the droplet has a terminal status (delivered or cancelled).
func (c *Client) CloseItem(id string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE droplets SET status = 'delivered', assigned_aqueduct = '', updated_at = ? WHERE id = ? AND status NOT IN ('delivered', 'cancelled')`,
		time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("cistern: close %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cistern: close %s: rows affected: %w", id, err)
	}
	if n == 0 {
		var status string
		if err := tx.QueryRow(`SELECT status FROM droplets WHERE id = ?`, id).Scan(&status); err != nil {
			return fmt.Errorf("cistern: droplet %s not found", id)
		}
		return fmt.Errorf("cistern: close %s: droplet has terminal status %q", id, status)
	}
	if err := c.recordEvent(tx, id, EventDelivered, "{}"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cistern: commit: %w", err)
	}
	return nil
}

// SetOutcome records the agent outcome on a droplet. Pass empty string to clear
// (sets the column to NULL). Agents call this via `ct droplet pass/recirculate/pool`.
func (c *Client) SetOutcome(id, outcome string) error {
	var err error
	var res sql.Result
	now := time.Now().UTC()
	if outcome == "" {
		res, err = c.db.Exec(
			`UPDATE droplets SET outcome = NULL, updated_at = ? WHERE id = ?`,
			now, id,
		)
	} else {
		res, err = c.db.Exec(
			`UPDATE droplets SET outcome = ?, updated_at = ? WHERE id = ?`,
			outcome, now, id,
		)
	}
	if err != nil {
		return fmt.Errorf("cistern: set outcome %s: %w", id, err)
	}
	return checkRowsAffected(res, id)
}

// SetCataractae updates the current_cataractae field on a droplet without changing
// any other fields. Used by the scheduler to mark a droplet as awaiting human approval.
func (c *Client) SetCataractae(id, cataractaeName string) error {
	res, err := c.db.Exec(
		`UPDATE droplets SET current_cataractae = ?, updated_at = ? WHERE id = ?`,
		cataractaeName, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("cistern: set cataractae %s: %w", id, err)
	}
	return checkRowsAffected(res, id)
}

// Get retrieves a single droplet by ID. Returns an error if not found.
func (c *Client) Get(id string) (*Droplet, error) {
	row := c.db.QueryRow(
		`SELECT id, repo, title, description, priority, complexity, status, assignee, current_cataractae, outcome, assigned_aqueduct, last_reviewed_commit, external_ref, last_heartbeat_at, created_at, updated_at, stage_dispatched_at
		 FROM droplets WHERE id = ?`,
		id,
	)
	droplet, err := scanDroplet(row)
	if err != nil {
		return nil, fmt.Errorf("cistern: get %s: %w", id, err)
	}
	if droplet == nil {
		return nil, fmt.Errorf("cistern: droplet %s not found", id)
	}
	return droplet, nil
}

// List returns droplets filtered by repo and/or status. Empty strings mean no filter.
// Cancelled droplets are always excluded unless status is explicitly "cancelled".
func (c *Client) List(repo, status string) ([]*Droplet, error) {
	query := `SELECT id, repo, title, description, priority, complexity, status, assignee, current_cataractae, outcome, assigned_aqueduct, last_reviewed_commit, external_ref, last_heartbeat_at, created_at, updated_at, stage_dispatched_at
		 FROM droplets WHERE 1=1`
	var args []any
	if repo != "" {
		query += ` AND repo = ? COLLATE NOCASE`
		args = append(args, repo)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	} else {
		// Exclude cancelled from default views; they are only shown when explicitly
		// requested with status="cancelled".
		query += ` AND status != 'cancelled'`
	}
	query += ` ORDER BY priority ASC, created_at ASC`

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("cistern: list: %w", err)
	}
	defer rows.Close()

	var droplets []*Droplet
	for rows.Next() {
		d, err := scanDropletFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("cistern: scan droplet: %w", err)
		}
		droplets = append(droplets, d)
	}
	return droplets, rows.Err()
}

// Search returns droplets matching the given filters. query is a case-insensitive
// substring match on title (empty means all). status is an exact match on status
// (empty means all). priority is an exact match on priority (0 means all).
// Results are ordered by priority ASC, created_at ASC.
func (c *Client) Search(query, status string, priority int) ([]*Droplet, error) {
	qry := `SELECT id, repo, title, description, priority, complexity, status, assignee, current_cataractae, outcome, assigned_aqueduct, last_reviewed_commit, external_ref, last_heartbeat_at, created_at, updated_at, stage_dispatched_at
		 FROM droplets WHERE 1=1`
	var args []any
	if query != "" {
		qry += ` AND lower(title) LIKE lower(?)`
		args = append(args, "%"+query+"%")
	}
	if status != "" {
		qry += ` AND status = ?`
		args = append(args, status)
	} else {
		// Exclude cancelled from default views; they are only shown when explicitly
		// requested with status="cancelled".
		qry += ` AND status != 'cancelled'`
	}
	if priority != 0 {
		qry += ` AND priority = ?`
		args = append(args, priority)
	}
	qry += ` ORDER BY priority ASC, created_at ASC`

	rows, err := c.db.Query(qry, args...)
	if err != nil {
		return nil, fmt.Errorf("cistern: search: %w", err)
	}
	defer rows.Close()

	var droplets []*Droplet
	for rows.Next() {
		d, err := scanDropletFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("cistern: scan droplet: %w", err)
		}
		droplets = append(droplets, d)
	}
	return droplets, rows.Err()
}

// scanDroplet scans a single row into a Droplet. Returns nil, nil for sql.ErrNoRows.
func scanDroplet(row *sql.Row) (*Droplet, error) {
	var d Droplet
	var assignee, currentCataracta, outcome, assignedAqueduct, lastReviewedCommit, externalRef sql.NullString
	var lastHeartbeatAt, stageDispatchedAt sql.NullTime
	err := row.Scan(
		&d.ID, &d.Repo, &d.Title, &d.Description,
		&d.Priority, &d.Complexity, &d.Status, &assignee, &currentCataracta, &outcome, &assignedAqueduct, &lastReviewedCommit, &externalRef,
		&lastHeartbeatAt, &d.CreatedAt, &d.UpdatedAt, &stageDispatchedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fillDropletFromNullable(&d, assignee, currentCataracta, outcome, assignedAqueduct, lastReviewedCommit, externalRef, lastHeartbeatAt, stageDispatchedAt)
	return &d, nil
}

// scanDropletFromRows scans a single row from a Rows iterator into a Droplet.
func scanDropletFromRows(rows *sql.Rows) (*Droplet, error) {
	var d Droplet
	var assignee, currentCataracta, outcome, assignedAqueduct, lastReviewedCommit, externalRef sql.NullString
	var lastHeartbeatAt, stageDispatchedAt sql.NullTime
	if err := rows.Scan(
		&d.ID, &d.Repo, &d.Title, &d.Description,
		&d.Priority, &d.Complexity, &d.Status, &assignee, &currentCataracta, &outcome, &assignedAqueduct, &lastReviewedCommit, &externalRef,
		&lastHeartbeatAt, &d.CreatedAt, &d.UpdatedAt, &stageDispatchedAt,
	); err != nil {
		return nil, err
	}
	fillDropletFromNullable(&d, assignee, currentCataracta, outcome, assignedAqueduct, lastReviewedCommit, externalRef, lastHeartbeatAt, stageDispatchedAt)
	return &d, nil
}

// fillDropletFromNullable populates nullable fields from sql.Null* scan targets.
func fillDropletFromNullable(
	d *Droplet,
	assignee, currentCataracta, outcome, assignedAqueduct, lastReviewedCommit, externalRef sql.NullString,
	lastHeartbeatAt, stageDispatchedAt sql.NullTime,
) {
	d.Assignee = assignee.String
	d.CurrentCataractae = currentCataracta.String
	d.Outcome = outcome.String
	d.AssignedAqueduct = assignedAqueduct.String
	d.LastReviewedCommit = lastReviewedCommit.String
	d.ExternalRef = externalRef.String
	if lastHeartbeatAt.Valid {
		d.LastHeartbeatAt = lastHeartbeatAt.Time
	}
	if stageDispatchedAt.Valid {
		d.StageDispatchedAt = stageDispatchedAt.Time
	}
}

// Purge deletes delivered/pooled/cancelled droplets older than olderThan, cascading to
// cataractae_notes and events. Returns the count of droplets deleted (or that would be
// deleted in dry-run mode).
func (c *Client) Purge(olderThan time.Duration, dryRun bool) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan)

	if dryRun {
		var count int
		err := c.db.QueryRow(
			`SELECT COUNT(*) FROM droplets WHERE status IN ('delivered', 'pooled', 'cancelled') AND updated_at < ?`,
			cutoff,
		).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("cistern: purge dry-run: %w", err)
		}
		return count, nil
	}

	tx, err := c.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cistern: purge begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM cataractae_notes WHERE droplet_id IN (
			SELECT id FROM droplets WHERE status IN ('delivered', 'pooled', 'cancelled') AND updated_at < ?
		)`, cutoff,
	); err != nil {
		return 0, fmt.Errorf("cistern: purge cataractae_notes: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM events WHERE droplet_id IN (
			SELECT id FROM droplets WHERE status IN ('delivered', 'pooled', 'cancelled') AND updated_at < ?
		)`, cutoff,
	); err != nil {
		return 0, fmt.Errorf("cistern: purge events: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM droplet_issues WHERE droplet_id IN (
			SELECT id FROM droplets WHERE status IN ('delivered', 'pooled', 'cancelled') AND updated_at < ?
		)`, cutoff,
	); err != nil {
		return 0, fmt.Errorf("cistern: purge droplet_issues: %w", err)
	}

	res, err := tx.Exec(
		`DELETE FROM droplets WHERE status IN ('delivered', 'pooled', 'cancelled') AND updated_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("cistern: purge delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cistern: purge rows affected: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cistern: purge commit: %w", err)
	}
	return int(n), nil
}

// RecentEvent is a summary entry from the events or step_notes table.
type RecentEvent struct {
	Time    time.Time `json:"time"`
	Droplet string    `json:"droplet"`
	Event   string    `json:"event"`
}

// ListRecentEvents returns up to limit recent entries from the events and
// cataractae_notes tables, ordered newest-first.
func (c *Client) ListRecentEvents(limit int) ([]RecentEvent, error) {
	rows, err := c.db.Query(`
		SELECT droplet_id, event_type, created_at FROM events
		UNION ALL
		SELECT droplet_id, cataractae_name, created_at FROM cataractae_notes
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("cistern: list recent events: %w", err)
	}
	defer rows.Close()

	var events []RecentEvent
	for rows.Next() {
		var e RecentEvent
		if err := rows.Scan(&e.Droplet, &e.Event, &e.Time); err != nil {
			return nil, fmt.Errorf("cistern: scan recent event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// CountEventsByType returns the number of events with the given eventType for
// the specified droplet created after the since timestamp.
func (c *Client) CountEventsByType(id, eventType string, since time.Time) (int, error) {
	var count int
	err := c.db.QueryRow(
		`SELECT COUNT(*) FROM events WHERE droplet_id = ? AND event_type = ? AND created_at > ?`,
		id, eventType, since,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cistern: count events by type %s for %s: %w", eventType, id, err)
	}
	return count, nil
}

// DropletChange represents a note or event change for a single droplet.
// It is used by the tail command to stream events to stdout.
type DropletChange struct {
	Time  time.Time `json:"time"`
	Kind  string    `json:"kind"`  // "note" or "event"
	Value string    `json:"value"` // note content or event payload
}

// GetDropletChanges returns up to limit most recent changes for a specific droplet,
// ordered oldest-first. Changes come from two sources:
//   - cataractae notes (content and cataractae_name)
//   - events table (event_type + payload)
func (c *Client) GetDropletChanges(id string, limit int) ([]DropletChange, error) {
	rows, err := c.db.Query(`
		SELECT created_at, kind, value FROM (
			SELECT created_at, 'note' AS kind, cataractae_name || ': ' || content AS value
			FROM cataractae_notes WHERE droplet_id = ?
			UNION ALL
			SELECT created_at, 'event' AS kind, event_type || ': ' || COALESCE(payload, '') AS value
			FROM events WHERE droplet_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		) ORDER BY created_at ASC`, id, id, limit)
	if err != nil {
		return nil, fmt.Errorf("cistern: get droplet changes %s: %w", id, err)
	}
	defer rows.Close()

	var changes []DropletChange
	for rows.Next() {
		var ch DropletChange
		if err := rows.Scan(&ch.Time, &ch.Kind, &ch.Value); err != nil {
			return nil, fmt.Errorf("cistern: scan droplet change: %w", err)
		}
		changes = append(changes, ch)
	}
	return changes, rows.Err()
}

// DropletStats holds counts of droplets grouped by display status.
type DropletStats struct {
	Flowing   int // status=in_progress
	Queued    int // status=open
	Delivered int // status=delivered
	Pooled    int // status=pooled
}

// Stats returns counts of droplets grouped by status.
func (c *Client) Stats() (DropletStats, error) {
	rows, err := c.db.Query(`SELECT status, COUNT(*) FROM droplets GROUP BY status`)
	if err != nil {
		return DropletStats{}, fmt.Errorf("cistern: stats: %w", err)
	}
	defer rows.Close()

	var s DropletStats
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return DropletStats{}, fmt.Errorf("cistern: stats scan: %w", err)
		}
		switch status {
		case "in_progress":
			s.Flowing += count
		case "open":
			s.Queued += count
		case "delivered":
			s.Delivered += count
		case "pooled":
			s.Pooled += count
		}
	}
	return s, rows.Err()
}

// AddDependency adds a dependency edge: dropletID must wait for dependsOnID.
// Returns an error if either ID does not exist.
func (c *Client) AddDependency(dropletID, dependsOnID string) error {
	for _, id := range []string{dropletID, dependsOnID} {
		var exists int
		if err := c.db.QueryRow(`SELECT COUNT(*) FROM droplets WHERE id = ?`, id).Scan(&exists); err != nil {
			return fmt.Errorf("cistern: validate %s: %w", id, err)
		}
		if exists == 0 {
			return fmt.Errorf("cistern: droplet %s not found", id)
		}
	}
	_, err := c.db.Exec(
		`INSERT OR IGNORE INTO droplet_dependencies (droplet_id, depends_on) VALUES (?, ?)`,
		dropletID, dependsOnID,
	)
	if err != nil {
		return fmt.Errorf("cistern: add dependency %s->%s: %w", dropletID, dependsOnID, err)
	}
	return nil
}

// RemoveDependency removes a dependency edge.
func (c *Client) RemoveDependency(dropletID, dependsOnID string) error {
	_, err := c.db.Exec(
		`DELETE FROM droplet_dependencies WHERE droplet_id = ? AND depends_on = ?`,
		dropletID, dependsOnID,
	)
	if err != nil {
		return fmt.Errorf("cistern: remove dependency %s->%s: %w", dropletID, dependsOnID, err)
	}
	return nil
}

// GetDependencies returns the IDs of all droplets that dropletID depends on.
func (c *Client) GetDependencies(dropletID string) ([]string, error) {
	rows, err := c.db.Query(
		`SELECT depends_on FROM droplet_dependencies WHERE droplet_id = ? ORDER BY depends_on`,
		dropletID,
	)
	if err != nil {
		return nil, fmt.Errorf("cistern: get dependencies %s: %w", dropletID, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cistern: scan dependency: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetBlockedBy returns the IDs of undelivered dependencies that are blocking dropletID.
func (c *Client) GetBlockedBy(dropletID string) ([]string, error) {
	rows, err := c.db.Query(
		`SELECT dep.depends_on
		 FROM droplet_dependencies dep
		 JOIN droplets d ON d.id = dep.depends_on
		 WHERE dep.droplet_id = ? AND d.status != 'delivered'
		 ORDER BY dep.depends_on`,
		dropletID,
	)
	if err != nil {
		return nil, fmt.Errorf("cistern: get blocked-by %s: %w", dropletID, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cistern: scan blocked-by: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetDependents returns the IDs of droplets that depend on dropletID (reverse direction).
func (c *Client) GetDependents(dropletID string) ([]string, error) {
	rows, err := c.db.Query(
		`SELECT droplet_id FROM droplet_dependencies WHERE depends_on = ? ORDER BY droplet_id`,
		dropletID,
	)
	if err != nil {
		return nil, fmt.Errorf("cistern: get dependents %s: %w", dropletID, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cistern: scan dependent: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func checkRowsAffected(res sql.Result, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cistern: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cistern: droplet %s not found", id)
	}
	return nil
}

// DropletIssue is a reviewer finding tracked as a first-class DB record.
type DropletIssue struct {
	ID          string     `json:"id"`
	DropletID   string     `json:"droplet_id"`
	FlaggedBy   string     `json:"flagged_by"`
	FlaggedAt   time.Time  `json:"flagged_at"`
	Description string     `json:"description"`
	Status      string     `json:"status"` // open | resolved | unresolved
	Evidence    string     `json:"evidence,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

// generateIssueID returns a unique issue ID derived from the droplet ID.
func generateIssueID(dropletID string) (string, error) {
	b := make([]byte, 5)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return dropletID + "-" + string(b), nil
}

// AddIssue creates a new open issue for a droplet and returns it.
func (c *Client) AddIssue(dropletID, flaggedBy, description string) (*DropletIssue, error) {
	id, err := generateIssueID(dropletID)
	if err != nil {
		return nil, fmt.Errorf("cistern: generate issue id: %w", err)
	}
	now := time.Now().UTC()
	_, err = c.db.Exec(
		`INSERT INTO droplet_issues (id, droplet_id, flagged_by, flagged_at, description, status)
		 VALUES (?, ?, ?, ?, ?, 'open')`,
		id, dropletID, flaggedBy, now.Format("2006-01-02T15:04:05Z"), description,
	)
	if err != nil {
		return nil, fmt.Errorf("cistern: add issue: %w", err)
	}
	return &DropletIssue{
		ID:          id,
		DropletID:   dropletID,
		FlaggedBy:   flaggedBy,
		FlaggedAt:   now,
		Description: description,
		Status:      "open",
	}, nil
}

// ResolveIssue marks an issue as resolved with supporting evidence.
func (c *Client) ResolveIssue(issueID, evidence string) error {
	now := time.Now().UTC()
	res, err := c.db.Exec(
		`UPDATE droplet_issues SET status = 'resolved', evidence = ?, resolved_at = ? WHERE id = ?`,
		evidence, now.Format("2006-01-02T15:04:05Z"), issueID,
	)
	if err != nil {
		return fmt.Errorf("cistern: resolve issue %s: %w", issueID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cistern: resolve issue rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cistern: issue %s not found", issueID)
	}
	return nil
}

// RejectIssue marks an issue as unresolved (still present) with evidence.
func (c *Client) RejectIssue(issueID, evidence string) error {
	res, err := c.db.Exec(
		`UPDATE droplet_issues SET status = 'unresolved', evidence = ? WHERE id = ?`,
		evidence, issueID,
	)
	if err != nil {
		return fmt.Errorf("cistern: reject issue %s: %w", issueID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cistern: reject issue rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cistern: issue %s not found", issueID)
	}
	return nil
}

// ListIssues returns all issues for a droplet. If openOnly is true, only open issues are returned.
// If flaggedBy is non-empty, only issues with that flagged_by value are returned.
func (c *Client) ListIssues(dropletID string, openOnly bool, flaggedBy string) ([]DropletIssue, error) {
	query := `SELECT id, droplet_id, flagged_by, flagged_at, description, status, COALESCE(evidence,''), resolved_at
	          FROM droplet_issues WHERE droplet_id = ?`
	args := []any{dropletID}
	if openOnly {
		query += ` AND status = 'open'`
	}
	if flaggedBy != "" {
		query += ` AND flagged_by = ?`
		args = append(args, flaggedBy)
	}
	query += ` ORDER BY flagged_at ASC`

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("cistern: list issues %s: %w", dropletID, err)
	}
	defer rows.Close()

	var issues []DropletIssue
	for rows.Next() {
		var iss DropletIssue
		var resolvedAt sql.NullString
		var flaggedAt string
		if err := rows.Scan(&iss.ID, &iss.DropletID, &iss.FlaggedBy, &flaggedAt,
			&iss.Description, &iss.Status, &iss.Evidence, &resolvedAt); err != nil {
			return nil, fmt.Errorf("cistern: scan issue: %w", err)
		}
		if t, err := time.Parse("2006-01-02T15:04:05Z", flaggedAt); err == nil {
			iss.FlaggedAt = t
		}
		if resolvedAt.Valid && resolvedAt.String != "" {
			if t, err := time.Parse("2006-01-02T15:04:05Z", resolvedAt.String); err == nil {
				iss.ResolvedAt = &t
			}
		}
		issues = append(issues, iss)
	}
	return issues, rows.Err()
}

// CountOpenIssues returns the number of open issues for a droplet.
func (c *Client) CountOpenIssues(dropletID string) (int, error) {
	var count int
	err := c.db.QueryRow(
		`SELECT COUNT(*) FROM droplet_issues WHERE droplet_id = ? AND status = 'open'`,
		dropletID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cistern: count open issues %s: %w", dropletID, err)
	}
	return count, nil
}

// Restart resets a stuck or failed droplet back to 'open' status at the specified
// cataractae stage. It clears the assignee and outcome fields, sets current_cataractae
// to cataractaeName, and records a restart event. All existing notes
// are preserved. Returns the updated droplet.
func (c *Client) Restart(id, cataractaeName string) (*Droplet, error) {
	droplet, err := c.Get(id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	tx, err := c.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("cistern: begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE droplets SET status = 'open', assignee = '', outcome = NULL,
		 current_cataractae = ?, assigned_aqueduct = '', updated_at = ?,
		 stage_dispatched_at = NULL, last_heartbeat_at = NULL
		 WHERE id = ?`,
		cataractaeName, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("cistern: restart %s: %w", id, err)
	}
	if err := checkRowsAffected(res, id); err != nil {
		return nil, err
	}

	restartPayload, _ := json.Marshal(map[string]any{"cataractae": cataractaeName})
	if err := c.recordEvent(tx, id, EventRestart, string(restartPayload)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cistern: commit: %w", err)
	}

	droplet.Status = "open"
	droplet.Assignee = ""
	droplet.Outcome = ""
	droplet.CurrentCataractae = cataractaeName
	droplet.AssignedAqueduct = ""
	droplet.UpdatedAt = now
	droplet.StageDispatchedAt = time.Time{}
	droplet.LastHeartbeatAt = time.Time{}
	return droplet, nil
}

// FilterSession represents a LLM filter/refine conversation session.
type FilterSession struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Messages     string    `json:"messages"`       // JSON array of {role,content} objects
	SpecSnapshot string    `json:"spec_snapshot"`  // Current refined spec text
	LLMSessionID string    `json:"llm_session_id"` // LLM provider conversation session ID
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// FilterMessage represents a single message in a filter session conversation.
type FilterMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// CreateFilterSession creates a new filter session with the given title.
func (c *Client) CreateFilterSession(title, description string) (*FilterSession, error) {
	id, err := c.generateID()
	if err != nil {
		return nil, fmt.Errorf("cistern: generate id: %w", err)
	}
	now := time.Now().UTC()
	messagesJSON := "[]"
	_, err = c.db.Exec(
		`INSERT INTO filter_sessions (id, title, description, messages, spec_snapshot, llm_session_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, '', '', ?, ?)`,
		id, title, description, messagesJSON, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("cistern: create filter session: %w", err)
	}
	return &FilterSession{
		ID:          id,
		Title:       title,
		Description: description,
		Messages:    messagesJSON,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetFilterSession returns a filter session by ID.
func (c *Client) GetFilterSession(id string) (*FilterSession, error) {
	row := c.db.QueryRow(
		`SELECT id, title, description, messages, COALESCE(spec_snapshot,''), COALESCE(llm_session_id,''), created_at, updated_at
		 FROM filter_sessions WHERE id = ?`, id,
	)
	var s FilterSession
	var createdAt, updatedAt string
	if err := row.Scan(&s.ID, &s.Title, &s.Description, &s.Messages, &s.SpecSnapshot, &s.LLMSessionID, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("cistern: filter session %s not found", id)
		}
		return nil, fmt.Errorf("cistern: get filter session %s: %w", id, err)
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err == nil {
		s.CreatedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", updatedAt); err == nil {
		s.UpdatedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", updatedAt); err == nil {
		s.UpdatedAt = t
	}
	return &s, nil
}

// ListFilterSessions returns all filter sessions ordered by most recently updated first.
func (c *Client) ListFilterSessions() ([]FilterSession, error) {
	rows, err := c.db.Query(
		`SELECT id, title, description, messages, COALESCE(spec_snapshot,''), COALESCE(llm_session_id,''), created_at, updated_at
		 FROM filter_sessions ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("cistern: list filter sessions: %w", err)
	}
	defer rows.Close()

	var sessions []FilterSession
	for rows.Next() {
		var s FilterSession
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.Title, &s.Description, &s.Messages, &s.SpecSnapshot, &s.LLMSessionID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("cistern: scan filter session: %w", err)
		}
		if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err == nil {
			s.CreatedAt = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
			s.CreatedAt = t
		}
		if t, err := time.Parse("2006-01-02T15:04:05Z", updatedAt); err == nil {
			s.UpdatedAt = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", updatedAt); err == nil {
			s.UpdatedAt = t
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// UpdateFilterSessionMessages appends a message to the session and updates the spec snapshot and LLM session ID.
func (c *Client) UpdateFilterSessionMessages(id string, messages string, specSnapshot string, llmSessionID string) error {
	now := time.Now().UTC()
	res, err := c.db.Exec(
		`UPDATE filter_sessions SET messages = ?, spec_snapshot = ?, llm_session_id = ?, updated_at = ? WHERE id = ?`,
		messages, specSnapshot, llmSessionID, now, id,
	)
	if err != nil {
		return fmt.Errorf("cistern: update filter session %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cistern: update filter session rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cistern: filter session %s not found", id)
	}
	return nil
}

// DeleteFilterSession removes a filter session by ID.
func (c *Client) DeleteFilterSession(id string) error {
	res, err := c.db.Exec(`DELETE FROM filter_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("cistern: delete filter session %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cistern: delete filter session rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cistern: filter session %s not found", id)
	}
	return nil
}
