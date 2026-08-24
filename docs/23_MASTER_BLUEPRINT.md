# Project 3 Master Blueprint

> **Verified Task-version blueprint:** เอกสารนี้อธิบาย runtime ที่ tag
> `v0.1.0-predeploy` ส่วน Order Processing Platform รุ่นถัดไปยังเป็น Planned และใช้
> [26_ORDER_PLATFORM_OVERVIEW.md](26_ORDER_PLATFORM_OVERVIEW.md),
> [27_ORDER_PLATFORM_DESIGN.md](27_ORDER_PLATFORM_DESIGN.md),
> [28_ORDER_IMPLEMENTATION_PLAN.md](28_ORDER_IMPLEMENTATION_PLAN.md) และ
> [29_ORDER_REVISION_HANDOFF.md](29_ORDER_REVISION_HANDOFF.md) เป็น source-of-truth

เอกสารนี้คือภาพรวมทางเทคนิคฉบับหลักสำหรับ `golang-essential-3` และเป็น contract
ระหว่าง Design กับ Luna High ผู้ implement คำว่า **Design ready 100%** หมายถึง scope,
boundary, flow, trade-off, test และ stop condition ถูกตัดสินใจครบ ไม่ได้หมายความว่า
source หรือ deployment ผ่านครบแล้ว สถานะ runtime ล่าสุดอยู่ใน
[25_PHASE_CONTEXT_RESUME.md](25_PHASE_CONTEXT_RESUME.md)

## 1. Product Overview

ชื่อ Portfolio UI คือ **Signal Ledger** เป็นระบบจัดการ Task ขนาดเล็กที่ตั้งใจทำให้แนวคิด
ระบบขนาดใหญ่เห็นได้จากหน้าเว็บเดียว ผู้ใช้สร้าง Task แบบ transactional แล้วดูผลที่ถูก
project ผ่าน Kafka ไปเป็น Activity timeline และ ClickHouse analytics พร้อมดูสุขภาพระบบ
ผ่าน Prometheus/Grafana

### Actor และ Use Case

| Actor | ทำอะไรได้ |
|---|---|
| Visitor | อ่าน landing page และ OpenAPI/Swagger |
| Member | Login, refresh/logout และ CRUD Task ของตนเอง |
| Admin | ทำสิทธิ์ Member และดู Activity, Analytics, System signals |
| Operator | ดู metrics/dashboard/alert และใช้ runbook แก้ปัญหา |

### สิ่งที่ Project นี้ต้องสาธิต

1. REST public API ด้วย Fiber และ internal synchronous RPC ด้วย gRPC
2. PostgreSQL/GORM สำหรับ OLTP พร้อม logical CQRS reader/writer
3. Redis cache-aside และ refresh-session state ที่มี failure policy ต่างกัน
4. Kafka, Transactional Outbox, idempotent consumer และ eventual consistency
5. ClickHouse สำหรับ OLAP projection ไม่ใช่ transactional source of truth
6. React portfolio UI, Swagger, unit/integration/E2E tests
7. Prometheus/Grafana สำหรับ metrics, dashboard, alert และ troubleshooting
8. Docker Compose/CI และ Render deployment หลัง Local/GitHub gate ผ่าน

## 2. Target Architecture

```text
                              USERS
                                |
                                v
                    +-------------------------+
                    | React + TypeScript      |
                    | Signal Ledger Website   |
                    +------------+------------+
                                 |
                       HTTPS / REST / JSON
                                 |
                                 v
                    +-------------------------+
                    | Fiber API Gateway       |
                    | JWT, RBAC, Swagger      |
                    +------------+------------+
                                 |
             gRPC + metadata + deadline + request ID
                                 |
        +------------------------+------------------------+
        |                        |                        |
        v                        v                        v
 +---------------+       +---------------+       +----------------+
 | Identity      |       | Task          |       | Activity       |
 | gRPC service  |       | gRPC service  |       | gRPC + consumer|
 +-------+-------+       +-------+-------+       +--------+-------+
         |                       |                        |
 Postgres + Redis         Postgres R/W + Redis        Postgres
                                 |
                         Task + Outbox TX
                                 |
                                 v
                               Kafka
                         task.events.v1
                          /             \
                         v               v
              Activity projection   Analytics Worker
                                            |
                                            v
                                        ClickHouse
                                            |
                                            v
                                    Analytics gRPC API

 All Go components -- /metrics --> Prometheus --> Grafana --> Alert/Runbook
 ClickHouse ------------------------------------> Grafana Business Dashboard
```

