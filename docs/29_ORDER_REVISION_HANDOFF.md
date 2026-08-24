# Order Platform Revision — Context, Resume and Luna Handoff

> Updated: 2026-08-24 (Asia/Bangkok)
>
> สถานะ revision: **R0–R11 passed locally/CI; tag `v0.2.1-order-predeploy` created; R12 Render held**
>
> Runtime baseline: Task/Activity/Analytics บน `main`, tag `v0.1.0-predeploy`; revision runtime
> อยู่บน branch `codex/order-platform-revision`; local Compose/browser acceptance และ CI ผ่านแล้วที่
> commit `dbb326c`; tag `v0.2.1-order-predeploy` สร้างแล้ว

เอกสารนี้เป็นจุดกลับมาทำงานต่อเมื่อ context/token หมด และเป็น handoff prompt สำหรับ agent ที่จะ
implement โดยต้องอ่าน docs 26-29 ทั้งชุดก่อนแก้ runtime

## 1. Current Checkpoint

### สิ่งที่เสร็จแล้ว

- Task version เดิม implement/test/push/tag แล้วตาม evidence เดิม
- กำหนด business revision ใหม่เป็น Order Processing Platform
- สรุป sync gRPC flow, async Kafka flow, CQRS/cache, ClickHouse และ observability แล้ว
- กำหนด in-app Notification แบบ persisted inbox + polling
- กำหนด phased plan, rollback policy, acceptance/evidence matrix แล้ว
- Order-first frontend, admin projections, refresh-cookie boundary และ browser E2E ถูก implement ใน
  draft และผ่าน local/CI R0–R11 acceptance ตาม docs 30

### สิ่งที่ยังไม่ทำ / ต้องปิดก่อน release

- R9/R10 quality และ full acceptance ผ่านที่ commit `dbb326c`; CI run `32701188316` ผ่านครบ
- R11 tag `v0.2.1-order-predeploy` สร้างแล้ว โดยไม่ overwrite `v0.2.0-order-predeploy` เดิม
- ยังไม่ deploy Render

ดังนั้นห้ามใช้ภาพหรือผลทดสอบ Task version เป็นหลักฐานว่า Order revision ผ่าน

## 2. Source of Truth

อ่านตามลำดับ:

1. [26_ORDER_PLATFORM_OVERVIEW.md](26_ORDER_PLATFORM_OVERVIEW.md) — ภาพรวมและ use cases
2. [27_ORDER_PLATFORM_DESIGN.md](27_ORDER_PLATFORM_DESIGN.md) — ownership/contracts/invariants
3. [28_ORDER_IMPLEMENTATION_PLAN.md](28_ORDER_IMPLEMENTATION_PLAN.md) — Phase R0-R12 และ acceptance
4. เอกสารนี้ — สถานะ, exact resume point และ handoff rules

เมื่อขัดกับเอกสาร Task รุ่นเดิม ให้ใช้ docs 26-29 สำหรับ **Order revision เท่านั้น** แต่ใช้ evidence
เดิมเพื่อยืนยัน baseline/rollback ห้าม rewrite ประวัติว่า Order ถูก implement แล้ว

## 3. Locked Decisions

| เรื่อง | Decision |
|---|---|
| Public edge | REST/Swagger ผ่าน API Gateway |
| Internal sync | gRPC/Protobuf พร้อม deadline/cancellation |
| Async workflow | Kafka choreography, at-least-once + idempotent consumers |
| Transaction safety | local DB transaction + transactional outbox |
| Domain owner | Order, Inventory, Payment แยกเจ้าของ state/database |
| Product catalog | Inventory Service เป็น owner ใน revision นี้ |
| Price snapshot | Order เรียก Inventory `QuoteProducts` ผ่าน gRPC ก่อน transaction; client กำหนดราคาเองไม่ได้ |
| Payment | deterministic simulator; ไม่รับ/เก็บ card data |
| Notifications | persisted in-app inbox, frontend polling |
| Operational DB | PostgreSQL แยก logical database/schema ตาม service |
| Cache | Redis สำหรับ cache/session/rate-control ตาม boundary ไม่ใช่ source of truth |
| Analytics | ClickHouse append-only projection จาก Kafka |
| Metrics/UI | Prometheus scrape metrics, Grafana visualize/alert |
| Money | integer minor units + currency code |
| Topic | `order.events.v1` และ DLQ; event type อยู่ใน envelope |
| Deploy | หยุดหลัง R11; Render R12 ต้องได้รับคำสั่งใหม่ |

