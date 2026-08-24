# Test and Acceptance Matrix

> **Historical Task-version matrix:** หลักฐานชุดนี้รองรับ `v0.1.0-predeploy`
> Order revision ใช้ OR-* matrix ใน
> [28_ORDER_IMPLEMENTATION_PLAN.md](28_ORDER_IMPLEMENTATION_PLAN.md)

เอกสารนี้บอกว่า “แต่ละส่วนต้องทดสอบอะไร” เพื่อไม่ใช้คำว่าเสร็จจากการ build ผ่านเพียงอย่างเดียว
รหัส test ใช้อ้างใน PR/commit และ status ledger ได้

## 1. Test Pyramid

```text
                 Browser E2E / Production smoke
              Compose integration / failure tests
          Contract + component + repository integration
       Unit tests: validation, policy, mapping, retry, query
```

Unit test เร็วและชี้ตำแหน่ง bug ส่วน Compose/E2E พิสูจน์ wiring จริง ทั้งสองชั้นแทนกันไม่ได้

## 2. Functional Matrix

| ID | Area | Scenario | Expected evidence |
|---|---|---|---|
| T-AUTH-01 | Login | member/admin valid และ invalid password | 200 + claims ถูกต้อง, invalid เป็น 401 |
| T-AUTH-02 | Refresh | rotate cookie/session และ token เก่าใช้ซ้ำไม่ได้ | Redis session เปลี่ยน, replay ถูกปฏิเสธ |
| T-AUTH-03 | Logout | body ว่างแต่มี cookie | session ถูก revoke, cookie expired, refresh ต่อไม่ได้ |
| T-AUTH-04 | Key security | ไม่มี key ใน production | Identity/Gateway fail fast; DEV fallback แยกชัด |
| T-RBAC-01 | Member | เข้า activity/analytics/admin route | HTTP 403 และไม่มี RPC side effect |
| T-RBAC-02 | Ownership | member อ่าน/แก้ Task ผู้อื่น | service ตอบ PermissionDenied/HTTP 403 |
| T-TASK-01 | CRUD | create/get/list/update/delete | status/body/database ตรง contract |
| T-TASK-02 | Validation | title/status/page/limit ผิด | 400/InvalidArgument แบบสม่ำเสมอ |
| T-CQRS-01 | Read path | cache miss แล้ว hit | miss อ่าน Reader DSN, hit ไม่ query DB ซ้ำ |
| T-CQRS-02 | Mutation | update/delete | Writer commit แล้ว invalidate relevant cache |
| T-CQRS-03 | Redis down | Task query เทียบ Identity refresh | Task fallback DB; refresh fail closed |
| T-OUTBOX-01 | Atomicity | mutation หรือ outbox insert fail | rollback ทั้งคู่ ไม่มี orphan state |
| T-OUTBOX-02 | Kafka down | command สำเร็จ | outbox pending และ publish ได้เมื่อ Kafka กลับมา |
| T-EVENT-01 | Activity duplicate | event_id เดิมสองครั้ง | activity หนึ่งแถว, offset commit หลัง DB success |
| T-EVENT-02 | Invalid event | JSON/schema/type ผิด | DLQ มี structured reason, source offset ตาม policy |
| T-EVENT-03 | Analytics retry | ClickHouse down/up | ไม่ commit ก่อน insert, replay แล้วข้อมูลไม่หาย |
| T-EVENT-04 | Analytics duplicate | event_id เดิม replay | immediate aggregate และหลัง merge ไม่นับซ้ำ |
| T-EVENT-05 | Shutdown | worker มี partial batch | flush ภายใน timeout หรือไม่ commit ข้อมูลที่ยังไม่เขียน |
| T-AN-01 | Query bounds | from/to/limit invalid/เกินเพดาน | InvalidArgument; ไม่มี unbounded scan |
| T-AN-02 | Projection | create/update/delete Task | summary/timeseries/statuses ตรง ClickHouse |
| T-API-01 | OpenAPI | ทุก public route/security/error | contract test และ Swagger UI โหลด spec ได้ |
| T-WEB-01 | Session | reload protected route | refresh cookie restore session; access tokenไม่ลง storage |
| T-WEB-02 | UX states | loading/empty/error/success | component test ครบทุก state |
| T-WEB-03 | Production routing | Nginx/Render URL | SPA deep link, `/api`, `/openapi.yaml`, `/swagger/` ผ่าน |

## 3. Observability Matrix

