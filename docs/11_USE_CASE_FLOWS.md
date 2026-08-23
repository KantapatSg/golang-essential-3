# Use Case Layer Flows

เอกสารนี้เริ่มจาก flow baseline ที่มีจริงจาก Project 2 และต่อท้ายด้วย target flow ของ Project 3 เพื่อใช้ทบทวน โดยหัวข้อที่ระบุว่า Target ยังไม่ถือว่า implement แล้ว

## 1. ภาพรวมระบบ

```text
                              USERS
                                |
                                v
                    +-----------------------+
                    | API Gateway           |
                    | Fiber / REST / Swagger|
                    +-----------+-----------+
                                |
                     JWT + Request ID + Deadline
                                |
              +-----------------+-----------------+
              |                 |                 |
              v                 v                 v
      Identity Service      Task Service      Activity Service
          gRPC                  gRPC                gRPC
              |                 |                 |
              v                 v                 v
        identity_db          task_db          activity_db
              |                 |
              v                 +---- Redis Task Cache
       Redis Sessions           |
                                +---- Task + Outbox Transaction
                                              |
                                              v
                                            Kafka
                                              |
                                              v
                                      Activity Consumer
                                              |
                                      Idempotency Check
                                              |
                                              v
                                         activity_db
```

Gateway เป็น public edge เพียงจุดเดียว ส่วน service ภายในใช้ gRPC สำหรับงาน synchronous และ Kafka สำหรับงาน asynchronous ที่ผู้ใช้ไม่ต้องรอผลใน request เดิม

## 2. Layer และ Network Boundary

| Layer | หน้าที่ | ตำแหน่งในโค้ด |
|---|---|---|
| Public edge | REST, Swagger, JWT, request ID, deadline และ HTTP error | `services/api-gateway` |
| Contract | Protobuf message, gRPC service และ generated stubs | `contracts` |
| Identity boundary | Login, refresh, logout และ token/session | `services/identity-service` |
| Task boundary | CRUD, ownership, CQRS, cache และ outbox | `services/task-service` |
| Activity boundary | Activity query และ Kafka consumer | `services/activity-service` |
| Transaction storage | Database ที่แต่ละ service เป็นเจ้าของ | PostgreSQL logical databases |
| Shared optimization | Task cache และ refresh session | Redis |
| Event transport | ส่ง Task Event ข้าม service | Kafka `task.events.v1` |

แต่ละ gRPC call ข้าม network จึงต้องคิดเรื่อง timeout, service unavailable, retry และ partial failure เพิ่มจาก Project 1

## 3. Use Case: Login

```text
User
  |
  | POST /api/v1/auth/login
  v
Fiber Gateway
  |
  | LoginRequest ผ่าน gRPC + deadline
  v
Identity Service
  |
  +----> identity_db: Find User
  |
  +----> bcrypt: Compare Password
  |
  +----> RS256: Sign Access Token
  |
  +----> Redis: Store Refresh Session + TTL
  |
  v
Gateway แปลง gRPC Response
  |
  v
Access Token + Refresh Token
```

Gateway ไม่ตรวจ password เอง Identity Service เป็นเจ้าของ credential และ session lifecycle

## 4. Use Case: Refresh และ Logout

```text
Refresh:
Client -> Gateway -> Identity.Refresh
       -> Redis ตรวจ Refresh Session
       -> ออก Access Token/Refresh Token ชุดใหม่

Logout:
Client -> Gateway -> Identity.Logout
       -> Redis ลบ Refresh Session
       -> 204 No Content
```

Redis ใน Identity เป็น security state หากตรวจ session ไม่ได้ต้องตอบ error ไม่ควร fallback แบบ Task Cache

## 5. Use Case: List Tasks แบบ CQRS และ Cache-Aside

```text
Member/Admin
  |
  | GET /api/v1/tasks + Bearer Token
  v
Gateway JWT Middleware
  |
  | gRPC metadata: x-user-id, x-user-role
  v
TaskService.ListTasks
  |
  +----> Redis GET task list scope
           |
           +---- cache hit ----> gRPC Response
           |
           +---- miss/error ---> Reader GORM Connection
                                      |
                                      v
                                   task_db
                                      |
                                      v
                                  Redis SET + TTL
                                      |
                                      v
                                  gRPC Response
```

