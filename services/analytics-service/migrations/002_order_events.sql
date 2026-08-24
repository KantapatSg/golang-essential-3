CREATE TABLE IF NOT EXISTS analytics.order_events
(
  event_id UUID,
  event_type LowCardinality(String),
  customer_id UUID,
  order_id UUID,
  occurred_at DateTime64(3, 'UTC'),
  payload_json String
)
ENGINE = ReplacingMergeTree(occurred_at)
ORDER BY (order_id, event_id)
TTL toDateTime(occurred_at) + INTERVAL 180 DAY;
