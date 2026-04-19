-- Migration 010: Add assigned_aqueduct column to droplets.
-- Idempotent: SQLite ADD COLUMN silently ignores duplicate columns.
ALTER TABLE "droplets" ADD COLUMN "assigned_aqueduct" TEXT DEFAULT '';