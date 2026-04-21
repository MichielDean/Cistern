-- Migration 018: Migrate historical scheduler notes to structured events.
--
-- Before structured event types existed, the scheduler wrote diagnostic notes to
-- cataractae_notes. Now that events have typed payloads, this one-time migration
-- backfills those historical notes into the events table and deletes the originals.
--
-- Idempotent: staging table is cleaned at the start, and INSERT INTO events uses
-- a NOT EXISTS guard on (droplet_id, event_type, created_at). Re-running is safe.
-- Unparsable notes are left untouched (no data loss).
-- [scheduler:loop-recovery-pending] markers are left untouched per spec.
--
-- Patterns handled:
--   [scheduler:exit-no-outcome] Session <s> exited without outcome (worker=<w>, cataractae=<c>). [<ts>]
--   [scheduler:zombie] Session <s> died without outcome (worker=<w>, cataractae=<c>). [<ts>]
--   [scheduler:stall] elapsed=<dur> heartbeat=<ts_or_none>
--   [scheduler:recovery] Orphan reset to open (cataractae=<c>).
--   [scheduler:recovery] reset orphaned in_progress droplet to open — no assignee, no active session
--   [scheduler:loop-recovery] detected <from>→<to> loop on reviewer issue <issue> — routing to reviewer
--   [scheduler:routing] Auto-promoted: cataractae=<c> signaled recirculate but has no on_recirculate route — routing via on_pass to <dest>
--   [scheduler:routing] cataractae=<c> signaled recirculate but has no on_recirculate route and no on_pass route — droplet pooled
--   [scheduler:routing] cataractae=<c> signaled recirculate but has no on_recirculate route — restarting at <dest>
--   [circuit-breaker] <n> dead sessions in <dur> with no outcome — pooling
--   cancelled: <reason> [<ts>]  (with cataractae_name = 'scheduler')
--   cancelled [<ts>]  (variant with no reason, with cataractae_name = 'scheduler')
--   restarted at cataractae "<c>" [<ts>]  (with cataractae_name = 'scheduler')

-- Step 1: Create a staging table of (note_id, droplet_id, event_type, payload, created_at)
-- for all migratable scheduler notes. This is the parsing logic.

CREATE TABLE IF NOT EXISTS _migrate_scheduler_notes_staging (
    note_id INTEGER PRIMARY KEY,
    droplet_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL
);

-- Clean staging from any prior partial run of this migration.
DELETE FROM _migrate_scheduler_notes_staging;

