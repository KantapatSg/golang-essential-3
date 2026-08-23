# Troubleshooting

## Gateway ต่อ gRPC ไม่ได้

```powershell
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml logs api-gateway task-service identity-service
```

ตรวจชื่อ service/port ใน environment และรอให้ database/Redis/Kafka healthy ก่อน

## JWT invalid

ตรวจว่า Identity และ Gateway mount named volume `keys` เดียวกัน ถ้า volume มี key เก่าที่ไม่ตรงกับ environment ให้หยุดระบบและลบ volume เฉพาะเมื่อต้องการสร้าง key ใหม่

## Migration/database error

ตรวจว่า database ทั้งสามถูกสร้างจาก `deploy/postgres-init/001_databases.sql` และ DSN ชี้ชื่อถูกต้อง:

```text
identity_db
task_db
activity_db
```

## ไม่มี Buf/Protoc

ใช้ generated fallback stubs ที่ commit ไว้เพื่อ test/build ได้ก่อน เมื่อพร้อมค่อยติดตั้งเครื่องมือและรัน `make proto`

## Kafka event ไม่เข้า Activity

ตรวจ `KAFKA_BROKERS`, topic `task.events.v1`, consumer group `activity-service-v1` และ log ของ outbox publisher ตรวจ `published_at` ใน task database ว่าถูก mark หลัง Kafka ack หรือไม่
