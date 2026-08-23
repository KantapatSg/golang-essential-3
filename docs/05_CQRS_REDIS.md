# CQRS และ Redis

## Read/write flow

```text
POST/PUT/DELETE → writer DB → transaction สำเร็จ
GET             → Redis cache
                    ├── hit  → return
                    └── miss → reader DB → set cache → return
```

`TASK_DB_WRITE_DSN` และ `TASK_DB_READ_DSN` แยกกันตั้งแต่ contract แม้ local Compose จะชี้ไป PostgreSQL server เดียวกันได้

## Cache keys

```text
tasks:{user_id}  # member list
tasks:all        # admin list
```

หลัง mutation ต้องล้างทั้ง scope ของ owner และ `tasks:all` เพื่อไม่ให้ response เก่าอยู่ต่อ

Redis เป็น optimization ไม่ใช่ source of truth; query ที่ cache miss หรือ Redis error ควรยังอ่าน PostgreSQL ได้ ส่วน session ของ Identity เป็นข้อมูลที่ต้องตอบ error หาก Redis ใช้งานไม่ได้

## จุดที่ควรศึกษา

- `ListTasks`: cache-aside และ reader connection
- `invalidate`: ล้าง cache หลัง command
- `taskServer` fields `writer`, `reader`, `redis`: dependency ที่แยกตามหน้าที่
