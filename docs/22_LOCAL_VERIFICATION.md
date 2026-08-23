# Local Verification Record

วันที่ตรวจล่าสุด: 2026-08-23 23:52 (Asia/Bangkok)

เอกสารนี้เก็บผลที่เคยรันจริง ส่วนสถานะ phase และจุด resume อยู่ใน
[25_PHASE_CONTEXT_RESUME.md](25_PHASE_CONTEXT_RESUME.md) การมี source/config ไม่ถือว่าผ่าน
runtime acceptance

## Automated Checks ที่ผ่าน

- `scripts/test.ps1` rerun 2026-08-23 23:10 GMT+7 ผ่าน (Go packages + Vitest 5/5)
- Go tests ทุก module ผ่านผ่าน `scripts/test.ps1`
- `scripts/vet.ps1` ผ่าน
- `scripts/build.ps1` ผ่านทั้ง Go services และ frontend production build (Vite bundle warning เท่านั้น)
- Frontend Vitest ผ่าน 5 tests
- Frontend lint/typecheck ผ่าน
- Playwright isolated landing/login ผ่าน 1 test
- `docker compose -f deploy/docker-compose.yml config --quiet` ผ่าน
- clean-volume Kafka topic initialization + projection startup race ผ่าน (`compose acceptance ok events=1`)
- `npx --yes swagger-cli validate services/api-gateway/cmd/api-gateway/openapi.yaml` ผ่าน
- PowerShell parse ของ `scripts/compose-acceptance.ps1` ผ่าน
- GitHub CLI authenticated user `KantapatSg` ณเวลาที่ตรวจ
- secret grep ไม่พบ private key/token จริง; พบเฉพาะ local example credentials

หมายเหตุ: `npm audit --audit-level=high` ผ่าน high/critical gate แต่ยังมี 2 moderate จาก
React Router 6.x; การแก้เป็น v7 เป็น breaking change จึงบันทึกไว้เป็น follow-up ก่อน production

`go test -race` ยังรันไม่ได้ในเครื่องนี้เพราะ toolchain ปิด CGO (`-race requires cgo`)
ไม่ใช่ผลว่า code มี race และต้อง rerun ใน CI/runner ที่เปิด CGO

## Provider-backed Runtime ที่ผ่าน

หลังแก้ Kafka health timeout, ClickHouse image, shared JWT key init และ ClickHouse HTTP
credentials/DateTime64 boundary:

```text
PASS PostgreSQL, Redis, Kafka, ClickHouse healthy
PASS Login -> RS256 JWT
PASS Create Task -> Get Task
PASS task.created event exists in Kafka topic task.events.v1
PASS Activity consumer group activity-service-v1 reached lag 0
PASS Analytics worker inserted task event into ClickHouse using app credentials
PASS ReplacingMergeTree FINAL query returned projected event
PASS Prometheus targets all UP and Grafana `/api/health` returned database=ok
PASS Frontend landing and Nginx OpenAPI proxy served on port 3000
PASS clean-volume startup and restart persistence (`events=2->2`)
```

Smoke output ที่บันทึกไว้:

```text
smoke ok task=18810784-e9bd-4526-814f-d83d3c6e6abf
compose acceptance ok events=4
restart persistence ok events=2->2
```

## ยังไม่ผ่าน/ยังไม่มีหลักฐานพอ

- Analytics duplicate/DLQ/ClickHouse outage-replay แบบ fault injection ที่แยกเป็น scenario ยังไม่ครบ
- Grafana panel query assertion เชิง API และ alert firing จริงยังเป็น follow-up (provision/runtime health ผ่าน)
- cookie rotation/logout แบบ browser session และ Redis failure matrix ยังไม่มีหลักฐานแยก test ID
- dependency security remediation (2 moderate) และ race suite ใน runner ที่เปิด CGO
- GitHub public repository และ clean-clone CI run `32652675076` ผ่าน; release tag อยู่ใน Phase 10 closeout
- Render deployment

## Runtime Fixes ที่ต้องอยู่ใน commit เดียวกับ implementation

```text
working tree clean ณ `69b72a4`; runtime fixes ถูก commit แล้ว
```

ห้ามลบ runtime fixes ใน `deploy/docker-compose.yml`, `deploy/keygen.Dockerfile` หรือ
`services/identity-service/cmd/keygen/main.go` โดยไม่ตรวจ diff เพราะเป็น fix ที่ทำให้ local
auth/task/analytics smoke ผ่าน

## Next Verification

ใช้ one-pass order และ test ID จาก
[24_TEST_ACCEPTANCE_MATRIX.md](24_TEST_ACCEPTANCE_MATRIX.md) โดยเริ่มจาก targeted tests ของ
G1–G9 ก่อน clean-volume full pass เพื่อไม่วน build Compose โดยไม่มีสมมติฐาน

Acceptance command ที่ผ่านล่าสุด:

```powershell
$env:SKIP_BUILD='1'; pwsh -NoProfile -File scripts/compose-acceptance.ps1
```

ผลล่าสุด: `compose acceptance ok events=1` และ cleanup ด้วย `docker compose down` สำเร็จ
