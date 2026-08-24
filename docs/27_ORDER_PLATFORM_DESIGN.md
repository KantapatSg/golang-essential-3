# Order Platform Revision — Detailed Design

> สถานะ: **Implemented contract; local R0–R8 evidence recorded**
>
> เอกสารนี้กำหนด ownership, invariants, API/event contracts และ failure behavior สำหรับ revision
> จาก Task domain ไป Order domain; runtime/release status ให้อ้าง ledger ใน
> [Order Revision Evidence](30_ORDER_REVISION_EVIDENCE.md)

## 1. Architecture Principles

1. Gateway เป็น public backend edge เพียงจุดเดียว
2. Command/query ภายในใช้ native gRPC/Protobuf และมี deadline/cancellation
3. แต่ละ service เป็นเจ้าของ business state, migration และ idempotency ledger ของตัวเอง
4. Order mutation และ Outbox insertion commit ใน PostgreSQL transaction เดียวกัน
5. Kafka delivery เป็น at-least-once; consumer ต้อง idempotent และ commit offset หลัง side effect สำเร็จ
6. PostgreSQL เป็น transactional source of truth; ClickHouse เป็น analytical projection
7. Activity เป็น immutable audit view; Notification เป็น user inbox ที่มี read state
8. Metrics ห้ามมี user/order/payment/event/email เป็น label
9. Local simulator ต้อง deterministic และห้ามรับ payment credential จริง

## 2. Bounded Contexts และ Ownership

### Identity

- Owns: users, password hashes, roles, refresh sessions
- Redis session failure: fail closed
- Events: ไม่อยู่ใน Order event chain รอบแรก

### Order

- Owns: order aggregate, items, state transitions, command idempotency, outbox
- ไม่แก้ stock และ payment records โดยตรง
- รับเฉพาะ outcome events เพื่อเปลี่ยน Order state

### Inventory

- Owns: products, available stock, reservations
- Reserve/release ต้อง atomic และ idempotent
- ไม่อ่าน Order DB โดยตรง; ใช้ event payload

### Payment

- Owns: payment attempt/result/idempotency
- ใช้ demo scenario จาก Order event (`success` หรือ `decline`)
- ไม่เก็บ PAN/CVV/token ของบัตรจริง

### Activity

- Owns: ordered audit timeline ของ domain events
- Admin query ทั้งระบบ; Member query เฉพาะ Order ที่ตนเป็นเจ้าของตาม identity metadata

### Notification

- Owns: user-facing messages และ read/unread state
- Event เดียวสร้าง notification ตาม mapping ที่กำหนดหนึ่งครั้ง

### Analytics

- Worker owns ingestion behavior; Analytics Service owns bounded read API
- ClickHouse เก็บ append-like event projection และ deduplicate ด้วย `event_id`

## 3. gRPC Contracts

Contract source of truth อยู่ใต้ `contracts/proto/<context>/v1`; generated filesห้ามแก้มือ

### IdentityService

คง `Login`, `Refresh`, `Logout` จาก baseline

### OrderService

```text
CreateOrder(customer_id, items, payment_scenario, idempotency_key) -> Order
ListOrders(actor, page, page_size) -> Orders
GetOrder(actor, order_id) -> Order
ApplyInventoryOutcome(event) -> Empty       # internal consumer adapter/use case
ApplyPaymentOutcome(event) -> Empty
```

Consumer adapter เรียก application use case ภายใน ไม่เปิด outcome RPC เป็น public REST

### InventoryService

```text
ListProducts(page, page_size) -> Products
GetProduct(product_id) -> Product
QuoteProducts(items) -> ProductQuote       # internal authoritative catalog/pricing
ListReservations(admin, page, page_size) -> Reservations
```

`QuoteProducts` ตรวจ active product และคืน name/unit price/currency snapshot แต่ไม่ reserve stock
Order Service เรียกด้วย bounded deadline ก่อนเปิด DB transaction ถ้า Inventory ใช้งานไม่ได้ Create
Order ต้อง fail โดยไม่สร้าง partial order การยอมรับ synchronous dependency นี้ช่วยให้ราคาไม่มาจาก
untrusted client; stock availability ยังคงตัดสินแบบ async หลัง `OrderCreated`

### PaymentService

```text
ListPayments(admin, page, page_size) -> Payments
GetPayment(admin, order_id) -> Payment
```

### ActivityService

```text
ListActivities(actor, order_id?, page, page_size) -> Activities
```

### NotificationService

```text
ListNotifications(actor, page, page_size) -> Notifications
UnreadCount(actor) -> Count
MarkAsRead(actor, notification_id) -> Notification
MarkAllAsRead(actor) -> Empty
```