## 4. Business Flow to Preserve

```text
Browser
  -> REST Gateway
     -> gRPC Order Service
        -> gRPC Inventory QuoteProducts (catalog/pricing only; no reservation)
        -> PostgreSQL: order + items + outbox (transaction)
        <- PENDING
     <- HTTP response

Outbox -> Kafka: OrderCreated
  -> Inventory Service
     -> InventoryReserved | InventoryRejected
        -> Payment Service (เฉพาะ reserved)
           -> PaymentCompleted | PaymentFailed
        -> Order Service
           -> CONFIRMED | CANCELLED
           -> InventoryReleaseRequested เมื่อ payment fail

ทุก domain event
  -> Activity PostgreSQL
  -> Notification PostgreSQL
  -> Analytics Worker -> ClickHouse
  -> Prometheus metrics -> Grafana
```

Critical invariants:

- synchronous response ไม่รอ Inventory/Payment
- Create Order เชื่อ authoritative catalog quote; ถ้า quote ล้มเหลวต้องไม่มี partial order
- stock ไม่ติดลบและ duplicate event ไม่ reserve/release ซ้ำ
- terminal Order state ไม่ย้อนกลับ
- payment decline ต้อง release stock แบบ idempotent
- member อ่าน order/notification ของคนอื่นไม่ได้
- analytics/notification failure ไม่ rollback order transaction

## 5. Phase Status Ledger

| Phase | Status | Evidence | Next action |
|---|---|---|---|
| R0 Baseline | Passed | branch `codex/order-platform-revision` | baseline/tag preserved |
| R1 Contracts | Passed | `06cce7a` | generated contracts and additive schema |
| R2 Order slice | Passed | `06cce7a` | quote -> transaction/outbox -> PENDING |
| R3 Inventory | Passed | `0b8f635`, `b0b654c` | idempotent reservation and Kafka wiring |
| R4 Payment/Saga | Passed | `ff890f9`, `89859ad`, `aae8de0` | decline compensation/retry |
| R5 Activity/Notification | Passed | `7eb048a` | persisted projections and polling |
| R6 Analytics | Passed | `b01ab0b`, `93a8b85` | ClickHouse projection/query |
| R7 Frontend | Passed locally/CI | Playwright 3/3 | complete |
| R8 Observability | Passed locally/CI | Compose Grafana/Prometheus checks | complete |
| R9 Quality/Recovery | Passed locally/CI | `make test/vet/build`, compose config, frontend gates | complete at `dbb326c` |
| R10 Local acceptance | Passed locally/CI | Compose acceptance exit 0; `task_events=13`, `order_events=79` | complete at `dbb326c` |
| R11 GitHub/tag | Passed | final release commit `dbb326c`, CI run `32701188316`; existing tags preserved | stop before R12 |
| R12 Render | HOLD | none | ต้องมีคำสั่งใหม่หลัง R11 |

อัปเดตตารางนี้ทุกครั้งที่จบ Phase พร้อม commit SHA และลิงก์/ตำแหน่ง evidence

## 6. Exact Resume Point

รอบ implement ถัดไปให้เริ่มตรงนี้ตามลำดับ:

1. ตรวจ `git status --short --branch` และ review uncommitted draft ทุกไฟล์
2. รักษา `main`, `v0.1.0-predeploy` และ existing `v0.2.0-order-predeploy` โดยห้าม reset/retag
3. รัน R9 quality/recovery checks หลังแก้ refresh-cookie boundary
4. อัปเดต evidence ให้ผูกกับ commit SHA เดียว
5. commit/push, ตรวจ CI และสร้าง tag ใหม่ที่ไม่ทับ tag เดิม
6. หยุดก่อน Render R12

