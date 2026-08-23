CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS idx_tasks_owner_created ON tasks (owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;
