# Cost, Domain และ URL Plan

ตัวเลขนี้เป็น budget estimate สำหรับเตรียม interview ต้องตรวจราคาและอัตราแลกเปลี่ยนอีกครั้งก่อน deploy จริง

## Interview mode ที่แนะนำ

```text
Frontend              Render Static Site / free URL
Gateway + Services    Render paid instances เปิดเฉพาะช่วงสัมภาษณ์
PostgreSQL            free/trial หรือ paid ขนาดเล็ก
Redis                 free tier สำหรับ demo
Kafka                 Aiven free tier
ClickHouse            ClickHouse Cloud trial
Prometheus/Grafana    Grafana Cloud free หรือ local compose สำหรับสาธิตเต็ม
Domain                ซื้อ .com หนึ่งชื่อและสร้าง subdomain
```

งบเป้าหมายช่วงสัมภาษณ์ประมาณ `700–900 บาท` เมื่อเปิด paid compute ราวหนึ่งสัปดาห์และซื้อ domain ปีแรก หากเปิด architecture เต็มตลอดเดือนอาจอยู่ประมาณ `2,800–3,500 บาท/เดือน` ขึ้นกับ managed services และ bandwidth

## URL plan

```text
https://your-domain.com          Frontend
https://api.your-domain.com      API Gateway
https://grafana.your-domain.com  Grafana (ถ้า self-host)
```

Domain เดียวสร้าง subdomain ได้ ไม่ต้องซื้อสาม domain Render URL และ TLS ใช้ฟรีได้ในช่วงตรวจระบบ

## Cost guardrails

- ใช้ Hobby workspace จนมีเหตุผลต้องอัปเกรด
- เปิด paid services ก่อน interview 2–3 วันและหยุดหลังจบ
- ตั้ง billing notification ทุก provider
- ใช้บัตรที่เปิด online/international purchase และกำหนดวงเงินแยก
- ตรวจ auto-renew ของ domain และ trial end date
- ห้าม deploy Phase 11 จน Phase 10 ผ่านและผู้ใช้ตรวจ estimate ล่าสุด

## ค่าใช้จ่ายที่อาจเพิ่ม

- outbound bandwidth
- persistent disk และ backup
- custom domain mapping ที่เกิน quota
- VAT, foreign exchange และค่าธรรมเนียมบัตร
- Kafka/ClickHouse เมื่อเกิน free/trial limit

