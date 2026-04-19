-- Migration 007: Create droplet_dependencies table.
-- Idempotent: IF NOT EXISTS.
CREATE TABLE IF NOT EXISTS "droplet_dependencies" (
    "droplet_id" TEXT NOT NULL REFERENCES "droplets"("id"),
    "depends_on" TEXT NOT NULL REFERENCES "droplets"("id"),
    PRIMARY KEY ("droplet_id", "depends_on")
);