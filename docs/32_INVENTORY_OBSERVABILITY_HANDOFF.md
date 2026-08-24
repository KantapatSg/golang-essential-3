# Inventory and Observability Revision — Plan, Resume and Luna High Handoff

> Updated: 2026-08-24 (Asia/Bangkok)
>
> Status: **P0–P8 implementation complete; release evidence and CI handoff in progress**
>
> Release baseline: `v0.2.1-order-predeploy` at verified Order runtime commit `dbb326c`
>
> Current post-release implementation branch: `codex/inventory-observability-revision`; preserve
> all existing tags and history. Machine-readable phase evidence is in
> `docs/33_INVENTORY_OBSERVABILITY_EVIDENCE.md`.

เอกสารนี้เป็นจุดกลับมาทำงานต่อเมื่อ context/token หมด และเป็น source of truth สำหรับ Luna High
ที่จะ implement Inventory, Redis visibility, ClickHouse correctness และ transaction observability

## 1. Source of Truth และ Reading Order

ก่อนแก้ runtime ต้องอ่านครบตามลำดับ:

1. `AGENTS.md`
2. `docs/26_ORDER_PLATFORM_OVERVIEW.md`
3. `docs/27_ORDER_PLATFORM_DESIGN.md`
4. `docs/28_ORDER_IMPLEMENTATION_PLAN.md`
5. `docs/29_ORDER_REVISION_HANDOFF.md`
6. `docs/30_ORDER_REVISION_EVIDENCE.md`
7. `docs/31_INVENTORY_OBSERVABILITY_REVISION.md`
8. เอกสารนี้

Docs 26–30 อธิบาย verified Order baseline และ historical evidence ส่วน docs 31–32 เป็น target ใหม่
เมื่อมี conflict สำหรับ P0–P8 ให้ docs 31–32 ชนะเฉพาะ Inventory/terminal events/cache/analytics/
observability revision โดยห้าม rewrite ว่า baseline เก่ามีพฤติกรรมใหม่แล้ว

## 2. Current Checkpoint

### Verified baseline ที่ต้องรักษา

- Public GitHub: `https://github.com/KantapatSg/golang-essential-3`
- Current working branch ก่อน design draft: `codex/order-platform-revision`
- Pre-design HEAD: `da86f7b` (`docs(order): set post-release resume point`)
- Verified Order tag: `v0.2.1-order-predeploy` -> runtime commit `dbb326c`
- Preserve tags: `v0.1.0-predeploy`, `v0.2.0-order-predeploy`, `v0.2.1-order-predeploy`
- Render deployment ยังคง HOLD

### Runtime audit ที่ทำให้เกิด revision นี้

- ClickHouse V1 มี historical synthetic Order events จริงและสะสมข้าม test run
- Live audit: created 32, reserved 23, payment completed 15, payment failed 8, inventory rejected 8
- Operational PostgreSQL ตอน audit มี orders 4 รายการ จึงยืนยัน lifecycle mismatch
- ClickHouse payload รวม completed amount ได้ 28,683 cents แต่ UI revenue แสดงศูนย์
- Summary ใช้ Payment outcome แทน terminal Order outcome; funnel รอ terminal events ที่ไม่ถูก publish
- Grafana Business Analytics ยัง query `task_events`
- Prometheus scrape targets healthy แต่ไม่มี Order/Payment/Inventory/Redis transaction metrics
- Redis live keyspace มี Identity sessions; Order/Inventory catalog cache ยังไม่มี
- Inventory stock/reservation อยู่ใน memory และไม่มี Add Stock persistence

ผล audit เป็น baseline diagnosis ก่อน implementation; phase evidence อยู่ใน docs 33
และไม่ถูกใช้แทน reconciliation ของ P0–P8

## 3. Locked Decisions สำหรับ P0–P8

