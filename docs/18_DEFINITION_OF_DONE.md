# Definition of Done

> **Verified Task-version gate:** checklist นี้ใช้ปิด release `v0.1.0-predeploy`
> สำหรับ Order revision ให้ใช้ Definition of Complete และ matrix ใน
> [28_ORDER_IMPLEMENTATION_PLAN.md](28_ORDER_IMPLEMENTATION_PLAN.md)

Project 3 จะเรียกว่า “เสร็จ” เมื่อโค้ด เอกสาร การทดสอบ และการสาธิตสอดคล้องกัน ไม่ใช่เพียง container start ได้

Design package พร้อม 100% ไม่ทำให้ checkbox ด้านล่างผ่านโดยอัตโนมัติ ทุก `[x]` ต้องอ้าง
test ID/evidence จาก [24_TEST_ACCEPTANCE_MATRIX.md](24_TEST_ACCEPTANCE_MATRIX.md) และบันทึกใน
[25_PHASE_CONTEXT_RESUME.md](25_PHASE_CONTEXT_RESUME.md)

## Functional

```text
[x] Login/refresh/logout ใช้งานได้ (local smoke/OpenAPI)
[x] member/admin authorization ถูกต้องทั้ง Gateway และ service
[x] Task CRUD + GET by ID ใช้งานได้
[x] Redis cache hit/miss/invalidation มี implementation และ focused tests
[x] Task + Outbox commit เป็น transaction เดียวกัน
[x] Activity consumer idempotent และ commit offset หลัง DB success
[x] Analytics Worker ส่ง Kafka event เข้า ClickHouse
[x] Analytics API และ Frontend charts ใช้ ClickHouse จริง
[x] Swagger ครบทุก public endpoint
```

## Observability

```text
[x] ทุก Go service มี live/ready/metrics
[x] Prometheus scrape ผ่าน
[x] Grafana dashboards provision จาก Git
[x] มี Platform, Event Pipeline และ Business dashboard
[x] alert และ runbook สำคัญพร้อม
```

## Quality

```text
[x] backend unit/component/integration tests ผ่าน (local suites)
[x] frontend unit/component/E2E tests ผ่าน
[x] make test / vet / build ผ่าน
[x] frontend lint/test/build ผ่าน
[x] compose config และ clean startup ผ่าน
[ ] failure/retry/idempotency scenarios ผ่านครบทุก fault-injection matrix
[ ] ไม่มี race ที่ตรวจพบใน concurrent code ที่ครอบคลุม
```

## Documentation และ Learning

```text
[x] README และ docs ภาษาไทยอัปเดตตาม implementation จริง
[x] architecture/use-case diagrams ตรงกับ service จริง
[x] code มี comment ตาม Commenting Guide
[x] มีสรุปข้อดี/ข้อเสีย/trade-off สำหรับ interview
[ ] ไม่มีข้อความที่บอกว่า feature เสร็จก่อน test ยืนยัน
```

## GitHub ก่อน Deploy

```text
[x] CI green จาก clean clone — run `32652675076`
[x] secret scan ผ่านใน Compose job
[x] ไม่มี .env, private key, token หรือ production dump
[x] git status clean ก่อน docs/tag closeout
[x] repository เป็น Public และมี LICENSE
[x] สร้าง pre-deploy release/tag หลัง CI green
```

## Deploy

```text
[ ] budget และ payment method ได้รับการยืนยัน
[ ] environment variables/secret source ครบ
[ ] Render health checks ผ่าน
[ ] custom domain/TLS/CORS/cookie settings ถูกต้อง
[ ] production smoke test ผ่าน
[ ] rollback และ resource cleanup procedure พร้อม
```
