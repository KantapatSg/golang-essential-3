# Interview Guide

## Elevator pitch

`golang-essential-3` เป็น Task Operations portfolio ที่เริ่มจาก REST Gateway แล้วแยก bounded context ด้วย gRPC ใช้ PostgreSQL/Redis สำหรับ transactional workload ใช้ Transactional Outbox และ Kafka เพื่อส่ง event อย่างเชื่อถือได้ ใช้ ClickHouse สำหรับ analytics และใช้ Prometheus/Grafana ตรวจสุขภาพระบบทั้งหมด โดยมี React frontend แสดง business use case จริง

## Flow ที่ควรเล่าได้ใน 60 วินาที

```text
User creates Task
-> React calls Fiber Gateway
-> Gateway authenticates JWT and calls Task gRPC with deadline
-> Task Service commits Task + Outbox in PostgreSQL
-> Redis cache is invalidated
-> Outbox worker publishes Kafka event
-> Activity consumer builds PostgreSQL read model
-> Analytics worker batches event into ClickHouse
-> Frontend/Grafana query analytics
-> Prometheus observes latency, errors, outbox and consumer lag
```

## Trade-offs ที่ต้องตอบได้

| Tool/Pattern | ข้อดี | ข้อเสีย/สิ่งที่ต้องรับผิดชอบ |
|---|---|---|
| Fiber | เร็วและ API กระชับ | ecosystem/context ต่างจาก `net/http` บางส่วน |
| gRPC | contract ชัด, typed, เหมาะ service-to-service | debug ผ่าน browser ยากและมี network failure |
| PostgreSQL | ACID, relational query, source of truth | analytical scan ใหญ่ไม่ใช่งานถนัด |
| Redis | latency ต่ำสำหรับ cache/session | invalidation และ persistence policy ซับซ้อน |
| Kafka | decouple, replay, consumer groups | operational complexity และ eventual consistency |
| ClickHouse | columnar aggregation เร็วและประหยัดสำหรับ event | ไม่เหมาะ OLTP และ dedup ไม่เหมือน unique constraint |
| Prometheus | time-series metrics และ alerting ดี | label cardinality/retention ต้องควบคุม |
| Grafana | รวมหลาย datasource และ dashboard as code | dashboard ที่ไม่มี SLO/runbook กลายเป็นเพียงภาพสวย |
| Render | deploy ง่าย เหมาะ portfolio | free/low tiers มีข้อจำกัดและหลาย service เพิ่มค่าใช้จ่าย |

## Failure scenarios

### Kafka ล่มหลังสร้าง Task

Task ยังสำเร็จเพราะ Task และ Outbox commit แล้ว Worker retry ภายหลัง ผู้ใช้ไม่ต้องรอ Kafka แต่ Activity/Analytics จะยังไม่อัปเดตชั่วคราว

### Redis ล่ม

Task cache เป็น optimization จึง fallback ไป Reader DB ได้ แต่ Identity refresh session เป็น security state จึงควร fail closed และตอบ service unavailable

### ClickHouse ล่ม

CRUD ยังทำงาน Kafka เก็บ event ไว้ตาม retention และ Analytics Worker ไม่ commit offset จน insert สำเร็จ Dashboard จะแสดงข้อมูลเก่าและ ingestion delay สูงขึ้น

### Event ซ้ำ

ระบบยอมรับ at-least-once delivery Consumer ใช้ `event_id` และ transaction/idempotency strategy เพื่อไม่สร้าง side effect ซ้ำ ไม่อ้างว่า network เป็น exactly-once

### Read replica lag

CQRS read อาจไม่เห็น mutation ทันที ต้องกำหนด UX/read-after-write policy เช่นตอบ entity จาก writer หลัง mutation แล้วค่อย revalidate list

## ประโยคสรุป architecture

Project 1 เน้น layer ใน process เดียว Project 2 เพิ่ม microservices/gRPC/Kafka และ Project 3 เพิ่ม user-facing frontend, analytical workload และ observability เพื่อแสดงทั้ง business flow และ production trade-offs

