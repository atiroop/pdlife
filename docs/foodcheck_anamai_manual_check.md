# Food Check — เช็ค Anamai ด้วยมือจากเครื่อง local (รายเดือน)

## ทำไมต้องรันจากเครื่องนี้ ไม่ใช่ VPS

VPS ของ pdlife.app ต่อ `thaifcd.anamai.moph.go.th` ไม่ได้เลย (ยืนยันแล้ว 2026-07-08: ping/TCP timeout
100% ทั้งที่ internet ทั่วไปและ `inmu.mahidol.ac.th` ใช้งานได้ปกติจาก VPS เดียวกัน) น่าจะเป็นเว็บรัฐบาลไทย
บล็อก IP ต่างประเทศของ VPS — ดู [docs/foodcheck_survey.md](foodcheck_survey.md) และ memory
`foodcheck-port-progress` สำหรับรายละเอียดเต็ม

Cron บน VPS เช็คแค่ฝั่ง INMU (`-source=inmu`) อัตโนมัติทุกเดือนอยู่แล้ว — ไฟล์นี้อธิบายเฉพาะขั้นตอนเช็ค
**ฝั่ง Anamai** ด้วยมือจากเครื่อง local (มี IP ไทยจริง ผ่าน block นี้ได้)

## ⚠️ คำเตือนก่อนเริ่ม

- เครื่องมือนี้เชื่อมต่อ **production database ตรงๆ** ผ่าน SSH tunnel — ไม่ใช่ database ทดสอบ
- **ปิด tunnel ทันทีหลังใช้งานเสร็จทุกครั้ง** อย่าเปิดทิ้งไว้
- ห้ามใส่ credential จริง (รหัสผ่าน DB, SMTP) ลงในไฟล์เอกสารนี้หรือไฟล์ใดๆ ที่ push เข้า git —
  ทุกขั้นตอนด้านล่างดึงค่าจาก `.env` ที่มีอยู่แล้วในเครื่อง (gitignored) เท่านั้น
- เครื่องมือมี safety check ในตัว ยืนยันทุกครั้งที่รันว่าไม่แตะ `foodcheck_foods`/`foodcheck_anamai_foods`
  เลย (ดูบรรทัด `ASSERTION OK` ในผลลัพธ์) — ถ้าเห็น `ASSERTION FAILED` ให้หยุดใช้งานทันทีและแจ้งทีมพัฒนา

## ขั้นตอน

### 1. เปิด SSH tunnel ไปยัง production MySQL

เปิด PowerShell หรือ terminal สักหน้าต่าง แล้วรัน (ปล่อยหน้าต่างนี้ค้างไว้ ห้ามปิด):

```powershell
ssh -L 13306:localhost:3306 myserver -N
```

(พอร์ต `13306` ตรงกับที่ตั้งไว้ใน `.claude/launch.json` ของโปรเจกต์อยู่แล้ว — ใช้พอร์ตเดิมเพื่อความสม่ำเสมอ)

ทดสอบว่า tunnel เปิดสำเร็จ (เปิด terminal อีกหน้าต่าง ไม่ต้องปิดหน้าต่างแรก):

```powershell
Test-NetConnection -ComputerName 127.0.0.1 -Port 13306
```

ควรเห็น `TcpTestSucceeded : True`

### 2. ตั้งค่า DB_HOST/DB_PORT ชั่วคราว (เฉพาะ session นี้)

ในหน้าต่าง terminal ที่สอง (ไม่ใช่หน้าต่างที่รัน tunnel ค้างอยู่):

```powershell
cd D:\claude_pdlife
$env:DB_HOST = "127.0.0.1"
$env:DB_PORT = "13306"
```