| ID | Check | Expected |
|---|---|---|
| T-OBS-01 | `/health/live` | ตอบตาม process โดยไม่ query dependency หนัก |
| T-OBS-02 | `/health/ready` | DB/Kafka/ClickHouse ที่จำเป็นล่มแล้ว non-2xx; Redis task cache degraded ยังอธิบายได้ |
| T-OBS-03 | Prometheus targets | Gateway และ Go components ทุกตัวเป็น `UP` |
| T-OBS-04 | Metric format | Prometheus parse ผ่าน มี HELP/TYPE และ counter/histogram semantics ถูกต้อง |
| T-OBS-05 | Cardinality | ไม่มี user/task/event/email/request ID ใน label values |
| T-OBS-06 | Platform dashboard | request rate/error/duration/readiness query มีข้อมูล |
| T-OBS-07 | Event dashboard | outbox pending/fail, processed/fail, DLQ, Kafka lag มีข้อมูล |
| T-OBS-08 | Business dashboard | Grafana query ClickHouse และตรง Frontend analytics |
| T-OBS-09 | Alert | force dependency fail แล้ว rule pending/firing และ runbook link ใช้ได้ |

## 4. Quality, Security และ Delivery Matrix

| ID | Gate | Command/Evidence |
|---|---|---|
| T-GO | Go test/vet/build | `make test`, `make vet`, `make build` |
| T-RACE | Concurrent code | `go test -race` สำหรับ worker/cache/outbox modules ที่รองรับ |
| T-FE | Frontend | `npm ci`, `npm run lint`, `npm test`, `npm run build` |
| T-E2E | Browser | `npm run e2e` กับ production-like route |
| T-COMPOSE | Wiring | `docker compose ... config --quiet`, clean-volume up, smoke |
| T-RESTART | Persistence | restart service/container แล้ว PostgreSQL/ClickHouse/key data ยังอยู่ |
| T-SEC | Dependency/secret | Go vulnerability scan, `npm audit`, gitleaks; ไม่มี real secret |
| T-CI | Clean clone | GitHub Actions backend/frontend/compose/integration green |
| T-DOC | Learning | diagram/comment/status ตรง source และ command copy-run ได้ |

## 5. One-Pass Local Acceptance Run

ใช้ลำดับนี้เพื่อลด engineering loop หาก step ใด fail ให้บันทึก ID, log สั้น ๆ, root cause
และแก้เฉพาะ responsible layer แล้ว rerun จาก step ที่เกี่ยวข้องก่อน full pass

```powershell
make test
make vet
make build

Push-Location frontend
npm ci
npm run lint
npm test
npm run build
npm run e2e
Pop-Location

docker compose -f deploy/docker-compose.yml config --quiet
docker compose -f deploy/docker-compose.yml up --build -d
./scripts/smoke-test.ps1
./scripts/compose-acceptance.ps1
```

จากนั้นตรวจด้วย script/คำสั่ง reproducible ไม่ใช้การมอง UI อย่างเดียว:

1. Role negative paths และ cookie rotation/logout
2. Redis cache hit/invalidation/fallback
3. Outbox pending/published และ Kafka consumer lag
4. Activity idempotency และ Analytics duplicate/DLQ/retry
5. ClickHouse rows/aggregate
6. Prometheus `/api/v1/targets` และ rule state
7. Grafana provisioning API/dashboard datasource
8. Browser production route และ screenshot หลักสำหรับ portfolio

`scripts/compose-acceptance.ps1` จะตรวจ readiness, Swagger/OpenAPI, frontend proxy, Activity,
ClickHouse projection, Prometheus targets และ Grafana health แล้ว cleanup ด้วย `docker compose down`.
สำหรับ rerun ที่ไม่ต้อง rebuild image ใช้ `$env:SKIP_BUILD='1'`; CI ใช้ default full build.

### Latest evidence

```text
Local clean-volume: PASS — compose acceptance ok events=1 (Kafka topic init enabled)
GitHub Actions: PASS — run 32652675076
  backend, frontend, compose+gitleaks และ integration jobs ทั้งหมดเขียว
GitHub Actions tag: PASS — run 32653171124 for `v0.1.0-predeploy`
```

## 6. Evidence Format

บันทึกผลใน `docs/25_PHASE_CONTEXT_RESUME.md` รูปแบบ:

```text
YYYY-MM-DD HH:mm GMT+7 | TEST-ID | PASS/FAIL/BLOCKED
command: <exact command>
evidence: <exit code / key assertion / artifact path>
commit: <sha or working-tree>
note: <only if needed>
```

ห้าม check Definition of Done จากความจำ ต้องมี evidence line หรือ CI URL/commit ที่อ้างกลับได้

## 7. Render Acceptance (Phase 11 เท่านั้น)

- Provider migrations สำเร็จและ rollback plan พร้อม
- Private service address ใช้งานได้จาก Gateway แต่ไม่ public
- Cookie `Secure`, domain, SameSite และ CORS ผ่าน browser จริง
- Public health, login, Task CRUD, eventual Activity/Analytics ผ่าน
- Metrics/logs ไม่เผย secret และมี provider health/alert
- Frontend/API Render URL ทำงานก่อนผูก custom domain
- Custom domain/TLS ผ่านแล้วจึงบันทึก final portfolio URL
- ทดสอบ rollback และคำสั่งปิด resource เพื่อหยุดค่าใช้จ่าย