### AnalyticsService

```text
OrderSummary(from, to) -> totals/revenue/statuses
OrderFunnel(from, to) -> stage counts
OrderTimeseries(from, to, interval, limit) -> points
ProductRanking(from, to, limit) -> products
```

## 4. REST API Surface

```text
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh
POST   /api/v1/auth/logout

GET    /api/v1/products
GET    /api/v1/products/:id

POST   /api/v1/orders
GET    /api/v1/orders
GET    /api/v1/orders/:id

GET    /api/v1/activities?order_id=...
GET    /api/v1/notifications
GET    /api/v1/notifications/unread-count
PATCH  /api/v1/notifications/:id/read
POST   /api/v1/notifications/read-all

GET    /api/v1/admin/inventory/reservations
GET    /api/v1/admin/payments
GET    /api/v1/admin/analytics/orders/summary
GET    /api/v1/admin/analytics/orders/funnel
GET    /api/v1/admin/analytics/orders/timeseries
GET    /api/v1/admin/analytics/products/ranking
```

Create Order บังคับใช้ `Idempotency-Key` HTTP header; Gateway ใส่ค่าลง
`CreateOrderRequest.idempotency_key` ใน gRPC contract Request เดิมของ customer เดิมต้องได้ Order เดิม
ไม่สร้าง aggregate ซ้ำ และห้ามใช้ client-supplied customer ID แทน JWT subject

## 5. PostgreSQL Data Model

Local ใช้ physical PostgreSQL เดียวเพื่อประหยัด resource แต่แยก logical DB

### `order_db`

```text
orders
- id UUID PK
- customer_id UUID
- status TEXT
- payment_scenario TEXT
- currency CHAR(3)
- total_amount BIGINT             # minor unit เช่น satang
- idempotency_key TEXT
- idempotency_request_hash CHAR(64)
- created_at / updated_at

UNIQUE(customer_id, idempotency_key)

order_items
- order_id UUID
- product_id UUID
- product_name_snapshot TEXT
- unit_price BIGINT
- quantity INTEGER
- line_total BIGINT

outbox
- event_id UUID PK
- aggregate_id UUID
- event_type TEXT
- payload JSONB
- occurred_at
- published_at NULL

processed_events
- consumer_name TEXT
- event_id UUID
- processed_at
- PK (consumer_name, event_id)
```

### `inventory_db`

```text
products(id, sku UNIQUE, name, unit_price, active, created_at, updated_at)
inventory(product_id PK, available_qty, reserved_qty, version)
reservations(id, order_id, product_id, quantity, status, created_at, updated_at)
outbox(...)
processed_events(...)
```

Reserve ใช้ transaction/row lock หรือ atomic conditional update เพื่อห้าม stock ติดลบ

Create Order idempotency order:

1. Validate JWT subject, header key และ item shape
2. Query `(customer_id, idempotency_key)`; ถ้ามีและ canonical request hash ตรงกันให้คืน Order เดิม
   โดยไม่เรียก Inventory ถ้า key เดิมแต่ payload ต่างให้คืน conflict
3. เรียก `QuoteProducts` ก่อนเปิด Order transaction
4. Insert Order + items + Outbox ใน transaction และใช้ composite unique constraint ปิด concurrent race
5. ถ้า unique conflict จาก request คู่แข่ง ให้ query, เปรียบเทียบ hash แล้วคืน Order เดิมหรือ conflict

### `payment_db`

```text
payments(id, order_id UNIQUE, customer_id, amount, currency, scenario, status,
         provider_reference, created_at, updated_at)
outbox(...)
processed_events(...)
```

### `activity_db`

```text
activity_logs(id, event_id UNIQUE, order_id, customer_id, event_type,
              display_data JSONB, occurred_at, recorded_at)
processed_events(event_id PK, processed_at)
```

### `notification_db`

```text
notifications(id, event_id, user_id, order_id, type, title, message,
              is_read, created_at, read_at NULL)
UNIQUE(event_id, user_id, type)
processed_events(event_id PK, processed_at)
```

Migration เป็น versioned/forward-only ภายใน branch; rollback ใช้ application/tag และห้าม drop Task
tables/databases โดยอัตโนมัติ

## 6. Order State Machine และ Transition Ownership

