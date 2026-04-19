-- Migration 005: Add outcome column to droplets.
-- Idempotent: SQLite ADD COLUMN silently ignores duplicate columns.
ALTER TABLE "droplets" ADD COLUMN "outcome" TEXT DEFAULT NULL;