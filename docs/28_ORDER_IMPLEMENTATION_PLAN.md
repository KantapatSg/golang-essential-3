# Order Platform Revision — Implementation Plan

> สถานะ: **R0-R11 ผ่าน; R12 Render ถูก hold ตาม scope**
>
> แผนนี้ใช้สำหรับเปลี่ยน `golang-essential-3` จาก Task demo ไปเป็น Order Processing Platform
> โดยรักษา `main` และ tag `v0.1.0-predeploy` เป็น baseline ที่ย้อนกลับได้ ห้ามลบ schema,
> volume หรือ source เดิมก่อนผ่าน cutover และได้รับการยืนยันแยกต่างหาก

## 1. วิธีใช้แผนนี้

- ทำตาม Phase R0-R11 ตามลำดับ และหยุดก่อน R12 (Render)
- แต่ละ Phase ต้องจบด้วย focused tests, evidence และ commit ที่ย้อนกลับได้
- สถานะ `Done` หมายถึง acceptance criteria ของ Phase นั้นผ่านจริง ไม่ใช่แค่มีไฟล์
- หากแก้ runtime ระหว่างทดสอบ ต้อง commit รวมใน Phase ที่เกี่ยวข้อง ห้ามทิ้ง fix ค้างไว้
- comment ภาษาไทยเฉพาะ decision, invariant และ failure boundary ไม่ comment syntax ทั่วไป
- Contract/schema ใช้แนวทาง expand -> migrate/verify -> cut over -> contract
- Consumer และ command ที่ retry ได้ต้อง idempotent; Kafka ยอมรับ at-least-once delivery

## 2. เป้าหมายที่ผู้ใช้ต้องมองเห็น

เมื่อจบ R11 ผู้ใช้ต้องสามารถทำ demo ต่อไปนี้ใน local ได้:

1. Login เป็น member แล้วดูสินค้าและ stock ที่ขายได้
2. สร้าง Order ผ่าน REST Gateway โดย Gateway เรียก Order Service ผ่าน gRPC
3. เห็น Order ตอบกลับทันทีเป็น `PENDING` โดยไม่รอ Inventory/Payment
4. เห็น Inventory และ Payment ทำงานต่อผ่าน Kafka จน Order เป็น `CONFIRMED`
5. เห็น Activity timeline และ in-app Notification เกิดแบบ eventual consistency
6. เห็นกรณี `OUT_OF_STOCK` และ `PAYMENT_DECLINED` พร้อม compensation ที่ถูกต้อง
7. เห็น Analytics จาก ClickHouse, operational metrics ใน Prometheus และ dashboard ใน Grafana
8. Restart consumer แล้วระบบตาม event ที่ค้างทันโดยไม่สร้างผลลัพธ์ซ้ำ

## 3. สิ่งที่ไม่ทำใน revision นี้

- payment gateway และบัตรเงินจริง
- cart, shipping, coupon, tax, multi-currency และคืนเงินจริง
- email, SMS, push notification, WebSocket หรือ SSE
- distributed transaction / exactly-once delivery
- Kubernetes, service mesh, distributed tracing และ schema registry
- multi-region และ autoscaling tuning
- Render deployment ก่อน local R0-R11 ผ่าน

## 4. Phase Map

| Phase | Outcome | สถานะเริ่มต้น |
|---|---|---|
| R0 | Lock baseline, branch และ evidence format | Passed — `codex/order-platform-revision` |
| R1 | Contracts, generated code และ additive schema | Passed — `06cce7a` |
| R2 | Product catalog + synchronous Order vertical slice | Passed — `06cce7a` |
| R3 | Inventory reservation ผ่าน Kafka | Passed — `0b8f635`, `b0b654c` |
| R4 | Payment simulation + Order saga/compensation | Passed — `ff890f9`, `89859ad`, `aae8de0` |
| R5 | Activity + in-app Notification projections | Passed — `7eb048a` |
| R6 | ClickHouse analytics projection และ query API | Passed — `b01ab0b`, `93a8b85` |
| R7 | Portfolio frontend ครบ success/failure flows | Passed — `94bdd30`, `264b1d9` |
| R8 | Prometheus, Grafana และ alerts | Passed — `2349ea4` |
| R9 | Security, failure recovery และ quality gates | Passed — `8ee2e2b` |
| R10 | Full local acceptance + migration cutover readiness | Passed — `1304c6b` |
| R11 | Public GitHub, CI และ pre-deploy tag | Passed — CI run `32694549379`, tag `v0.2.0-order-predeploy` |
| R12 | Render deployment | **HOLD — ไม่ทำจนกว่าจะสั่ง** |

