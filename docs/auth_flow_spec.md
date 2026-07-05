# Auth Flow Spec — pdlife.app

สรุป auth flow ที่ตกลงกันไว้ ใช้เป็น source of truth ตอน implement
(pattern พื้นฐานยึดตาม nhe.one: Echo + JWT + bcrypt, ทุกตารางมี migration file แยก)

## ภาพรวม

Registration แบ่งเป็น 2 ขั้น คั่นกลางด้วย email verification:

```
Step 1: สมัคร (email / password / ชื่อเล่น)
   → ระบบส่งอีเมล verification (token)
   → user คลิกลิงก์ยืนยันอีเมล
Step 2: Onboarding wizard (ทำได้หลังยืนยันอีเมลแล้วเท่านั้น)
   → treatment_type: CAPD / APD / HD
   → coverage_type (สิทธิการรักษา)
   → hospital_name
   → เสร็จแล้ว set patient_profiles.profile_completed_at = NOW()
```

จนกว่าจะผ่านครบทั้ง 2 ขั้น user จะยังใช้งาน log book ไม่ได้ (ดู Middleware ด้านล่าง)

## Roles

| Role | ความหมาย |
|------|----------|
| `Admin` | ผู้ดูแลระบบ จัดการ user / ข้อมูลทั้งหมด |
| `Member` | user ที่ยืนยันอีเมลแล้ว |
| `Unverified` | สมัคร step 1 แล้วแต่ยังไม่ยืนยันอีเมล |

- สมัครใหม่ role = `Unverified`
- ยืนยันอีเมลสำเร็จ → เปลี่ยนเป็น `Member`
- Role เก็บที่ `users.role`

## Token

- **JWT (access token)**: อายุสั้น ใส่ role + user_id ใน claims ใช้กับทุก API call
- **Refresh token**: อายุยาว ใช้ขอ access token ใหม่ (เก็บฝั่ง server เพื่อให้ revoke ได้)
- Secret จาก env `JWT_SECRET`

## ตาราง email_verifications

เก็บ token สำหรับยืนยันอีเมล **เก็บเฉพาะ hash ของ token** (เช่น SHA-256)
ห้ามเก็บ token ดิบลง DB — token ดิบอยู่ในลิงก์ที่ส่งไปในอีเมลเท่านั้น

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| user_id | BIGINT UNSIGNED | FK → users.id |
| token_hash | CHAR(64) | SHA-256 hex ของ token, UNIQUE |
| expires_at | DATETIME | หมดอายุ (เช่น 24 ชม.) |
| used_at | DATETIME NULL | ใช้แล้วเมื่อไหร่ (กันใช้ซ้ำ) |
| created_at | DATETIME | |

## Endpoints

| Method | Path | หน้าที่ |
|--------|------|---------|
| POST | `/register` | Step 1: รับ email / password / ชื่อเล่น → สร้าง user (role=Unverified) + ส่งอีเมล verification |
| GET | `/verify-email?token=` | hash token ที่รับมา → หา record ที่ยังไม่หมดอายุ/ยังไม่ถูกใช้ → mark used, เปลี่ยน role เป็น Member |
| POST | `/resend-verification` | ส่งอีเมล verification ใหม่ (invalidate token เก่า, ควรมี rate limit) |

หลังจากนั้นคือ endpoint ปกติ: login, refresh, onboarding (step 2) — รายละเอียดจะตามมาตอน implement

## Middleware — บล็อก log book

Route กลุ่ม log book ต้องผ่านเงื่อนไขทั้งหมดนี้:

1. JWT valid
2. `role != Unverified` (ยืนยันอีเมลแล้ว)
3. `patient_profiles.profile_completed_at IS NOT NULL` (ผ่าน onboarding wizard แล้ว)

ถ้าไม่ผ่านข้อ 2 → 403 พร้อม code บอกให้ไปยืนยันอีเมล
ถ้าไม่ผ่านข้อ 3 → 403 พร้อม code บอกให้ไปทำ onboarding ให้จบ
(frontend ใช้ code นี้ redirect ไปหน้าที่ถูกต้อง)
