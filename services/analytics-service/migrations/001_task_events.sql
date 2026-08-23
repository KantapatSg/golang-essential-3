CREATE DATABASE IF NOT EXISTS analytics;
CREATE TABLE IF NOT EXISTS analytics.task_events
(
 event_id UUID, event_type LowCardinality(String), task_id UUID, actor_id UUID,
 task_status LowCardinality(String), occurred_at DateTime64(3,'UTC'),
 ingested_at DateTime64(3,'UTC') DEFAULT now64(3), event_date Date DEFAULT toDate(occurred_at), payload_json String
) ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(event_date)
ORDER BY (event_date,event_type,task_id,event_id)
TTL event_date + INTERVAL 90 DAY;