## 5. Phase Details

### R0 — Baseline and Reversible Start

**Context**

Task version เดิมทำงานได้และเป็นหลักฐาน portfolio อยู่แล้ว การเริ่ม revision ต้องไม่ทำให้ baseline
ที่ tag ไว้สูญหาย และต้องรู้ว่ามี local runtime/data อะไรอยู่ก่อนเปลี่ยน

**Deliverables**

- ตรวจ `git status`, remote, branch, tag และ CI ของ `v0.1.0-predeploy`
- บันทึก baseline test/compose evidence โดยไม่แก้หรือลบ volume
- สร้าง branch `codex/order-platform-revision`
- กำหนด evidence directory/ledger สำหรับ R0-R11
- ยืนยันว่า docs 26-29 คือ source-of-truth ของ revision

**Focused verification**

- `go test ./...` สำหรับ baseline backend
- frontend lint/test/build ตาม command ที่ repo กำหนด
- `docker compose config` และ smoke test ของ Task version
- `git status --short` ต้องอธิบายได้ทุกไฟล์

**Acceptance**

- baseline ยัง checkout/run ได้
- ไม่มี destructive migration หรือ volume deletion
- มี evidence และ rollback point ชัดเจน

**Rollback**

- checkout `v0.1.0-predeploy`; local data เดิมยังไม่ถูกแก้แบบย้อนกลับไม่ได้

---

### R1 — Contracts and Additive Foundation

**Context**

ล็อกภาษากลางของระบบก่อนเขียน business logic เพื่อลด contract drift ระหว่าง Gateway,
services, Kafka, frontend และ tests

**Deliverables**

- Protobuf v1 สำหรับ Order, Inventory, Payment, Activity, Notification และ Analytics
- REST/OpenAPI contract ตาม [Detailed Design](27_ORDER_PLATFORM_DESIGN.md)
- Kafka envelope และ event catalog สำหรับ `order.events.v1` + DLQ
- generated Go stubs ที่ reproduce ได้จาก command เดียว
- additive migrations สำหรับ Order, Inventory, Payment, Activity และ Notification databases
- shared correlation/idempotency/error mapping conventions
- Compose service skeleton และ health/readiness checks โดยยังไม่ cut over UI เดิม

**จุดที่ต้อง comment ภาษาไทย**

- เหตุผลที่แยก transport DTO ออกจาก domain model
- invariant ของ event identity/correlation/causation
- boundary ที่ต้อง map gRPC status เป็น HTTP status
- เหตุผลที่ migration ระยะนี้ additive เท่านั้น

**Focused tests**

- proto generation cleanliness/contract compile
- migration up บนฐานว่างและฐานที่มี Task data
- OpenAPI route/schema validation
- unit tests ของ error mapping และ event envelope validation

**Acceptance**

- contracts compile และไม่มี hand-written generated code
- migration ไม่ drop/rename Task objects
- service skeleton start และ health checks แยก dependency failure ได้

**Evidence**

- command + result + commit SHA + รายการ generated/schema files

---

### R2 — Product Catalog and Synchronous Order Slice

**Context**

สร้างเส้นทาง synchronous ที่ผู้ใช้คาดหวัง: REST Gateway -> gRPC -> business transaction ->
PostgreSQL/outbox -> `PENDING` response โดยยังไม่แกล้งทำว่า Inventory/Payment สำเร็จ

**Deliverables**

