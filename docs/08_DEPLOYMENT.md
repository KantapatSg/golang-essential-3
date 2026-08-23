# Deployment Plan

> ยังไม่ deploy ในขั้น design ให้ implement ถึง Phase 10, push GitHub และตรวจค่าใช้จ่ายก่อนเริ่ม Phase 11

## Local

```powershell
docker compose -f deploy/docker-compose.yml config --quiet
docker compose -f deploy/docker-compose.yml up --build
```

Compose จะ build image แยกตาม `SERVICE` argument, สร้าง database แยกสามชุด และแชร์ named volume `keys` ให้ Identity สร้าง RSA key และ Gateway อ่าน public key

## Startup dependencies

PostgreSQL, Redis และ Kafka มี health check ส่วน Gateway/บริการ gRPC มี retry/deadline ใน code เพื่อไม่ผูกความพร้อมกับ container start event เพียงอย่างเดียว

## Production checklist

- ใช้ secret manager แทน password และ RSA key ใน volume
- เปลี่ยน `TASK_DB_READ_DSN` ไป read replica จริง
- ตั้ง Kafka replication factor มากกว่า 1
- เพิ่ม TLS/mTLS ตาม trust boundary
- เพิ่ม metrics, tracing และ centralized logs
- แยก Redis policy ระหว่าง session กับ cache
- ใช้ versioned migration และ backup/restore plan

Kubernetes, service mesh, tracing backend และ schema registry ไม่รวมใน Project 3 รอบแรก เพราะยังไม่จำเป็นต่อ acceptance ของ portfolio

## Target Render topology

```text
Render Static Site  -> React frontend
Render Web Service  -> API Gateway (public)
Render Private      -> Identity / Task / Activity / Analytics
Render Worker       -> Analytics Worker

Managed dependencies ตาม budget:
PostgreSQL / Redis / Kafka / ClickHouse / Grafana Cloud
```

## Environment groups

- `public-config`: frontend origin, public API URL, cookie domain
- `service-addresses`: gRPC private addresses และ metrics ports
- `identity-secrets`: JWT keys, passwords และ refresh settings
- `data-connections`: PostgreSQL/Redis/Kafka/ClickHouse credentials
- `observability`: scrape/auth/dashboard configuration

ห้าม commit value จริงลง Git ใช้เฉพาะชื่อ variable และตัวอย่างที่ไม่ใช่ secret ใน `.env.example`

## Deploy order

1. ตรวจ CI จาก clean clone
2. เตรียม managed data services และ migrations
3. Deploy private services/worker
4. Deploy Gateway และตรวจ health/readiness
5. Deploy Frontend
6. ตั้ง CORS, cookie, custom domain และ TLS
7. รัน production smoke/E2E
8. บันทึก rollback และ resource cleanup

รายละเอียดลำดับงานและ Hold Point อยู่ใน [16_IMPLEMENTATION_PHASES.md](16_IMPLEMENTATION_PHASES.md)