ถ้า R0 พบ baseline test fail ให้ diagnose และแยกให้ได้ว่าเป็น pre-existing หรือเกิดจาก revision
ก่อนแก้ ห้ามข้ามไป R1 ด้วย assumption

## 7. Expected Runtime Topology

ชื่อจริงปรับได้เมื่อ R1 แต่ responsibility ต้องไม่รวมกลับเป็น monolith:

```text
frontend
gateway
identity-service
order-service
inventory-service
payment-service
activity-service
notification-service
analytics-service / analytics-worker

postgres-order
postgres-inventory
postgres-payment
postgres-activity
postgres-notification
redis
kafka
clickhouse
prometheus
grafana
```

การใช้ PostgreSQL instance เดียวแต่แยก database/user ใน local ทำได้เพื่อประหยัด resource แต่ code,
migration และ ownership ต้องไม่ query ข้าม service database โดยตรง

## 8. Thai Comment Standard

เพิ่ม comment ภาษาไทยเฉพาะส่วนที่คนอ่านอนุมานจาก syntax ไม่ได้:

- business invariant และ legal state transition
- transaction/outbox boundary
- idempotency และ retry behavior
- ownership/security check
- deadline/cancellation/failure fallback
- eventual consistency/watermark
- metric cardinality และ alert semantics

ไม่ comment getter, struct field, loop หรือ statement ที่ชื่อสื่อความหมายอยู่แล้ว และไม่ใส่คำอธิบาย
ยาวจน source อ่านยาก; รายละเอียดภาพรวมให้อยู่ใน docs

## 9. Evidence Update Template

เมื่อจบแต่ละ Phase ให้เพิ่มบันทึก:

```text
Phase: R?
Status: Passed | Blocked
Commit SHA:
Started/finished (Asia/Bangkok):
Files/components changed:
Commands and results:
Manual scenario evidence:
Known limitations:
Rollback point:
Git status:
```

หาก blocked ต้องระบุ reproduced command, observed output, expected output และ safe next step
ไม่แก้แบบสุ่มหรือวน restart โดยไม่มี hypothesis

## 10. Handoff Prompt for Luna High

```text
Implement the Order Platform revision in golang-essential-3.

Read AGENTS.md and docs/26_ORDER_PLATFORM_OVERVIEW.md through
docs/29_ORDER_REVISION_HANDOFF.md completely. Start exactly at section 6
"Exact Resume Point" in docs/29_ORDER_REVISION_HANDOFF.md.

Preserve main and tag v0.1.0-predeploy as the Task-version rollback baseline.
Do R0-R11 in order. Use additive/compatible migrations, local transactions +
transactional outbox, at-least-once Kafka with idempotent consumers, gRPC for
internal synchronous calls, REST only at the Gateway, PostgreSQL per service
ownership, Redis cache, ClickHouse analytics, Prometheus and Grafana.

Implement focused tests and record evidence at every Phase. Add Thai comments
only at decision, invariant, ownership, retry/idempotency and failure boundaries.
Do not claim a Phase passed without evidence. Do not discard pre-existing or
uncommitted work, do not delete Task data/volumes automatically, and do not
leave uncommitted runtime fixes.

After R10 full local acceptance passes, push to the public GitHub repository,
verify CI, create tag v0.2.1-order-predeploy without overwriting existing v0.2.0-order-predeploy,
update all status/evidence docs,
then stop before R12 Render deployment and report the exact deploy handoff.
```

## 11. Stop Conditions

หยุดและรายงานก่อนทำต่อเมื่อ:

- ต้อง drop/rename/delete data, schema, topic, source หรือ volume เดิม
- contract decision ใหม่ขัดกับ locked decisions และเปลี่ยน scope อย่างมีนัยสำคัญ
- ต้องใช้ credential/บริการเสียเงินที่ผู้ใช้ยังไม่ได้อนุญาต
- baseline rollback point ใช้ไม่ได้
- R11 ผ่านแล้วและขั้นถัดไปคือ Render R12