- Inventory-owned product catalog แบบ read-only สำหรับ member และ seed data สำหรับ demo
- internal `QuoteProducts` gRPC ให้ Order ใช้ authoritative name/price/currency ก่อน transaction
- Order aggregate, order items, money as integer minor units และ state `PENDING`
- `POST /api/v1/orders`, list/get order และ product endpoints
- transaction เดียวสำหรับ order + items + outbox event
- required `Idempotency-Key` header ซึ่ง unique ต่อ customer สำหรับ create order
- outbox publisher ส่ง `OrderCreated` ไป Kafka
- RBAC: member เห็น order ของตน; admin เห็นทั้งหมด

**จุดที่ต้อง comment ภาษาไทย**

- เหตุผลที่ snapshot ชื่อ/ราคาไว้ใน order item
- transaction boundary ของ order + outbox
- เหตุผลที่ quote catalog ผ่าน gRPC แต่ reserve stock ผ่าน Kafka
- ownership check ป้องกัน member อ่าน order คนอื่น
- failure boundary เมื่อ Kafka ล่ม: order commit ได้และ outbox retry ภายหลัง

**Focused tests**

- order validation, total calculation และ legal initial state
- repository transaction rollback
- Inventory quote timeout/unavailable ต้องไม่สร้าง partial order
- duplicate/concurrent `Idempotency-Key` + payload เดิมคืน resource เดิม; retry ของ Order ที่มีแล้ว
  ไม่ต้อง quote ซ้ำ ส่วน key เดิมแต่ payload ต่างต้องคืน conflict
- authorization/ownership tests
- integration: create -> DB/outbox -> Kafka event

**Acceptance**

- HTTP create คืน `202 Accepted` หรือ contract ที่ล็อกไว้ พร้อม order `PENDING`
- retry request เดิมไม่สร้าง order ซ้ำ
- Kafka ล่มไม่ทำให้ committed order สูญหาย และ recovery publish ได้

---

### R3 — Inventory Reservation

**Context**

แสดงบทบาท async อย่างชัดเจน: Order Service ไม่เรียก reserve ผ่าน gRPC ใน request เดิม แต่
Inventory consumer ตัดสิน stock หลังรับ `OrderCreated`

**Deliverables**

- stock/on-hand/reserved model และ atomic reservation transaction
- consumer group `inventory-reservation-v1`
- `InventoryReserved` หรือ `InventoryRejected`
- processed-event ledger / unique constraint ป้องกัน reserve ซ้ำ
- admin REST/gRPC query สำหรับดู stock และ reservations
- deterministic seed สำหรับ success และ out-of-stock demo

**จุดที่ต้อง comment ภาษาไทย**

- atomic check-and-reserve invariant
- at-least-once/idempotency boundary
- commit DB ก่อน publish outcome ผ่าน outbox

**Focused tests**

- sufficient/insufficient stock และ concurrent reservation
- duplicate event ไม่ตัด stock ซ้ำ
- malformed event ไป DLQ พร้อม metric/log ที่ค้นได้
- integration: `OrderCreated` -> reservation -> outcome event

**Acceptance**

- stock ไม่ติดลบ
- event ซ้ำให้ผล business เดิม
- out-of-stock มีเหตุผลที่แสดงใน Order/UI ได้

---

### R4 — Payment and Saga/Compensation

**Context**

Payment เป็น simulator เพื่อสอน orchestration ผ่าน events ไม่เก็บข้อมูลบัตร และ Order Service
เป็นเจ้าของ final order state จากผล Inventory/Payment

**Deliverables**

- Payment consumer รับ `InventoryReserved`
- deterministic rule สำหรับ completed/declined โดย request ไม่รับ card data
- payment attempts และ processed-event ledger
- `PaymentCompleted` / `PaymentFailed`
- Order consumer เปลี่ยน `PENDING` -> `CONFIRMED` หรือ `CANCELLED`
- เมื่อ payment fail ให้ Order ส่ง `InventoryReleaseRequested`
- Inventory release แบบ idempotent และ `InventoryReleased`
- reason code ที่ปลอดภัยสำหรับ UI

**จุดที่ต้อง comment ภาษาไทย**

- legal state transitions และ terminal-state invariant
- เหตุผลที่ payment simulator ห้ามรับ/เก็บ card secret
- compensation retry semantics