| เรื่อง | Decision |
|---|---|
| Revision branch | สร้าง/ใช้ `codex/inventory-observability-revision` จาก clean current HEAD |
| Target tag | `v0.3.0-inventory-observability-predeploy` หลัง local + CI ผ่านเท่านั้น |
| Source of truth | PostgreSQL ต่อ bounded context |
| Inventory storage | Durable `inventory_db`; memory maps ใช้ได้เฉพาะ focused unit fixture |
| Cache | Redis cache-aside สำหรับ catalog; fail open ไป DB |
| Session | Identity Redis session fail closed เหมือนเดิม |
| Terminal success | `OrderConfirmed` เท่านั้น |
| Terminal decline | `OrderCancelled` หลัง `InventoryReleased` เท่านั้น |
| Terminal stock failure | `OrderRejected` |
| Compensation state | `CANCELLING` ระหว่าง PaymentFailed และ InventoryReleased |
| Stock success closeout | Inventory consume reservation เมื่อรับ `OrderConfirmed` และ emit `InventoryConsumed` |
| Analytics | ClickHouse V2 typed projection; V1 history preserved |
| Revenue | Sum confirmed `amount_minor`, ไม่ infer จาก Payment summary |
| Prometheus | Aggregate metrics; ห้าม identifier labels |
| Grafana | 6 provisioned dashboards จาก Prometheus/ClickHouse V2 (Business Analytics และ V2 correctness แยกมุมมอง) |
| Deploy | Stop after P8 pre-deploy tag; Render ต้องมีคำสั่งใหม่ |

## 4. Exact Resume Point

Luna High ต้องเริ่มตามลำดับนี้ ห้ามเริ่มเขียน feature ก่อน audit:

1. ตรวจ `git status --short --branch`, `git diff --stat`, remote และ tags
2. Review design-doc diff ปัจจุบัน ห้าม discard/reset/overwrite docs 31–32
3. ยืนยัน `v0.2.1-order-predeploy` ยังชี้ verified Order runtime และ tags เดิมไม่ถูกเปลี่ยน
4. สร้าง branch `codex/inventory-observability-revision` เฉพาะเมื่อยังไม่มี และไม่ force/reset branch เดิม
5. รัน read-only baseline checks: focused Go/frontend tests, Compose config, live schema/metrics เมื่อ stack พร้อม
6. บันทึก baseline discrepancy โดยไม่ลบ/reset Docker volumes
7. เริ่ม P0 และทำ P0–P8 ตามลำดับ; ห้ามข้าม acceptance gate

ถ้า worktree มี runtime change นอกจาก design docs ให้ audit/จัดประเภทก่อน ห้าม discard และห้ามรวมแบบ
ไม่เข้าใจ ownership

## 5. Phase Map

| Phase | Outcome | Current status |
|---|---|---|
| P0 | Lock contracts, baseline และ reconciliation fixtures | Passed |
| P1 | Canonical terminal Order events และ compensation state | Passed |
| P2 | Durable Inventory + Add/Adjust Stock + ledger | Passed |
| P3 | Redis catalog cache และ visible hit/miss/fallback | Passed |
| P4 | ClickHouse V2 correctness, typed projection และ revenue | Passed |
| P5 | Prometheus transaction/cache/gRPC metrics | Passed |
| P6 | Grafana dashboards และ frontend portfolio UX | Passed |
| P7 | Focused/full local/browser/failure acceptance | Passed |
| P8 | Docs/evidence, commit/push/CI/pre-deploy tag | In progress — waiting for CI/tag |
| Render | Deployment | **HOLD** |

## 6. Phase Details

### P0 — Baseline, Contracts and Fixture Reconciliation

**Context**

ล็อกความหมาย business ก่อน migration เพื่อไม่แก้ตัวเลขเพียงหน้า UI และไม่เอา historical
Payment events มาปลอมเป็น terminal Order events

**Deliverables**

- Baseline status/branch/tag/volume/schema/metric inventory
- Canonical state/event catalog ตาม docs 31
- Additive Protobuf/OpenAPI/event schema change plan
- Unique acceptance `run_id` และ known fixtures สำหรับ success/decline/out-of-stock
- PostgreSQL named-volume/lifecycle plan ที่ไม่ลบข้อมูลเดิม
- Evidence ledger ใหม่ `docs/33_INVENTORY_OBSERVABILITY_EVIDENCE.md`

