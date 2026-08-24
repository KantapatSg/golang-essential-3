-- Order V2 keeps the V1 history intact; terminal semantics are sourced only from live events.
CREATE TABLE IF NOT EXISTS analytics.order_events_v2
(
  event_id UUID,
  schema_version UInt16 DEFAULT 1,
  event_type LowCardinality(String),
  order_id UUID,
  customer_id UUID,
  order_status LowCardinality(String) DEFAULT '',
  product_ids Array(String) DEFAULT [],
  amount_minor Int64 DEFAULT 0,
  currency LowCardinality(String) DEFAULT 'USD',
  reason LowCardinality(String) DEFAULT '',
  environment LowCardinality(String) DEFAULT 'local',
  run_id String DEFAULT '',
  occurred_at DateTime64(3, 'UTC'),
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
  event_date Date MATERIALIZED toDate(occurred_at),
  payload_json String DEFAULT '',
  INDEX idx_customer_id customer_id TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(event_date)
ORDER BY (event_date, event_type, order_id, event_id)
TTL event_date + INTERVAL 180 DAY;