### Trust และ Network Boundary

- Frontend และ API Gateway เป็น public; gRPC, databases, Kafka, metrics endpoint และ
  Grafana admin interface ต้องอยู่ private network หรือมี access control
- Gateway ตรวจ JWT เพื่อหยุด request เร็ว แต่ service ปลายทางต้องตรวจ actor/ownership ซ้ำ
- Identity ถือ private key; Gateway ใช้เฉพาะ public keyและต้อง fail startup ใน production
  ถ้าโหลด key ไม่ได้ ห้ามสร้าง fallback key นอก `DEV_MODE`
- Secret ใช้ environment/secret manager ไม่ commit `.env`, key, DSN หรือ token จริง

## 3. Service Catalog และ Data Ownership

| Component | Ownership | Sync API | Async role | Store |
|---|---|---|---|---|
| Frontend | UX/session-in-memory | REST | ไม่มี | Browser memory + HttpOnly cookie |
| API Gateway | Public transport policy | Fiber REST | ไม่มี | Stateless |
| Identity | Credential/token/session lifecycle | gRPC | ไม่มี | `identity_db`, Redis session |
| Task | Task command/query policy | gRPC | Outbox producer | `task_db`, Redis cache |
| Activity | Audit-style read model | gRPC | Kafka consumer | `activity_db` |
| Analytics Worker | OLAP projection | health/metrics | Kafka consumer + DLQ | ClickHouse writer |
| Analytics Service | Bounded analytical queries | gRPC | ไม่มี | ClickHouse reader |
| Prometheus | Time-series metric collection | HTTP scrape | ไม่มี | Local TSDB/managed equivalent |
| Grafana | Visualization/alert UI | Web | Alert evaluation | Provisioned dashboards |

กฎสำคัญ: service ห้ามอ่าน database ของ service อื่นโดยตรง และห้าม import business code
ข้าม bounded context การแชร์ทำผ่าน Protobuf contract หรือ versioned event เท่านั้น

## 4. End-to-End Flows

### 4.1 Login, Refresh และ Logout

```text
Browser -> POST /auth/login -> Gateway -> Identity
                                      -> identity_db verify user/bcrypt
                                      -> sign RS256 access token
                                      -> Redis refresh session + TTL
Browser <- access token in memory + refresh token in HttpOnly cookie

Browser -> POST /auth/refresh (cookie) -> Redis rotate session -> new token pair
Browser -> POST /auth/logout  (cookie) -> Redis revoke -> clear browser cookie
```

Failure policy: Redis session เป็น security state จึง fail closed; ห้ามออก refresh token
เมื่อ Redis ยืนยัน session ไม่ได้ Access token อายุสั้น ส่วน refresh token ต้อง rotate/revoke ได้

### 4.2 Task Query: CQRS + Cache-Aside

```text
Browser -> Gateway JWT -> Task gRPC + actor metadata
                         -> Redis cache hit -> response
                         -> cache miss/error -> Reader DSN -> PostgreSQL
                                             -> SET cache TTL -> response
```

Reader/Writer เป็น logical CQRS: local อาจชี้ server เดียวกัน แต่ config ต้องแยกเพื่อเปลี่ยน
Reader ไป replica ได้ Cache เป็น optimization จึง fail open ไป Reader DB ได้

### 4.3 Task Command + Reliable Event

```text
Create/Update/Delete
  -> validate + ownership
  -> PostgreSQL transaction [task mutation + outbox insert]
  -> commit
  -> invalidate Redis
  -> return HTTP response

Outbox worker -> publish Kafka -> broker ACK -> mark published_at
```

ห้าม dual-write PostgreSQL และ Kafka ตรงจาก request เพราะอาจสำเร็จเพียงฝั่งเดียว Outbox
ทำให้ event retry ได้แม้ Kafka ล่ม และ HTTP request ไม่ต้องรอ downstream projection

### 4.4 Activity Projection

