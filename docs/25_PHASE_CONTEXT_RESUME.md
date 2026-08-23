# Phase Context and Resume Ledger

ไฟล์นี้เป็น checkpoint สำหรับกลับมาทำงานต่อเมื่อ Codex limit/token หมดหรือเปลี่ยน agent
ให้อัปเดตทุกครั้งที่ phase เปลี่ยนสถานะหรือมีหลักฐาน runtime ใหม่ ห้ามเปลี่ยน `Pending` เป็น
`Verified` เพียงเพราะมี source file

## Checkpoint

```text
Updated: 2026-08-23 23:20 GMT+7
Branch: main
HEAD: working tree (pre-Phase-10 commit)
Design package: 100% implementation-ready
Implementation: G1-G9 verified locally; Phase 10 in progress
Deploy: not started; Phase 11 hold remains active
```

Working tree ณ checkpoint ก่อน design update:

```text
M  deploy/docker-compose.yml
?? deploy/keygen.Dockerfile
?? services/identity-service/cmd/keygen/main.go
```

ไฟล์เหล่านี้เป็น runtime fixes ที่ตั้งใจเก็บ: Kafka health timeout, ClickHouse base image และ
one-shot shared JWT key generation ห้ามลบทิ้งหรือ overwrite โดยไม่อ่าน diff

## Status Vocabulary

| State | ความหมาย |
|---|---|
| Planned | scope/acceptance ถูกออกแบบแล้ว แต่ยังไม่มี source |
| Implemented | มี source/config แต่ยังไม่มีหลักฐานครบ |
| Partially verified | บาง test ผ่าน ยังเหลือ gate |
| Verified | acceptance ของ phase ผ่านพร้อม evidence |
| Blocked | มี external blocker ระบุชัด |
| Hold | ตั้งใจหยุดรอ authorization |

## Current Phase Snapshot

| Phase | Context/Goal | Current state | หลักฐานที่มี | สิ่งที่ต้องทำต่อ |
|---|---|---|---|---|
| 0 Baseline | แยก identity ของ Project 3 | Verified | test/vet/build/compose config, commit `5ab3266` | ไม่แก้ยกเว้น regression |
| 1 Backend hardening | gRPC, CRUD, auth, migration, Swagger | Verified | T-API-01, T-AUTH-04, Go tests และ smoke ผ่าน | browser cookie negative path เป็น follow-up |
| 2 Frontend/session | Portfolio UI + secure browser flow | Verified | T-WEB-02/T-WEB-03, lint/build/Playwright ผ่าน | full cookie rotation matrix เป็น follow-up |
| 3 Analytics contract/schema | Proto + ClickHouse bounded model | Verified | T-AN-01/T-AN-02, OpenAPI validation, clean ClickHouse migration | fault-injection matrix เป็น follow-up |
| 4 Analytics worker | Kafka -> ClickHouse + retry/DLQ | Verified | T-EVENT-01 และ compose projection `events=2` | DLQ/outage replay แยก scenario เป็น follow-up |
| 5 Analytics UI/API | Query + admin charts | Verified | admin analytics ผ่าน Gateway -> ClickHouse ใน Compose | panel data assertion เชิง API เป็น follow-up |
| 6 Prometheus | health/readiness/metrics | Verified | T-OBS-01..05, Prometheus targets all UP | alert firing จริงเป็น follow-up |
| 7 Grafana | dashboards/alerts/runbook | Verified | Grafana 11.5.2 `/api/health` ok, provisioning files loaded | panel query assertion เป็น follow-up |
| 8 Quality/security | failure tests + dependency security | Partially verified | T-GO/T-FE/T-E2E ผ่าน, npm high/critical ผ่าน | 2 moderate, race ต้อง runner เปิด CGO |
| 9 Production-like local | clean Compose + persistence | Verified | T-COMPOSE/T-RESTART, `compose acceptance ok events=2`, restart `2->2` | ไม่มี local blocker |
| 10 GitHub readiness | public portfolio repo/CI/tag | In progress | local tree/evidence พร้อม, `gh` login พร้อม | commit, public repo, CI green, pre-deploy tag |
| 11 Render | public deployment | Hold | design only | ทำหลังผู้ใช้อนุมัติ budget/provider/env |

