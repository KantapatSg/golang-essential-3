# Order Platform Revision — System Overview

> สถานะ: **R0–R8 implement และ local acceptance ผ่าน; R9–R11 release handoff ยัง pending**
>
> Runtime บน `main` และ tag `v0.1.0-predeploy` ยังคงเป็น Task/Activity/Analytics baseline เดิม
> ส่วน Order runtime อยู่บน revision branch และมี evidence local ใน
> [Order Revision Evidence](30_ORDER_REVISION_EVIDENCE.md)

## 1. เป้าหมาย

เปลี่ยน business use case ของ `golang-essential-3` จาก Task CRUD ที่แสดง Kafka ได้ไม่เด่น
เป็น Order Processing Platform ซึ่งมี workflow ข้ามหลาย bounded contexts และทำให้เห็นบทบาทของ
REST, gRPC, Kafka, PostgreSQL, Redis, ClickHouse, Prometheus และ Grafana อย่างเป็นธรรมชาติ

ระบบต้องตอบคำถามการเรียนรู้ต่อไปนี้ได้จาก demo เดียว:

- REST Gateway และ gRPC แบ่ง public/internal transport อย่างไร
- คำสั่ง synchronous จบตรงไหน และ event-driven workflow เริ่มตรงไหน
- Kafka ช่วยเรื่อง decoupling, buffering, retry, replay และ fan-out อย่างไร
- แต่ละ service เป็นเจ้าของ state และ idempotency ของตัวเองอย่างไร
- Activity audit กับ In-app Notification ต่างกันอย่างไร
- PostgreSQL, Redis, ClickHouse, Prometheus และ Grafana มีหน้าที่ต่างกันอย่างไร

## 2. Actors และ Use Cases

### Member / Customer

- Login, refresh session และ logout
- ดูสินค้าที่ seed ไว้และ stock ที่พร้อมขาย
- สร้าง Order โดยเลือกสินค้า จำนวน และ demo payment scenario
- ดู Order ของตัวเองและสถานะล่าสุด
- ดู Order timeline
- ดู In-app Notification และ mark as read

### Admin / Operator

- ทำทุกอย่างที่ Member ทำได้
- ดู Order, Inventory, Payment และ Activity ทั้งระบบ
- ดู Order funnel และ business analytics
- ดู Prometheus/Grafana และ operational health

## 3. System Context

```text
Member / Admin
      |
      | HTTPS REST/JSON
      v
React Frontend --> Fiber API Gateway
                        |
                        | synchronous gRPC
        +---------------+----------------+----------------+
        |               |                |                |
        v               v                v                v
    Identity          Order          Inventory       Activity / Notification
        |               |                |                |
        v               v                v                v
  identity_db       order_db       inventory_db     activity/notification_db
                        |
                 Task? No: Order + Outbox
                        |
                        v
                Kafka order.events.v1
                        |
          +-------------+-------------+----------------+
          |             |             |                |
          v             v             v                v
     Inventory       Payment       Activity       Notification
          |             |                              |
          v             v                              v
   inventory_db    payment_db                    notification_db
                        |
                        +--> outcome events --> Order state transition
                        |
                        +--> Analytics Worker --> ClickHouse

All Go services -- /metrics --> Prometheus --> Grafana
ClickHouse -------------------------------> Grafana business dashboard
```

Gateway เป็น public backend edge เพียงจุดเดียว ส่วน gRPC, Kafka, databases, Prometheus และ
ClickHouse อยู่ใน private/container network

ตอน Create Order นั้น Order Service เรียก `InventoryService.QuoteProducts` ผ่าน gRPC เพื่ออ่าน
ชื่อ/ราคา/currency ที่ Inventory เป็นเจ้าของก่อนเปิด Order transaction การ call นี้เป็นเพียง catalog
quote ไม่ reserve stock; การ reserve จริงยังเกิดผ่าน Kafka หลังตอบ `PENDING`

## 4. เส้นทาง Synchronous และ Asynchronous

### 4.1 Create Order — synchronous boundary

```text
POST /api/v1/orders
  -> Gateway ตรวจ JWT/role และแปลง REST เป็น gRPC
  -> OrderService.CreateOrder()
  -> OrderService -> gRPC InventoryService.QuoteProducts() (catalog/pricing only)
  -> PostgreSQL transaction: INSERT order + order_items + outbox(OrderCreated)
  -> ตอบ { order_id, status: PENDING }
```

