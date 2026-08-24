-- Inventory-owned additive schema; existing Order/Task tables are untouched.
CREATE TABLE IF NOT EXISTS inventory_products (
  id text PRIMARY KEY, name text NOT NULL, currency text NOT NULL,
  unit_price_minor bigint NOT NULL, on_hand integer NOT NULL DEFAULT 0,
  reserved integer NOT NULL DEFAULT 0, version bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS inventory_stock_movements (
  id uuid PRIMARY KEY, product_id text NOT NULL REFERENCES inventory_products(id),
  delta integer NOT NULL, reason text NOT NULL, balance_after integer NOT NULL,
  idempotency_key text NOT NULL UNIQUE, request_hash text NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS inventory_reservations (
  id uuid PRIMARY KEY, order_id uuid NOT NULL, product_id text NOT NULL REFERENCES inventory_products(id),
  quantity integer NOT NULL, status text NOT NULL, reason text NOT NULL DEFAULT '', created_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS inventory_processed_events (
  event_id uuid PRIMARY KEY, event_type text NOT NULL, processed_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS inventory_outbox (
  event_id uuid PRIMARY KEY, event_type text NOT NULL, aggregate_id text NOT NULL,
  payload jsonb NOT NULL, occurred_at timestamptz NOT NULL, published_at timestamptz NULL
);
