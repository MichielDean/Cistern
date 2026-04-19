-- Migration 004: Normalize stored repo values to canonical casing.
-- Tracked: runs exactly once per database.
-- The migration runner wraps this in a transaction automatically.
UPDATE "droplets" SET repo = 'cistern' WHERE LOWER(repo) = LOWER('cistern') AND repo != 'cistern';
UPDATE "droplets" SET repo = 'ScaledTest' WHERE LOWER(repo) = LOWER('ScaledTest') AND repo != 'ScaledTest';
UPDATE "droplets" SET repo = 'PortfolioWebsite' WHERE LOWER(repo) = LOWER('PortfolioWebsite') AND repo != 'PortfolioWebsite';