# ภาพรวมและ Mind Map

`golang-essential-3` ต่อจาก Project 2 โดยคง Microservices/gRPC/CQRS/Kafka เดิม แล้วเพิ่ม Frontend, analytical data path และ observability ที่สาธิตผ่านเว็บไซต์ได้

## สถานะที่ต้องอ่านก่อน

```text
มีใน baseline แล้ว     Gateway, Identity, Task, Activity, PostgreSQL, Redis, Kafka
ออกแบบแล้วแต่ยังไม่มี  Frontend, Analytics API/Worker, ClickHouse, Prometheus, Grafana
ทำภายหลัง             GitHub public portfolio และ Render deployment
```

## Target architecture

```text
                                  USERS
                                    |
                                    v
                         +---------------------+
                         | React Frontend      |
                         | Portfolio Website   |
                         +----------+----------+
                                    |
                                    v
                         +---------------------+
                         | Fiber API Gateway   |
                         | REST + Swagger      |
                         +----------+----------+
                                    |
                         JWT + Request ID + Deadline
                                    |
             +----------------------+-----------------------+
             |                      |                       |
             v                      v                       v
      Identity Service        Task Service          Activity Service
          gRPC                    gRPC                    gRPC
             |                      |                       |
      PostgreSQL + Redis     PostgreSQL R/W + Redis   PostgreSQL Read Model
                                    |
                          Task + Outbox Transaction
                                    |
                                    v
                                  Kafka
                                    |
                   +----------------+----------------+
                   |                                 |
                   v                                 v
          Activity Consumer                 Analytics Worker
                                                     |
                                                     v
                                                 ClickHouse
                                                     |
                                                     v
                                          Analytics Service gRPC

All Go Services --> Prometheus --> Grafana
ClickHouse -----------------------> Grafana Business Dashboard
```

## Mind map สำหรับทบทวน

```text
Golang Essential 3
├── User Experience
│   ├── React + TypeScript
│   ├── Login / protected routes
│   ├── Task Board
│   ├── Activity Timeline
│   └── Analytics Dashboard
├── Public Edge
│   ├── Fiber REST
│   ├── Swagger/OpenAPI
│   ├── JWT + RBAC
│   └── HTTP ↔ gRPC mapping
├── Microservices
│   ├── Identity
│   ├── Task
│   ├── Activity
│   └── Analytics API/Worker
├── Data
│   ├── PostgreSQL OLTP
│   ├── CQRS reader/writer
│   ├── Redis cache/session
│   └── ClickHouse OLAP
├── Event-driven
│   ├── Transactional Outbox
│   ├── Kafka consumer groups
│   ├── At-least-once + idempotency
│   ├── Retry / DLQ
│   └── Eventual consistency
├── Observability
│   ├── Live / Ready / Metrics
│   ├── Prometheus
│   ├── Grafana
│   └── Alerts + Runbooks
└── Delivery
    ├── Unit / Integration / E2E
    ├── Docker Compose
    ├── GitHub Actions
    └── Render + Domain
```

## Flow ที่ต้องจำ

```text
Synchronous:
Browser -> React -> REST Gateway -> gRPC -> PostgreSQL/Redis -> Response

Reliable asynchronous:
Task + Outbox -> Kafka -> Activity PostgreSQL + Analytics ClickHouse

Observability:
Service metrics -> Prometheus -> Grafana -> Alert/Runbook
```

