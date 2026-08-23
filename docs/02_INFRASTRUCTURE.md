# Infrastructure: Baseline และ Project 3 Target

ส่วนแรกคือ Compose baseline ที่คัดลอกจาก Project 2 ส่วน Target topology ด้านล่างเป็นสิ่งที่ Luna ต้องเพิ่มตาม phase plan

## Local Compose topology

```text
Public host
└── 8080 → api-gateway

Private service network
├── api-gateway → identity-service:50051
├── api-gateway → task-service:50052
└── api-gateway → activity-service:50053

Data network
├── postgres:5432
│   ├── identity_db
│   ├── task_db
│   └── activity_db
├── redis:6379
├── zookeeper:2181
└── kafka:9092
```

## Container responsibilities

| Container | หน้าที่ | Port ที่เปิดให้ host |
|---|---|---:|
| `api-gateway` | REST, Swagger, JWT verification | `8080` |
| `identity-service` | Login/refresh/logout ผ่าน gRPC | `50051` localhost |
| `task-service` | CRUD, CQRS, cache, outbox | `50052` localhost |
| `activity-service` | Kafka consumer และ activity query | `50053` localhost |
| `postgres` | ฐานข้อมูลของแต่ละ bounded context | ไม่จำเป็นต้องเปิด |
| `redis` | session และ cache | ไม่จำเป็นต้องเปิด |
| `kafka` | event transport | ไม่จำเป็นต้องเปิด |

`deploy/postgres-init/001_databases.sql` สร้าง database แยกตอนเริ่ม volume ใหม่ ส่วน migration ของแต่ละ service อยู่ใกล้ code ของ service นั้น

## ต่างจาก Project 1 อย่างไร

Project 1 มี API container เดียวและ infrastructure เป็น dependency ของ API โดยตรง ส่วน Project 2 มี failure domain แยกกัน: Task Service หยุดไม่ได้แปลว่า Gateway หรือ Identity ต้องหยุดด้วย

ใน local compose ใช้ PostgreSQL server เดียวแต่แยก logical database เพื่อประหยัดเครื่อง เมื่อ deploy จริงสามารถเปลี่ยน DSN เป็นคนละ instance/read replica ได้โดยไม่เปลี่ยน business contract

## สิ่งที่ต้องตรวจ

```powershell
docker compose -f deploy/docker-compose.yml config --quiet
docker compose -f deploy/docker-compose.yml up --build
docker compose -f deploy/docker-compose.yml ps
```

`DEV_MODE=true` ใช้ได้เฉพาะตอนรัน service เดี่ยวเพื่อทดสอบง่าย ๆ; compose ปกติใช้ PostgreSQL และ Redis จริง

## Project 3 target topology

```text
Public host
├── 3000/Static -> frontend
└── 8080        -> api-gateway

Private application network
├── identity-service:50051
├── task-service:50052
├── activity-service:50053
├── analytics-service:50054
└── analytics-worker (no public port)

Private data/observability network
├── postgres:5432
├── redis:6379
├── kafka:9092
├── clickhouse:8123/9000
├── prometheus:9090
└── grafana:3000 (local admin access only)
```

## Ownership เป้าหมาย

| Component | เจ้าของข้อมูล/หน้าที่ |
|---|---|
| Identity | user credential และ refresh session |
| Task | Task source of truth, CQRS และ Outbox |
| Activity | PostgreSQL activity read model |
| Analytics Worker | Kafka -> ClickHouse ingestion |
| Analytics Service | bounded analytical query |
| Prometheus | operational time-series metrics |
| Grafana | visualization จาก Prometheus/ClickHouse |

Frontend และ Gateway เป็น public surface ส่วน gRPC, databases, Kafka, Prometheus และ ClickHouse ต้องอยู่ private network เมื่อ deploy
