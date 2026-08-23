# Testing และ Verification

เพราะ repository เป็น `go.work` หลายโมดูล จึงใช้คำสั่งจาก Makefile ไม่ใช่ `go test ./...` ที่ root

```powershell
make test
make vet
make build
docker compose -f deploy/docker-compose.yml config --quiet
```

## Test layers

| Layer | ตัวอย่าง |
|---|---|
| Unit | bcrypt, ownership, validation, idempotency |
| Component | Gateway routes/OpenAPI, gRPC handlers |
| Integration | PostgreSQL migration, Redis, Kafka |
| Smoke/E2E | Login → Create Task → Activity Log |

Smoke test:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/smoke-test.ps1
```

Unit tests ไม่ต้องเปิด infrastructure เพราะใช้ fallback/fake ที่จำเป็น ส่วน Compose path ใช้ PostgreSQL, Redis และ Kafka จริง