-- 1a: [scheduler:exit-no-outcome] Session <s> exited without outcome (worker=<w>, cataractae=<c>). [<ts>]
-- Example: "[scheduler:exit-no-outcome] Session abc123 exited without outcome (worker=host1, cataractae=implement). [2026-04-21T10:00:00Z]"
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'exit_no_outcome',
    json_object(
        'session',    substr(
            n.content,
            instr(n.content, 'Session ') + 8,
            instr(substr(n.content, instr(n.content, 'Session ') + 8), ' ') - 1
        ),
        'worker',     substr(
            n.content,
            instr(n.content, 'worker=') + 7,
            instr(substr(n.content, instr(n.content, 'worker=') + 7), ',') - 1
        ),
        'cataractae', substr(
            n.content,
            instr(n.content, 'cataractae=') + 11,
            instr(substr(n.content, instr(n.content, 'cataractae=') + 11), ')') - 1
        )
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[scheduler:exit-no-outcome] Session %';

-- 1b: [scheduler:zombie] Session <s> died without outcome (worker=<w>, cataractae=<c>). [<ts>]
-- Zombies are an older name for exit_no_outcome and migrate to the same event type.
-- Format is identical to [scheduler:exit-no-outcome] except "died" instead of "exited".
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'exit_no_outcome',
    json_object(
        'session',    substr(
            n.content,
            instr(n.content, 'Session ') + 8,
            instr(substr(n.content, instr(n.content, 'Session ') + 8), ' ') - 1
        ),
        'worker',     substr(
            n.content,
            instr(n.content, 'worker=') + 7,
            instr(substr(n.content, instr(n.content, 'worker=') + 7), ',') - 1
        ),
        'cataractae', substr(
            n.content,
            instr(n.content, 'cataractae=') + 11,
            instr(substr(n.content, instr(n.content, 'cataractae=') + 11), ')') - 1
        )
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[scheduler:zombie] Session %';

-- 1c: [scheduler:stall] elapsed=<dur> heartbeat=<ts_or_none>
-- Example: "[scheduler:stall] elapsed=45m0s heartbeat=2026-04-21T10:00:00Z"
-- Example: "[scheduler:stall] elapsed=45m0s heartbeat=none"
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'stall',
    json_object(
        'cataractae', '',
        'elapsed',    substr(
            n.content,
            instr(n.content, 'elapsed=') + 8,
            instr(substr(n.content, instr(n.content, 'elapsed=') + 8), ' ') - 1
        ),
        'heartbeat',  substr(n.content, instr(n.content, 'heartbeat=') + 10)
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[scheduler:stall] elapsed=%';

-- 1d: [scheduler:recovery] Orphan reset to open (cataractae=<c>).
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'recovery',
    json_object(
        'cataractae', substr(
            n.content,
            instr(n.content, 'cataractae=') + 11,
            instr(substr(n.content, instr(n.content, 'cataractae=') + 11), ')') - 1
        )
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[scheduler:recovery] Orphan reset to open (cataractae=%';

-- 1e: [scheduler:recovery] reset orphaned in_progress droplet to open — no assignee, no active session
-- No cataractae name available in this variant so use empty string.
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'recovery',
    json_object('cataractae', ''),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[scheduler:recovery] reset orphaned%';

-- 1f: [scheduler:loop-recovery] detected <from>→<to> loop on reviewer issue <issue> — routing to reviewer
-- The arrow character → is U+2192 (char(8594) in SQLite).
-- Example: "[scheduler:loop-recovery] detected implement→implement loop on reviewer issue iss-001 — routing to reviewer"
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'loop_recovery',
    json_object(
        'from',  substr(
            n.content,
            instr(n.content, 'detected ') + 9,
            instr(substr(n.content, instr(n.content, 'detected ') + 9), char(8594)) - 1
        ),
        'to',    substr(
            n.content,
            instr(n.content, char(8594)) + 1,
            instr(substr(n.content, instr(n.content, char(8594)) + 1), ' ') - 1
        ),
        'issue', substr(
            n.content,
            instr(n.content, 'issue ') + 6,
            instr(substr(n.content, instr(n.content, 'issue ') + 6), ' ') - 1
        )
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[scheduler:loop-recovery] detected%';

-- 1g: [scheduler:routing] Auto-promoted: cataractae=<c> signaled recirculate but has no on_recirculate route — routing via on_pass to <dest>
-- "on_pass to " is 11 characters.
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'auto_promote',
    json_object(
        'cataractae', substr(
            n.content,
            instr(n.content, 'cataractae=') + 11,
            instr(substr(n.content, instr(n.content, 'cataractae=') + 11), ' ') - 1
        ),
        'routed_to',  substr(n.content, instr(n.content, 'on_pass to ') + 11)
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[scheduler:routing] Auto-promoted%';

-- 1h: [scheduler:routing] cataractae=<c> signaled recirculate but has no on_recirculate route and no on_pass route — droplet pooled
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'no_route',
    json_object(
        'cataractae', substr(
            n.content,
            instr(n.content, 'cataractae=') + 11,
            instr(substr(n.content, instr(n.content, 'cataractae=') + 11), ' ') - 1
        )
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[scheduler:routing] cataractae=%'
  AND n.content LIKE '%droplet pooled%';

-- 1i: [scheduler:routing] cataractae=<c> signaled recirculate but has no on_recirculate route — restarting at <dest>
-- Older variant that restarted instead of pooling, map to no_route since there was no valid route.
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'no_route',
    json_object(
        'cataractae', substr(
            n.content,
            instr(n.content, 'cataractae=') + 11,
            instr(substr(n.content, instr(n.content, 'cataractae=') + 11), ' ') - 1
        )
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[scheduler:routing] cataractae=%'
  AND n.content LIKE '%restarting at%';

-- 1j: [circuit-breaker] <n> dead sessions in <dur> with no outcome — pooling
-- Example: "[circuit-breaker] 5 dead sessions in 15m0s with no outcome — pooling"
-- "] " is 2 chars after the bracket, we find the number after "] ".
-- "sessions in " is 12 characters.
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'circuit_breaker',
    json_object(
        'death_count', cast(
            substr(
                n.content,
                instr(n.content, '] ') + 2,
                instr(substr(n.content, instr(n.content, '] ') + 2), ' ') - 1
            ) as integer
        ),
        'window', substr(
            n.content,
            instr(n.content, 'sessions in ') + 12,
            instr(substr(n.content, instr(n.content, 'sessions in ') + 12), ' ') - 1
        )
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE '[circuit-breaker]%';

-- 1k: cancelled: <reason> [<timestamp>]  (written by old scheduler as cataractae_name='scheduler')
-- The format is "cancelled: <reason> [<timestamp>]" where the reason goes to <timestamp>.
-- We strip " [<timestamp>]" from the end to extract just the reason.
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'cancel',
    json_object('reason', substr(
        substr(n.content, 12),
        1,
        instr(substr(n.content, 12), ' [') - 1
    )),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE 'cancelled: %';

-- 1k-alt: cancelled [<timestamp>]  (variant with no reason)
-- Format: "cancelled [<timestamp>]" — no colon, no reason text.
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'cancel',
    json_object('reason', ''),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE 'cancelled [%'
  AND n.content NOT LIKE 'cancelled: %';

-- 1l: restarted at cataractae "<c>" [<timestamp>]  (written by old scheduler as cataractae_name='scheduler')
-- Example: 'restarted at cataractae "implement" [2026-04-21 10:00:05]'
-- "cataractae \"" is 12 characters.
INSERT INTO _migrate_scheduler_notes_staging (note_id, droplet_id, event_type, payload, created_at)
SELECT
    n.id,
    n.droplet_id,
    'restart',
    json_object(
        'cataractae', substr(
            n.content,
            instr(n.content, 'cataractae "') + 12,
            instr(substr(n.content, instr(n.content, 'cataractae "') + 12), '"') - 1
        )
    ),
    n.created_at
FROM cataractae_notes n
WHERE n.cataractae_name = 'scheduler'
  AND n.content LIKE 'restarted at cataractae%';

-- Step 2: Insert into events, skipping rows that already exist with the same
-- (droplet_id, event_type, created_at) triple. This makes the migration idempotent.
-- There is no unique constraint on events for this triple, but in practice collisions
-- are impossible because each note has a distinct timestamp matching a distinct event.

INSERT INTO events (droplet_id, event_type, payload, created_at)
SELECT s.droplet_id, s.event_type, s.payload, s.created_at
FROM _migrate_scheduler_notes_staging s
WHERE NOT EXISTS (
    SELECT 1 FROM events e
    WHERE e.droplet_id = s.droplet_id
      AND e.event_type = s.event_type
      AND e.created_at = s.created_at
);

-- Step 3: Delete migrated notes from cataractae_notes. Only delete rows whose IDs
-- are in the staging table — unparsable notes and loop-recovery-pending markers
-- are left untouched.

DELETE FROM cataractae_notes
WHERE id IN (SELECT note_id FROM _migrate_scheduler_notes_staging);

-- Step 4: Drop the staging table.
DROP TABLE IF EXISTS _migrate_scheduler_notes_staging;