```text
Kafka -> activity consumer group -> validate schema/event
      -> DB transaction [processed_events unique(event_id) + activity row]
      -> commit DB -> commit Kafka offset
```

Delivery เป็น at-least-once ไม่ใช่ exactly-once ความถูกต้องเกิดจาก idempotency key และลำดับ
commit side effect ก่อน offset

### 4.5 Analytics Projection

```text
Kafka -> analytics consumer group -> validate
      -> invalid: publish DLQ with structured reason -> commit source offset
      -> valid: batch -> retry ClickHouse insert
              -> ClickHouse ACK -> commit Kafka offsets
```

`event_id` เป็น deduplication identity สำหรับ Portfolio นี้ใช้ `ReplacingMergeTree` และ query
แบบ bounded `FINAL` หรือกลยุทธ์ equivalent ที่พิสูจน์ด้วย duplicate test เพื่อไม่ให้นับซ้ำก่อน
background merge ห้ามอ้าง idempotency จาก engine โดยไม่ทดสอบ immediate query

### 4.6 Observability Flow

```text
Services /metrics -> Prometheus scrape -> RED + pipeline metrics
Prometheus rules  -> alert state -> runbook
Prometheus + ClickHouse -> Grafana operational/business dashboards
```

Metrics ใช้ low-cardinality labels เช่น `service`, `method`, `status_class`; ห้ามใช้ user ID,
task ID, event ID, email หรือ request ID สิ่งเหล่านี้อยู่ใน structured logs

## 5. Stack Decisions

| Stack | เหตุผลที่ใช้ | ข้อดี | ข้อเสีย/วิธีรับมือ |
|---|---|---|---|
| Go | เหมาะกับ concurrent network services | compile เร็ว, binary เดียว, goroutine เบา | error handling ซ้ำ; แยก helper เฉพาะ transport |
| Fiber | REST edge และ middleware เรียนรู้ง่าย | เร็ว, API กระชับ | ไม่ใช่ `net/http` ตรง; จำกัดไว้ Gateway |
| gRPC + Protobuf | Contract ภายในแบบ typed | codegen, status/deadline, schema ชัด | browser เรียกตรงยาก, breaking change; ผ่าน Gateway และ version proto |
| GORM + PostgreSQL | OLTP, transaction, migration | ecosystem ดีและ query ยืดหยุ่น | ORM ซ่อน query/เสี่ยง N+1; log SQL, index และ integration test |
| Logical CQRS | แยก read/write concern โดยไม่เพิ่มระบบเกินจำเป็น | รองรับ replica ภายหลัง | read-after-write อาจ stale; document consistency |
| Redis | cache และ TTL session | latency ต่ำ, TTL ในตัว | data loss/eviction; cache fail-open แต่ auth fail-closed |
| Kafka | durable event stream และ consumer group | replay, scale consumer, decouple service | operation ซับซ้อน; Outbox, idempotency, DLQ และ lag monitoring |
| ClickHouse | aggregate event ปริมาณมาก | columnar query เร็ว, compression ดี | ไม่เหมาะ transaction/update รายแถว; PostgreSQL ยังเป็น source of truth |
| React + TypeScript | Portfolio UI แบบ typed | ecosystem/testability ดี | bundle/dependency เพิ่ม; route split และ audit dependency |
| TanStack Query | server-state lifecycle | cache/loading/error/refetch ชัด | cache ซ้อน backend Redis; แยก UX cache จาก business cache |
| Recharts | แสดง analytics เร็ว | component-based | bundle ใหญ่; lazy-load analytics route |
| Prometheus | pull metrics/PromQL | standard, alertable | high cardinality แพง; จำกัด labels/retention |
| Grafana | รวม operational + business view | provisioning และ datasource หลายแบบ | ไม่ได้เก็บ metrics เอง; ต้องมี Prometheus/ClickHouse |
| Docker Compose | reproducible local integration | เปิด stack ได้คำสั่งเดียว | ไม่แทน production orchestrator; ใช้เพื่อ local gate |
| GitHub Actions | clean-clone quality gate | feedback อัตโนมัติ | integration ใช้เวลา; แยก fast CI กับ Compose job |
| Render | Portfolio deploy ที่จัดการ TLS/URL ง่าย | ลดงาน server operation | หลาย private service มีค่าใช้จ่าย/cold start; deploy หลัง budget gate |

