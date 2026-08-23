CREATE TABLE IF NOT EXISTS activity_logs (id uuid PRIMARY KEY, event_id uuid UNIQUE NOT NULL, event_type text NOT NULL, task_id uuid NOT NULL, actor_id uuid NOT NULL, occurred_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS processed_events (event_id uuid PRIMARY KEY, processed_at timestamptz NOT NULL);
