# golang-essential-3

Portfolio project สำหรับเรียนรู้ Go Microservices แบบ end-to-end โดยต่อยอด source code จาก `golang-essential-2` และเพิ่ม Frontend, ClickHouse analytics, Prometheus metrics, Grafana dashboards และแผน deploy บน Render

> สถานะปัจจุบัน: **G1–G9 ผ่าน local verification; กำลังปิด Phase 10**
>
> Local Compose acceptance ผ่าน Browser/API/Event/ClickHouse/Prometheus/Grafana และ restart
> persistence แล้ว เหลือ public GitHub, clean-clone CI และ pre-deploy tag; ยังไม่เริ่ม Render Phase 11

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

- React + TypeScript frontend สำหรับ Login, Task Board, Activity และ Analytics
- Analytics API/Worker และ ClickHouse schema
- Prometheus metrics, health/readiness endpoints และ Grafana dashboards
- Frontend unit/component/E2E tests
- CI pipeline, dashboards, alerts/runbook และ production-like local environment
- เอกสาร interview, cost และ use-case flow ที่ครอบคลุม Browser ถึง ClickHouse/Grafana

หลักฐานล่าสุดอยู่ใน [Local Verification](docs/22_LOCAL_VERIFICATION.md) และ
[Phase Context & Resume](docs/25_PHASE_CONTEXT_RESUME.md); moderate dependency advisories และ
race test ที่ต้องใช้ CGO ถูกบันทึกเป็น follow-up ไม่ปกปิดเป็น pass

## ตรวจ baseline ก่อนเริ่ม implement

```powershell
make test
make vet
make build
docker compose -f deploy/docker-compose.yml config --quiet
```

Baseline ยังไม่ใช่ Project 3 ที่เสร็จแล้ว การทดสอบผ่านในขั้นนี้พิสูจน์เพียงว่า source ที่รับมาจาก Project 2 ยังทำงานหลัง fork

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
| [Implementation Phases](docs/16_IMPLEMENTATION_PHASES.md) | Luna High ต้องทำงานตามลำดับใด |
| [Commenting Guide](docs/17_COMMENTING_GUIDE.md) | จุดใดต้องมี comment และควรอธิบายแบบไหน |
| [Definition of Done](docs/18_DEFINITION_OF_DONE.md) | เกณฑ์ตัดสินว่า implement เสร็จจริง |
| [Interview Guide](docs/19_INTERVIEW_GUIDE.md) | ใช้อธิบาย architecture และ trade-off อย่างไร |
| [Master Blueprint](docs/23_MASTER_BLUEPRINT.md) | Product overview, architecture, service flow, stack และเหตุผลทั้งหมด |
| [Test & Acceptance Matrix](docs/24_TEST_ACCEPTANCE_MATRIX.md) | แต่ละส่วนต้องทดสอบอะไรและเก็บหลักฐานอย่างไร |
| [Phase Context & Resume](docs/25_PHASE_CONTEXT_RESUME.md) | สถานะจริง จุดค้าง และวิธีกลับมาทำต่อเมื่อ task/token ถูกตัด |

## กติกาการส่งมอบ

1. Implement และทดสอบ local ให้ผ่านก่อน
2. ตรวจว่าไม่มี secret, `.env`, private key หรือข้อมูลส่วนตัวใน Git
3. Push ไป GitHub เพื่อใช้เป็น portfolio
4. หยุดรอการตรวจค่าใช้จ่ายและ environment variables
5. Deploy Render เมื่อได้รับคำสั่งในขั้น deploy เท่านั้น
