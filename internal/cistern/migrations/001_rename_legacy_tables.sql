-- Migration 001: Rename legacy table and column names to canonical vocabulary.
-- All statements are idempotent — errors are expected on already-renamed or fresh DBs.
-- The migration runner ignores errors for ALTER TABLE RENAME statements.
ALTER TABLE work_items RENAME TO droplets;
ALTER TABLE drops RENAME TO droplets;
ALTER TABLE step_notes RENAME TO cataractae_notes;
ALTER TABLE cataractae_notes RENAME COLUMN item_id TO droplet_id;
ALTER TABLE cataractae_notes RENAME COLUMN drop_id TO droplet_id;
ALTER TABLE cataractae_notes RENAME COLUMN step_name TO cataractae_name;
ALTER TABLE events RENAME COLUMN item_id TO droplet_id;
ALTER TABLE events RENAME COLUMN drop_id TO droplet_id;
ALTER TABLE droplets RENAME COLUMN current_step TO current_cataractae;