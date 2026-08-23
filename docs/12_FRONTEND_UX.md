# Frontend UX และ Use Cases

Frontend ของ Project 3 เป็น portfolio website ที่ทำให้ผู้สัมภาษณ์เห็นว่า backend แต่ละส่วนสร้างประโยชน์กับผู้ใช้อย่างไร ไม่ใช่ dashboard ที่มีกราฟโดยไม่มี business flow

## Stack เป้าหมาย

- React + TypeScript + Vite
- React Router สำหรับ route และ protected route
- TanStack Query สำหรับ server state, loading, retry และ invalidation
- Tailwind CSS สำหรับ design system ขนาดเล็ก
- Recharts สำหรับ business analytics
- Vitest + React Testing Library สำหรับ component tests
- Playwright สำหรับ critical end-to-end flows

ไม่เพิ่ม state library แยกในรอบแรก ใช้ component state, context สำหรับ auth และ TanStack Query สำหรับข้อมูลจาก server

## Information architecture

```text
Public
├── /                 Portfolio landing + architecture summary
├── /login            Login form
└── /api-docs         Link ไป Swagger ของ Gateway

Authenticated member/admin
├── /app              Overview
├── /app/tasks        Task board + CRUD
└── /app/profile      Current identity/session information

Admin only
├── /app/activities   Activity timeline
├── /app/analytics    ClickHouse business analytics
└── /app/system       Service health + Grafana link
```

## Persona และสิทธิ์

| Persona | ทำได้ | ทำไม่ได้ |
|---|---|---|
| Visitor | ดู landing, architecture, GitHub และ login | ดูข้อมูลระบบ |
| Member | CRUD Task ของตนเอง, ดู summary ของตนเอง | ดู Task/Activity ทั้งระบบ |
| Admin | ดู Task ทั้งหมด, Activity, Analytics และ System page | แก้ credential จาก frontend รอบแรก |

Frontend ซ่อนเมนูตาม role เพื่อ UX แต่ backend ต้องตรวจ authorization ซ้ำเสมอ การซ่อนปุ่มไม่ใช่ security boundary

## Critical flows

### Login

```text
Login Form -> POST /api/v1/auth/login -> Gateway -> Identity gRPC
           <- access token + refresh session
           -> Auth context -> redirect /app
```

เป้าหมาย production คือเก็บ access token ใน memory และ refresh token ใน `HttpOnly Secure` cookie เพื่อไม่ให้ JavaScript อ่าน refresh credential ได้ Baseline Project 2 ส่ง refresh token ใน JSON ดังนั้น Phase 2 ต้องออกแบบ transition และทดสอบ CORS/cookie ให้ครบ

### Task Board

```text
Task Board -> GET /api/v1/tasks -> Redis cache หรือ Reader DB
Create/Edit/Delete -> Writer DB + Outbox -> invalidate query cache
UI optimistic state ใช้เฉพาะเมื่อ rollback error ชัดเจน
```

### Analytics

```text
Analytics Page -> Gateway -> Analytics gRPC -> ClickHouse
               <- summary + timeseries + status breakdown
```

หน้า Analytics ต้องแสดง `last updated` และคำว่า eventual consistency เพราะ event อาจยังเดินทางผ่าน Outbox/Kafka/Worker ไม่ครบในทันที

## Page acceptance

- ทุกหน้ามี loading, empty, error และ success state
- Layout ใช้งานได้ตั้งแต่ mobile 360px ถึง desktop
- Keyboard focus และ form label มองเห็นได้
- Member เปิด admin route ได้ผลเป็น 403 page ไม่ใช่ข้อมูลว่าง
- Refresh browser แล้วยังสร้าง session ใหม่ได้ตาม refresh flow
- ไม่แสดง raw stack trace, token หรือ secret ใน UI
