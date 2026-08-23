# Kafka และ Transactional Outbox

## ทำไมต้อง Outbox

ถ้าเขียน Task แล้ว publish Kafka แยกกัน อาจเกิดเหตุการณ์นี้:

```text
Task commit สำเร็จ → process crash → Kafka event หาย
```

จึงบันทึก Task และ Outbox row ใน database transaction เดียวกัน:

```text
BEGIN
  INSERT tasks
  INSERT outbox (published_at = NULL)
COMMIT
```

Publisher worker จะ poll เฉพาะ row ที่ยังไม่ published ส่งเข้า `task.events.v1` และ mark `published_at` หลัง Kafka ack ถ้าส่งไม่สำเร็จ row จะถูกลองใหม่

## Consumer

`activity-service` ใช้ consumer group `activity-service-v1` และเก็บ `event_id` เป็น unique key ใน database

```text
event ใหม่  → insert processed/activity + commit
event ซ้ำ   → unique conflict/ตรวจพบ → ไม่ทำซ้ำ
payload ผิด → log DLQ/retry path
```

ระบบนี้ใช้ at-least-once delivery ดังนั้น consumer ต้อง idempotent เสมอ

## Event envelope

```text
event_id, event_type, task payload, occurred_at
```

เมื่อเพิ่ม schema ในอนาคตให้เพิ่ม `version` และใช้ topic/version ใหม่เมื่อ breaking change