| Current | Event | Next | Side effect |
|---|---|---|---|
| `PENDING` | `InventoryReserved` | `STOCK_RESERVED` | outbox state event |
| `PENDING` | `InventoryRejected` | `REJECTED` | terminal outbox event |
| `STOCK_RESERVED` | `PaymentCompleted` | `CONFIRMED` | `OrderConfirmed` |
| `STOCK_RESERVED` | `PaymentFailed` | `CANCELLED` | `OrderCancelled` + `InventoryReleaseRequested` |
| `CANCELLED` | `InventoryReleased` | `CANCELLED` | timeline/notification only |

Rules:

- Unknown/invalid transition ไป DLQ หรือ failure metric; ห้าม silently mutate
- Duplicate outcome ทำ no-op success หลัง idempotency check
- Event ที่มาผิดลำดับต้อง retry/buffer ตาม bounded policy; รอบแรกใช้ retry แล้ว DLQ พร้อม runbook
- Terminal order ห้ามย้อนกลับเป็น active state

## 7. Kafka Contract

### Topics

```text
order.events.v1
order.events.v1.dlq
```

หนึ่ง topic ลด local complexity และใช้ `order_id` เป็น message key เพื่อรักษาลำดับต่อ aggregate

### Envelope

```json
{
  "schema_version": 1,
  "event_id": "uuid",
  "event_type": "OrderCreated",
  "aggregate_type": "order",
  "aggregate_id": "order-uuid",
  "customer_id": "user-uuid",
  "occurred_at": "RFC3339 UTC",
  "correlation_id": "request/order correlation",
  "payload": {}
}
```

PII ขั้นต่ำ: event ไม่มี email, password, refresh token หรือ payment credential

### Event Catalog

| Event | Producer | Required payload |
|---|---|---|
| `OrderCreated` | Order | items, total, currency, payment_scenario |
| `InventoryReserved` | Inventory | reservation IDs/items |
| `InventoryRejected` | Inventory | reason code, unavailable items |
| `PaymentCompleted` | Payment | payment ID, amount, reference |
| `PaymentFailed` | Payment | payment ID, reason code |
| `OrderConfirmed` | Order | final total/status |
| `OrderCancelled` | Order | cancellation reason |
| `InventoryReleaseRequested` | Order | reservation/order reference, reason code |
| `InventoryReleased` | Inventory | released reservation IDs |

### Consumer Groups

```text
inventory-service-v1
payment-service-v1
order-service-v1
activity-service-v1
notification-service-v1
analytics-service-v1
```

### Delivery Invariants

1. Producer marks `published_at` only after Kafka ack
2. Consumer validates schema/required fields before side effect
3. DB consumer records `processed_events` ใน transaction เดียวกับ state change/outbox
4. Commit Kafka offset หลัง transaction/ClickHouse batch สำเร็จ
5. Poison message ไป DLQ พร้อม reason metadata ที่ไม่ใส่ secret
6. Replay command/process ต้อง idempotent และมี runbook

## 8. CQRS และ Redis

- Order command ใช้ writer connection
- Order list/detail ใช้ reader connection; local ชี้ DB เดียวได้ แต่ config แยก DSN
- Redis cache-aside ใช้กับ product catalog และ order list เฉพาะเมื่อ invalidation ownership ชัด
- Cache failure ต้อง fail open ไป reader DB; Identity session ยังคง fail closed
- Mutation invalidate owner/admin scopes หลัง commit

ไม่ใช้ Redis เป็น durable queue หรือ source of truth

## 9. Notification Design

- Consumer map domain event เป็นข้อความคงที่/ปลอด PII
- Write path: Kafka -> notification DB แบบ async
- Read/read-state path: REST -> Gateway -> gRPC -> notification DB แบบ sync
- Frontend polling default 5 วินาที; back off เมื่อ tab hidden/error
- Member เห็นเฉพาะ `user_id` ของตน; Admin endpoint แยกหากต้องดูทั้งหมด
- Mark read ต้องตรวจ ownership ใน service ไม่เชื่อ frontend guard

Activity เก็บหลักฐานเชิงระบบทั้งหมด ส่วน Notification เก็บเฉพาะข้อความที่ผู้ใช้ควรรู้

## 10. ClickHouse Analytics

Target table `analytics.order_events`:

Query patterns หลักทุกตัวกรอง `event_type` หรือ finite event-type set และมี bounded `event_date` range
ดังนั้น target DDL คือ:

