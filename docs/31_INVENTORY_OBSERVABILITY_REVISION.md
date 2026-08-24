# Inventory and Observability Revision — Detailed Design

> Updated: 2026-08-24 (Asia/Bangkok)
>
> Status: **P0–P8 verified; pre-deploy tag handoff recorded in docs/33**
>
> เอกสารนี้เป็น post-release hardening revision ต่อจาก Order release
> `v0.2.1-order-predeploy` โดยไม่แก้ย้อนหลังหรือใช้แทน evidence ใน docs 26–30

## 1. เหตุผลของ Revision

การทดสอบ Order Platform ใน local แสดงว่า workflow หลักทำงาน แต่บทบาทของ ClickHouse,
Prometheus, Grafana และ Redis ยังสื่อจาก UI ไม่ชัด และ runtime บางส่วนไม่ตรงกับ design เดิม

ผล audit แบบ read-only ก่อนสร้างแผน:

| ขอบเขต | สิ่งที่พบใน runtime | ผลกระทบ |
|---|---|---|
| ClickHouse | `OrderCreated=32`, `PaymentCompleted=15`, `PaymentFailed=8`, `InventoryRejected=8` | เป็น event จริงจาก deterministic demo/test ที่สะสมใน named volume ไม่ใช่ตัวเลข hardcode |
| PostgreSQL | operational `orders` ปัจจุบันมี 4 รายการ | lifecycle ของ operational data ไม่ตรงกับ ClickHouse history |
| Analytics summary | นับ `PaymentCompleted` เป็น confirmed และ `PaymentFailed` เป็น cancelled | business meaning ปน payment outcome กับ terminal Order outcome |
| Analytics funnel | รอ `OrderConfirmed`/`OrderCancelled` แต่ producer ไม่ได้ส่ง event ครบ | Summary แสดง confirmed แต่ funnel confirmed เป็นศูนย์ |
| Revenue | payload มี `amount_minor`; live ClickHouse รวมได้ 28,683 cents แต่ API คืนศูนย์ | aggregation ยังไม่ได้คำนวณ revenue จริง |
| Grafana | Business Analytics dashboard ยัง query `analytics.task_events` | dashboard ชื่อ Order แต่ยังแสดง projection รุ่น Task |
| Prometheus | service readiness และ pipeline metrics มีจริง แต่ Order/Payment/Inventory metrics ไม่มี | ไม่เห็น business transaction rate, latency และ failure ratio |
| Redis | live keys เป็น Identity `session:*`; Order/Inventory ยังไม่ใช้ cache | ผู้ใช้ไม่เห็น catalog cache hit/miss/invalidation |
| Inventory | product, stock, reservation อยู่ใน memory maps | restart แล้ว state หาย และไม่มี Add Stock/stock ledger |

คำว่า synthetic ในเอกสารนี้หมายถึง event ที่เกิดจาก payment simulator และ acceptance scenario
แต่ไหลผ่าน REST, gRPC, PostgreSQL/Outbox และ Kafka จริง ไม่ใช่ mock response ที่ frontend hardcode

## 2. เป้าหมายที่ผู้ใช้ต้องมองเห็น

เมื่อ revision นี้เสร็จ ผู้ใช้ต้อง demo ได้จาก browser เดียวว่า:

1. Admin เพิ่ม stock แล้วเห็น balance และ stock movement ที่ persist หลัง restart
2. Catalog request แรกเป็น Redis MISS, request ถัดไปเป็น HIT
3. การเปลี่ยน stock ทำให้ cache invalidate หลัง PostgreSQL commit
4. Redis ล่มแล้ว Catalog fallback PostgreSQL แต่ Identity refresh session fail closed
5. Order success จบด้วย `OrderConfirmed` และมี confirmed revenue ใน ClickHouse
6. Payment decline รอคืน stock ก่อนจบด้วย `OrderCancelled`
7. Out-of-stock จบด้วย `OrderRejected`
8. Prometheus แสดง aggregate rate/error/latency และ Grafana มี dashboard ที่ใช้งานได้ทันที
9. Activity/Notification แสดง event timeline ราย Order ส่วน ClickHouse แสดง business aggregate
10. ข้อมูลทุกหน้าแจ้ง source, time range และ eventual watermark อย่างตรงไปตรงมา

## 3. Architecture และ Ownership

