# ClickHouse Analytics Design

ClickHouse เป็น analytical database สำหรับเก็บ event จำนวนมากและ aggregate เร็ว ส่วน PostgreSQL ยังคงเป็น transactional source of truth ของ Identity, Task และ Activity

## สิ่งที่ ClickHouse ไม่ได้แทน

- ไม่แทน PostgreSQL สำหรับ CRUD/transaction
- ไม่แทน Prometheus สำหรับ infrastructure metrics
- ไม่แทน Grafana ซึ่งเป็น visualization layer

```text
PostgreSQL = สถานะธุรกิจปัจจุบันและ transaction
ClickHouse = ประวัติ event และ analytical query
Prometheus = metrics ตามเวลา เช่น latency/error/lag
Grafana    = dashboard ที่อ่าน Prometheus และ ClickHouse
```

## Data flow

```text
Task command
  -> PostgreSQL transaction: tasks + outbox
  -> Outbox publisher
  -> Kafka topic task.events.v1
  -> Analytics Worker consumer group analytics-service-v1
  -> validate + deduplicate + batch insert
  -> ClickHouse task_events
  -> Analytics Service gRPC
  -> Gateway REST
  -> Frontend charts
```

Activity Service และ Analytics Worker ใช้คนละ consumer group จึงได้รับ event เดียวกันคนละชุดตามหน้าที่ของตน

## Raw event schema เป้าหมาย

```sql
CREATE TABLE analytics.task_events
(
    event_id UUID,
    event_type LowCardinality(String),
    task_id UUID,
    actor_id UUID,
    task_status LowCardinality(String),
    occurred_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
    event_date Date DEFAULT toDate(occurred_at),
    payload_json String
)
ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(event_date)
ORDER BY (event_date, event_type, task_id, event_id);
```

`ReplacingMergeTree` ช่วยลด duplicate ใน background merge แต่ไม่รับประกันว่า query ทันทีจะเห็นข้อมูล deduplicate แล้ว Worker จึงยังต้องออกแบบ idempotency และ query สำคัญอาจใช้ event id semantics เพิ่มเติม

## Query use cases

- จำนวน Task created/updated/deleted ตามช่วงเวลา
- จำนวน Task แยก `todo`, `doing`, `done`
- Activity volume ต่อวัน
- ผู้ใช้ที่มี activity สูงสุดสำหรับ admin
- Event ingestion delay: `ingested_at - occurred_at`

ไม่ query raw events โดยไม่มี time range ทุก endpoint ต้องมี default range และ limit

## Worker reliability

1. ใช้ `FetchMessage` เพื่อยังไม่ commit offset ก่อนประมวลผล
2. Validate schema version และ required fields
3. Batch insert ตามจำนวนหรือเวลา
4. Commit Kafka offset หลัง ClickHouse ยืนยันสำเร็จ
5. Retry transient error ด้วย bounded exponential backoff
6. ส่ง poison message ไป `task.events.v1.dlq` พร้อม reason
7. วัด processed, failed, retry, DLQ และ consumer lag metrics

## Migration และ retention

- SQL migration ต้อง versioned และรันเป็น command/job แยกจาก request path
- Local เริ่มด้วย retention 90 วันเพื่อฝึก TTL
- Production portfolio เลือก retention ตามงบ ไม่ hard-delete จาก application
- Migration ทุกตัวต้องมีคำอธิบาย forward/rollback; ClickHouse DDL บางแบบ rollback ด้วย migration ใหม่แทน transaction rollback

