# golang-essential-3

Portfolio project สำหรับเรียนรู้ Go Microservices แบบ end-to-end โดยต่อยอด source code จาก `golang-essential-2` และเพิ่ม Frontend, ClickHouse analytics, Prometheus metrics, Grafana dashboards และแผน deploy บน Render

> สถานะปัจจุบัน: **Order-first R0–R11 ผ่าน local/CI verification; tag `v0.2.1-order-predeploy` สร้างแล้ว; Render ยังหยุดไว้**
>
> Local Compose acceptance ผ่าน Browser/API/Event/ClickHouse/Prometheus/Grafana แล้ว โดยไม่ลบ
> volumes; ยังไม่ deploy Render

> **Order revision (R0–R11 locally/CI verified):** เปลี่ยน business use case จาก Task เป็น Order Processing Platform
> เพื่อให้เห็น gRPC synchronous CRUD และ Kafka asynchronous workflow ชัดขึ้น พร้อม Inventory,
> Payment, Activity, in-app Notification และ ClickHouse Analytics ดู [Order Platform Overview](docs/26_ORDER_PLATFORM_OVERVIEW.md) และ
> [Order Implementation Plan](docs/28_ORDER_IMPLEMENTATION_PLAN.md)

## เป้าหมายระบบ

```text
Browser
  |
  +--> React Frontend ------------------------------------+
  |                                                       |
  +--> Fiber API Gateway --> gRPC Services                |
             |                |                            |
             |                +--> PostgreSQL (CQRS)      |
             |                +--> Redis                  |
             |                +--> Outbox --> Kafka ------+--> Activity Service
             |                                           |
             +--> Analytics gRPC API                     +--> Analytics Worker
                                                              |
                                                              v
                                                          ClickHouse

Every Go service --> Prometheus --> Grafana
Grafana ---------------------------> ClickHouse datasource
```

## สิ่งที่รับมาจาก Project 2

- Fiber REST API Gateway และ Swagger/OpenAPI baseline
- Identity, Task และ Activity gRPC services
- PostgreSQL + GORM, read/write DSN และ Redis cache/session
- JWT RS256 และ role `admin` / `member`
- Transactional Outbox, Kafka และ idempotent Activity consumer
- Unit tests, Docker Compose และเอกสารภาษาไทย
- Comment ในจุดเรียนรู้สำคัญ เช่น CQRS, cache-aside, outbox, deadline และ idempotency

## สิ่งที่ Project 3 เพิ่มแล้ว (local)

- React + TypeScript frontend สำหรับ Order Relay: Login, Catalog, Orders, Notifications และ Admin operations
- Analytics API/Worker และ ClickHouse schema
- Prometheus metrics, health/readiness endpoints และ Grafana dashboards
- Frontend unit/component/E2E tests
- CI pipeline, dashboards, alerts/runbook และ production-like local environment
- เอกสาร interview, cost และ use-case flow ที่ครอบคลุม Browser ถึง ClickHouse/Grafana

หลักฐานล่าสุดอยู่ใน [Local Verification](docs/22_LOCAL_VERIFICATION.md) และ
[Phase Context & Resume](docs/25_PHASE_CONTEXT_RESUME.md); moderate dependency advisories และ
race test ที่ต้องใช้ CGO ถูกบันทึกเป็น follow-up ไม่ปกปิดเป็น pass

## Order Platform Revision (R0–R11 locally/CI verified; pre-deploy tag created; Render held)

Revision ใหม่จะรักษา REST Gateway เป็น public edge และใช้ gRPC สำหรับ synchronous call ภายใน
ส่วนการสร้าง Order จะตอบ `PENDING` หลัง transaction ของ Order + Outbox แล้วให้ Kafka กระจายงาน
ไป Inventory และ Payment แบบ asynchronous จากนั้น Activity, Notification และ Analytics จะสร้าง
projection ของตนเองโดยไม่ block request แรก

```text
Browser -> REST Gateway -> gRPC Order -> PostgreSQL + Outbox -> PENDING
                                      |
                                      v
                                    Kafka
                   +------------------+------------------+
                   v                  v                  v
               Inventory          Activity         Notification
                   |
                   v
                Payment -> Order final state -> ClickHouse Analytics

All services -> Prometheus -> Grafana
ClickHouse -----------------> Grafana business dashboards
```

Use cases หลักคือ happy path, out-of-stock และ payment-declined พร้อม stock compensation
และ in-app notification ขอบเขต/API/state/event/failure behavior อยู่ใน
[Detailed Design](docs/27_ORDER_PLATFORM_DESIGN.md) ส่วนสถานะและจุดกลับมาทำต่ออยู่ใน
[Order Revision Handoff](docs/29_ORDER_REVISION_HANDOFF.md)

