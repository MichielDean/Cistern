-- Migration 011: Add last_reviewed_commit column to droplets.
-- Idempotent: SQLite ADD COLUMN silently ignores duplicate columns.
ALTER TABLE "droplets" ADD COLUMN "last_reviewed_commit" TEXT DEFAULT NULL;