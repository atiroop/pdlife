# Schema Spec — pdlife.app

Data model หลักของระบบ ใช้เป็น source of truth ตอนเขียน migration + GORM model

## กฎเหล็ก (ยึดตาม nhe.one pattern)

- **ห้ามใช้ `AutoMigrate` เด็ดขาด** — ทุกตารางต้องมี migration file แยกใน `migrations/`
  (SQL ล้วน, run เองด้วย `mysql -u USER -p DB_NAME < migrations/xxx.sql`)
- GORM model ทุกตัว **ต้องมี `TableName()` method** ระบุชื่อตารางชัดเจน
- gorm tag ระบุ `column:` ชัดเจนทุก field
- ตาราง: InnoDB, utf8mb4, มี created_at / updated_at เป็นมาตรฐาน
- ชื่อไฟล์ migration: `YYYYMMDD_<description>.sql`

## users

ตาม pattern nhe.one (ตัดส่วนที่ไม่ใช้ เช่น 2FA/OAuth ออกก่อน เพิ่มทีหลังได้):

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK AUTO_INCREMENT | |
| email | VARCHAR(255) UNIQUE | |
| password_hash | VARCHAR(255) | bcrypt hash |
| nickname | VARCHAR(100) | ชื่อเล่น (จาก register step 1) |
| role | ENUM('Admin','Member','Unverified') DEFAULT 'Unverified' | ดู docs/auth_flow_spec.md |
| is_active | TINYINT(1) DEFAULT 1 | |
| email_verified_at | DATETIME NULL | |
| last_login_at | DATETIME NULL | |
| created_at / updated_at | DATETIME | |
| deleted_at | DATETIME NULL | soft delete (gorm.DeletedAt) |

## patient_profiles (1:1 กับ users)

สร้าง/เติมค่าตอน onboarding wizard (register step 2)

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| user_id | BIGINT UNSIGNED UNIQUE | FK → users.id (1:1) |
| treatment_type | ENUM('CAPD','APD','HD') | วิธีการรักษา |
| hospital_name | VARCHAR(255) | โรงพยาบาลที่รักษา |
| coverage_type | ENUM('บัตรทอง','ประกันสังคม','ข้าราชการ','อื่นๆ') | สิทธิการรักษา |
| profile_completed_at | DATETIME NULL | NULL = ยังทำ onboarding ไม่จบ → middleware บล็อก log book |
| created_at / updated_at | DATETIME | |

## email_verifications

ดูรายละเอียดใน [auth_flow_spec.md](auth_flow_spec.md) — token เก็บแบบ hash + expires_at + used_at

## Log book — core + ตารางแยกตาม treatment_type

ออกแบบเป็น **core table เก็บ field ร่วม** ของทุกวิธีรักษา แล้วแยกรายละเอียดเฉพาะทาง
ของแต่ละ treatment_type เป็นตารางลูก (1:1 กับ entry)

### log_entries (core — field ร่วมทุกวิธีรักษา)

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| user_id | BIGINT UNSIGNED | FK → users.id |
| entry_date | DATE | วันที่บันทึก, UNIQUE ร่วมกับ user_id ต่อวัน (ยืนยันตอน implement) |
| weight_kg | DECIMAL(5,2) NULL | น้ำหนักตัว |
| bp_systolic | SMALLINT NULL | ความดันตัวบน |
| bp_diastolic | SMALLINT NULL | ความดันตัวล่าง |
| notes | TEXT NULL | |
| created_at / updated_at | DATETIME | |

### ตารางเฉพาะแต่ละ treatment_type (โครงเตรียมไว้ — รายละเอียด field จะเติมทีหลัง)

- `log_capd_details` — 1:1 กับ log_entries (entry_id UNIQUE FK) — รายละเอียดรอบน้ำยา CAPD ฯลฯ *(TBD)*
- `log_hd_details` — 1:1 กับ log_entries — ข้อมูลรอบฟอกเลือด *(TBD)*

หมายเหตุ: ยังไม่ต้องเขียน migration ของกลุ่ม log book ของ CAPD/HD จนกว่า field
เฉพาะทางจะสรุปเสร็จ — spec นี้แค่ล็อกโครงสร้าง core + child ไว้ก่อน

## APD Log Book (`apd_log_entries`, `apd_prescriptions`)

APD ถูก migrate มาจากระบบเดิม (apd.jocky.website, Prisma/MySQL) ก่อนที่ core
`log_entries` + child pattern ด้านบนจะถูกใช้งานจริง จึงผูกตรงกับ
`patient_profiles.id` แทนที่จะผ่าน `log_entries` — ดู
[migrations/20260708_create_apd_log_book.sql](../migrations/20260708_create_apd_log_book.sql)

### apd_prescriptions (ใบสั่งการรักษา)

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| patient_profile_id | BIGINT UNSIGNED | FK → patient_profiles.id |
| name | VARCHAR(255) | ชื่อโปรไฟล์ |
| solution_bag_1 / solution_bag_2 | VARCHAR(100) | น้ำยาถุงที่ 1/2 |
| total_volume_ml | INT | ปริมาตรรวม |
| therapy_time_minutes | INT | เวลารักษารวม |
| fill_volume_ml | INT | ปริมาตรเติมแต่ละรอบ |
| cycles | INT | จำนวนรอบ |
| dwell_time_minutes | INT | เวลาค้างน้ำยา |
| last_fill_ml | INT NULL | น้ำยาค้างสุดท้าย |
| manual_exchange | TEXT NULL | บันทึกการเปลี่ยนน้ำยาแบบ manual |
| is_default_profile | TINYINT(1) | |
| created_at / updated_at | DATETIME | |

### apd_log_entries (บันทึกรายวัน)

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| patient_profile_id | BIGINT UNSIGNED | FK → patient_profiles.id |
| entry_date | DATE | UNIQUE ร่วมกับ patient_profile_id |
| treatment_start_time | VARCHAR(20) | เวลาเริ่มทำ APD |
| weight_kg | DECIMAL(5,2) | |
| bp_systolic / bp_diastolic | SMALLINT | ความดันตัวบน/ล่าง |
| pulse | SMALLINT | ชีพจร |
| blood_glucose_mg_dl | SMALLINT NULL | น้ำตาลในเลือด |
| i_drain_volume_ml | INT | |
| total_uf_ml | INT | กรอกเอง ไม่ได้คำนวณจาก field อื่น |
| urine_avg_day_ml | INT | ปัสสาวะเฉลี่ย/วัน |
| drainage_appearance | VARCHAR(50) NULL | ลักษณะน้ำยาออก (ใส/เหลืองอ่อน/ขุ่น/มีเส้นไฟบริน/ชมพู-มีเลือดปน/อื่นๆ) |
| remark | TEXT NULL | |
| prescription_id | BIGINT UNSIGNED NULL | FK → apd_prescriptions.id (ON DELETE SET NULL) |
| created_at / updated_at | DATETIME | |

ค่าเฉลี่ย 7 วัน (Total UF, น้ำหนัก) = arithmetic mean ธรรมดาของ record ที่ไม่ null
ใน 7 วันปฏิทินล่าสุด ตาม pattern ของระบบเดิม
