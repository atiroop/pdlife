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
| account_deletion_requested_at | DATETIME NULL | ตั้งจาก `/profile` (ต้องใส่รหัสผ่าน + พิมพ์ "ลบบัญชี" ยืนยัน — ดู `internal/handler/profile.go`'s `ProfileDeleteAccount`) NOT NULL = login ถูกบล็อก (ดู `internal/handler/login.go`) และเป็นเป้าหมายให้ `cmd/purge_deleted_accounts` ลบถาวรหลัง `handler.AccountDeletionGraceDays` (90) วัน |
| terms_accepted_at | DATETIME NULL | ตั้งครั้งเดียวตอนสมัคร ผ่าน scroll-to-accept modal บน `/register` (ดู `internal/handler/auth.go`'s `Register`) — ไม่มีอยู่สำหรับ user เก่าที่สมัครก่อนฟีเจอร์นี้ |
| terms_accepted_version | VARCHAR(50) NULL | `handler.LegalContentUpdatedDate` ณ ตอนที่ยอมรับ — pattern เดียวกับ `patient_profiles.health_data_consent_version`; server เช็คว่าค่าที่ client ส่งมาตรงกับ constant ปัจจุบันเป๊ะก่อนบันทึกเสมอ ไม่เชื่อ client เฉยๆ |
| suspended_at | DATETIME NULL | ตั้ง/ล้างโดย admin จากหน้า `/admin/users/:id` เท่านั้น (ดู `internal/handler/admin_users.go`) NOT NULL = login ถูกบล็อก (จุดเช็คเดียวกับ account_deletion_requested_at) และ security_stamp ถูก rotate + refresh token ถูก revoke ทันทีตอนระงับ (บังคับ logout ทุกอุปกรณ์) |
| suspended_reason | TEXT NULL | เหตุผลที่ admin ต้องกรอกก่อนระงับ (บังคับ) — ล้างเป็น NULL พร้อม suspended_at ตอนยกเลิกการระงับ |
| created_at / updated_at | DATETIME | |
| deleted_at | DATETIME NULL | soft delete (gorm.DeletedAt) — แยกจาก `account_deletion_requested_at` โดยเจตนา: อันนี้คือ GORM ใช้เองตอน `.Delete()` ปกติ (query ปกติจะกรองออกอัตโนมัติ) ส่วน account_deletion_requested_at คือ "รอลบถาวรใน 90 วัน" ที่ user เห็น/ควบคุมได้จากหน้า UI |

### admin_action_logs (audit trail — `migrations/20260719_create_admin_action_logs_and_user_suspension.sql`)

ทุก action ที่ admin ทำต่อบัญชี user คนอื่นต้องมีแถวในตารางนี้เสมอ (เขียนใน transaction
เดียวกับตัว action — ดู `internal/handler/admin_users.go`) หน้า admin ทั้งหมดในฟีเจอร์นี้
เข้าถึงเฉพาะข้อมูลระดับบัญชี **ห้าม join ไปที่ log entries / lab results / food search
history เด็ดขาด**

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK AUTO_INCREMENT | |
| admin_id | BIGINT UNSIGNED FK→users.id | admin ที่ทำ action |
| target_user_id | BIGINT UNSIGNED FK→users.id | user ที่ถูกกระทำ |
| action | ENUM('manual_verify_email','unlock_account','suspend_account','unsuspend_account') | |
| reason | TEXT NULL | บังคับกรอกสำหรับ suspend_account, ที่เหลือ optional |
| created_at | DATETIME DEFAULT CURRENT_TIMESTAMP | |

### บัญชี "ผู้ใช้ที่ถูกลบ" (placeholder ถาวร)

`cmd/purge_deleted_accounts` find-or-create บัญชี `deleted-user@pdlife.app` (nickname
"ผู้ใช้ที่ถูกลบ", role=Unverified, password_hash ไม่ใช่ bcrypt hash จริง — login ไม่ได้เด็ดขาด)
ไว้เป็น author ถาวรสำหรับ `editorial_articles` ที่เคย publish แล้วของ user ที่ถูกลบ — กัน
ลิงก์ `/articles/:slug` สาธารณะพังหลัง purge บทความ draft (ไม่เคย publish) ของ user ที่ถูกลบ
จะถูกลบทิ้งตรงๆ แทน (ไม่มีลิงก์สาธารณะให้ต้องรักษา)

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
| health_data_consent_at | DATETIME NULL | NULL = ยังไม่ได้ยินยอม (หรือถอนความยินยอมแล้ว) → middleware บล็อก APD/CAPD/HD Log Book และ Food Check (แยกจาก `profile_completed_at` โดยเจตนา — ถอนได้อิสระโดยไม่กระทบข้อมูล onboarding อื่น ดู PDPA มาตรา 26) |
| health_data_consent_version | VARCHAR(50) NULL | ตรงกับ `handler.HealthDataConsentVersion` ที่ตอนขอ consent — บอกว่า user ยินยอมภายใต้ privacy policy revision ไหน |
| created_at / updated_at | DATETIME | |

Consent เก็บที่ผ่านทาง 2 ทาง: (1) onboarding form มี checkbox บังคับติ๊กในฟอร์มเดียวกับ
treatment_type/hospital/coverage (`OnboardingSubmit` เซ็ตทุกฟิลด์พร้อมกันครั้งเดียว) — ใช้กับ user
ใหม่ทั้งหมด (2) `/consent` (`ConsentForm`/`ConsentSubmit`) — หน้าแยกสำหรับ user ที่ทำ onboarding
เสร็จไปแล้วก่อนฟีเจอร์นี้จะมี (`profile_completed_at` ไม่ NULL แต่ `health_data_consent_at` ยัง NULL)
ทั้ง `postLoginPath` และ guard ทุกตัว (`requireOnboardedUser`, `requireApdPatient`,
`requireCapdPatient`, `requireLoggedInMember`) เช็ค consent เพิ่มจาก onboarding check เดิม — ยกเว้น
`requireOnboardedUser` (คุม /dashboard, /news, /profile) ที่ไม่บล็อกทั้งหน้า แต่ Dashboard handler
จะซ่อนการ์ดสรุป KPI (ข้อมูลสุขภาพ) ไว้หลัง CTA ให้ไปยินยอมก่อน แทน — เพราะ /dashboard ยังเป็นหน้า
landing รวมข่าว/admin ที่ไม่ควรบล็อกทั้งหน้า. ถอนความยินยอมทำผ่าน `POST /consent/withdraw`
(ปุ่มใน user dropdown menu, ยืนยันด้วย `confirm()` เหมือน delete pattern อื่นในระบบ) — เซ็ต
`health_data_consent_at`/`version` กลับเป็น NULL เท่านั้น ไม่ลบ log entries เดิม.

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

### apd_log_entries (บันทึกรายรอบ — หลายรอบต่อวันได้)

เดิมเป็นบันทึกรายวัน (UNIQUE ต่อ patient+วัน ตามระบบ legacy) แต่ผู้ป่วยจริง
ล้างหลายรอบต่อวัน (ดูสมุดกระดาษตัวอย่าง: 4-5 รอบ/วัน) จึงเพิ่ม `cycle_number`
แบบเดียวกับ CAPD ใน
[migrations/20260720_add_cycle_number_to_apd_log_entries.sql](../migrations/20260720_add_cycle_number_to_apd_log_entries.sql)
— แถวเดิมทั้งหมดกลายเป็นรอบที่ 1 ของวันนั้น KPI/กราฟรวมเป็นรายวัน
(Total UF = ผลรวมทุกรอบของวัน, น้ำหนัก/ความดัน = รอบล่าสุดของวัน — ดู
`handler.aggregateApdDaily`)

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| patient_profile_id | BIGINT UNSIGNED | FK → patient_profiles.id |
| entry_date | DATE | UNIQUE ร่วมกับ patient_profile_id + cycle_number |
| cycle_number | TINYINT UNSIGNED | รอบที่ 1-6 ต่อวัน (default 1) |
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

ค่าเฉลี่ย 7 วัน (Total UF, น้ำหนัก) = arithmetic mean ของค่ารายวัน (รวมทุกรอบ
เป็นรายวันก่อนด้วย `aggregateApdDaily`) ใน 7 วันปฏิทินล่าสุด

## CAPD Log Book (`capd_log_entries`)

New feature — no legacy data to migrate (unlike APD, which was ported from
apd.jocky.website). Also bound directly to `patient_profiles.id` rather
than via the generic `log_entries` core/child pattern above, for
consistency with `apd_log_entries`. See
[migrations/20260710_create_capd_log_book.sql](../migrations/20260710_create_capd_log_book.sql).

Key difference from APD: a CAPD patient logs one row **per exchange
cycle** (typically 1-5/day), not one row per day, so the uniqueness
constraint is `(patient_profile_id, log_date, cycle_number)` instead of
`(patient_profile_id, log_date)`.

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| patient_profile_id | BIGINT UNSIGNED | FK → patient_profiles.id |
| log_date | DATE | |
| cycle_number | TINYINT UNSIGNED | รอบที่ 1-5 ต่อวัน |
| dextrose_concentration | DECIMAL(4,2) | % เดกซ์โทรส (1.5 / 2.5 / 4.25 หรือกรอกเอง) |
| fill_start_time / fill_end_time | VARCHAR(20) | เวลาเติมน้ำยาเข้า |
| fill_volume_ml | INT | |
| drain_start_time / drain_end_time | VARCHAR(20) | เวลาปล่อยน้ำยาออก |
| drain_volume_ml | INT | |
| uf_volume_ml | INT | คำนวณฝั่ง server = drain_volume_ml - fill_volume_ml ตอน insert/update เสมอ ไม่รับค่าจาก user โดยตรง |
| dialysate_appearance | ENUM('clear','cloudy','bloody') | บังคับเลือกทุกครั้ง ไม่มีค่า default — ดูตรรกะ peritonitis alert ด้านล่าง |
| weight_kg | DECIMAL(5,2) | |
| bp_systolic / bp_diastolic | SMALLINT | |
| urine_output_ml | INT NULL | จดครั้งเดียวต่อวัน ผูกกับรอบสุดท้ายของ log_date นั้น (ไม่ได้บังคับด้วย constraint ระดับ DB) |
| created_at / updated_at | DATETIME | |

Peritonitis alert: ดูค่า `dialysate_appearance` ของ**รอบล่าสุดที่บันทึกไว้ทั้งหมด**
(เรียงตาม log_date DESC, cycle_number DESC — ไม่ผูกกับวันที่ปัจจุบัน) ถ้าเป็น
`cloudy`/`bloody` แสดง banner เต็มความกว้างจนกว่าจะมีรอบใหม่ที่บันทึกเป็น `clear`.

Reference ranges (KPI status thresholds) อยู่ที่ `internal/kpi/reference_ranges.go`
(`CapdDailyUFML` — คนละชุดกับของ APD เพราะ CAPD มีปริมาตรน้อยกว่า) — เกณฑ์
น้ำหนัก/ความดันใช้ร่วมกับ APD (`WeightChangeKG`, `BloodPressure`). ทุกค่าผ่านการ
ตรวจสอบและอนุมัติจากทีมแพทย์ผู้ดูแลผู้ป่วยโรคไต (PD Clinic) แล้ว — ดู
[docs/clinical_review.md](clinical_review.md).

## HD Log Book (`hd_log_entries`)

Phase 5 — new feature, no legacy data to migrate. Bound directly to
`patient_profiles.id` เหมือน APD/CAPD ไม่ผ่าน core `log_entries` pattern
ด้านบน — ดู [migrations/20260716_create_hd_log_book.sql](../migrations/20260716_create_hd_log_book.sql).

เหมือน APD ตรงที่ log หนึ่งแถวต่อ**หนึ่งครั้งที่ฟอกไต** (ไม่ใช่ต่อ cycle แบบ CAPD)
ดังนั้น uniqueness constraint คือ `(patient_profile_id, log_date)`.

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| patient_profile_id | BIGINT UNSIGNED | FK → patient_profiles.id |
| log_date | DATE | UNIQUE ร่วมกับ patient_profile_id |
| dry_weight_kg | DECIMAL(5,2) | น้ำหนักแห้ง |
| pre_dialysis_weight_kg | DECIMAL(5,2) | น้ำหนักก่อนฟอก |
| post_dialysis_weight_kg | DECIMAL(5,2) | น้ำหนักหลังฟอก |
| pre_dialysis_bp_systolic / pre_dialysis_bp_diastolic | SMALLINT | ความดันก่อนฟอก |
| post_dialysis_bp_systolic / post_dialysis_bp_diastolic | SMALLINT | ความดันหลังฟอก |
| uf_removed_ml | INT | คำนวณฝั่ง server = (pre_dialysis_weight_kg - post_dialysis_weight_kg) * 1000 ตอน insert/update เสมอ ไม่รับค่าจาก user โดยตรง — เป็น**ปริมาณรวมทั้งครั้ง ไม่ใช่อัตราต่อชั่วโมง** |
| notes | TEXT NULL | |
| created_at / updated_at | DATETIME | |

Reference ranges (KPI status thresholds, ผ่านการตรวจสอบและอนุมัติจากทีมแพทย์แล้ว
เหมือน APD/CAPD — ดู [docs/clinical_review.md](clinical_review.md)) อยู่ที่
`internal/kpi/reference_ranges.go`:
- `HDPostVsDryKG` — น้ำหนักหลังฟอกเทียบน้ำหนักแห้ง (delta = post - dry): ปกติ
  ±0.5kg / เฝ้าระวัง เกิน 0.5-1.5kg (น้ำเกิน) หรือต่ำกว่าแห้ง 0.5-1kg (เสี่ยง
  over-ultrafiltration) / ผิดปกติ เกิน >1.5kg หรือต่ำกว่า >1kg — asymmetric
  โดยเจตนา
- `HDInterdialyticGainKG` — น้ำหนักเพิ่มระหว่างรอบ (pre_dialysis_weight_kg ของ
  ครั้งนี้ ลบ post_dialysis_weight_kg ของครั้งก่อนหน้า): ปกติ <=2kg / เฝ้าระวัง
  2-3kg / ผิดปกติ >3kg
- ความดันก่อนฟอก ใช้เกณฑ์เดียวกับ APD/CAPD (`BloodPressure`)
- `HDPostBPSystolic` — ความดันหลังฟอก (เฉพาะ systolic, สัญญาณความดันตกหลังฟอก):
  ปกติ >=100 / เฝ้าระวัง 90-100 / ผิดปกติ <90

## Lab Results (`lab_results`)

Phase 6 — new feature, no legacy data to migrate. Bound directly to
`patient_profiles.id` เหมือน APD/CAPD/HD — ดู
[migrations/20260718_create_lab_results.sql](../migrations/20260718_create_lab_results.sql).

ต่างจาก APD/CAPD/HD ตรงที่ **แทบทุก column เป็น nullable** (มีแค่
`patient_profile_id`/`log_date` ที่ NOT NULL) — ผลตรวจแต่ละตัวมีความถี่การตรวจ
ต่างกัน (บางตัวตรวจทุก 3 เดือน บางตัวทุก 6 เดือน-1 ปี) คนไข้กรอกเฉพาะค่าที่ตรวจจริง
วันนั้น ไม่บังคับครบทุกช่อง — uniqueness constraint คือ `(patient_profile_id, log_date)`
เหมือน APD/HD (หนึ่งแถวต่อหนึ่งวันที่ตรวจ)

**ใช้ได้ทุก treatment_type** — ไม่ผูก gate ตาม treatment_type แบบ APD/CAPD/HD
เดิม (ดู `internal/handler.requireLabResultsPatient`) มีแค่ `kt_v_value`/`urr`/`npcr`
ที่แสดงในฟอร์มเฉพาะเมื่อ `patient_profiles.treatment_type == 'HD'` (แต่ column
เก็บได้ทุก treatment_type ตาม schema เดียวกัน — แอปเป็นตัวตัดสินใจว่าจะโชว์/ประเมิน
หรือไม่)

**Reference ranges** (`internal/labrange`) คัดลอกตรงจากแบบฟอร์มติดตามผลตรวจของ
ผู้ป่วยรายหนึ่ง ไม่ใช่มาตรฐานสากล — disclaimer เข้มกว่าปกติเพราะเหตุนี้
(`labrange.Disclaimer`) ทุกช่วงค่าใช้ `labrange.Range` ที่แยก inclusive/exclusive
ต่อขอบเขตได้อิสระ (ฟอร์มต้นฉบับปนกันทั้ง "70-110" inclusive, ">65" exclusive,
">=200" inclusive) — ดู `internal/labrange/ranges.go` สำหรับค่าทั้งหมด
`kt_v_value` **ไม่ auto-classify สี** เพราะเป้าหมายขึ้นกับตารางฟอกเลือดของคนไข้เอง
(`labrange.KtVReferenceText` เป็นข้อความอ้างอิงล้วนๆ)

**การ์ดสรุปผลตรวจล่าสุด** (`/dashboard`, `/lab-results` — ใช้ partial ร่วม
`_lab_results_shell.html`) เป็น **rule-based ล้วนๆ ห้ามใช้ AI สร้างข้อความสังเคราะห์**
(`internal/handler.buildLabAbnormalItems`) —ดึงค่า "ล่าสุดที่มีอยู่จริง" ของแต่ละ
field แยกกันอิสระ (ไม่ใช่ทั้งแถวเดียวกัน เพราะคนละความถี่ตรวจ) แล้วเทียบกับ
reference range ทีละตัว แสดงเป็นรายการ "ค่าที่ผิดปกติ" ตรงไปตรงมา
**ห้ามมีประโยคตีความ/สังเคราะห์ข้ามค่า** (เช่นห้ามรวม PTH+Ca+PO4 แล้วสรุปเป็น
"เสี่ยงโรคกระดูก") — แสดงทีละค่าเท่านั้น

## News & Research (`news_articles`)

Phase 4 — เนื้อหาความรู้/งานวิจัยที่ดึงมาจากภายนอกอัตโนมัติแล้วให้ AI แปล+สรุปเป็นภาษาไทย ก่อน
publish ให้ผู้ป่วยเห็น ดูรายละเอียดที่มาของแต่ละแหล่ง (สถานะทางกฎหมาย/ToS) ที่
[news_sources_survey.md](news_sources_survey.md) ก่อนเพิ่มแหล่งใหม่ — ตอนนี้ต่อกับ **PubMed/NCBI
E-utilities** ([cmd/news_ingest_pubmed](../cmd/news_ingest_pubmed/main.go)) และ **nephrothai.org**
([cmd/news_ingest_nephrothai](../cmd/news_ingest_nephrothai/main.go)) — แหล่งอื่นในสำรวจถูกห้ามโดย
ToS

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| source | VARCHAR(50) | `pubmed` หรือ `nephrothai` |
| external_id | VARCHAR(100) | PMID สำหรับ pubmed, WordPress post id (หรือ hash ของ link ถ้าไม่มี) สำหรับ nephrothai; UNIQUE ร่วมกับ `source` เพื่อกันดึงซ้ำ |
| title | VARCHAR(500) | ชื่อบทความต้นฉบับ ภาษาต้นทาง (อังกฤษสำหรับ pubmed, ไทยอยู่แล้วสำหรับ nephrothai) |
| title_th | VARCHAR(500) | ชื่อบทความภาษาไทย — แปลโดย AI สำหรับ pubmed, เหมือน `title` เป๊ะสำหรับ nephrothai (ไม่ต้องแปล) |
| summary_th | TEXT | pubmed: สรุปภาษาไทยโดย AI (3-5 ประโยค, **ต้องเป็นการสรุป/พารามาเฟรสเสมอ ห้ามเก็บ abstract ต้นฉบับแบบคำต่อคำ** เพราะเป็นลิขสิทธิ์ของวารสารต้นทาง ไม่ใช่ของ NCBI). nephrothai: excerpt แบบกลไก (ตัดข้อความล้วนๆ ไม่ใช้ AI เพราะเนื้อหาเป็นไทยอยู่แล้วและมี permission reuse เต็ม) |
| content_html | MEDIUMTEXT NULL | เนื้อหาเต็มแบบ HTML — populate เฉพาะ nephrothai (มี permission reuse เต็ม, ดึงตรงจาก RSS `content:encoded`) เป็น NULL เสมอสำหรับ pubmed |
| journal_name | VARCHAR(255) NULL | ชื่อวารสาร (จาก esummary) — pubmed เท่านั้น |
| published_at | DATE NULL | วันที่ตีพิมพ์/โพสต์ตามต้นฉบับ — parse ไม่ได้ปล่อย NULL ไม่เดา |
| credit_source_name | VARCHAR(255) | ข้อความเครดิตที่ต้องแสดงคู่กับสรุปเสมอ เช่น "PubMed/NCBI — BMC Nephrology" หรือ "สมาคมโรคไตแห่งประเทศไทย (nephrothai.org)" |
| credit_url | VARCHAR(500) | ลิงก์กลับต้นฉบับ — UI ต้องแสดงเสมอ |
| feature_image_url | VARCHAR(500) NULL | URL บน R2 CDN ของภาพประกอบที่ AI สร้าง — ดูหัวข้อ Feature image ด้านล่าง |
| feature_image_status | ENUM('pending','generated','failed') DEFAULT 'pending' | แยกจาก `status` เด็ดขาด — ภาพล้มเหลวไม่เคย block การ insert บทความ |
| status | ENUM('pending','published','rejected') DEFAULT 'pending' | pipeline insert เป็น `pending` เสมอ ต้องผ่าน `/admin/content-queue` ก่อน promote เป็น `published`/`rejected` |
| reviewed_by | BIGINT UNSIGNED NULL | FK → users.id (ON DELETE SET NULL) — ผู้อนุมัติ/ปฏิเสธ ตั้งพร้อม `reviewed_at` ครั้งเดียวโดย `/admin/content-queue` เท่านั้น |
| reviewed_at | DATETIME NULL | |
| created_at / updated_at | DATETIME | |

UNIQUE KEY `(source, external_id)` กัน insert ซ้ำเวลารัน ingestion ซ้ำ (เช็คก่อน insert อยู่แล้วในโค้ด
แต่มี constraint เป็น safety net อีกชั้น)

### Feature image (AI-generated, `internal/newsimage`)

ทุกบทความ (ทั้ง 2 แหล่ง) พยายามสร้างภาพประกอบผ่าน OpenAI Image API (`gpt-image-2`,
1536x1024, quality medium) ทันทีหลังดึงเนื้อหา — prompt style คงที่ทุกภาพ (semi-3D vector
illustration แบบ editorial news คล้าย fintech/crypto journalism: ตัวละคร/วัตถุทรงกลม
มน สีพื้นเรียบ+gradient เบาๆ ให้มิติ เส้นขอบหนา แสงแบบสตูดิโอ) มีแค่ "มินิซีน" ที่เปลี่ยนไปตาม
บทความ — ไม่ใช่แค่ topic phrase สั้นๆ แบบเดิม แต่เป็นคำอธิบายฉากเฉพาะเจาะจง 2-3 ประโยค
สรุปจาก "ประเด็นเฉพาะ" ของบทความจริง (`internal/newsimage.DescribeScene`, ใช้ AI ตัวเดียว
กับที่ใช้แปล อ่านจาก title_th+summary_th) — ออกแบบมาเพื่อแก้ปัญหาที่ topic phrase สั้นๆ
แบบเดิมทำให้บทความคล้ายกันได้ภาพซ้ำโครงเดิม. **กฎเหล็ก: ห้ามภาพอวัยวะภายใน/ร่างกายมนุษย์
แสดงพยาธิสภาพ/โรค/การติดเชื้อ/บาดแผลเด็ดขาด ไม่ว่าจะสมจริงหรือ stylized** — ถ้าบทความเกี่ยว
กับการติดเชื้อ/ป่วย ให้สื่อผ่านสัญลักษณ์ (เช่น โล่กันเชื้อโรค, ป้ายเตือน, ขวดยา) แทนการแสดง
ให้เห็นว่าเกิดขึ้นในตัวคน ตัวละครคนแบบการ์ตูนเรียบง่าย (หมอ, ผู้ป่วย) ปรากฏในฉากได้ แต่ต้อง
ไม่มีรายละเอียดกายวิภาคเด็ดขาด

Path บน R2: `pdlife/news/{source}/{external_id}.png` (bucket/CDN เดียวกับที่ตั้งไว้ใน
`.env` อยู่แล้ว `R2_*`). ล้มเหลว (rate limit/content policy/timeout) → retry แบบ
exponential backoff + jitter 3 ครั้ง ก่อน mark `feature_image_status='failed'` —
ไม่ block การ insert บทความไม่ว่ากรณีใด. Admin กด "สร้างภาพใหม่" ใน
`/admin/content-queue` ได้ (ลบไฟล์เก่าใน R2 ก่อน generate ใหม่).

**CDN cache-busting:** `internal/r2store.Upload` แนบ `?v=<unix timestamp>` ต่อท้าย URL
ที่คืนกลับมาเสมอ — key บน R2 เป็นแบบ deterministic (`{source}/{external_id}.png`) และ
"regenerate" เขียนทับ key เดิม แต่ Cloudflare cache หน้า R2 (`cdn.pdlife.app`) cache ตาม
URL เต็มรวม query string ไว้นานถึง 4 ชม. (`Cache-Control: max-age=14400`) — ถ้าไม่แนบ
`?v=` ใหม่ทุกครั้งที่ upload, regenerate จะเขียนภาพใหม่ลง R2 สำเร็จแต่ผู้ใช้จะยังเห็นภาพ
เก่าจาก edge cache นานถึง 4 ชม. (พบจริงระหว่างทดสอบ). `KeyFromURL` ตัด query string ออก
ก่อน match prefix เพื่อให้ regenerate's delete-old-object ยังหา key เดิมเจอ.

### Provider config (`cmd/news_ingest_pubmed`, `cmd/news_ingest_nephrothai`, และ
`/admin/content-queue` regenerate — ไม่ใช่ web server ทั้งหน้าเว็บ)

`PROVIDER`/`FALLBACK_PROVIDER` (ค่าที่รองรับ: `openai`, `groq`) + `OPENAI_API_KEY`/`OPENAI_MODEL`
และ/หรือ `GROQ_API_KEY`/`GROQ_MODEL` แล้วแต่ว่าตัวไหนถูกเลือก — ดู `.env.example`. ทั้งสอง
ingestion command เช็ค config ครบก่อนเริ่มงานเสมอ (ผ่าน `internal/llmprovider.Require`,
exit ทันทีถ้าขาด ไม่เดา/ข้ามการแปล); `/admin/content-queue`'s regenerate endpoint ใช้
`internal/llmprovider.Resolve` แทน (คืน error กลับเป็น JSON แทนที่จะ crash ทั้ง web server).
`OPENAI_API_KEY` ยังถูกใช้ตรงสำหรับ image generation เองด้วย ไม่ว่า `PROVIDER` จะเลือกอะไร
(image gen เป็น OpenAI-only เสมอ). **Pattern นี้ออกแบบใหม่สำหรับ pdlife โดยเฉพาะ — ไม่ตรงกับ
ของ nhe.one** (nhe.one มีแค่ `internal/handler/translate_handler.go` ซึ่งเป็น endpoint แปล UI
string ทั่วไป ไม่มี pipeline ดึงข่าว/สร้างภาพแบบนี้).

## Food Check (`foodcheck_*`)

Port มาจาก foodcheck.jocky.website (FastAPI/SQLite ค้นหาโภชนาการอาหารไทย) — ดูสำรวจเต็มๆ
ที่ [foodcheck_survey.md](foodcheck_survey.md) และ migration ที่
[migrations/20260709_create_foodcheck.sql](../migrations/20260709_create_foodcheck.sql)

**ต่างจาก source schema ตรงนี้ (เหตุผลอยู่ในคอมเมนต์หัวไฟล์ migration):**
- ตัด `usda_food_mapping`/`usda_nutrient_cache` ทิ้ง — 0 แถวใน source ไม่เคยมีโค้ดเรียก USDA จริง
- ตัด `user` ทิ้ง — recipe/search history ผูกกับ `patient_profiles` ของระบบนี้แทน
- ตัด `pd_nutrient.risk_direction` ทิ้ง — seed ไว้ใน source แต่ไม่เคยถูกอ่านจากโค้ดไหนเลย
  threshold ไฟจราจรจริงอยู่ที่ `internal/foodrisk` (Go config, ยังไม่ได้เขียนในรอบนี้)
- `per_100g`/`min_val`/`max_val`/`sd` เป็น `DECIMAL` ไม่ใช่ `TEXT` — source ปนเลขจริงกับ
  string `'-'` สำหรับค่าที่ไม่มีข้อมูล, cast เป็น `NULL` ตอน import แทนที่จะ parse ทุกครั้งที่ query
- ทุกตารางมี prefix `foodcheck_` ตาม pattern เดียวกับ `apd_*`

**ตาราง:** `foodcheck_food_groups`, `foodcheck_foods`, `foodcheck_food_nutrients` (INMU),
`foodcheck_anamai_foods`, `foodcheck_anamai_nutrients` (กรมอนามัย), `foodcheck_pd_nutrients` +
`foodcheck_nutrient_name_maps` (config/seed data), `foodcheck_recipes` + `foodcheck_recipe_ingredients`
(placeholder — schema เตรียมไว้ ยังไม่มี endpoint), `foodcheck_search_history` (ตารางใหม่ ไม่มีใน source)

**Views:** `v_foodcheck_food_nutrients`, `v_foodcheck_pd_nutrients` (รวม INMU+Anamai ผ่าน
`food_uid` string เช่น `'thaifcd_inmu:129'` / `'thaifcd_anamai:07034'`), `v_foodcheck_recipe_nutrition`

**Data migration:** ข้อมูลจริง (INMU 1,781 foods/60,751 nutrients, Anamai 1,484 foods/24,364
nutrients) copy ครั้งเดียวผ่าน `cmd/migrate_foodcheck` (อ่าน SQLite ต้นทางแบบ read-only, เขียนเข้า
MySQL ที่นี่) — ยังไม่ได้รันบน production ต้องรันเองตามคำสั่งใน doc comment ของไฟล์นั้น
หลัง apply migration SQL แล้ว. `foodcheck_pd_nutrients`/`foodcheck_nutrient_name_maps` seed มาจาก
migration SQL โดยตรง ไม่ต้องรัน migrate tool สำหรับสองตารางนี้

## PDLife Editorial Articles (`editorial_articles`)

Admin-authored rich-text articles — distinct from the AI-summarized `news_articles` pipeline
above. Handlers: `internal/handler/editorial.go` (admin CRUD + media upload),
`internal/handler/editorial_public.go` (public `/articles`, `/articles/:slug`).

| column | type | หมายเหตุ |
|--------|------|----------|
| id | BIGINT UNSIGNED PK | |
| author_id | BIGINT UNSIGNED | FK → users.id |
| title | VARCHAR(255) | |
| slug | VARCHAR(255) UNIQUE | generate ครั้งเดียวตอนสร้างบทความจาก title (`slugify` ใน editorial.go) — แก้ title ทีหลังไม่กระทบ slug (กัน URL แตก); ชนกัน → เติม `-2`, `-3`, ... ต่อท้าย (`uniqueSlug`) |
| content_html | MEDIUMTEXT | sanitize ผ่าน `internal/sanitize.HTML` (bluemonday whitelist) **ทุกครั้ง** ก่อน insert/update ไม่มีข้อยกเว้น รวมถึง autosave — render ตรงๆ ฝั่ง template เพราะ sanitize แล้วตอน save |
| cover_image_url | VARCHAR(500) NULL | เหมือน `news_articles.feature_image_url` — NULL = แสดง placeholder ไม่ใช่ error |
| status | ENUM('draft','published') DEFAULT 'draft' | |
| published_at | DATETIME NULL | set **ครั้งเดียว** ตอน publish ครั้งแรกเท่านั้น — publish ซ้ำ (เช่นแก้ไขบทความที่เผยแพร่แล้ว) ไม่เปลี่ยนค่านี้ |
| created_at / updated_at | DATETIME | |

Index `(status, published_at)` สำหรับ query หน้า `/articles` (published เท่านั้น เรียงตาม published_at)

**Media upload** (`POST /admin/editorial/upload-media`, admin-only): validate ประเภทไฟล์จาก
magic bytes จริง (`http.DetectContentType` บน 512 byte แรก) ไม่ใช่แค่ extension — รูป
JPG/PNG/WEBP จำกัด 10MB, วิดีโอ MP4/WEBM จำกัด 200MB. Upload ไป R2 path
`pdlife/editorial/{author_id}/{timestamp}-{random-hex}.{ext}` ผ่าน `internal/r2store` (ตัวเดียว
กับ news feature image — `r2store.ConfigFromEnv()` เป็น shared helper, ย้ายมาจาก
`internal/newsimage` ตอนทำฟีเจอร์นี้เพราะไม่เกี่ยวกับ news โดยเฉพาะ)

**Editor** (`/admin/editorial/new`, `/admin/editorial/:id/edit`): Quill.js 2.x โหลดผ่าน CDN
(jsdelivr) ไม่ build. ปุ่มแทรกรูป/วิดีโอ custom handler เรียก upload-media แล้วแทรก URL กลับเข้า
editor — รูปใช้ Quill's built-in `image` format, วิดีโอใช้ custom Parchment blot ชื่อ `videoFile`
(`class VideoFileBlot extends BlockEmbed` — **ต้องใช้ ES6 `class`/`extends` จริง ไม่ใช่ ES5
prototype assignment มือ**, เพราะ Parchment ต้องการ static inheritance ผ่าน `Object.setPrototypeOf`
ที่ `extends` ทำให้อัตโนมัติ ลอง manual `Object.create(Super.prototype)` แล้วเจอ "[Parchment] Unable
to create videoFile blot" เพราะ static methods ไม่ inherit) — render เป็น `<video controls
preload="metadata">` ไม่ใช่ iframe แบบ Quill's built-in video format.

Auto-save ทุก 30 วินาทีระหว่างพิมพ์ (debounce, dirty-flag guard กันยิง fetch ตอนไม่มีอะไรเปลี่ยน)
POST ไป endpoint เดียวกับปุ่ม "บันทึกฉบับร่าง"/"เผยแพร่" (`/admin/editorial/new` หรือ
`/admin/editorial/:id/edit` แล้วแต่มี id หรือยัง) ด้วย `action` ต่างกัน — `"draft"`/`"autosave"`
ทั้งคู่ **ไม่แตะ** `status`/`published_at` เลย (กัน autosave ดัน draft บทความที่เผยแพร่แล้วกลับไป
draft โดยไม่ได้ตั้งใจ) มีแค่ `"publish"` เท่านั้นที่เปลี่ยน status

**Slug เป็นภาษาไทย (Unicode) — เจอบั๊กจริงตอน dev:** สภาพแวดล้อม dev บางที Echo's
`c.Param("slug")` คืนค่ามาแบบยัง percent-encode อยู่ (`%e0%b8...`) แทนที่จะ decode เป็น UTF-8
ทำให้ query DB ไม่เจอ (ทั้งที่ browser ส่ง URL ถูกต้องและ slug ตรงกับที่เก็บไว้เป๊ะ) — แก้ด้วย
`url.PathUnescape` แบบ defensive ใน `ArticleDetail` ก่อน query เสมอ (no-op ถ้า decode แล้วอยู่แล้ว
เพราะ slug จริงไม่มี literal `%XX` ปน)

## Profile management (`/profile`)

หน้าเดียวรวมทุกอย่างเกี่ยวกับบัญชี — handler: `internal/handler/profile.go`. ทุก POST endpoint
redirect กลับ `/profile?success=X` หรือ `?error=Y` แทนการ re-render inline (ดู `profilePageData`
สำหรับตาราง code→ข้อความไทย) เพื่อเลี่ยงปัญหา resubmit-on-refresh.

- **ชื่อ** (`POST /profile/name`) — แก้ `users.nickname` ตรงๆ ไม่มี confirm (ไม่ใช่ข้อมูลสำคัญ)
- **รหัสผ่าน** (`POST /profile/password`) — ตรวจรหัสผ่านเดิมด้วย `auth.CheckPassword` ก่อน,
  ใช้กฎเดียวกับ register/reset-password (`auth.ValidatePasswordStrength`), revoke
  refresh_tokens ทั้งหมด **แต่** re-issue session cookie ใหม่ทันทีให้ device ปัจจุบัน (ผู้ใช้เพิ่ง
  พิสูจน์ตัวเองด้วยรหัสผ่านเดิมแล้ว) — device อื่นที่ login ค้างไว้จะต้อง login ใหม่ตอน request
  ถัดไป (security_stamp เปลี่ยน)
- **ข้อมูลการรักษา** (`POST /profile/treatment`) — แก้ `treatment_type`/`coverage_type`/
  `hospital_name` ไม่แตะ log entries เดิมเลย (แค่เปลี่ยน field ที่ gate เมนู/การเข้าถึง) การเปลี่ยน
  `treatment_type` มี confirm modal ฝั่ง client (ข้อความต่างกันตาม HD vs CAPD↔APD — ดู
  `web/templates/profile.html`'s `<dialog id="treatment-confirm-dialog">`) โดย server ไม่รู้เรื่อง
  modal เลย แค่ save ตามที่ POST มา
- **ความยินยอมข้อมูลสุขภาพ** — ย้ายปุ่ม "ถอนความยินยอม" มาจาก dropdown เดิม (ดู
  `internal/handler/consent.go`'s `WithdrawConsent`, redirect ไป `/profile` แทน `/consent`)
- **PDPA export** (`GET /profile/export-data`) — JSON indent, ไม่มี password_hash/security_stamp
  (ใช้ `profileExportAccount` struct เลือก field เอง ไม่ marshal `models.User` ตรงๆ), รวม
  apd_log_entries+prescriptions หรือ capd_log_entries ตาม treatment_type ปัจจุบันเท่านั้น (ไม่ใช่
  export ทุกอย่างที่เคยมี), food_search_history เสมอ (ปกติจะว่างเปล่า — dormant feature ยังไม่มี code
  เขียนจริง), editorial_articles เฉพาะถ้ามีเขียนไว้ (`omitempty`-style — ไม่ใส่ key เลยถ้าไม่มี)
- **ขอลบบัญชี** (`POST /profile/delete-account`) — ต้องใส่รหัสผ่านถูก + พิมพ์ "ลบบัญชี" ตรงตัว
  (`deleteAccountConfirmPhrase`) set `users.account_deletion_requested_at = now()`, revoke
  refresh_tokens ทั้งหมด, ส่งอีเมลยืนยันผ่าน `internal/mailer.SendAccountDeletionEmail`
  (template ใหม่ `account_deletion.{html,txt}`), logout ทันที

**Confirm modal เป็น native `<dialog>` element (ตัวแรกในโค้ดเบสนี้)** — ไม่ใช่ browser
`confirm()` เพราะต้องมีข้อความ dynamic (treatment) หรือรับ input (password + typed phrase สำหรับ
ลบบัญชี) ซึ่ง `confirm()` ทำไม่ได้ ใช้ `showModal()`/`.close()` ธรรมดา ไม่ต้อง polyfill (browser
สมัยใหม่รองรับหมด) — เปลี่ยนรหัสผ่านกับถอนความยินยอมยังใช้ `window.confirm()` ธรรมดาต่อ (ข้อความ
คงที่ ไม่ต้องรับ input) ตาม pattern เดิมของ APD/CAPD delete button

## Account deletion purge (`cmd/purge_deleted_accounts`)

Standalone cron tool (ไม่ใช่ web endpoint) — รันวันละครั้ง ค้นหา `users.account_deletion_requested_at`
ที่เก่ากว่า `handler.AccountDeletionGraceDays` (90) วัน แล้วลบ/ย้ายข้อมูลตามลำดับที่สำคัญ:

1. `editorial_articles` ที่ publish แล้ว → เปลี่ยน `author_id` เป็นบัญชี placeholder ถาวร
   `deleted-user@pdlife.app` (find-or-create ทุก run — ดู "บัญชีผู้ใช้ที่ถูกลบ" ด้านบน) —
   **ต้องทำก่อนลบ user เสมอ** เพราะ `editorial_articles.author_id` มี `ON DELETE CASCADE`
   ถ้าลบ user ก่อนโดยยังมี article ค้าง author_id เดิมอยู่ DB จะ cascade ลบ article ทิ้งไปด้วย
   (ลิงก์สาธารณะพัง — ตรงข้ามกับที่ต้องการ)
2. `editorial_articles` ที่ยังเป็น draft (ไม่เคย publish) → ลบทิ้งตรงๆ ไม่มีลิงก์สาธารณะต้องรักษา
3. `patient_profiles` + ทุกตารางที่ผูกด้วย `patient_profile_id` (apd_log_entries,
   apd_prescriptions, capd_log_entries, foodcheck_search_history) → ลบทั้งหมด
4. refresh_tokens, password_reset_tokens, email_verifications → ลบทั้งหมด
5. แถว `users` เอง → `Unscoped().Delete()` (hard delete จริง ไม่ใช่ soft-delete ปกติ — PDPA
   right to erasure หมายถึงลบจริง ไม่ใช่แค่กรองออกจาก query)

ทุกขั้นตอนอยู่ใน 1 transaction ต่อ 1 user (ถ้า step ไหน fail ทั้งหมด rollback, log แล้วไปคนถัดไป
ไม่ทำให้ทั้ง run ล้ม) log ผลทุกครั้งทั้งสำเร็จ/ล้มเหลว

Cron บน VPS (รันตอนกลางคืน):

    0 2 * * *  cd /home/pdlife/web/pdlife.app/public_html && ./purge_deleted_accounts >> /var/log/pdlife/purge_deleted_accounts.log 2>&1
