-- Migration 015: Rename cataracta → cataractae (final spelling).
-- Idempotent: errors ignored on already-renamed DBs.
ALTER TABLE "cataracta_notes" RENAME TO "cataractae_notes";
ALTER TABLE "cataractae_notes" RENAME COLUMN "cataracta_name" TO "cataractae_name";
ALTER TABLE "droplets" RENAME COLUMN "current_cataracta" TO "current_cataractae";