# gRPC Guide

## Contract location

Proto อยู่ใน `contracts/proto/{identity,task,activity}/v1` และ generated client/server อยู่ใน `contracts/gen/go` เป็น source ที่ทุก service ใช้ร่วมกัน

ถ้าเครื่องมี Buf/Protoc ให้รัน:

```powershell
make proto
```

ใน repository มี generated fallback stubs และ deterministic JSON codec เพื่อให้ unit test ทำงานได้แม้ไม่มีเครื่องมือ generate

## Request flow

```text
Fiber handler
  → สร้าง context deadline 5 วินาที
  → เติม metadata x-request-id
  → เติม x-user-id/x-user-role หลัง JWT ผ่าน
  → เรียก gRPC client
  → แปลง gRPC status เป็น HTTP status
```

## Metadata สำคัญ

| Metadata | ความหมาย |
|---|---|
| `x-request-id` | ใช้ตาม log ข้าม service |
| `x-user-id` | principal ที่ผ่าน Gateway แล้ว |
| `x-user-role` | ใช้ตรวจ policy ใน Task Service |

Task Service ตรวจ metadata ซ้ำที่ service boundary ไม่ควรเชื่อ Gateway เพียงชั้นเดียว

## Status mapping

```text
InvalidArgument    → HTTP 400
Unauthenticated    → HTTP 401
PermissionDenied   → HTTP 403
NotFound           → HTTP 404
DeadlineExceeded   → HTTP 504
Unavailable        → HTTP 503/500 ตาม handler
```

## จุดที่ควรศึกษาใน code

- Gateway: `rpcCtx`, `metadataAppend`, `grpcHTTP`
- Identity/Task: `deadlineInterceptor`
- Contracts: generated `New*ServiceClient` และ `Register*ServiceServer`