**Focused tests**

- payment success/decline deterministic cases
- duplicate payment/outcome/release events
- illegal transition ไม่แก้ state
- integration success และ payment-declined compensation

**Acceptance**

- success จบ `CONFIRMED`
- stock reject หรือ payment decline จบ `CANCELLED`
- payment decline คืน reserved stock เพียงครั้งเดียว

---

### R5 — Activity and In-app Notification

**Context**

Activity ทำให้เห็น audit trail ส่วน Notification แปล domain event เป็นข้อความที่ผู้ใช้ต้องสนใจ
ทั้งสองเป็น eventual projections จึงต้องรับ event ซ้ำและตาม event ที่ค้างได้

**Deliverables**

- Activity consumer/projector และ order timeline query
- Notification consumer/projector
- inbox: list, unread count, mark one read, mark all read
- notification types สำหรับ order created/confirmed/cancelled
- frontend polling contract; ไม่เพิ่ม WebSocket/SSE
- ownership filter ทุก query/write

**จุดที่ต้อง comment ภาษาไทย**

- projection idempotency unique key
- eventual consistency ที่ UI ต้องสื่อ
- ownership boundary ของ mark-read

**Focused tests**

- event ซ้ำไม่สร้าง activity/notification ซ้ำ
- member ไม่เห็นหรือ mark notification ของคนอื่น
- consumer หยุด/เริ่มแล้ว catch up
- unread count ถูกต้องหลัง mark one/all

**Acceptance**

- timeline เล่า success/failure flow ได้ตามลำดับ business time
- notification ปรากฏหลัง event โดยไม่ block order request
- badge/read state ถูกต้องหลัง refresh

---

### R6 — ClickHouse Analytics

**Context**

Operational state อยู่ใน PostgreSQL ส่วน ClickHouse เก็บ append-only analytical projection
สำหรับ aggregate จำนวนมาก ไม่ใช้แทน transaction database

**Deliverables**

- `order_events` schema, migration และ retention decision
- analytics worker consume event catalog แบบ batch + idempotent insert strategy
- summary, funnel, time-series, status mix และ product ranking APIs
- filter ตาม event timestamp/range ที่ผู้ใช้เลือก
- ClickHouse datasource/dashboard provision สำหรับ Grafana

**จุดที่ต้อง comment ภาษาไทย**

- เหตุผลที่ analytical row เป็น append-only
- batching/flush failure boundary
- ความหมายของ eventual watermark ใน API/UI

**Focused tests**

- event-to-row mapping และ timestamp/timezone
- retry/batch failure ไม่ทำให้ aggregate business count เพี้ยน
- query range/funnel/ranking correctness
- integration: Kafka -> worker -> ClickHouse -> gRPC/REST

**Acceptance**

- analytics ตรงกับชุด event fixture ที่ทราบคำตอบ
- UI แสดง `through/watermark` ไม่อ้างว่า real-time แบบ strong consistency
- ClickHouse ล่มไม่กระทบ Order transaction

---

### R7 — Portfolio Frontend

**Context**

Frontend ต้องทำให้ผู้สัมภาษณ์เห็น sync/async flow ได้โดยไม่ต้องเปิด source ก่อน และยังคง
ใช้งานผ่าน Gateway จุดเดียว

**Deliverables**

- routes: login, products, create order, my orders, order detail/timeline
- notification bell/inbox และ polling lifecycle
- admin routes: orders, inventory, payment attempts, analytics, system
- pending/confirmed/cancelled badges และ reason display
- explicit loading/empty/error/eventual states
- responsive, accessible portfolio UI และ demo guide

**Focused tests**

- component/state tests ของ order transitions และ notification badge
- API client auth/refresh/error handling
- role route guards
- browser E2E สำหรับ success, out-of-stock และ payment-declined

**Acceptance**

- demo ทั้งสาม scenario ทำผ่าน browser ได้
- refresh หน้าแล้ว state มาจาก backend ไม่ใช่ hard-coded
- member/admin เห็นเมนูและข้อมูลตาม role

---

### R8 — Prometheus, Grafana and Alerts

**Context**