```sql
CREATE TABLE IF NOT EXISTS analytics.order_events
(
    event_id UUID,
    event_type LowCardinality(String),
    order_id UUID,
    customer_id UUID,
    order_status LowCardinality(String) DEFAULT '',
    product_ids Array(UUID) DEFAULT [],
    product_skus Array(String) DEFAULT [],
    amount_minor UInt64 DEFAULT 0,
    currency LowCardinality(String) DEFAULT '',
    occurred_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC') DEFAULT now64(3, 'UTC'),
    event_date Date MATERIALIZED toDate(occurred_at),
    payload_json String DEFAULT ''
)
ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(event_date)
ORDER BY (event_type, event_date, order_id, event_id)
TTL occurred_at + INTERVAL 180 DAY DELETE
SETTINGS index_granularity = 8192;
```

`payload_json` เป็น opaque replay/debug payload เท่านั้น; business query ใช้ typed columns

- Per `schema-pk-plan-before-creation`, `schema-pk-prioritize-filters` และ
  `schema-pk-cardinality-order`: ล็อก immutable ORDER BY จาก query pattern และเรียง key จาก
  cardinality ต่ำไปสูงก่อน migration
- Per `schema-types-native-types`, `schema-types-minimize-bitwidth`,
  `schema-types-lowcardinality` และ `schema-types-avoid-nullable`: ใช้ UUID/Date/DateTime64/UInt64,
  dictionary encoding สำหรับ status/type/currency และ default แทน Nullable ที่ไม่มี semantic need
- Per `schema-partition-low-cardinality` และ `schema-partition-lifecycle`: monthly partition ใช้เพื่อ
  retention/TTL ไม่ใช่แทน primary-key filtering
- Per `query-join-consider-alternatives`: denormalize product SKU และ fields ที่ aggregate ต้องใช้
  ลง event row เพื่อลด runtime JOIN; `payload_json` เป็น opaque debug copy ไม่ query field ภายใน
- Immediate dedup query ใช้ bounded `FINAL` หรือ equivalent `argMax` pattern บน `event_id`;
  ห้ามเรียก `OPTIMIZE TABLE ... FINAL` เป็น routine ตาม `insert-optimize-avoid-final`
- Per `schema-pk-filter-on-orderby`, query ทุกตัวต้องกรอง event type + time range, ใส่ `LIMIT`
  เมื่อคืน rows และตรวจ key usage ด้วย `EXPLAIN indexes = 1`

Worker สะสม batch โดย size/time threshold; production target 10,000-100,000 rows ต่อ insert ตาม
`insert-batch-size` ถ้า local/demo มี event น้อยให้ใช้ `async_insert=1` และ
`wait_for_async_insert=1` ตาม `insert-async-small-batches` เพื่อไม่สร้าง tiny parts โดยไม่รับรู้ error
ใช้ Native/driver columnar insert ตาม `insert-format-native` เมื่อ client รองรับ และไม่ทำ mutation
ใน normal flow ตาม `insert-mutation-avoid-update` / `insert-mutation-avoid-delete`

`DateTime64(3)` ใช้เพราะต้องรักษา millisecond event ordering; หาก evidence แสดงว่า precision ระดับวินาที
เพียงพอจึงลดเป็น `DateTime` ได้ก่อนสร้าง production table

Business queries:

- Order status counts และ total revenue
- Funnel: created -> reserved -> paid -> confirmed
- Payment/stock rejection rate
- Average processing duration
- Product ranking และ daily timeseries

## 11. Observability

### Metrics Catalog

```text
http_requests_total{service,route,method,status_class}
grpc_requests_total{service,method,code}
grpc_request_duration_seconds{service,method}
orders_created_total
order_state_transitions_total{from,to}
inventory_reservations_total{result}
payments_total{result}
notifications_created_total{type}
notification_processing_failures_total
outbox_pending_events{service}
outbox_publish_failed_total{service}
kafka_consumer_lag{consumer}
consumer_retry_total{consumer}
consumer_dlq_total{consumer}
analytics_processing_latency_seconds
service_ready{service}
```

Labels จำกัดค่าที่ bounded; ห้าม user/order/event IDs

### Dashboards

1. Platform Overview — RED/readiness ของ services
2. Order Event Pipeline — outbox, lag, retry, DLQ, consumer rate
3. Business Analytics — ClickHouse funnel/revenue/statuses
4. Inventory & Payment — reservation/payment results และ failure rate

### Alerts

- Service not ready
- Outbox pending สูงต่อเนื่อง
- Kafka lag สูง
- Consumer DLQ/retry เพิ่ม
- Payment failure rate สูงใน bounded window
- Analytics processing latency สูงเมื่อมี backlog

ห้ามใช้ “เวลาตั้งแต่ event ล่าสุด” เป็น ingestion delay alert เพราะ workload ว่างทำให้ false positive;
ใช้ processing latency หรือ combine กับ lag/backlog

## 12. Authentication และ Authorization