ผู้ใช้รอถึง Order transaction commit เท่านั้น ไม่รอ Inventory, Payment, Activity,
Notification หรือ Analytics ถ้า Inventory quote ใช้งานไม่ได้ ระบบจะ fail ก่อนสร้าง Order ด้วย
`503` แทนการเชื่อราคาจาก request หรือสร้าง Order ที่ไม่มี authoritative price

### 4.2 Order workflow — asynchronous boundary

```text
OrderCreated
  -> Inventory Service
     -> InventoryReserved
        -> Payment Service
           -> PaymentCompleted
              -> Order Service: CONFIRMED
              -> OrderConfirmed
           -> PaymentFailed
              -> Order Service: CANCELLED
              -> OrderCancelled + InventoryReleaseRequested
              -> Inventory Service -> InventoryReleased
     -> InventoryRejected
        -> Order Service: REJECTED
```

Kafka ส่งและเก็บ event; business logic ทำโดย consumer service ไม่ใช่ Kafka

### 4.3 Read paths

การเขียน projection เป็น async แต่การอ่านยังเป็น sync:

```text
GET /api/v1/orders/:id
  -> Gateway -> gRPC Order Service -> order_db

GET /api/v1/activities?order_id=...
  -> Gateway -> gRPC Activity Service -> activity_db

GET /api/v1/notifications
  -> Gateway -> gRPC Notification Service -> notification_db

GET /api/v1/analytics/orders/summary
  -> Gateway -> gRPC Analytics Service -> ClickHouse
```

## 5. Services

| Service | รับคำสั่ง/Query | Kafka role | State owner |
|---|---|---|---|
| API Gateway | REST public API | ไม่มี | ไม่มี business DB |
| Identity | Login/Refresh/Logout ผ่าน gRPC | ไม่มี | `identity_db`, Redis session |
| Order | Create/List/Get orders ผ่าน gRPC | Producer + outcome consumer | `order_db` |
| Inventory | List products ผ่าน gRPC | Order consumer + reservation producer | `inventory_db` |
| Payment | Admin payment query ผ่าน gRPC | Reservation consumer + payment producer | `payment_db` |
| Activity | Timeline query ผ่าน gRPC | Consumer ทุก order event | `activity_db` |
| Notification | Inbox/read state ผ่าน gRPC | Consumer user-facing events | `notification_db` |
| Analytics Worker | ไม่มี public RPC | Consumer ทุก order event | เขียน ClickHouse |
| Analytics Service | Summary/Funnel/Timeseries ผ่าน gRPC | ไม่มี | อ่าน ClickHouse |

หนึ่ง PostgreSQL container ใช้ได้ใน local แต่แยก logical database และ migration ownership ต่อ
service เพื่อแสดง database-per-service boundary; production design อาจแยก managed instance ตาม budget

## 6. Order State Machine

```text
PENDING
  +--> STOCK_RESERVED
  |      +--> CONFIRMED
  |      +--> CANCELLED (reason: PAYMENT_DECLINED)
  +--> REJECTED
```

Invariant สำคัญ:

- ห้าม `PENDING -> CONFIRMED` โดยไม่มี `InventoryReserved` และ `PaymentCompleted`
- Outcome event เดิมต้องไม่เปลี่ยน state ซ้ำ
- Payment failure หลัง reserve ต้องนำไปสู่ compensation คืน stock
- `InventoryReleased` เป็นผลของ compensation ไม่ใช่ Order state; Order คงเป็น `CANCELLED`
- Terminal states คือ `CONFIRMED`, `REJECTED`, `CANCELLED`

## 7. In-app Notification

Notification เป็น inbox เฉพาะผู้ใช้ ไม่ใช่ Activity audit:

```text
Kafka event
  -> Notification Service สร้างข้อความแบบ idempotent
  -> notification_db
  -> Frontend polling REST ทุก 5-10 วินาที
  -> Gateway เรียก gRPC ListNotifications/UnreadCount
```

ตัวอย่าง:

- `OrderCreated`: “ได้รับคำสั่งซื้อ ORD-1001 แล้ว”
- `InventoryReserved`: “จองสินค้าเรียบร้อยแล้ว”
- `PaymentCompleted`: “ชำระเงินสำเร็จแล้ว”
- `InventoryRejected`: “สินค้าไม่เพียงพอ”
- `PaymentFailed`: “การชำระเงินไม่สำเร็จ ระบบจะคืน Stock”
- `OrderConfirmed` / `OrderCancelled`: สถานะสุดท้ายของ Order

