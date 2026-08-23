# Authentication และ Authorization

## Login flow

```text
Client → Gateway → Identity.Login
                 ├── query user จาก identity_db
                 ├── compare password ด้วย bcrypt
                 ├── sign access JWT ด้วย RSA private key
                 └── เก็บ refresh session ใน Redis พร้อม TTL
```

ค่าเริ่มต้นสำหรับเรียนรู้:

```text
access token: 15 นาที
refresh session: 7 วัน
role: admin/member
```

## Token ownership

- Identity Service เท่านั้นที่ถือ private key และสร้าง JWT
- Gateway ใช้ public key ตรวจ signature
- Task Service รับ principal ผ่าน gRPC metadata และตรวจ ownership/role อีกครั้ง
- Refresh token ถูก rotate และ session เก่าถูกลบเมื่อ refresh

## Authorization policy

| Role | สิทธิ์ |
|---|---|
| `member` | เห็น/แก้ไข/ลบ Task ของตัวเอง |
| `admin` | เห็น/แก้ไข/ลบ Task ของทุกคน และอ่าน Activity ทั้งหมด |

`owner_id` ต้องมาจาก JWT principal ไม่รับจาก request body เพื่อป้องกันการปลอมเจ้าของ Task

ค่า password ใน Compose เป็น development defaults เท่านั้น ต้องเปลี่ยนผ่าน environment หรือ secret manager ก่อนใช้งานจริง
