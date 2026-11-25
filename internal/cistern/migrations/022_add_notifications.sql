-- 022_add_notifications.sql — persistent dashboard notifications.
-- Notifications are derived from the existing events table: every qualifying
-- event (see cistern.NotificationEventTypes) is promoted into one notification
-- row. event_row_id is UNIQUE so the promotion sync is idempotent: re-running
-- the sync over an overlapping range can never duplicate a notification.
-- created_at carries no default: every insert supplies an explicit UTC
-- timestamp so the column holds exactly one comparable format.
CREATE TABLE IF NOT EXISTS "notifications" (
    "id"           INTEGER PRIMARY KEY AUTOINCREMENT,
    "event_row_id" INTEGER NOT NULL UNIQUE,
    "droplet_id"   TEXT NOT NULL DEFAULT '',
    "event_type"   TEXT NOT NULL,
    "title"        TEXT NOT NULL DEFAULT '',
    "detail"       TEXT NOT NULL DEFAULT '',
    "created_at"   DATETIME NOT NULL,
    "read_at"      DATETIME
);
CREATE INDEX IF NOT EXISTS "idx_notifications_unread" ON "notifications" ("read_at", "id");
