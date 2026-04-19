-- Migration 013: Add stage_dispatched_at column to droplets.
-- Idempotent: SQLite ADD COLUMN silently ignores duplicate columns.
ALTER TABLE "droplets" ADD COLUMN "stage_dispatched_at" DATETIME DEFAULT NULL;