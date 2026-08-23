# Code Commenting Guide

Project นี้ใช้ comment เพื่อการเรียนรู้ จึงต้องอธิบาย decision, invariant และ failure mode ไม่ใช่แปล syntax ของ Go เป็นภาษาไทยทีละบรรทัด

## หลักการ

### Comment เมื่อ

- มี boundary ระหว่าง HTTP, gRPC, Kafka, cache หรือ database
- ลำดับ operation สำคัญต่อ consistency
- มี security decision เช่น token, cookie, RBAC หรือ secret
- มี concurrency, goroutine, channel, shutdown หรือ retry
- มี trade-off ที่อ่านจาก code อย่างเดียวไม่ชัด
- behavior เป็น fallback/degraded mode

### ไม่ Comment เมื่อ

- โค้ดบอกสิ่งเดียวกันชัดอยู่แล้ว เช่น `count++`
- เป็น generated protobuf code
- comment อ้าง implementation ที่ไม่จริง
- ใช้ comment กลบ function ที่ยาวหรือ ownership ไม่ชัด

## รูปแบบที่ต้องการ

```go
// Task และ Outbox ต้อง commit ใน transaction เดียวกัน
// เพื่อไม่ให้ command สำเร็จแต่ event สำหรับ downstream หาย
err := db.Transaction(func(tx *gorm.DB) error { ... })
```

ควรตอบ “ทำไม” และ “ถ้าทำผิดจะเกิดอะไร” ภายใน 1–3 บรรทัด

## Comment checklist ตาม component

| Component | จุดที่ต้องอธิบาย |
|---|---|
| Gateway | public edge, deadline, metadata propagation, HTTP/gRPC error mapping |
| Identity | bcrypt, RS256 key ownership, refresh rotation, HttpOnly cookie |
| Task | CQRS reader/writer, cache-aside, invalidation, ownership, outbox transaction |
| Activity | consumer group, idempotency, DB transaction, Kafka offset commit |
| Analytics Worker | batching, retry, DLQ, ClickHouse idempotency, graceful flush |
| Analytics API | time range, query limit, eventual consistency |
| Prometheus | label cardinality, readiness/liveness, metrics around failures |
| Frontend | auth storage, role guard limitation, query invalidation, error boundary |
| Deployment | secret source, private/public network, health-check expectation |

## Comment review gate

- ทุก comment ต้องตรงกับ code ปัจจุบัน
- ถ้าเปลี่ยนลำดับ transaction/commit ต้องปรับ comment ใน PR เดียวกัน
- TODO ต้องมี phase/issue owner เช่น `TODO(P3-Phase4)`
- ห้ามใส่ password, token, DSN หรือข้อมูลผู้ใช้จริงใน comment/example
- ใช้ภาษาไทยในจุดสอนแนวคิด และใช้ศัพท์อังกฤษเดิมเมื่อเป็นชื่อ pattern/tool

