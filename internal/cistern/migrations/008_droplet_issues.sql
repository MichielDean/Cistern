-- Migration 008: Create droplet_issues table.
-- Idempotent: IF NOT EXISTS.
CREATE TABLE IF NOT EXISTS "droplet_issues" (
    "id"          TEXT PRIMARY KEY,
    "droplet_id"  TEXT NOT NULL REFERENCES "droplets"("id"),
    "flagged_by"  TEXT NOT NULL,
    "flagged_at"  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    "description" TEXT NOT NULL,
    "status"      TEXT NOT NULL DEFAULT 'open',
    "evidence"    TEXT,
    "resolved_at" DATETIME
);