รอบแรกใช้ polling เพื่อคงขอบเขตเล็ก ไม่มี WebSocket, SSE, Email, SMS หรือ Mobile Push

## 8. Frontend Portfolio Pages

```text
/                         Landing และ architecture story
/login                    Login
/app/products             Product/stock catalog
/app/orders               Orders ของผู้ใช้
/app/orders/:id           Order detail + state + timeline
/app/notifications        Notification inbox
/app/admin/orders         Orders ทั้งระบบ
/app/admin/inventory      Stock/reservations
/app/admin/payments       Payment outcomes
/app/admin/activities     Audit trail
/app/admin/analytics      Order funnel/ยอดขาย
/app/admin/system         Health, Prometheus, Grafana links
```

Navigation มี notification bell และ unread count; frontend role guard เป็น UX เท่านั้น ส่วน
authorization จริงต้องอยู่ที่ Gateway/service

## 9. Demo Scenarios

### Success

```text
OrderCreated -> InventoryReserved -> PaymentCompleted -> OrderConfirmed
```

ผลลัพธ์: Order `CONFIRMED`, stock ลด, payment สำเร็จ, notification/timeline/analytics ครบ

### Out of stock

```text
OrderCreated -> InventoryRejected -> Order REJECTED
```

### Payment declined

```text
OrderCreated -> InventoryReserved -> PaymentFailed
             -> OrderCancelled + InventoryReleaseRequested -> InventoryReleased
```

Payment เป็น deterministic simulator (`success` / `decline`) ไม่มีการรับบัตรหรือเรียก provider จริง

### Consumer outage

หยุด Notification หรือ Analytics Worker แล้วสร้าง Order: core Order flow ต้องไม่ล้ม, event ค้างใน
Kafka, และ consumer ต้อง catch up หลัง restart โดยไม่สร้างข้อมูลซ้ำ

## 10. Stack Map

| Stack | ใช้ทำอะไรใน Use Case |
|---|---|
| Fiber REST | Public API, Swagger, auth middleware และ HTTP/gRPC translation |
| gRPC/Protobuf | Command/query แบบ synchronous ระหว่าง Gateway กับ services และ Order -> Inventory catalog quote |
| Kafka | Durable event stream, fan-out, retry, replay และ backlog |
| PostgreSQL/GORM | Transactional state และ Outbox/processed-event ledgers |
| Redis | Identity session และ cache-aside สำหรับ read ที่เหมาะสม |
| ClickHouse | Order event analytics/funnel/time series |
| Prometheus | Pull operational metrics และ evaluate alerts |
| Grafana | Platform/Event Pipeline dashboards จาก Prometheus และ business dashboard จาก ClickHouse |
| React/TypeScript | Portfolio UI และแสดง eventual workflow ให้ผู้ใช้เห็น |

## 11. Non-goals

- Payment provider หรือข้อมูลบัตรจริง
- Shipping/Fulfillment, Cart, Coupon, Promotion, Tax และหลายสกุลเงิน
- Email/SMS/Push และ WebSocket/SSE
- Kafka exactly-once; ใช้ at-least-once + idempotency
- Kubernetes, service mesh, tracing backend และ schema registry
- Render deploy ก่อน local revision ผ่านทุก acceptance gate

## 12. Success Definition

Revision นี้จะมีคุณค่าเมื่อผู้สัมภาษณ์สามารถเห็นจาก UI และ fault demo ว่า:

1. Create Order ตอบ `PENDING` ผ่าน REST -> gRPC หลัง local transaction
2. Inventory/Payment/Notification/Analytics ทำต่อผ่าน Kafka โดยอิสระ
3. Success, stock rejection และ payment compensation ให้ผล state ถูกต้อง
4. Consumer หยุดได้โดยไม่ทำให้ Order command ล้ม และ catch up หลัง restart
5. Activity, Notification และ Analytics แสดงข้อมูลจาก event pipeline จริง
6. Prometheus/Grafana แสดง lag, outbox, retries, failures และ business funnel

รายละเอียด contract/invariant อยู่ใน [Detailed Design](27_ORDER_PLATFORM_DESIGN.md) และลำดับงานอยู่ใน
[Implementation Plan](28_ORDER_IMPLEMENTATION_PLAN.md)
