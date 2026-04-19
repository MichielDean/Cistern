-- Migration 002: Add complexity column to droplets.
-- Idempotent: SQLite ADD COLUMN silently ignores duplicate columns.
ALTER TABLE droplets ADD COLUMN complexity INTEGER DEFAULT 2;