## ตรวจ stable Task baseline ก่อนเริ่ม Order revision

```powershell
make test
make vet
make build
docker compose -f deploy/docker-compose.yml config --quiet
```

คำสั่งนี้พิสูจน์ Task baseline ปัจจุบันเท่านั้น Order revision ใช้ R0-R8 evidence ใน
[Order Revision Evidence](docs/30_ORDER_REVISION_EVIDENCE.md)

## คำสั่งตรวจทั้งหมด

```powershell
make test
make vet
make build
make compose-config
make frontend-lint
make frontend-test
make frontend-build
docker compose -f deploy/docker-compose.yml up --build
./scripts/smoke-test.ps1
```

Frontend อยู่ที่ `http://localhost:3000`, Gateway ที่ `http://localhost:8080` และ Grafana ที่ `http://localhost:3001` เมื่อ Compose ทำงาน Demo accounts คือ `member@example.com/member123` และ `admin@example.com/admin123` (local only)

## เอกสารหลัก

| เอกสาร | ใช้ตอบคำถามอะไร |
|---|---|
| [Overview Mind Map](docs/00_OVERVIEW_MINDMAP.md) | ระบบทั้งหมดมีอะไรและข้อมูลไหลอย่างไร |
| [Project Comparison](docs/01_COMPARISON_WITH_V1.md) | Project 1, 2 และ 3 ต่างกันอย่างไร |
| [Infrastructure](docs/02_INFRASTRUCTURE.md) | container, network และ data store แบ่งอย่างไร |
| [Frontend UX](docs/12_FRONTEND_UX.md) | หน้าเว็บและ role แต่ละแบบทำอะไรได้ |
| [ClickHouse Analytics](docs/13_CLICKHOUSE_ANALYTICS.md) | event เข้า analytics และ query อย่างไร |
| [Observability](docs/14_OBSERVABILITY.md) | Prometheus และ Grafana วัดอะไร |
| [Cost, Domain, URL](docs/15_COST_DOMAIN_URL.md) | ต้องเตรียมค่าใช้จ่ายเท่าไร |
| [Implementation Phases](docs/16_IMPLEMENTATION_PHASES.md) | Phase ที่ใช้สร้าง Task release เดิม (historical evidence) |
| [Commenting Guide](docs/17_COMMENTING_GUIDE.md) | จุดใดต้องมี comment และควรอธิบายแบบไหน |
| [Definition of Done](docs/18_DEFINITION_OF_DONE.md) | เกณฑ์ที่ Task release เดิมผ่านแล้ว |
| [Interview Guide](docs/19_INTERVIEW_GUIDE.md) | ใช้อธิบาย architecture และ trade-off อย่างไร |
| [Master Blueprint](docs/23_MASTER_BLUEPRINT.md) | Architecture ของ Task release เดิม |
| [Test & Acceptance Matrix](docs/24_TEST_ACCEPTANCE_MATRIX.md) | Test/evidence ของ Task release เดิม |
| [Phase Context & Resume](docs/25_PHASE_CONTEXT_RESUME.md) | Evidence ledger ของ Task release และ pointer ไป Order resume |
| [Order Platform Overview](docs/26_ORDER_PLATFORM_OVERVIEW.md) | Order use case ใหม่และ sync/async flow ทำงานอย่างไร |
| [Order Platform Design](docs/27_ORDER_PLATFORM_DESIGN.md) | service ownership, API/gRPC/event/schema/state/failure contract คืออะไร |
| [Order Implementation Plan](docs/28_ORDER_IMPLEMENTATION_PLAN.md) | Phase R0-R12 และ acceptance ของ Order revision |
| [Order Revision Handoff](docs/29_ORDER_REVISION_HANDOFF.md) | สถานะ revision และ handoff/rollback rules |
| [Order Revision Evidence](docs/30_ORDER_REVISION_EVIDENCE.md) | ผลทดสอบ R0-R8 และ release handoff evidence |

## กติกาการส่งมอบ

1. Implement และทดสอบ local ให้ผ่านก่อน
2. ตรวจว่าไม่มี secret, `.env`, private key หรือข้อมูลส่วนตัวใน Git
3. Push ไป GitHub เพื่อใช้เป็น portfolio
4. หยุดรอการตรวจค่าใช้จ่ายและ environment variables
5. Deploy Render เมื่อได้รับคำสั่งในขั้น deploy เท่านั้น
