# Luna High Implementation Phases

เอกสารนี้เป็นลำดับงานหลักสำหรับ implement `golang-essential-3` ห้ามข้าม phase โดยยังไม่ผ่าน acceptance ของ phase ก่อนหน้า และต้องหยุดหลัง GitHub readiness ก่อนเริ่ม deploy

## Phase 0 — Fork Baseline และตั้ง Project Identity

**สถานะ: เสร็จแล้ว (baseline verification 2026-08-23)**

### งาน

- ใช้ Project 2 เป็น baseline โดยไม่คัดลอก `.git`, secret หรือ runtime volume
- เปลี่ยน module/import/proto package จาก `golang-essential-2` เป็น `golang-essential-3`
- คง comment และเอกสารที่อธิบาย CQRS, Redis, Outbox, Kafka, gRPC และ auth
- สร้าง local Git repository บน branch `main`

### Acceptance

```text
[x] ไม่มี module/import/proto path ของ golang-essential-2 เหลือใน source
[x] make test / vet / build ผ่าน
[x] compose config ผ่าน
[x] ไม่มี secret หรือ generated private key
```

## Phase 1 — Harden Project 2 Baseline

### งาน

- เพิ่ม REST `GET /api/v1/tasks/:id` ให้ครบกับ gRPC contract
- เปลี่ยน checked-in JSON fallback ให้ native protobuf เป็น production path; fallback ใช้เฉพาะ test/dev ถ้ายังจำเป็น
- เพิ่ม signal-aware graceful shutdown ให้ gRPC server, Kafka worker และ DB/Redis clients
- เปลี่ยน Activity consumer เป็น `FetchMessage` และ commit offset หลัง DB transaction สำเร็จ
- เพิ่ม request validation, pagination และ error mapping `Unavailable -> 503`
- ทำ versioned PostgreSQL migration command

### Comment บังคับ

- transport boundary และ status mapping
- deadline/cancellation propagation
- offset commit หลัง side effect
- migration ownership ต่อ bounded context

### Acceptance

```text
[ ] Project 2 use cases ยังผ่าน
[ ] shutdown ไม่ทิ้ง goroutine/connection ค้าง
[ ] consumer retry event เดิมได้โดยไม่สร้าง Activity ซ้ำ
[ ] API และ Swagger ตรงกัน
```

## Phase 2 — Frontend Foundation และ Secure Session

### งาน

- สร้าง `frontend/` ด้วย React + TypeScript + Vite
- ทำ design tokens, responsive shell, routes และ error boundary
- Implement Login, Logout, Refresh และ protected/admin routes
- ปรับ Gateway ให้รองรับ CORS และ HttpOnly refresh cookie
- Implement Task Board CRUD และ query invalidation
- เพิ่ม landing page อธิบาย architecture/stack/GitHub

### Comment บังคับ

- เหตุผลที่ access token อยู่ memory และ refresh token อยู่ cookie
- frontend role guard เป็น UX ไม่ใช่ security boundary
- query invalidation หลัง mutation

### Acceptance

```text
[ ] member/admin login และ refresh browser ได้
[ ] member ไม่เห็น/เข้า admin page ไม่ได้
[ ] Task CRUD ทำงานผ่าน Gateway จริง
[ ] loading/empty/error/success states ครบ
[ ] component tests ของ auth และ task flows ผ่าน
```

## Phase 3 — Analytics Contracts และ ClickHouse Schema

### งาน

- เพิ่ม `analytics/v1/analytics.proto`
- กำหนด REST endpoints สำหรับ summary, timeseries และ status breakdown
- สร้าง `services/analytics-service` และ `services/analytics-worker`
- เพิ่ม ClickHouse versioned migrations และ connection health check
- เพิ่ม time range, timezone, pagination/limit validation

### Comment บังคับ

- PostgreSQL vs ClickHouse ownership
- เหตุผลของ partition/order key
- eventual consistency ที่ API/UI ต้องสื่อสาร

### Acceptance

```text
[ ] proto generation reproducible
[ ] ClickHouse migration รันซ้ำได้อย่างปลอดภัย
[ ] analytics query มี time range และ limit
[ ] schema รองรับ task.created/updated/deleted
```

## Phase 4 — Kafka to ClickHouse Analytics Worker

### งาน

- สร้าง consumer group `analytics-service-v1`
- Validate event version และ required fields
- Batch insert เข้า ClickHouse
- Commit offset หลัง insert สำเร็จ
- Implement retry แบบ bounded exponential backoff
- ส่ง poison message ไป `task.events.v1.dlq`
- ทำ idempotency strategy ด้วย event ID

### Comment บังคับ

- at-least-once delivery ไม่ใช่ exactly-once
- batch size/time trade-off
- จุดที่ commit offset และเหตุผล
- DLQ ใช้เมื่อใดและ replay อย่างไร

### Acceptance

```text
[ ] event เดิมไม่ทำให้ dashboard นับซ้ำตาม semantics ที่กำหนด
[ ] ClickHouse ล่มแล้ว Kafka message ไม่หาย
[ ] invalid message ไป DLQ พร้อม reason
[ ] worker shutdown แล้ว flush batch ตาม timeout
```