```text
Admin
  |
  | REST Add/Adjust Stock
  v
Fiber API Gateway
  |
  | synchronous gRPC
  v
Inventory Service
  |
  +--> inventory_db transaction
  |      +-- inventory_balances
  |      +-- stock_movements
  |      +-- outbox_events
  |
  +--> invalidate Redis catalog cache after commit
  |
  +--> Kafka: StockAdjusted / LowStockDetected
          +--> Activity / Notification
          +--> Analytics Worker --> ClickHouse

Member catalog read
  -> Gateway -> Inventory gRPC
                  +--> Redis HIT
                  +--> Redis MISS -> inventory_db -> Redis SET

Member submit order
  -> Gateway -> Order gRPC -> order_db + outbox -> Kafka choreography
                                                   +--> Inventory
                                                   +--> Payment
                                                   +--> Order terminal state
                                                   +--> Activity/Notification
                                                   +--> Analytics Worker

All Go services -> Prometheus -> Grafana operational dashboards
ClickHouse ------------------> Grafana business dashboard
```

Locked boundaries:

- PostgreSQL เป็น transactional source of truth
- Redis เป็น cache/session state ตาม boundary เท่านั้น ไม่ใช่ stock source of truth หรือ queue
- ClickHouse เป็น eventually consistent analytical projection ไม่ใช่ Order database
- Prometheus เก็บ aggregate time-series ไม่เก็บ transaction ราย Order
- Grafana visualize Prometheus/ClickHouse แต่ไม่เป็น datastore
- Kafka รับประกัน ordering ต่อ `order_id` เมื่อ producer ใช้ `order_id` เป็น message key
- Gateway เป็น public backend edge; internal gRPC, Kafka และ databases ไม่เปิดตรงสู่ browser

## 4. Canonical Order Saga

### 4.1 Success

```text
OrderCreated
  -> InventoryReserved
  -> PaymentCompleted
  -> OrderConfirmed
  -> InventoryConsumed
```

`OrderConfirmed` เป็น canonical terminal business event สำหรับ confirmed count และ revenue
ส่วน `InventoryConsumed` ปิด reservation ledger หลัง Order confirmed

### 4.2 Payment declined และ compensation

```text
OrderCreated
  -> InventoryReserved
  -> PaymentFailed
  -> Order status CANCELLING
  -> InventoryReleaseRequested
  -> InventoryReleased
  -> OrderCancelled
```

Order จะเป็น `CANCELLING` ระหว่างรอคืน stock และเปลี่ยนเป็น `CANCELLED` หลังรับ
`InventoryReleased` เท่านั้น เพื่อไม่อ้างว่า compensation เสร็จก่อน side effect สำเร็จ

### 4.3 Out of stock

```text
OrderCreated
  -> InventoryRejected
  -> OrderRejected
```

### 4.4 Transition และ event invariants

| Current | Input event | Next | Outbox event |
|---|---|---|---|
| `PENDING` | `InventoryReserved` | `STOCK_RESERVED` | state/activity event ตาม contract |
| `PENDING` | `InventoryRejected` | `REJECTED` | `OrderRejected` |
| `STOCK_RESERVED` | `PaymentCompleted` | `CONFIRMED` | `OrderConfirmed` |
| `STOCK_RESERVED` | `PaymentFailed` | `CANCELLING` | `InventoryReleaseRequested` |
| `CANCELLING` | `InventoryReleased` | `CANCELLED` | `OrderCancelled` |
| `CONFIRMED` | `OrderConfirmed` ที่ Inventory consume | terminal | `InventoryConsumed` จาก Inventory |

- การเปลี่ยน state, processed-event record และ terminal outbox event ต้อง commit transaction เดียวกัน
- Duplicate input event ต้องคืนผลสำเร็จแบบ no-op และไม่สร้าง terminal event ซ้ำ
- Invalid/out-of-order transition ห้ามแก้ state; ใช้ bounded retry แล้ว DLQ พร้อม reason
- Analytics/Notification ใช้ terminal Order events เป็น canonical definition ห้ามอนุมาน terminal state จาก Payment event

## 5. Durable Inventory และ Add Stock

Inventory Service เป็นเจ้าของ logical `inventory_db` แม้ local จะใช้ PostgreSQL container เดียว

### 5.1 Data model