## Verified Evidence So Far

```text
PASS 2026-08-23 23:10 GMT+7 scripts/test.ps1 (Go packages + Vitest 5/5)
PASS Go tests across modules
PASS go vet across modules
PASS Go service builds
PASS Frontend Vitest: 5 tests
PASS Frontend lint/typecheck
PASS Frontend production build (มี bundle-size warning)
PASS Playwright isolated landing/login: 1 test
PASS docker compose config --quiet
PASS Runtime login -> JWT -> create/get Task smoke
PASS Kafka task.created observed
PASS Activity consumer group offset=log-end, lag=0 ณรอบที่ตรวจ
PASS T-API-01 OpenAPI YAML validated by swagger-cli; Swagger UI route served
PASS T-AUTH-04 production key fail-fast tests; DEV key fallback isolated
PASS T-WEB-03 Nginx frontend landing + `/openapi.yaml` proxy
PASS T-AN-02 Kafka -> ClickHouse projection and bounded analytics summary
PASS T-OBS-03 Prometheus active targets all UP
PASS T-OBS-08 Grafana health + ClickHouse datasource/dashboard provisioning
PASS T-COMPOSE clean-volume Compose acceptance: `events=4` (รวม ClickHouse datasource provisioning)
PASS T-RESTART task/analytics restart preserved ClickHouse rows: `events=2->2`
```

ยังไม่ Verified:

```text
Analytics duplicate/DLQ/outage/replay fault injection scenarios
Grafana panel query and alert firing assertions
Cookie rotation/logout browser session and Redis failure matrix
Dependency audit remediation (2 moderate React Router advisories)
Race suite in CGO-enabled runner
GitHub clean-clone CI/tag
Render deployment
```

## Exact Resume Point

Luna High ให้เริ่มจากลำดับนี้เท่านั้น:

1. อ่าน `git status`, `git diff` และสาม uncommitted runtime fix; ห้าม discard
2. อ่าน G1–G9 ใน `23_MASTER_BLUEPRINT.md`
3. ทำ Phase 1/2 gap ที่เกี่ยวกับ Swagger, cookie, production proxy และ key fail-fast
4. ทำ Phase 3/4 ClickHouse immediate dedup + integration test
5. ทำ Phase 6/7 readiness/Prometheus/Grafana/Kafka lag compatibility
6. ทำ Phase 8 dependency/failure/race gates
7. รัน one-pass Phase 9 และบันทึก test IDs จาก `24_TEST_ACCEPTANCE_MATRIX.md`
8. อัปเดต `16_IMPLEMENTATION_PHASES.md`, `18_DEFINITION_OF_DONE.md` และไฟล์นี้
9. Commit แบบ phase-scoped แล้วทำ Phase 10 GitHub
10. หยุดก่อน Phase 11

## Bounded Debug Rule

เพื่อป้องกัน engineering loop ในแต่ละ test failure:

1. เก็บ command, exit code และ log เฉพาะช่วงที่เกี่ยวข้อง
2. ตั้งสมมติฐาน root cause ไม่เกิน 2 ข้อ
3. ใช้ read-only check แยกสาเหตุ
4. แก้ที่ responsible layer หนึ่งจุดและเพิ่ม regression test
5. rerun targeted test แล้วจึง full gate
6. หากครบ 2 รอบแล้วยัง fail ให้บันทึก BLOCKED พร้อมหลักฐาน ไม่วน refactor เพิ่ม

## Context Update Template

```markdown
### YYYY-MM-DD HH:mm GMT+7 — <phase/test ID>

- State: Implemented / Partially verified / Verified / Blocked
- Changed: <files/behavior>
- Commands: `<exact commands>`
- Evidence: <assertion/exit code/artifact>
- Remaining: <next smallest bounded step>
- Git: <branch + SHA + dirty files>
```

## Hold Point

เมื่อ Phase 10 ผ่าน ให้สรุป service topology, test evidence, public repository URL,
environment-variable names, estimated monthly/interview cost และ rollback plan แล้วหยุดรอ
คำสั่งผู้ใช้ ห้ามสร้าง paid Render/provider resource, ผูก domain หรือใส่บัตรอัตโนมัติ