Member เห็น Task ของตัวเอง ส่วน Admin ใช้ role จาก gRPC metadata เพื่อเข้าถึงข้อมูลตาม policy Reader และ Writer แยก DSN แม้ Local Compose จะชี้ PostgreSQL server เดียวกัน

## 6. Use Case: Create Task พร้อม Transactional Outbox

```text
User
  |
  | POST /api/v1/tasks
  v
Gateway
  |
  | CreateTaskRequest + actor metadata
  v
Task Service
  |
  | Validate actor/input
  v
+---------------- DB TRANSACTION ----------------+
| INSERT tasks                                   |
| INSERT outbox(event_id, task.created, payload) |
+---------------------+---------------------------+
                      |
                    COMMIT
                      |
            Invalidate Redis Cache
                      |
                      v
                Return gRPC Response
                      |
                      v
                HTTP 201 Created

Separate Background Flow:

Outbox Worker
  |
  | SELECT published_at IS NULL
  v
Kafka: task.events.v1
  |
  | ACK สำเร็จ
  v
UPDATE outbox SET published_at = now()
```

Task และ Outbox ต้อง commit ใน transaction เดียวกัน หาก Kafka ล่ม Event ยังอยู่ใน Outbox และ Worker retry ภายหลังได้ ผู้ใช้ไม่ต้องรอ Activity Service ใน request สร้าง Task

## 7. Use Case: Update และ Delete Task

```text
Request + Actor Metadata
          |
          v
TaskService.UpdateTask/DeleteTask
          |
          +----> Reader หา Task ปัจจุบัน
          |
          +----> ตรวจ Owner/Admin Policy
          |          |
          |          +---- ไม่มีสิทธิ์ --> gRPC PermissionDenied
          |
          +----> Transaction
          |       ├── Update/Delete Task
          |       └── Insert Outbox Event
          |
          +----> Invalidate Redis
          |
          v
Gateway แปลง gRPC status เป็น HTTP status
```

Event ที่ได้คือ `task.updated` หรือ `task.deleted` และมี `event_id` สำหรับ idempotency ฝั่ง consumer

## 8. Use Case: Kafka Event ไป Activity Read Model

```text
Task Outbox Worker
        |
        | task.created / task.updated / task.deleted
        v
Kafka topic: task.events.v1
        |
        | consumer group: activity-service-v1
        v
Activity Consumer
        |
+-------------- DB TRANSACTION ---------------+
| INSERT processed_events(event_id)            |
| INSERT activity record                       |
+----------------------+------------------------+
                       |
                     COMMIT
                       |
                Commit Kafka Offset
```

หาก `event_id` มีอยู่แล้ว Consumer ถือว่า Event ถูกประมวลผลไปแล้วและไม่สร้าง Activity ซ้ำ รูปแบบนี้คือ at-least-once delivery ร่วมกับ idempotent consumer ไม่ใช่การอ้างว่าเครือข่ายเป็น exactly-once

## 9. Use Case: Admin ดู Activity Log

```text
Admin
  |
  | GET /api/v1/activities
  v
Gateway ตรวจ JWT
  |
  +---- role != admin ----> 403 Forbidden
  |
  v
ActivityService.ListActivities ผ่าน gRPC
  |
  v
activity_db
  |
  v
Activity Response
```

Gateway ปฏิเสธ non-admin ก่อนยิง RPC และ Activity Service เป็นเจ้าของ read model ที่สร้างจาก Kafka Event

## 10. Deadline และ Error Flow

```text
REST Request
   |
   v
Gateway สร้าง Context Deadline
   |
   v
gRPC Service
   |
   +---- Success ------------> HTTP 2xx
   +---- InvalidArgument ----> HTTP 400
   +---- Unauthenticated ----> HTTP 401
   +---- PermissionDenied ---> HTTP 403
   +---- NotFound -----------> HTTP 404
   +---- DeadlineExceeded ---> HTTP 504
   +---- Unavailable --------> HTTP 503
```

เมื่อ client ยกเลิก request หรือ deadline หมด context ต้องถูกยกเลิกต่อไปยัง database operation เพื่อไม่ให้งานที่ไม่มีผู้รอผลทำต่อโดยไม่จำเป็น

