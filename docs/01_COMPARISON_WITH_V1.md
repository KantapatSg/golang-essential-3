# เปรียบเทียบ Project 1, 2 และ 3

## สรุปหนึ่งประโยค

- Project 1: สร้าง API ที่แยก layer ดีใน application เดียว
- Project 2: แยก Microservices และจัดการ network/eventual consistency
- Project 3: เพิ่ม Frontend, Analytics และ Observability เพื่อแสดงระบบแบบ portfolio ที่ใกล้ production

| หัวข้อ | Project 1 | Project 2 | Project 3 Target |
|---|---|---|---|
| รูปแบบ | Modular Monolith | Microservices Monorepo | Microservices + Frontend + Analytics |
| Public entry | Fiber API | Fiber Gateway | React + Fiber Gateway |
| Internal call | Go interface/function | gRPC | gRPC พร้อม metrics/deadline |
| Transaction DB | PostgreSQL | DB ownership ต่อ service | PostgreSQL เหมือนเดิม |
| Analytical DB | ไม่มี | ไม่มี | ClickHouse |
| Cache/session | Redis | Redis แยกหน้าที่ | Redis พร้อม metrics/failure policy |
| Event | Kafka best effort | Outbox + idempotent consumer | หลาย consumer groups + retry/DLQ |
| UI | Swagger | Swagger | Portfolio website + Swagger |
| Observability | logs | health/log baseline | Prometheus + Grafana + alerts |
| Test | unit | unit/gRPC/smoke | unit/integration/contract/E2E/failure |
| Deploy | API image | service images/Compose | GitHub CI + Render + domain |

## Infrastructure ต่างกันอย่างไร

```text
Project 1
Browser -> API process -> PostgreSQL/Redis/Kafka

Project 2
Browser -> Gateway -> Identity/Task/Activity
                   -> PostgreSQL/Redis/Kafka

Project 3
Browser -> React -> Gateway -> Identity/Task/Activity/Analytics
                         |       PostgreSQL/Redis/Kafka/ClickHouse
                         +-----> Prometheus -> Grafana
```

Project 3 ไม่ได้แทน Project 2 แต่เพิ่ม workload สองชนิด: user-facing frontend และ analytical/operational visibility ดังนั้น failure domain, ค่าใช้จ่าย และการทดสอบเพิ่มขึ้นตามไปด้วย

## Trade-off หลัก

- Microservices แยก deploy/scale ได้ แต่มี timeout, partial failure และ tracing/debug complexity
- ClickHouse aggregate event เร็ว แต่ข้อมูลเป็น eventually consistent และไม่ใช่ transactional source of truth
- Prometheus/Grafana ช่วยวินิจฉัยระบบ แต่ต้องควบคุม label cardinality และ retention
- Frontend ทำให้ portfolio เข้าใจง่าย แต่เพิ่ม security boundary, CORS/cookie และ E2E testing