**Focused tests/checks**

- Contract compilation และ generated-code cleanliness
- Existing Order focused testsเป็น baseline
- ClickHouse V1 grouped counts เทียบ PostgreSQL operational counts
- Grafana JSON scan ต้องบันทึก dashboard ที่ยัง query Task
- Redis keyspace/Prometheus series ตรวจแบบไม่แสดง secrets

**Acceptance**

- ทุก discrepancy reproduced ด้วยคำสั่ง read-only
- Contract decisions ไม่มี ambiguity และ migration เป็น additive
- ไม่มี runtime/data/volume ถูกลบ

---

### P1 — Canonical Order Terminal Events

**Context**

แยก Payment outcome ออกจาก final Order outcome และไม่ประกาศ cancellation ก่อน compensation เสร็จ

**Deliverables**

- State `CANCELLING`
- `OrderConfirmed`, `OrderRejected`, `OrderCancelled` และ `InventoryConsumed` contracts
- Payment success: state + processed event + `OrderConfirmed` outbox ใน transaction เดียว
- Stock reject: state + processed event + `OrderRejected` outbox ใน transaction เดียว
- Payment fail: `CANCELLING` + `InventoryReleaseRequested`; หลัง `InventoryReleased` จึง `CANCELLED` + `OrderCancelled`
- Inventory consume reservation แบบ idempotent หลัง `OrderConfirmed`
- Activity/Notification mappings สำหรับ canonical terminal events

**Thai comments**

- legal transition/terminal invariant
- compensation ownership และเหตุผลที่ยังไม่ `CANCELLED`
- processed event + state + outbox transaction boundary
- retry/DLQ failure boundary

**Focused tests**

- Table-driven state transitions
- Duplicate/out-of-order Payment/Inventory outcomes
- Atomic terminal outbox rollback
- Compensation retry และ duplicate release
- Activity/Notification canonical mapping

**Acceptance**

- แต่ละ Order มี canonical terminal event ไม่เกินหนึ่งผลลัพธ์
- Success/decline/out-of-stock timeline ตรง docs 31
- Payment event ไม่ถูกนับเป็น Order terminal event โดยตรง

---

### P2 — Durable Inventory and Stock Operations

**Context**

ย้าย product/stock/reservation จาก memory ไป Inventory-owned PostgreSQL และเพิ่ม admin use case ที่
ผู้ใช้มองเห็น ownership, transaction และ audit ledger

**Deliverables**

- Additive inventory migrations: products, balances, movements, reservations, processed events, outbox
- Deterministic seed ที่ idempotent และไม่ overwrite operator adjustment
- gRPC `ListInventory`, `AdjustStock`, `ListStockMovements`
- Admin REST inventory/adjustments/movements
- Admin-only authorization ใน Gateway และ service metadata boundary
- Idempotency-Key + request hash สำหรับ Adjust Stock
- Atomic reserve/release/consume/adjust พร้อม version/row-lock invariant
- `StockAdjusted`, optional `LowStockDetected`, reservation/consume events ผ่าน outbox

**Focused tests**

- Migration บนฐานว่างและฐานที่มี Order baseline
- Positive add, signed correction, invalid negative result
- Same/different-payload idempotency retry
- Concurrent reserve/adjust; stockห้ามติดลบ
- Restart persistence และ seedไม่ทับข้อมูล
- Outbox failure rollback

**Acceptance**

- Add Stock แสดง balance/ledger ถูกต้องและ persist หลัง restart
- Reserve/release/consume reconcile: `on_hand = available + reserved`
- Duplicate command/event เปลี่ยน stockเพียงครั้งเดียว

---

### P3 — Redis Catalog Cache

**Context**

ทำ use case Redis ให้เห็นจริง โดยยังรักษา PostgreSQL เป็น stock truth และแยก failure policy จาก
Identity session

**Deliverables**

- Inventory Redis client/config/readiness semantics
- Cache-aside list/product projections พร้อม TTL/schema version
- Invalidation หลัง committed stock/product/reservation mutation
- DB fallback เมื่อ Redis timeout/error
- gRPC metadata -> Gateway `X-Cache-Status`
- CORS exposed header และ frontend cache source badge
- Metrics hit/miss/bypass/error/invalidation/duration

