CREATE TABLE IF NOT EXISTS "filter_sessions" (
    "id" TEXT PRIMARY KEY,
    "title" TEXT NOT NULL,
    "description" TEXT DEFAULT '',
    "messages" TEXT DEFAULT '[]',
    "spec_snapshot" TEXT DEFAULT '',
    "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    "updated_at" DATETIME DEFAULT CURRENT_TIMESTAMP
);