ClickHouse ไม่ใช่ Grafana/Prometheus: ClickHouse เก็บและ query business event, Prometheus เก็บ
operational metrics และ Grafana เป็นหน้าจอที่อ่านจากทั้งสอง datasource

## 6. Consistency, Reliability และ Security Model

- Strong consistency อยู่ใน transaction ของแต่ละ service เท่านั้น
- Activity/Analytics เป็น eventually consistent; UI ต้องแสดง `data_through` และสถานะ waiting
- ทุก RPC/HTTP client call มี deadline; retry เฉพาะ operation ที่ปลอดภัยหรือมี idempotency
- Goroutine ทุกตัวรับ lifecycle context, ไม่มี fire-and-forget ที่ shutdown ไม่ได้
- Readiness ตรวจ dependency ที่จำเป็นจริง; liveness ตรวจ process เท่านั้น
- Password hash ด้วย bcrypt, JWT RS256, refresh token rotate/revoke และ cookie `HttpOnly`,
  `Secure` ใน production, `SameSite`/CORS ตาม topology
- Authorization ตรวจทั้ง edge และ domain service; frontend role guard เป็นเพียง UX

## 7. Mandatory Implementation Gap Register

Luna ต้องปิดรายการนี้ก่อน Phase 10:

| ID | Gap ปัจจุบัน | Target decision | Phase/Test |
|---|---|---|---|
| G1 | `/swagger/` เป็นเพียง link และ OpenAPI ไม่ครบ | ใช้ Swagger UI จริงและ contract ครบ auth/activity/analytics/error/security | P1, T-API |
| G2 | Production Nginx ยังไม่ proxy `/api`, `/openapi.yaml` | proxy same-origin ไป Gateway หรือ build-time API URL ที่มี CORS test | P2/P9, T-WEB |
| G3 | Logout ไม่ fallback cookie/clear cookie ครบ | อ่าน refresh cookie, revoke Redis, expire cookie | P2/P8, T-AUTH |
| G4 | หลาย `/health/ready` ตอบ ready แบบคงที่ | probe dependency จำเป็นแบบ bounded; cache degraded ไม่ทำให้ Task down | P6/P9, T-OBS |
| G5 | Metric/dashboard labels และ Kafka lag ยังไม่ตรงกัน | Prom client format, `service=job` relabel หรือ query by job, เพิ่ม exporter/lag metric | P6/P7, T-OBS |
| G6 | ClickHouse duplicate semantics ยังไม่พิสูจน์ทันที | bounded deduplicated query + replay/duplicate integration test | P3/P4/P8, T-EVENT |
| G7 | Gateway สร้าง fallback RSA key ได้โดยไม่แยก prod ชัด | fallback เฉพาะ `DEV_MODE`; production fail fast | P1/P8, T-AUTH |
| G8 | CI ยังไม่มี provider-backed Compose/E2E gate | เพิ่ม integration job พร้อม timeout/log artifact | P8/P10, T-CI |
| G9 | Frontend dependency audit เคยรายงาน high/critical | upgrade แบบไม่ break และบันทึก audit result | P8, T-SEC |

## 8. Scope Guardrails

รอบนี้ไม่เพิ่ม Kubernetes, service mesh, distributed tracing backend, Elasticsearch,
RabbitMQ, schema registry หรือ multi-region เพราะไม่ได้ช่วย acceptance หลักและเพิ่ม operation
surface มากเกิน Portfolio หากต้องเพิ่มภายหลังต้องมี use case และ test ใหม่รองรับก่อน

## 9. Completion Meaning

Project 3 จะเรียก Local/GitHub complete ได้เมื่อ:

1. G1–G9 ปิดพร้อม test evidence
2. Test matrix ผ่านและ `docs/18_DEFINITION_OF_DONE.md` ถูก check ตามหลักฐาน
3. Clean-volume Compose flow เห็น Task -> Activity -> ClickHouse -> Frontend/Grafana
4. CI green จาก clean clone, secret scan ผ่าน, git status clean และมี pre-deploy tag
5. จากนั้นหยุดที่ Hold Point จนผู้ใช้อนุมัติ Render Phase 11
