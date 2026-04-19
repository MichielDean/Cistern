-- Migration 006: Migrate legacy status vocabulary to canonical values.
-- Idempotent: UPDATE ... WHERE IN is naturally idempotent.
UPDATE "droplets" SET "status" = 'pooled' WHERE "status" IN ('stagnant', 'blocked', 'escalated');