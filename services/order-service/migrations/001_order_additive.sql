-- Additive Order-owned schema. Task tables are intentionally not touched.
CREATE TABLE IF NOT EXISTS orders (
  id uuid PRIMARY KEY, customer_id uuid NOT NULL, status text NOT NULL,
  payment_scenario text NOT NULL, total_minor bigint NOT NULL,
  currency text NOT NULL, reason text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS order_items (
  id uuid PRIMARY KEY, order_id uuid NOT NULL REFERENCES orders(id),
  product_id text NOT NULL, name text NOT NULL, quantity integer NOT NULL,
  unit_price_minor bigint NOT NULL, line_total_minor bigint NOT NULL, currency text NOT NULL
);
CREATE TABLE IF NOT EXISTS order_outbox (
  event_id uuid PRIMARY KEY, event_type text NOT NULL, aggregate_id uuid NOT NULL,
  payload jsonb NOT NULL, occurred_at timestamptz NOT NULL, published_at timestamptz NULL
);
CREATE TABLE IF NOT EXISTS order_idempotency (
  key text PRIMARY KEY, customer_id uuid NOT NULL, request_hash text NOT NULL,
  order_id uuid NOT NULL REFERENCES orders(id), created_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS order_processed_events (
  event_id uuid PRIMARY KEY, event_type text NOT NULL, processed_at timestamptz NOT NULL
);