```text
products
- id TEXT PK
- sku TEXT UNIQUE
- name TEXT
- unit_price_minor BIGINT
- currency CHAR(3)
- active BOOLEAN
- created_at / updated_at

inventory_balances
- product_id TEXT PK
- available_qty BIGINT CHECK available_qty >= 0
- reserved_qty BIGINT CHECK reserved_qty >= 0
- version BIGINT
- updated_at

stock_movements
- id UUID PK
- product_id TEXT
- movement_type TEXT       # RESTOCK, RESERVE, RELEASE, CONSUME, CORRECTION
- quantity_delta BIGINT
- available_after BIGINT
- reserved_after BIGINT
- reason TEXT
- reference TEXT
- idempotency_key TEXT
- actor_id TEXT
- created_at
- UNIQUE(actor_id, idempotency_key)

reservations
- id UUID PK
- order_id UUID
- product_id TEXT
- quantity BIGINT
- status TEXT              # RESERVED, RELEASED, CONSUMED
- created_at / updated_at
- UNIQUE(order_id, product_id)

processed_events
outbox_events
```

`on_hand_qty` แสดงเป็น `available_qty + reserved_qty`; เมื่อ `InventoryConsumed` สำเร็จ
ระบบลด `reserved_qty` โดยไม่คืน `available_qty`

### 5.2 gRPC contracts

```text
ListProducts(page, page_size)
GetProduct(product_id)
QuoteProducts(items)
ListInventory(page, page_size, low_stock_only)
AdjustStock(product_id, quantity_delta, reason, reference, idempotency_key, actor)
ListStockMovements(product_id?, page, page_size)
ListReservations(page, page_size)
```

`AdjustStock` รองรับ Add Stock ด้วย positive delta และ correction ด้วย signed delta แต่ correction
ที่ทำให้ available stock ติดลบต้องถูก reject

### 5.3 REST surface

```text
GET  /api/v1/products
GET  /api/v1/products/:id

GET  /api/v1/admin/inventory
POST /api/v1/admin/inventory/adjustments
GET  /api/v1/admin/inventory/movements
GET  /api/v1/admin/inventory/reservations
```

- Adjustment endpoints เป็น admin-only ที่ Gateway และ Inventory Service
- ใช้ `Idempotency-Key` และ canonical request hash ป้องกัน retry payload ต่างกัน
- Update balance, movement และ outbox ต้องอยู่ transaction เดียวกัน
- Reserve ใช้ row lock หรือ atomic conditional update เพื่อห้าม oversell

## 6. Redis Catalog Cache

### 6.1 Keys และ policy

```text
catalog:v1:products:{page}:{page_size}
product:v1:{product_id}
```

- TTL เริ่มต้น 60 วินาทีและกำหนดผ่าน environment
- Cache value มี schema version และ generated timestamp
- Inventory Service เป็น owner ของ catalog keys และ invalidation
- Add stock, reserve, release, consume และ product mutation invalidate affected product/list keys หลัง DB commit
- Invalidation failure ต้อง log/metric และยอมให้ short TTL เยียวยา; ห้าม rollback DB transaction
- ห้ามเก็บ refresh token, PII หรือ payment data ใน catalog cache

### 6.2 Read behavior

```text
GET catalog
  -> Redis GET
     -> HIT: return cached projection
     -> MISS: read inventory_db -> SETEX -> return
     -> ERROR: read inventory_db -> return degraded response
```

Inventory gRPC ส่ง cache result ผ่าน response metadata และ Gateway แปลงเป็น
`X-Cache-Status: HIT | MISS | BYPASS` เพื่อใช้ศึกษาใน local/portfolio UI โดยไม่ปน business DTO

Failure contract แยกตาม ownership:

| Redis use | Failure policy |
|---|---|
| Identity refresh session | fail closed |
| Product catalog cache | fail open ไป PostgreSQL |
| Legacy Task cache | fail open ไป reader DB |

## 7. ClickHouse Order Analytics V2

### 7.1 Compatibility strategy

- ห้ามลบหรือ rewrite `analytics.order_events` เดิมอัตโนมัติ
- สร้าง `analytics.order_events_v2` และ cut over read API เมื่อ fixture reconciliation ผ่าน
- Historical v1 rows ถูก label เป็น legacy; ห้ามสร้าง `OrderConfirmed` ปลอมจาก `PaymentCompleted`
- CI/acceptance ใช้ unique `run_id`; manual data ใช้ environment/deployment metadata
- PostgreSQL local ต้องมี named volume เพื่อให้ lifecycle ของ source of truth ไม่หายก่อน ClickHouse

### 7.2 Typed schema target