## Phase 5 — Analytics API และ Portfolio Dashboard

### งาน

- Gateway เรียก Analytics Service ผ่าน gRPC deadline
- Implement `/api/v1/analytics/summary`, `/timeseries`, `/statuses`
- จำกัด analytics routes ตาม role
- สร้าง charts, date-range filter, last-updated และ empty/error states
- เพิ่ม Activity timeline และ System page

### Acceptance

```text
[ ] Dashboard แสดงข้อมูลที่สร้างจาก Kafka -> ClickHouse จริง
[ ] member/admin policy ตรง contract
[ ] UI บอก eventual consistency และเวลาข้อมูลล่าสุด
[ ] query ไม่ scan แบบไม่มีขอบเขต
```

## Phase 6 — Prometheus Instrumentation

### งาน

- เพิ่ม `/health/live`, `/health/ready`, `/metrics` ทุก Go service
- เพิ่ม HTTP/gRPC/DB/cache/outbox/Kafka/ClickHouse metrics ตาม catalog
- เพิ่ม request ID ใน structured logs แต่ไม่ใส่ลง metric label
- เพิ่ม Prometheus scrape config ใน Compose

### Comment บังคับ

- readiness vs liveness
- เหตุผลที่ห้าม high-cardinality label
- timer/counter อยู่รอบ operation ใด

### Acceptance

```text
[ ] Prometheus targets up
[ ] metric labels ไม่มี user/task/event/email
[ ] failure path เพิ่ม counter ถูกต้อง
[ ] metrics endpoint ไม่ต้อง authentication ภายใน private network
```

## Phase 7 — Grafana Dashboards และ Alerts

### งาน

- provision Prometheus และ ClickHouse datasources
- provision Platform, Event Pipeline และ Business dashboards
- เพิ่ม alerts พร้อม runbook link
- export dashboard JSON ลง Git เพื่อสร้าง environment ซ้ำได้

### Acceptance

```text
[ ] docker compose up แล้ว dashboard ปรากฏอัตโนมัติ
[ ] มี panel แสดง RED metrics, outbox, Kafka lag และ ingestion delay
[ ] Business dashboard query ClickHouse จริง
[ ] alert rule มี summary/impact/runbook
```

## Phase 8 — Tests, Security และ Failure Scenarios

### งาน

- unit tests ของ use case/policy/cache/idempotency/analytics
- integration tests PostgreSQL/Redis/Kafka/ClickHouse
- frontend component tests และ Playwright E2E
- contract tests Gateway ↔ gRPC
- ทดสอบ dependency failure, retry, deadline และ recovery
- dependency/security scan และ secret scan

### Acceptance

```text
[ ] member/admin positive และ negative paths ผ่าน
[ ] duplicate/retry/out-of-order event scenarios มี test
[ ] ClickHouse/Kafka/Redis unavailable มี expected behavior
[ ] test ไม่พึ่งลำดับและ cleanup resource ได้
```

## Phase 9 — Local Production-like Compose

### งาน

- เพิ่ม frontend, analytics, ClickHouse, Prometheus และ Grafana ใน Compose
- health checks และ startup dependencies
- resource limits ที่เหมาะกับเครื่องพัฒนา
- seed demo data ผ่าน explicit command
- smoke script ครอบคลุม Browser/API/Event/Dashboard flow

### Acceptance

```text
[ ] docker compose config ผ่าน
[ ] clean-volume startup ผ่าน
[ ] smoke flow สร้าง Task แล้วเห็น Activity/Analytics
[ ] restart service แล้วข้อมูลสำคัญยังอยู่
```

## Phase 10 — GitHub Portfolio Readiness

### งาน

- เพิ่ม GitHub Actions: backend test/vet/build, frontend lint/test/build, compose validation
- README มี screenshot, architecture, demo accounts และ local commands
- ตรวจ license, `.gitignore`, env example และไม่มี secret
- สร้าง public repository และ push เมื่อผู้ใช้อนุมัติ

### Acceptance

```text
[ ] CI green จาก clean clone
[ ] README links และ commands ใช้ได้
[ ] git status clean
[ ] repository ไม่มี .env/private key/token
[ ] release tag สำหรับ pre-deploy candidate
```

## HOLD POINT — หยุดก่อน Deploy

หลัง Phase 10 ให้สรุปผล test, service count, environment variables และค่าใช้จ่ายล่าสุด แล้วรอคำสั่ง deploy ห้ามสร้าง paid resource หรือผูก custom domain โดยอัตโนมัติ

## Phase 11 — Render Deploy

ทำเฉพาะหลังผ่าน Hold Point:

- สร้าง Render Blueprint/environment groups
- Deploy frontend, gateway และ private services
- เชื่อม managed PostgreSQL/Redis/Kafka/ClickHouse/Grafana ตาม budget ที่อนุมัติ
- ตั้ง health checks, CORS, cookie domain และ custom domain/TLS
- รัน production smoke/E2E และจัดทำ rollback procedure
