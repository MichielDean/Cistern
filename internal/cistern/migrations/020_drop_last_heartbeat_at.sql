-- Migration 020: Drop last_heartbeat_at column.
-- Agent heartbeats are replaced by Castellarius-driven liveness detection
-- (session log mtime). The column is no longer written or read.
ALTER TABLE "droplets" DROP COLUMN "last_heartbeat_at";