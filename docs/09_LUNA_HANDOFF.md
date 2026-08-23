# Luna High Handoff

อ่านตามลำดับ: `README.md` -> `23_MASTER_BLUEPRINT.md` ->
`25_PHASE_CONTEXT_RESUME.md` -> `16_IMPLEMENTATION_PHASES.md` ->
เอกสาร feature ของ phase ที่กำลังทำ -> `24_TEST_ACCEPTANCE_MATRIX.md` ->
`18_DEFINITION_OF_DONE.md`

ห้าม infer สถานะจากการมีไฟล์ใน repository ให้ใช้สามคำนี้เสมอ:

- `Implemented` = มี source/config แล้ว
- `Verified` = มีคำสั่งและผลทดสอบยืนยัน
- `Done` = acceptance ของ phase ผ่านครบและเอกสารอัปเดตแล้ว

## Implementation Mission

Implement และ verify G1–G9 จาก `23_MASTER_BLUEPRINT.md` ตาม phase order โดยใช้ test ID
จาก `24_TEST_ACCEPTANCE_MATRIX.md` ทุก behavior change ต้องมี focused regression test,
comment เฉพาะ decision/invariant/failure boundary และ evidence ใน
`25_PHASE_CONTEXT_RESUME.md` จากนั้นทำ Phase 10 public GitHub/CI/tag แล้วหยุดก่อน Render
Phase 11 ห้าม re-design architecture หรือเพิ่ม stack นอก scope ถ้า acceptance เดิมยังไม่ต้องใช้

## จุดที่ต้องรักษาไว้

- `contracts/proto` เป็น source of truth ของ gRPC
- `cmd/*/main.go` เป็น composition root ของแต่ละ service
- Service อื่นห้าม import business code ข้าม bounded context
- Task และ Outbox ต้อง commit ใน transaction เดียวกัน
- Activity consumer ต้อง idempotent ด้วย database key
- Gateway เป็น public edge; service/infrastructure ไม่ควรเปิดสู่ public network
- graceful shutdown ต้องยกเลิก context และหยุด gRPC/Kafka workers
- PostgreSQL เป็น transactional source of truth; ClickHouse เป็น analytical projection
- Prometheus metric label ห้ามมี user/task/event/email
- Frontend role guard เป็น UX; backend ต้องตรวจ authorization ซ้ำ
- Comment ต้องอธิบาย decision/invariant/failure ตาม `17_COMMENTING_GUIDE.md`

## Verification checklist

```text
[x] make test (scripts/test.ps1)
[x] make vet (scripts/vet.ps1)
[x] make build (scripts/build.ps1)
[x] docker compose config --quiet
[x] Login ผ่าน Gateway
[x] Create/Update/Delete Task ผ่าน gRPC path
[x] ตรวจ Redis cache/invalidation
[x] ตรวจ outbox published_at
[x] ตรวจ Kafka activity event
[x] ตรวจ admin/member policy
[x] ตรวจ README และ docs ภาษาไทย
[x] ตรวจ Frontend/ClickHouse/Prometheus/Grafana ตาม phase ที่เสร็จ
[ ] ตรวจ production frontend route `/api` และ `/openapi.yaml` ผ่าน Nginx/Render จริง
[x] ตรวจ Swagger UI และ OpenAPI ครบทุก public endpoint
[x] ตรวจ readiness ตาม dependency ไม่ใช่ตอบ ready แบบคงที่
[x] ตรวจ ClickHouse duplicate event ไม่ทำให้ aggregate นับซ้ำ
[x] ตรวจว่า implementation ตรงกับ comment และ diagram
[x] git status clean ก่อน tag closeout
```

## Stop conditions

- ห้ามบอกว่า Project 3 เสร็จเพราะ baseline tests ผ่าน
- ห้าม push GitHub จน Phase 10 checks ผ่าน
- ห้าม deploy หรือสร้าง paid resources ก่อน Hold Point ได้รับการยืนยัน
- อย่าเพิ่ม Kubernetes, service mesh, tracing backend หรือ schema registry จนมี acceptance ใหม่ที่ต้องใช้จริง

Definition of Done ฉบับเต็มอยู่ที่ [18_DEFINITION_OF_DONE.md](18_DEFINITION_OF_DONE.md)