ค่าอื่นที่จำเป็น (`DB_USER`, `DB_PASSWORD`, `DB_NAME`, `SMTP_HOST`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`)
จะถูกโหลดอัตโนมัติจาก `.env` ที่ root โปรเจกต์ (`D:\claude_pdlife\.env`) ตราบใดที่รันจาก path นี้ —
ตัวแปรที่ตั้งไว้ในขั้นตอนนี้ (`DB_HOST`/`DB_PORT`) จะ**ทับค่าใน `.env`** โดยอัตโนมัติ (ยืนยันพฤติกรรมนี้แล้ว)
ไม่ต้องแก้ไฟล์ `.env` เอง

เครื่อง local ต่อ internet ปกติอยู่แล้ว ยิง Resend SMTP ได้ตรงๆ ไม่ต้องผ่าน tunnel สำหรับส่วนอีเมล

### 3. รันตัวเช็ค (เฉพาะ Anamai)

```powershell
.\bin\foodcheck_diffcheck_anamai.exe -source=anamai
```

### 4. ผลลัพธ์ที่ควรเห็นถ้าสำเร็จ

**ครั้งแรก (สร้าง baseline — ไม่มีอีเมลส่ง เพราะยังไม่มีอะไรให้เทียบ):**

```
2026/0x/xx xx:xx:xx connected to database "pdlife_pdlife-db" at 127.0.0.1:13306
2026/0x/xx xx:xx:xx main data tables before run: foodcheck_foods=1781 foodcheck_anamai_foods=1484
2026/0x/xx xx:xx:xx [inmu] SKIPPED: not requested via -source
2026/0x/xx xx:xx:xx [anamai] BASELINE: no previous snapshot — recording count=1484 as the first baseline, no alert sent
2026/0x/xx xx:xx:xx ASSERTION OK: main data tables untouched (foodcheck_foods=1781 foodcheck_anamai_foods=1484)
2026/0x/xx xx:xx:xx DONE
```

**เดือนถัดไป ถ้าไม่มีอะไรเปลี่ยน:**

```
2026/0x/xx xx:xx:xx [anamai] OK: no change (count=1484)
2026/0x/xx xx:xx:xx ASSERTION OK: main data tables untouched (foodcheck_foods=1781 foodcheck_anamai_foods=1484)
2026/0x/xx xx:xx:xx DONE
```

**ถ้าเจอความเปลี่ยนแปลง (จะมีอีเมลไปที่ admin@pdlife.app):**

```
2026/0x/xx xx:xx:xx [anamai] DIFF: count 1484 -> 1490 (hash changed: true) — sending alert to admin@pdlife.app
2026/0x/xx xx:xx:xx [anamai] alert email sent successfully
2026/0x/xx xx:xx:xx ASSERTION OK: main data tables untouched (foodcheck_foods=1781 foodcheck_anamai_foods=1484)
2026/0x/xx xx:xx:xx DONE
```

→ ไปเช็คกล่องเมล admin@pdlife.app ควรได้อีเมลหัวข้อ "Food Check: พบความเปลี่ยนแปลงที่แหล่งข้อมูล
กรมอนามัย (thaifcd.anamai.moph.go.th) - pdlife.app"

**ถ้า connectivity ยังมีปัญหา** (ไม่ควรเกิดจากเครื่อง local ที่มี IP ไทย แต่เผื่อไว้):

```
2026/0x/xx xx:xx:xx [anamai] WARNING: robots.txt check failed (...timeout...)
2026/0x/xx xx:xx:xx [anamai] ERROR: fetching current data failed: ...timeout...
```

→ ถ้าเจอแบบนี้จากเครื่อง local ด้วย แปลว่าปัญหาไม่ใช่แค่ VPS ต้องสอบถามทีมพัฒนาเพิ่มเติม

### 5. ปิด tunnel ทันที

กลับไปที่หน้าต่างแรก (ขั้นตอน 1) กด `Ctrl+C` เพื่อปิด SSH tunnel — **ห้ามเปิดทิ้งไว้**

## ความถี่

รันเดือนละ 1 ครั้ง ให้ตรงกับรอบที่ cron ฝั่ง INMU รันบน VPS (วันที่ 1 ของเดือน) — ไม่จำเป็นต้องรันวันเดียวกัน
เป๊ะๆ แค่ให้อยู่ในเดือนเดียวกันก็พอ

**แนะนำ:** ใช้ระบบ scheduled task/reminder ของ Claude (เช่น Cowork scheduled task หรือ `/schedule`)
ตั้งเตือนตัวเองทุกวันที่ 1 ของเดือนแทนการจำเอง — ไม่ต้องเขียนโค้ดระบบแจ้งเตือนเพิ่มสำหรับเรื่องนี้
(บอก Claude ว่า "เตือนฉันวันที่ 1 ของทุกเดือนให้รัน Anamai diff-check ด้วยมือ" ก็พอ)

## ไฟล์ที่เกี่ยวข้อง

- Binary: `bin\foodcheck_diffcheck_anamai.exe` (Windows, cross-compiled จาก `cmd/foodcheck_diffcheck`,
  ไม่ได้ commit เข้า git — อยู่ใน `.gitignore` เพราะเป็น build artifact)
- Source code: [cmd/foodcheck_diffcheck/main.go](../cmd/foodcheck_diffcheck/main.go)
- ถ้าโค้ดใน `cmd/foodcheck_diffcheck` มีการแก้ไขในอนาคต ต้อง cross-compile ใหม่:
  ```powershell
  $env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
  go build -o bin\foodcheck_diffcheck_anamai.exe .\cmd\foodcheck_diffcheck
  ```