**Thai comments**

- cache ไม่ใช่ source of truth
- invalidation-after-commit ownership
- catalog fail-open เทียบ Identity fail-closed
- stale TTL/failure boundary

**Focused tests**

- MISS -> DB -> SET -> HIT
- Add/reserve/release/consume -> invalidate -> MISS พร้อมค่าใหม่
- Redis down -> DB BYPASS
- Identity fail-closed regression
- TTL/config และ no-secret key/value checks

**Acceptance**

- Browser แสดง MISS, HIT และ BYPASS ได้ตาม scenario
- Redis failure ไม่ทำให้ catalog/order core readล้ม
- Redis stale dataไม่ถูกใช้ตัดสิน atomic reservation

---

### P4 — ClickHouse Analytics V2

**Context**

แก้ business semantic และ schema/query pattern โดยไม่ลบ V1 history และไม่ fabricate terminal events

**Deliverables**

- `analytics.order_events_v2` typed schema/migration ตาม docs 31
- Worker V2 mapping สำหรับ canonical events, environment/run metadata
- P4 execution clarification: monthly `toYYYYMM(event_date)` partitions are retained for the
  explicit 180-day TTL lifecycle while preserving the existing V1 table and named volume.
- Low-volume async insert with acknowledgement หรือ bounded batch flush
- Offset commit หลัง insert success
- Summary/funnel/timeseries/ranking query V2 พร้อม bounded range
- Confirmed revenue จาก `OrderConfirmed.amount_minor`
- `through`/watermark และ run filter
- Explicit V1 legacy behavior/cutover; no automatic delete/backfill fabrication

**ClickHouse review requirements**

- Discover database/table/columns/engine/parts ก่อน migration
- ล็อก ORDER BY จาก real query patterns เพราะแก้ภายหลังไม่ได้
- ใช้ native typed columns; known fieldsห้าม queryจาก JSON
- ตรวจ `EXPLAIN indexes = 1`
- Row-returning query มี LIMIT และ time bound
- Tiny local inserts ใช้ async insert + `wait_for_async_insert=1` หรือ batch

**Focused tests**

- Mapping ของ canonical event ทุกชนิด
- Replay duplicate event และ query dedup
- Known fixture: status/funnel/revenue/product ranking
- Timezone/range/run filter
- Worker insert failure ไม่ commit offset
- ClickHouse down ไม่กระทบ Order transaction

**Acceptance**

- Fixture counts/revenue ตรง PostgreSQL terminal outcomes
- `PaymentCompleted` ไม่ถูกนับเป็น confirmed terminal โดยตรง
- V1 rowsยังอยู่และ V2 UIระบุ source/watermark

---

### P5 — Prometheus Transaction and Cache Metrics

**Context**

เพิ่ม instrumentation ที่ทำให้เห็น live operation โดยไม่พยายามใช้ Prometheus เป็น transaction DB

**Deliverables**

- HTTP/gRPC RED metrics พร้อม bounded route/method/code labels
- Order create/state/saga metrics
- Inventory reserve/adjust/low-stock metrics
- Payment result metrics
- Redis cache metrics
- Kafka/outbox/retry/DLQ/lag/analytics latency metricsที่ service ownerอัปเดตจริง
- Alert rules แยก idle system จาก stalled/backlogged system

**Focused tests**

- `/metrics` contains expected series after known fixture
- Counter increments once under duplicate delivery
- Histogram buckets/units correct
- Static check/review ห้าม identifier/error text labels
- Prometheus config/rule validation

**Acceptance**

- Success/decline/out-of-stock/Add Stock/cache scenariosเปลี่ยน metrics ตามคาด
- Restart counter semantics ถูกอธิบาย; historical business countไม่อ้างจาก Prometheus
- Idle queueไม่ทำ stalled alert fire

---

### P6 — Grafana and Portfolio Frontend

**Context**

ทำให้ผู้ใช้เห็นหน้าที่ของทุก datastore/observability tool ผ่าน curated views ไม่ใช่หน้า dashboard list