Prometheus เก็บ operational metrics; Grafana แสดงและ alert จาก metrics/ClickHouse โดยไม่เอา
Grafana ไปแทน database หรือ Prometheus

**Deliverables**

- RED metrics ของ Gateway/gRPC และ dependency health
- Kafka publish/consume/failure/DLQ/consumer lag/projection watermark metrics
- business counters: created/confirmed/cancelled, inventory rejected, payment failed
- dashboards: Platform Overview, Event Pipeline, Business Analytics (ClickHouse)
- alerts สำหรับ service down, error rate, DLQ, lag/stalled projection
- แก้ semantic ของ alert: ไม่มี event ใหม่ไม่เท่ากับ consumer เสีย ต้องใช้ lag/progress เมื่อมี backlog

**จุดที่ต้อง comment ภาษาไทย**

- metric cardinality invariant (ห้าม label ด้วย order/user/event ID)
- alert expression และ false-positive boundary

**Focused tests**

- `/metrics` exposition และ expected labels
- Prometheus rule tests/config validation
- Grafana provisioning/datasource/dashboard validation
- fault test: consumer down + backlog แล้ว alert fire; idle queue ไม่ fire

**Acceptance**

- Prometheus targets healthy
- dashboards เปิดแล้วมีข้อมูลหลัง demo
- alerts แยก idle system ออกจาก stalled consumer ได้

---

### R9 — Security, Recovery and Quality Gates

**Context**

รวม cross-cutting risks ก่อน full acceptance เพื่อไม่ให้ demo ผ่านเฉพาะ happy path

**Deliverables**

- JWT login/refresh/logout และ admin/member authorization ครบ route
- rate/size/timeouts และ safe error messages ที่ public edge
- secrets ผ่าน environment; ไม่มี secret/card data ใน repo/log/event
- graceful shutdown และ Kafka rebalance handling
- DLQ inspection/replay แบบมี runbook และ idempotency guard
- dependency/security scan ตาม CI ที่ repo รองรับ

**Focused tests**

- auth matrix, expired/invalid token และ object ownership
- request timeout/cancellation propagation REST -> gRPC
- race/unit/integration suites ตาม package ที่แก้
- kill/restart Inventory, Payment, Notification และ Analytics consumers
- malformed/duplicate/out-of-order fixtures

**Acceptance**

- ไม่มี cross-user data access
- retry/restart ไม่สร้างผล business ซ้ำ
- failure ถูกเปิดเผยผ่าน log/metric/DLQ และมีวิธีกู้คืน

---

### R10 — Full Local Acceptance and Cutover Readiness

**Context**

พิสูจน์ระบบทั้งชุดบนเครื่องเดียวก่อนแตะ Render และตัดสินว่าพร้อมเปลี่ยน portfolio default จาก
Task ไป Order หรือยัง

**Deliverables**

- Compose topology ครบทุก app/infra container และมี named volumes ที่จำเป็น
- clean startup, health ordering และ seeded demo accounts/data
- local start/stop/reset-data runbooks แยกชัดเจน
- architecture/use-case/port/endpoint/interview docs อัปเดตตามของจริง
- migration compatibility report และรายการ Task components ที่ยังคงอยู่
- cutover checklist; contraction/removal เป็นงานแยกที่ต้อง review

**Full acceptance scenarios**

1. Happy path: create -> reserve -> pay -> confirm -> activity -> notification -> analytics
2. Out of stock: create -> reject -> cancel -> notification/analytics
3. Payment decline: reserve -> decline -> cancel -> release -> notification/analytics
4. Consumer outage: create backlog -> restart -> catch up exactly once ในเชิง business effect
5. Restart Compose services โดย named data ยังอยู่ตาม policy
6. Admin/member RBAC และ object ownership
7. Prometheus/Grafana/ClickHouse views สอดคล้องกับ scenario fixture

**Acceptance**

- test matrix ทุก ID ในหัวข้อ 6 ผ่าน
- `docker compose config` และ health checks ผ่าน
- ไม่มี uncommitted runtime fix
- Task baseline tag ยังใช้งานได้
- ยังไม่ deploy Render

