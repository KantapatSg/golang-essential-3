# Local verification record

วันที่ตรวจ: 2026-08-23 (Asia/Bangkok)

ผ่าน:

- backend `go test ./...` ในทุก module — ผ่าน
- `frontend npm run lint`, `npm test -- --run` — ผ่าน (5 tests)
- `frontend npx playwright test` — ผ่าน (landing/login Chromium E2E; captured screenshot in `frontend/test-results/`)
- `scripts/vet.ps1` — ผ่าน
- `scripts/build.ps1` — Go services และ Vite production build ผ่าน
- `docker compose -f deploy/docker-compose.yml config --quiet` — ผ่าน
- `Invoke-WebRequest http://127.0.0.1:5173/` — Vite dev server ตอบ HTTP 200
- `gh auth status` — authenticated user `KantapatSg`, scopes `repo`, `workflow`

ยังไม่ได้อ้างว่าผ่าน:

- `docker compose up --build`, clean-volume smoke และ Kafka → ClickHouse ingestion ยังไม่ผ่านการรันจริง: Docker Desktop Server 27.5.1 ตอบ `read-only file system`/HTTP 500 while pulling images and remained unavailable after a bounded restart. `docker compose config --quiet` ผ่าน; no runtime integration pass is claimed.
- GitHub public repository creation/push จึงรอหลัง Compose provider check สำเร็จตาม Phase 10 gate

ไม่มี secret, `.env`, private key หรือ paid cloud resource ถูกสร้างโดยงานนี้
