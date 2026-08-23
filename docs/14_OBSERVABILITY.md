# Prometheus และ Grafana Design

Observability ของ Project 3 ต้องช่วยตอบว่า “ระบบช้า/ล่มตรงไหน” และ “business event ไหลครบหรือไม่” ไม่ใช่เพียงติดตั้งเครื่องมือให้มีชื่ออยู่ใน stack

## Signal flow

```text
Go Services -- /metrics --> Prometheus ----+
                                              +--> Grafana dashboards + alerts
ClickHouse ---------------- datasource ------+
```

## Endpoint ต่อ service

| Endpoint | หน้าที่ |
|---|---|
| `/health/live` | process ยังทำงาน |
| `/health/ready` | dependency สำคัญพร้อมรับ traffic |
| `/metrics` | Prometheus exposition format |

Health endpoint ห้าม query หนักและห้ามเปิด secret ส่วน readiness ต้องแยก degraded dependency ที่ fallback ได้ เช่น Task cache จาก dependency ที่ขาดไม่ได้ เช่น writer database

## Metrics catalog ขั้นต่ำ

```text
http_requests_total{service,method,route,status}
http_request_duration_seconds{service,method,route}
grpc_server_requests_total{service,method,code}
grpc_server_duration_seconds{service,method}
db_operation_duration_seconds{service,operation,result}
cache_operations_total{service,operation,result}
outbox_pending_events
outbox_publish_total{result}
kafka_consumer_messages_total{consumer,result}
kafka_consumer_lag
clickhouse_insert_rows_total{result}
clickhouse_insert_duration_seconds{result}
analytics_ingestion_delay_seconds
```

ห้ามใช้ `user_id`, `task_id`, `event_id`, email หรือ request ID เป็น metric label เพราะจะสร้าง high cardinality ข้อมูลเฉพาะรายการให้ค้นใน structured log แทน

## Grafana dashboards

### 1. Platform Overview

- request rate, error rate และ latency p50/p95/p99
- service readiness
- gRPC status distribution
- PostgreSQL/Redis dependency errors

### 2. Event Pipeline

- outbox pending/publish errors
- Kafka consumer lag และ retry
- Activity/Analytics processed vs failed
- ClickHouse ingestion delay และ insert duration

### 3. Business Analytics

- Task events ตามเวลา
- Task status distribution
- active actors และ event types
- datasource เป็น ClickHouse ไม่ใช่ Prometheus

## Alerts สำหรับ portfolio

- API error rate สูงกว่า threshold ต่อเนื่อง
- p95 latency สูง
- service not ready
- outbox pending โตต่อเนื่อง
- consumer lag สูง
- analytics ingestion delay สูง

Alert ต้องมี `summary`, `impact` และ runbook link เพื่อใช้ตอบในการสัมภาษณ์ว่าหลัง alarm ดังต้องตรวจอะไร

## Comment ที่ต้องมีตอน implement

- อธิบายเหตุผลของ low-cardinality labels
- อธิบาย readiness vs liveness
- อธิบายว่า metrics recording ต้องไม่เปลี่ยน business result
- อธิบายว่า Grafana query จาก ClickHouse มี eventual consistency