---

### R11 — GitHub, CI and Pre-deploy Tag

**Context**

เตรียม artifact ที่ตรวจซ้ำได้และหยุดก่อนมีค่าใช้จ่าย/ผลกระทบภายนอกจาก Render

**Deliverables**

- push revision ไป public GitHub repository ที่ผู้ใช้กำหนด
- CI ทุก required job เป็น green บน commit เดียวกับ local evidence
- README/demo screenshots/architecture diagrams และ interview notes ตาม runtime จริง
- tag `v0.2.0-order-predeploy` หลัง CI ผ่าน
- pre-deploy manifest: commit/tag/images/env inventory/migration order/rollback

**Acceptance**

- remote clean, public visibility ตรวจแล้ว, tag ชี้ commit ที่ CI green
- secret scan ไม่พบ credential
- local R10 evidence เชื่อมโยงกับ commit/tag
- หยุดและรายงานก่อน R12

---

### R12 — Render Deployment

**สถานะ: HOLD**

เริ่มเมื่อผู้ใช้สั่งหลัง R11 เท่านั้น ต้องทบทวน Render topology, managed services, cost,
secret/env, private networking, persistence, migration job, health checks และ rollback อีกครั้ง
ตามสถานะบริการ/ราคา ณ วัน deploy

## 6. Test and Evidence Matrix

| ID | Scope | ตัวอย่างหลักฐาน |
|---|---|---|
| OR-CONTRACT | Protobuf/OpenAPI/event compatibility | generation diff + contract tests |
| OR-API | REST/gRPC behavior | handler/client integration tests |
| OR-STATE | legal order/payment/inventory transitions | table-driven unit tests |
| OR-IDEMP | request/event retry safety | duplicate fixtures + DB assertions |
| OR-INVENTORY | atomic reserve/release | concurrency/integration tests |
| OR-PAYMENT | success/decline/compensation | deterministic saga tests |
| OR-NOTIFY | activity/inbox ownership/read state | projector/API/E2E tests |
| OR-ANALYTICS | ClickHouse mapping/query correctness | known fixture aggregates |
| OR-OBS | metrics/dashboards/alerts | rule/config/fault tests |
| OR-SEC | authentication, RBAC, ownership, secrets | auth matrix + scans |
| OR-RECOVERY | restart/backlog/DLQ replay | fault-injection evidence |
| OR-E2E | browser success/failure journeys | screenshots/video/test report |
| OR-MIG | forward/rollback compatibility | baseline + additive migration report |

Evidence ต่อ Phase ต้องบันทึกอย่างน้อย:

```text
Phase:
Commit SHA:
Date/time (Asia/Bangkok):
Commands:
Result:
Scenarios/fixtures:
Known limitations:
Rollback point:
Git status:
```

## 7. Commit and Change Policy

- หนึ่ง Phase อาจมีหลาย commit แต่ commit ต้องมีขอบเขตและทดสอบได้
- ไม่ squash หลักฐานที่จำเป็นก่อน review; ห้าม force-push โดยไม่ได้รับคำสั่ง
- schema/event contract breaking change ต้องมี version/compatibility plan
- ห้ามแก้ generated file ด้วยมือ
- ห้ามลบ Task tables/topics/source/volumes ใน R0-R11 โดยอัตโนมัติ
- หากจำเป็นต้อง contract ของเดิม ให้เสนอรายการ target + backup + rollback และรออนุมัติ

## 8. Definition of Ready for Implementation

พร้อมเริ่ม R0 เมื่อ:

- ผู้ใช้ยอมรับ docs 26-29
- worktree ไม่มีการเปลี่ยน runtime ที่ไม่ทราบที่มา
- baseline tag/remote เข้าถึงได้
- Docker และ disk มีพื้นที่พอสำหรับ stack โดยไม่ลบข้อมูลอื่นเอง

## 9. Definition of Complete for Revision

Revision ถือว่า complete เฉพาะเมื่อ R0-R11 ผ่านตาม evidence และ tag
`v0.2.0-order-predeploy` ถูกสร้างแล้ว จากนั้นต้อง **หยุดก่อน R12**
