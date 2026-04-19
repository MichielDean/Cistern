-- Migration 012: Add external_ref column to droplets.
-- Idempotent: SQLite ADD COLUMN silently ignores duplicate columns.
ALTER TABLE "droplets" ADD COLUMN "external_ref" TEXT DEFAULT NULL;