**Deliverables**

- Provisioned dashboards: Platform, Order Transactions, Kafka Saga, Redis & Inventory, ClickHouse V2 correctness, Business Analytics V2
- Explicit datasource ในทุก panel
- ลบ Task query ออกจาก Order business dashboard โดยไม่ลบ historical Task dashboard ถ้ายังใช้
- Direct dashboard links จาก System/Analytics pages
- Admin Inventory: balance cards, Add Stock form, ledger, reservation, low-stock
- Catalog cache badge
- Analytics range/run/watermark และ event-definition tooltips
- Loading/empty/error/eventual/degraded UI states

**Focused tests**

- Dashboard JSON/datasource/query validation
- React API/role/form/cache-state tests
- Admin/member navigation/authorization behavior
- Grafana panelsมี data หลัง deterministic fixture

**Acceptance**

- ผู้ใช้ไม่ต้องเขียน PromQL/SQL เองเพื่อเห็น demo หลัก
- Business dashboard query V2 Order data เท่านั้น
- UI ไม่แสดง synthetic/history เป็น current operational truth โดยไม่มี label

---

### P7 — Full Local and Browser Acceptance

**Context**

พิสูจน์ cross-service behavior และ failure recovery บน Docker Compose ก่อน commit release artifacts

**Required gates**

```powershell
make test
make vet
make build
docker compose -f deploy/docker-compose.yml config --quiet

Push-Location frontend
npm ci
npm run lint
npm test
npm run build
npm run e2e
Pop-Location

./scripts/smoke-test.ps1
```

เพิ่ม/ปรับ Compose acceptance ให้ใช้ unique run ID และไม่ delete volumes อัตโนมัติ

**Browser/Compose scenarios**

1. Add Stock persistence/ledger
2. Redis MISS -> HIT -> adjustment invalidation -> MISS -> HIT
3. Redis catalog down fallback และ Identity session fail closed
4. Success terminal + consume
5. Payment decline compensation + final cancellation
6. Out-of-stock rejection
7. Duplicate request/event idempotency
8. Consumer outage/restart/catch-up
9. ClickHouse V2 reconciliation
10. Prometheus expected series + Grafana six dashboards

**Acceptance**

- Focused/full/backend/frontend/browser/Compose gatesผ่านบน commit candidate เดียวกัน
- ไม่มี uncommitted runtime fix
- ไม่มี Task/V1/volume deletion
- Evidence มี commands, timestamps, fixture IDs/counts, known limitations และ rollback point

---

### P8 — Documentation, GitHub, CI and Pre-deploy Tag

**Context**

สร้าง artifact ที่ตรวจซ้ำได้และหยุดก่อน Render

**Deliverables**

- Update docs 26–32/README/architecture/interview/demo runbook ตาม runtimeจริง
- Complete `docs/33_INVENTORY_OBSERVABILITY_EVIDENCE.md`
- Commit phase-scoped runtime fixes; worktree clean
- Push `codex/inventory-observability-revision` ไป public GitHub
- ตรวจ CI required jobs บน final commit
- สร้าง tag `v0.3.0-inventory-observability-predeploy` หลัง CI green เท่านั้น
- Preserve existing branches/tags; no force-push/retag
- Pre-deploy manifest/rollback point แล้วหยุดก่อน Render

**Acceptance**

- Local evidence และ CI ชี้ commit เดียวกัน
- Tag ใหม่ชี้ CI-green commit และ tags เดิมไม่เปลี่ยน
- Secret scan ไม่พบ credential/session token
- รายงาน Render เป็น HOLD; ไม่สร้าง cloud resource

## 7. Evidence Template

สร้าง `docs/33_INVENTORY_OBSERVABILITY_EVIDENCE.md` ใน P0 และเติมทุก Phase:

```text
Phase: P?
Status: Passed | Blocked
Commit SHA:
Started/finished (Asia/Bangkok):
Files/components changed:
Commands and results:
Fixture/run ID:
Database/event/metric reconciliation:
Browser/Grafana evidence:
Known limitations:
Rollback point:
Git status:
```

