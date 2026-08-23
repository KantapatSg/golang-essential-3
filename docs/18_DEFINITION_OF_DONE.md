# Definition of Done

Project 3 จะเรียกว่า “เสร็จ” เมื่อโค้ด เอกสาร การทดสอบ และการสาธิตสอดคล้องกัน ไม่ใช่เพียง container start ได้

## Functional

```text
[ ] Login/refresh/logout ใช้งานได้
[ ] member/admin authorization ถูกต้องทั้ง Gateway และ service
[ ] Task CRUD + GET by ID ใช้งานได้
[ ] Redis cache hit/miss/invalidation ตรวจสอบได้
[ ] Task + Outbox commit เป็น transaction เดียวกัน
[ ] Activity consumer idempotent และ commit offset หลัง DB success
[ ] Analytics Worker ส่ง Kafka event เข้า ClickHouse
[ ] Analytics API และ Frontend charts ใช้ ClickHouse จริง
[ ] Swagger ครบทุก public endpoint
```

## Observability

```text
[ ] ทุก Go service มี live/ready/metrics
[ ] Prometheus scrape ผ่าน
[ ] Grafana dashboards provision จาก Git
[ ] มี Platform, Event Pipeline และ Business dashboard
[ ] alert และ runbook สำคัญพร้อม
```

## Quality

```text
[ ] backend unit/component/integration tests ผ่าน
[ ] frontend unit/component/E2E tests ผ่าน
[ ] make test / vet / build ผ่าน
[ ] frontend lint/test/build ผ่าน
[ ] compose config และ clean startup ผ่าน
[ ] failure/retry/idempotency scenarios ผ่าน
[ ] ไม่มี race ที่ตรวจพบใน concurrent code ที่ครอบคลุม
```

## Documentation และ Learning

```text
[ ] README และ docs ภาษาไทยอัปเดตตาม implementation จริง
[ ] architecture/use-case diagrams ตรงกับ service จริง
[ ] code มี comment ตาม Commenting Guide
[ ] มีสรุปข้อดี/ข้อเสีย/trade-off สำหรับ interview
[ ] ไม่มีข้อความที่บอกว่า feature เสร็จก่อน test ยืนยัน
```

## GitHub ก่อน Deploy

```text
[ ] CI green จาก clean clone
[ ] secret scan ผ่าน
[ ] ไม่มี .env, private key, token หรือ production dump
[ ] git status clean
[ ] repository visibility และ license ได้รับการยืนยัน
[ ] สร้าง pre-deploy release/tag
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

