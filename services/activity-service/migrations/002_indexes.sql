CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS idx_activity_occurred ON activity_logs (occurred_at DESC);
