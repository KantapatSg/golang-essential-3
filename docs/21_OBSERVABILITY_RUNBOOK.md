# Observability runbook

## Service not ready

1. Check `/health/live` and `/health/ready` for the affected service.
2. Inspect database/Redis/Kafka dependency logs; readiness is intentionally stricter than liveness.
3. Restart only after identifying the failed dependency. No user/task/event IDs are metric labels; use request logs for one operation.

## Analytics delay

1. Check Kafka consumer lag and ClickHouse availability.
2. Check analytics worker retry/DLQ counters and the `analytics.task_events` latest `ingested_at`.
3. If a poison event is in DLQ, preserve it for replay after correcting the schema. The API should continue to report its `data_through` timestamp and eventual consistency.