## 11. Synchronous และ Asynchronous Flow

```text
Synchronous — ผู้ใช้ต้องรอผล:
REST -> Gateway -> gRPC -> Service -> DB/Redis -> Response

Asynchronous — ผู้ใช้ไม่ต้องรอผล:
Task Transaction -> Outbox -> Kafka -> Activity Consumer -> Read Model
```

gRPC และ Kafka ไม่ได้ทดแทนกัน gRPC ใช้ขอผลทันที ส่วน Kafka ใช้กระจายเหตุการณ์และลด coupling ระหว่าง service

## 12. สถานะ Implementation ที่ควรรู้

- Gateway ปัจจุบัน expose REST สำหรับ List/Create/Update/Delete Task ส่วน `GetTask` มีใน gRPC contract/service แต่ยังไม่ได้ expose เป็น `GET /api/v1/tasks/:id`
- Repository มี `.proto` แต่ checked-in fallback stubs ใช้ deterministic JSON codec เพื่อให้ทดสอบได้โดยไม่มี `buf/protoc`; Production ควร generate native Protobuf ด้วย `make proto`
- ClickHouse, Prometheus และ Grafana ยังไม่อยู่ใน Flow ปัจจุบัน จึงไม่แสดงเป็น component ที่ implement แล้ว

## 13. Flow ที่ควรจำสำหรับ Interview

```text
Authentication:
REST -> Gateway -> Identity gRPC -> PostgreSQL + Redis -> JWT

Task Query:
REST -> Gateway -> Task gRPC -> Redis -> Reader DB

Reliable Mutation:
REST -> Gateway -> Task gRPC -> Task + Outbox Transaction

Eventual Consistency:
Outbox -> Kafka -> Idempotent Activity Consumer -> Activity Read Model
```

Project 2 เพิ่ม network boundary, database ownership, reliable event และ eventual consistency จากพื้นฐาน Layered Flow ของ Project 1

## 14. Target: Browser ถึง Backend

```text
Browser
  -> React Route + Auth Context
  -> TanStack Query / API Client
  -> Fiber Gateway JWT + Request ID
  -> gRPC metadata + deadline
  -> Identity / Task / Activity / Analytics Service
  -> Response -> UI loading/empty/error/success state
```

Frontend ตรวจ role เพื่อเลือก navigation แต่ service ปลายทางยังต้องตรวจ policy เพราะ client สามารถเรียก API โดยไม่ผ่าน UI ได้

## 15. Target: Task Event เข้า ClickHouse

```text
Create/Update/Delete Task
  -> Task + Outbox PostgreSQL transaction
  -> Outbox Worker -> Kafka task.events.v1
  -> Analytics Worker group analytics-service-v1
  -> Validate + Batch Insert
  -> ClickHouse task_events
  -> Commit Kafka Offset
```

ถ้า ClickHouse ล่ม Worker ไม่ commit offset และ retry ภายหลัง CRUD ยังทำงานได้ แต่หน้า Analytics จะแสดงข้อมูลเก่าชั่วคราว

## 16. Target: Analytics Dashboard

```text
Admin
  -> GET /api/v1/analytics/timeseries?from=&to=
  -> Gateway RBAC
  -> Analytics gRPC with deadline
  -> ClickHouse bounded query
  -> Response includes generated_at/data_through
  -> Frontend chart + eventual-consistency notice
```

## 17. Target: Observability Flow

```text
HTTP/gRPC/DB/Cache/Kafka/ClickHouse operation
  -> low-cardinality metric
  -> /metrics
  -> Prometheus scrape
  -> Grafana Platform/Event Pipeline dashboard
  -> Alert -> Runbook
```

Prometheus เก็บ operational metrics ส่วน Grafana Business dashboard อ่าน ClickHouse โดยตรง ทั้งสองเครื่องมือจึงไม่ได้แทนกัน

## 18. Target: GitHub ก่อน Deploy

```text
Local tests -> Compose smoke -> GitHub Actions -> Secret scan
-> Public portfolio repository -> Pre-deploy tag
-> HOLD: review cost/env -> Render deploy
```

ลำดับ implementation และ acceptance อยู่ใน [16_IMPLEMENTATION_PHASES.md](16_IMPLEMENTATION_PHASES.md)