```text
event_id UUID
schema_version UInt16
event_type LowCardinality(String)
order_id UUID
customer_id UUID
order_status LowCardinality(String)
product_ids Array(String)
amount_minor UInt64
currency LowCardinality(String)
reason LowCardinality(String)
environment LowCardinality(String)
run_id String
occurred_at DateTime64(3, UTC)
ingested_at DateTime64(3, UTC)
event_date Date MATERIALIZED
payload_json String
```

Target engine:

```text
ReplacingMergeTree(ingested_at)
ORDER BY (event_date, event_type, order_id, event_id)
TTL occurred_at + INTERVAL 180 DAY
```

เริ่มโดยไม่มี `PARTITION BY` เพราะ local/demo dataset เล็กและยังไม่มี lifecycle operation ที่ต้อง
จัดการระดับ partition; ทบทวนเมื่อ row/part volume หรือ retention evidence แสดงความจำเป็น

### 7.3 P4 execution decision (additive clarification)

P4 runtime evidence now retains a monthly `PARTITION BY toYYYYMM(event_date)` because the
180-day TTL is an explicit lifecycle operation and the named local ClickHouse volume already
contains retained fixtures. This bounded partition key is consistent with the V2 sort key and
avoids daily/high-cardinality partitions. The decision is additive: V1 is untouched, no existing
partition is dropped, and the design is revisited only with measured part/row evidence.

P4 execution clarification: the live V2 migration uses `Int64` for `amount_minor` so the typed
projection matches the Go aggregation and signed analytical test fixtures; business revenue still
sums only canonical `OrderConfirmed` amounts and does not infer outcomes from Payment events.

หลักออกแบบ:

- Query known business fields จาก typed columns ไม่ parse `payload_json`
- Summary/funnel ใช้ canonical terminal events และ deduplicate ต่อ Order
- Revenue รวม `amount_minor` จาก `OrderConfirmed`
- Query ทุกตัวมี bounded time range; row-returning query มี `LIMIT`
- ตรวจ sorting-key use ด้วย `EXPLAIN indexes = 1`
- Local event rate ต่ำให้ใช้ async insert พร้อม `wait_for_async_insert=1` หรือ bounded batch flush
- Commit Kafka offset หลัง ClickHouse insert acknowledgement เท่านั้น

## 8. Prometheus Metrics

Prometheus เก็บ aggregate time-series; individual transaction trace อยู่ใน Activity/PostgreSQL และ
business history อยู่ใน ClickHouse

### 8.1 Transport/RED metrics

```text
http_server_requests_total{service,route,method,status_class}
http_server_request_duration_seconds{service,route,method}
grpc_server_requests_total{service,method,code}
grpc_server_request_duration_seconds{service,method}
```

### 8.2 Business/pipeline metrics

```text
orders_created_total{result}
order_state_transitions_total{from,to}
order_saga_duration_seconds{outcome}
orders_in_state{state}
inventory_reservations_total{result}
inventory_adjustments_total{reason}
inventory_low_stock_products
payments_total{result}
cache_requests_total{service,cache,operation,result}
cache_operation_duration_seconds{service,cache,operation}
cache_fallback_total{service,cache}
cache_invalidations_total{service,cache,reason,result}
kafka_events_total{service,event_type,result}
kafka_consumer_lag{consumer}
outbox_pending_events{service}
consumer_retry_total{consumer}
consumer_dlq_total{consumer}
analytics_processing_latency_seconds
```

Metric labels ต้องเป็น bounded vocabulary ห้ามใช้ order/customer/product/event/request ID, email,
SKU แบบ unbounded หรือ error message เป็น label

## 9. Grafana Dashboards

1. **Platform Overview** — readiness, HTTP/gRPC throughput, error rate, p50/p95 latency
2. **Order Transactions** — create rate, state transitions, outcome ratio, in-flight และ saga p95
3. **Kafka Saga Pipeline** — publish/consume, lag, retry, DLQ, outbox backlog
4. **Redis & Inventory** — hit ratio, miss/bypass, Redis error, invalidation, adjustment และ low stock
5. **ClickHouse V2** — typed ingestion health, canonical event counts, watermark and projection correctness
6. **Business Analytics (ClickHouse V2)** — funnel, confirmed revenue, status mix, rejection reasons, product ranking

Grafana provisioned JSON ต้องระบุ datasource ชัดเจนและ dashboard test ต้องตรวจ query ไม่ให้ย้อนกลับ
ไป `task_events` สำหรับ Order dashboard

Business Analytics และ ClickHouse V2 เป็นคนละ curated view โดยตั้งใจ: อันแรกตอบคำถามทางธุรกิจ
และ revenue ส่วนอันหลังตอบ ingestion/schema/watermark correctness; ทั้งสองอ่าน V2 เท่านั้น

