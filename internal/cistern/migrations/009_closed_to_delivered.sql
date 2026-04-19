-- Migration 009: Migate closed → delivered status.
-- Idempotent: UPDATE ... WHERE is naturally idempotent.
UPDATE "droplets" SET "status" = 'delivered' WHERE "status" = 'closed';