| Capability | Member | Admin |
|---|---:|---:|
| Products | read | read |
| Create Order | own | own/test |
| Orders | own only | all |
| Timeline/Notifications | own only | own; audit all ผ่าน admin endpoint |
| Inventory/Payments | no | read |
| Analytics/System | no | read |

JWT verification ที่ Gateway ช่วย reject เร็ว แต่ service ต้องบังคับ ownership/role จาก trusted gRPC
metadata ด้วย Frontend role guard ไม่ใช่ security boundary

## 13. Failure Behavior

| Failure | Expected behavior |
|---|---|
| Redis down | Product/Order read fallback DB; Identity session fail closed |
| Kafka down | Create Order commit + Outbox ได้; publish retry; projections stale |
| Inventory down | Create ใหม่ fail ก่อน transaction เพราะ quote ไม่ได้; Order ที่สร้างแล้วค้างตาม backlog และตามต่อหลัง recovery |
| Payment down | Order อยู่ `STOCK_RESERVED`; backlog รอ; ห้ามตัดเงินซ้ำ |
| Notification down | Core order flowเดินต่อ; inbox catch up หลัง restart |
| ClickHouse down | Core flowเดินต่อ; workerไม่ commitและ retry; analytics stale/unavailable |
| Activity DB down | Activity consumerไม่ commit; event replay หลัง recovery |
| Duplicate event | State/notification/payment/reservationไม่ซ้ำ |
| Out-of-order event | Reject/retry/DLQ; ห้าม invalid state transition |

## 14. Migration และ Rollback Strategy

### Current baseline

- `v0.1.0-predeploy` เป็น Task runtime ที่ CI green และ rollback ได้
- ห้าม retag/overwrite tag นี้
- Task DB/topic/source ยังอยู่จน Order acceptance ผ่านครบ

### Forward path

1. สร้าง revision branch จาก clean `main`
2. Expand: เพิ่ม Order contracts/services/DBs/topic โดยไม่ drop Task artifacts
3. Implement และ verify targeted slices ทีละ context
4. เพิ่ม frontend Order routes โดยยังเก็บ old landing/docs ชั่วคราว
5. Cut over Compose/README/smoke เมื่อ Order matrix ผ่าน
6. Contract: ลบ Task runtime path เฉพาะ commit แยกและหลัง user review; เก็บ history/tag เป็น rollback

### Rollback path

- ก่อน cutover: revert phase commit หรือปิด Order services; Task path ยังทำงาน
- หลัง cutover local: checkout/tag `v0.1.0-predeploy` และ rebuild clean Compose
- ห้าม drop database/volume อัตโนมัติ; destructive cleanup ต้องได้รับอนุมัติแยก

ไม่มี production mixed-version requirement เพราะ Render Phase ยัง Hold แต่ contracts/events ต้อง versioned เพื่อ
ให้ local producer/consumer ต่าง commit ทำงานร่วมกันระหว่าง implementation ได้

## 15. Commenting Boundaries

เพิ่ม comment ภาษาไทยเฉพาะ:

- Order state invariant และ invalid transition
- Transaction ที่รวม aggregate/outbox/processed-event
- Kafka offset commit/retry/DLQ/replay boundary
- Inventory atomic reserve และ compensation
- Payment idempotency และห้ามเก็บ credential
- Notification ownership/idempotency
- ClickHouse batch/dedup/query-key trade-off
- readiness/degraded dependency และ metric cardinality

ไม่ comment syntax, DTO mapping ที่ตรงไปตรงมา หรือชื่อ function ที่อธิบายตัวเองแล้ว

## 16. Open Decisions ถูกปิดสำหรับรอบแรก

| Decision | เลือก | เหตุผล |
|---|---|---|
| Kafka topology | topic เดียว + DLQ | local/learning ง่ายและรักษา ordering ต่อ Order |
| Saga | choreography | แสดง event-driven interaction โดยไม่เพิ่ม orchestrator service |
| Notification delivery | polling 5 วินาที | ไม่เพิ่ม WebSocket/SSE lifecycle |
| Payment | deterministic simulator | ปลอดภัยและทดสอบ failure ได้ |
| Catalog | Inventory owns seeded products | ไม่เพิ่ม Product Service/Cart |
| Money | integer minor unit | เลี่ยง floating-point |
| Kafka semantics | at-least-once + idempotency | realistic และพิสูจน์ recovery ได้ |

ลำดับ implement/test อยู่ใน [28_ORDER_IMPLEMENTATION_PLAN.md](28_ORDER_IMPLEMENTATION_PLAN.md)