## 10. Frontend Portfolio UX

### Admin Inventory

- Cards: available, reserved, on-hand, low-stock products
- Add Stock/Adjust Stock dialog: product, quantity, reason, reference
- Stock movement ledger และ reservation status
- Pending/success/conflict/error state ของ idempotent command
- Cache source badge จาก `X-Cache-Status`

### Admin Analytics

- Date/time range และ optional run filter
- Projection `through`/watermark
- Tooltip ว่า Created/Confirmed/Rejected/Cancelled นับจาก event ใด
- Confirmed revenue จาก canonical `OrderConfirmed`
- Deep links ไป curated Grafana dashboards ไม่ใช่หน้า dashboard list

### System

- Prometheus/Grafana links พร้อมตัวอย่าง PromQL
- สถานะ dependency ที่ browser เข้าถึงได้เท่านั้น
- Learning note แยก transaction detail, analytical history และ operational metrics

## 11. Failure Behavior

| Failure | Expected behavior |
|---|---|
| Redis catalog unavailable | Catalog fallback PostgreSQL; metric `BYPASS`; Order core ทำงานต่อ |
| Redis Identity unavailable | Refresh/login session mutation fail closed |
| Kafka unavailable | Order/Inventory DB + outbox commit; publisher retry; projection stale |
| ClickHouse unavailable | Core flowทำงานต่อ; workerไม่ commit offset; analytics stale/unavailable |
| Prometheus/Grafana unavailable | Business flowไม่ล้ม; observability unavailable |
| Inventory DB unavailable | Catalog/adjust/reserve fail; ห้ามใช้ stale Redis ตัดสิน reservation |
| Duplicate AdjustStock | คืนผลเดิมจาก idempotency record; stockเพิ่มครั้งเดียว |
| Duplicate domain event | state/movement/notification/analytics business effect ไม่ซ้ำ |
| Compensation stalled | Order คง `CANCELLING`; alert จาก lag/retry/outbox ไม่แสดง `CANCELLED` ก่อน release |

## 12. Test and Acceptance Contract

### Focused tests

- Order state machine และ terminal outbox atomicity
- Inventory migration, balance invariant, idempotent adjustment และ concurrent reserve
- Redis hit/miss/invalidation/fallback และ Identity fail-closed regression
- ClickHouse event mapping, dedup, range, funnel และ revenue fixtures
- Metrics exposition, bounded labels และ histogram/counter semantics
- Grafana datasource/dashboard JSON validation
- Frontend role guard, adjustment form, cache badge และ analytics definitions

### Full browser/Compose scenarios

1. Admin Add Stock แล้ว restart Inventory/PostgreSQL; balance และ ledgerยังอยู่
2. Catalog request แรก MISS, requestถัดไป HIT, Add Stock แล้ว requestถัดไป MISS พร้อม stockใหม่
3. Redis down: Catalog BYPASS/DB success แต่ refresh session fail closed
4. Success: `OrderCreated -> InventoryReserved -> PaymentCompleted -> OrderConfirmed -> InventoryConsumed`
5. Decline: `PaymentFailed -> CANCELLING -> InventoryReleased -> OrderCancelled`
6. Out-of-stock: `InventoryRejected -> OrderRejected`
7. Duplicate request/event ไม่เปลี่ยน stock/order/revenue ซ้ำ
8. ClickHouse V2 counts/revenue reconcile กับ known run fixture
9. Prometheus queries คืน transaction/cache metrics และ Grafana 6 dashboardsมีข้อมูล
10. Consumer outage/restart catch up โดย business effect ไม่ซ้ำ

## 13. Non-goals และ Stop Boundary

- ไม่เพิ่ม real payment provider, card data, shipping, email/SMS, WebSocket/SSE
- ไม่เพิ่ม Kubernetes, service mesh, tracing backend หรือ schema registry
- ไม่ลบ Task source/schema/data/topic/volume อัตโนมัติ
- ไม่ลบ ClickHouse v1 history อัตโนมัติ
- ไม่ deploy Render หรือสร้าง paid cloud resource ใน revision นี้
- การ contract/drop ของเดิมและ Render deployment ต้องเป็น phase/approval แยก

ลำดับ implementation, evidence และ Exact Resume Point อยู่ใน
[32_INVENTORY_OBSERVABILITY_HANDOFF.md](32_INVENTORY_OBSERVABILITY_HANDOFF.md)
