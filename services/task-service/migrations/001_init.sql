CREATE TABLE IF NOT EXISTS tasks (id uuid PRIMARY KEY, owner_id uuid NOT NULL, title text NOT NULL, description text NOT NULL DEFAULT '', status text NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS outbox (event_id uuid PRIMARY KEY, event_type text NOT NULL, payload jsonb NOT NULL, occurred_at timestamptz NOT NULL, published_at timestamptz);
