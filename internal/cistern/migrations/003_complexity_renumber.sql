-- Migration 003: Remap old 4-level complexity to new 3-level scheme.
-- Old: 1=trivial, 2=standard, 3=full, 4=critical
-- New: 1=standard, 2=full, 3=critical
-- This runs exactly once per database, tracked by _schema_migrations.
-- The migration runner wraps this in a transaction automatically.
UPDATE "droplets" SET complexity = complexity - 1 WHERE complexity >= 2;