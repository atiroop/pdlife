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

## Email Sending Config (SMTP via Resend)

ใช้ [Resend](https://resend.com) เป็น SMTP provider สำหรับส่งอีเมล verification
(และอีเมลอื่นๆ ในอนาคต เช่น resend-verification, notification)

### Env vars

```
SMTP_HOST=smtp.resend.com
SMTP_PORT=587
SMTP_USER=resend
SMTP_PASSWORD=<Resend API Key, ขึ้นต้นด้วย re_>
```

### หมายเหตุสำคัญสำหรับตอน implement

- **`SMTP_USER=resend` เป็น literal string ตายตัว** ไม่ใช่ email address และไม่ใช่
  username ของบัญชีใดๆ — Resend กำหนดให้ค่านี้เป็นคำว่า `resend` เสมอ ไม่ว่าจะใช้
  project ไหนก็ตาม อย่า validate หรือ parse ค่านี้เป็นรูปแบบอีเมล
- **`SMTP_PASSWORD` คือ API Key ของ Resend** (ไม่ใช่รหัสผ่านจริง) ขึ้นต้นด้วย `re_`
  สร้างและคัดลอกได้จาก Resend dashboard → API Keys
- **Port เลือกใช้ 587 (STARTTLS)** แทน 465 (implicit TLS) เพราะ Go SMTP library
  ส่วนใหญ่ (เช่น `net/smtp`, `gopkg.in/gomail.v2`) รองรับ STARTTLS ได้ตรงไปตรงมากว่า
  ถ้า library ที่เลือกใช้รองรับ implicit TLS ได้ดีอยู่แล้ว จะสลับไปใช้ 465 ก็ได้
  แต่ต้องตั้งค่า TLS mode ให้ตรงกับ port ที่เลือก (465 = implicit, 587 = STARTTLS)
- **From address**: ต้องใช้โดเมนที่ verify กับ Resend แล้วเท่านั้น (เช่น
  `noreply@pdlife.app`) ถ้ายังไม่ได้ verify โดเมนใน Resend dashboard การส่งอีเมล
  จะ fail หรือถูกตีกลับ — ต้องเช็คสถานะ domain verification ก่อน implement จริง
- **Rate limit**: อีเมล verification / resend-verification ควรจำกัดความถี่ต่อ user
  (เช่น ส่งซ้ำได้ไม่เกิน 1 ครั้งต่อ 60 วินาที) กัน spam และกัน Resend account
  โดนระงับจากการส่งเกิน quota