ห้ามใช้ screenshot อย่างเดียวเป็น evidence; ต้องมี machine-readable assertion/query/result ควบคู่

## 8. Comment Standard

ใช้ comment ภาษาไทยเฉพาะ:

- decision/invariant/legal transition
- ownership/security boundary
- transaction/outbox/processed-event atomicity
- idempotency/retry/DLQ/offset commit
- Redis fail-open/fail-closed และ invalidation-after-commit
- ClickHouse batching/dedup/sorting-key/watermark
- Prometheus cardinality/alert false-positive boundary

ห้าม comment syntax, getter, DTO mapping หรือ code ที่ชื่ออธิบายตัวเองแล้ว

## 9. Luna High Implementation Prompt

ใช้ prompt นี้หลัง design docs ถูก review แล้ว:

```text
ทำงานต่อใน C:\profile - project\golang-essential-3

Implement Inventory and Observability revision ของ golang-essential-3

อ่าน AGENTS.md และ docs/26_ORDER_PLATFORM_OVERVIEW.md ถึง
docs/32_INVENTORY_OBSERVABILITY_HANDOFF.md ให้ครบ เริ่มจาก Exact Resume Point
ใน docs/32_INVENTORY_OBSERVABILITY_HANDOFF.md

ปัจจุบัน docs/31_INVENTORY_OBSERVABILITY_REVISION.md และ
docs/32_INVENTORY_OBSERVABILITY_HANDOFF.md เป็น approved design/handoff draft
ห้าม discard, reset หรือ overwrite โดยไม่ review diff

รักษา main, branch codex/order-platform-revision และ tags
v0.1.0-predeploy, v0.2.0-order-predeploy, v0.2.1-order-predeploy
ห้าม force-push หรือ retag

ทำ P0-P8 ตามลำดับ พร้อม focused tests และ evidence ทุก Phase:
P0 contracts/baseline, P1 canonical terminal events, P2 durable Inventory/Add Stock,
P3 Redis catalog cache, P4 ClickHouse V2, P5 Prometheus metrics,
P6 Grafana/frontend, P7 full local/browser acceptance และ P8 release handoff

ใช้ PostgreSQL เป็น source of truth, Redis catalog fail open, Identity session fail closed,
Kafka at-least-once + idempotent consumers, transactional outbox และ ClickHouse typed V2 projection
ห้าม fabricate OrderConfirmed/OrderCancelled จาก historical Payment events

ใช้ comment ภาษาไทยเฉพาะ decision, invariant, ownership, transaction boundary,
idempotency/retry/DLQ, cache failure/invalidation, ClickHouse batching/dedup และ metric cardinality

ห้ามลบ Task source/schema/data/topic, ClickHouse V1 history หรือ Docker volumes อัตโนมัติ
ห้ามทิ้ง uncommitted runtime fixes และห้ามอ้างว่า acceptance ผ่านหาก browser E2E,
ClickHouse reconciliation, Redis failure demo และ Grafana dashboards ยังไม่ผ่าน

เมื่อ P7 local acceptance ผ่านครบ ให้ update docs/evidence, commit/push branch
codex/inventory-observability-revision, ตรวจ public GitHub CI และสร้าง tag
v0.3.0-inventory-observability-predeploy หลัง CI green เท่านั้น
จากนั้นหยุดก่อน Render deployment และรายงาน Exact Deploy Resume Point
```

## 10. Stop Conditions

หยุดและรายงานก่อนทำต่อเมื่อ:

- ต้อง drop/delete/rename source, schema, table, topic, tag หรือ volume เดิม
- migration ต้อง fabricate/rewrite historical business outcomes
- baseline tag/rollback pointตรวจไม่ได้
- มี unreviewed runtime diff ที่ ownershipไม่ชัด
- ต้องใช้ secret, paid service หรือ cloud resource
- Local browser/ClickHouse/Redis/Grafana acceptance ยังไม่ผ่านแต่ขั้นต่อไปคือ push/tag
- P8 ผ่านแล้วและขั้นต่อไปคือ Render deployment
