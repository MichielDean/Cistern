-- Migration 014: Add last_heartbeat_at column to droplets.
-- Idempotent: SQLite ADD COLUMN silently ignores duplicate columns.
ALTER TABLE "droplets" ADD COLUMN "last_heartbeat_at" DATETIME DEFAULT NULL;