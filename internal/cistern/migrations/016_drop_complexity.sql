-- Migration 016: Remove the complexity column from droplets.
-- The complexity system (standard/full/critical) has been removed.
-- All droplets now flow through all pipeline stages unconditionally.
-- SQLite doesn't support DROP COLUMN before 3.35.0, so we recreate the table.

-- Create new table without complexity column.
CREATE TABLE IF NOT EXISTS "droplets_new" (
    "id" TEXT PRIMARY KEY,
    "repo" TEXT NOT NULL,
    "title" TEXT NOT NULL,
    "description" TEXT DEFAULT '',
    "priority" INTEGER DEFAULT 2,
    "status" TEXT DEFAULT 'open',
    "assignee" TEXT DEFAULT '',
    "current_cataractae" TEXT DEFAULT '',
    "outcome" TEXT DEFAULT NULL,
    "assigned_aqueduct" TEXT DEFAULT '',
    "last_reviewed_commit" TEXT DEFAULT NULL,
    "external_ref" TEXT DEFAULT NULL,
    "last_heartbeat_at" DATETIME DEFAULT NULL,
    "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    "updated_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    "stage_dispatched_at" DATETIME DEFAULT NULL
);

-- Copy data (omit complexity column).
INSERT OR IGNORE INTO "droplets_new" (
    id, repo, title, description, priority, status, assignee,
    current_cataractae, outcome, assigned_aqueduct, last_reviewed_commit,
    external_ref, last_heartbeat_at, created_at, updated_at, stage_dispatched_at
)
SELECT
    id, repo, title, description, priority, status, assignee,
    current_cataractae, outcome, assigned_aqueduct, last_reviewed_commit,
    external_ref, last_heartbeat_at, created_at, updated_at, stage_dispatched_at
FROM "droplets";

-- Drop old table and rename.
DROP TABLE "droplets";
ALTER TABLE "droplets_new" RENAME TO "droplets";