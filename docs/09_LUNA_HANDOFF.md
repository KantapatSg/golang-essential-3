# Luna High Handoff

อ่านตามลำดับ: `README.md` -> `00_OVERVIEW_MINDMAP.md` -> `16_IMPLEMENTATION_PHASES.md` -> เอกสาร feature ของ phase ที่กำลังทำ -> `18_DEFINITION_OF_DONE.md`

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
[ ] make test
[ ] make vet
[ ] make build
[ ] docker compose config --quiet
[ ] Login ผ่าน Gateway
[ ] Create/Update/Delete Task ผ่าน gRPC path
[ ] ตรวจ Redis cache/invalidation
[ ] ตรวจ outbox published_at
[ ] ตรวจ Kafka activity event
[ ] ตรวจ admin/member policy
[ ] ตรวจ README และ docs ภาษาไทย
[ ] ตรวจ Frontend/ClickHouse/Prometheus/Grafana ตาม phase ที่เสร็จ
[ ] ตรวจว่า implementation ตรงกับ comment และ diagram
[ ] git status clean
```

## Stop conditions

- ห้ามบอกว่า Project 3 เสร็จเพราะ baseline tests ผ่าน
- ห้าม push GitHub จน Phase 10 checks ผ่าน
- ห้าม deploy หรือสร้าง paid resources ก่อน Hold Point ได้รับการยืนยัน
- อย่าเพิ่ม Kubernetes, service mesh, tracing backend หรือ schema registry จนมี acceptance ใหม่ที่ต้องใช้จริง

Definition of Done ฉบับเต็มอยู่ที่ [18_DEFINITION_OF_DONE.md](18_DEFINITION_OF_DONE.md)
