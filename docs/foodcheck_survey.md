# foodcheck.jocky.website — สำรวจระบบเดิมก่อน Port เข้า pdlife.app

สำรวจแบบ read-only ผ่าน SSH (`ssh myserver`) เมื่อ 2026-07-08 ไม่มีการแก้ไฟล์หรือแตะ DB ของ foodcheck แต่อย่างใด

---

## 1. โครงสร้างโปรเจกต์

**Path จริงบน VPS:** `/home/jocky/apps/foodcheck/` (ไม่ได้อยู่ใต้ `web/*/public_html` — อันนั้นเป็นแค่ static fallback ของ HestiaCP)

```
/home/jocky/apps/foodcheck/
├── data/
│   ├── foodcheck.sqlite      # DB ที่ใช้งานจริง (9.7 MB)
│   └── thaifcd.sqlite        # DB เก่า (legacy, 6.7 MB) — เก็บไว้เป็น fallback เฉยๆ ไม่ได้ใช้แล้ว
├── venv/                     # Python 3.12 virtualenv
└── app/                      # = GitHub repo atiroop/foodcheck.git
    ├── scraper/               # ดึงข้อมูล (รันครั้งเดียว/บำรุงรักษา)
    ├── web/                   # FastAPI app
    ├── tests/
    ├── deploy/
    └── docs/PROJECT_INSTRUCTIONS.md   # source of truth (ไม่ใช่ PROJECT_INSTRUCTION.md ตัวเก่า)
```

**Entry point:** `web/main.py` (`FastAPI(title="FoodCheck API", docs_url=None, redoc_url=None)`)

**Systemd service:** `foodcheck.service` — `active (running)`, enabled, uptime ~1.2 วัน ตอนสำรวจ
```ini
ExecStart=/home/jocky/apps/foodcheck/venv/bin/uvicorn main:app --host 127.0.0.1 --port 8010 --workers 1
WorkingDirectory=/home/jocky/apps/foodcheck/app/web
Environment=DATABASE_PATH=/home/jocky/apps/foodcheck/data/foodcheck.sqlite
```

**Routing:** HestiaCP nginx (`/etc/nginx/conf.d/domains/foodcheck.jocky.website*.conf`) → internal proxy `109.123.233.155:8080/8443` → (ตาม `deploy/nginx.conf` ที่เตรียมไว้) → `127.0.0.1:8010` (uvicorn). รูปแบบเดียวกับ apd.jocky.website ที่เคย migrate มาแล้ว ดู [[pdlife-apd-log-book]]

**ไม่มี `.env`** บนเซิร์ฟเวอร์ — env var (`DATABASE_PATH`) ตั้งใน systemd unit ตรงๆ ไม่มี `USDA_API_KEY`/`SECRET_KEY` อยู่จริง (ดูข้อ 4 — USDA/user ไม่ได้ implement)

### Python dependencies (`requirements.txt`)

| Package | ใช้ทำอะไร | Go equivalent |
|---|---|---|
| `requests` | HTTP client (scraper เท่านั้น) | `net/http` |
| `beautifulsoup4` + `lxml` | parse HTML table จาก INMU/Anamai (scraper เท่านั้น) | `golang.org/x/net/html` หรือ `goquery` (github.com/PuerkitoBio/goquery) — **แต่ scraper ไม่ต้อง port เลย ดูข้อ 5** |
| `fastapi` | web framework | Echo v4 (ตรงตาม stack ปัจจุบัน) |
| `uvicorn` | ASGI server | ไม่ต้องมี — Go binary serve เอง |

ไม่มี ORM (ใช้ `sqlite3` stdlib ตรงๆ), ไม่มี auth library, ไม่มี template engine จริง (ดูข้อ 6). โดยรวม dependency surface เล็กมาก แทบไม่มีอะไรต้องหา Go equivalent ยาก

---

## 2. Database (SQLite)

**Schema จริง = `scraper/schema_complete.sql`** — ต่างจากที่ README บอกไว้ ("9 ตาราง 3 views") เพราะมีการเพิ่มแหล่งข้อมูล Anamai (กรมอนามัย) เข้ามาทีหลัง (commit `3d2dd68`, `068fe12`) เอกสาร README/PROJECT_INSTRUCTIONS ไม่ได้อัปเดตตาม จำนวนจริงคือ **11 ตาราง + 3 views**

### ตารางทั้งหมด (มีจริงใน DB ที่รันอยู่)

| ตาราง | Row count จริง (2026-07-08) | หมายเหตุ |
|---|---|---|
| `food_group` | 17 | กลุ่ม A–Z (เว้น I L O P R) ของ INMU |
| `food` | 1,781 | INMU master list, **ทุกแถว `nutrient_fetched=1`** (fetch เสร็จสมบูรณ์ ไม่มี error ค้าง) |
| `nutrient` | 60,751 | long format: 1 แถว = 1 nutrient ของ 1 food |
| `anamai_food` | 1,484 | กรมอนามัย — ตรงกับตัวเลขที่คาดไว้ ทุกแถว `nutrient_fetched=1` |
| `anamai_nutrient` | 24,364 | long format เหมือนกัน แต่ schema คนละแบบ (มี `category`) |
| `usda_food_mapping` | **0** | ตารางมีอยู่ แต่ไม่เคยมี record — ดูข้อ 4 |
| `usda_nutrient_cache` | **0** | เช่นกัน |
| `user` | **0** | schema มี ฟีเจอร์ auth/PD profile ไม่เคย implement ใน web app |
| `recipe` | **0** | Recipe Builder — schema มี แต่ **ไม่มี endpoint ใดๆ ใน main.py** |
| `recipe_ingredient` | **0** | เช่นกัน |
| `pd_nutrient` | 6 | seed data คงที่ (ดูตารางข้อ 4) |
| `nutrient_name_map` | 13 | mapping ชื่อ nutrient ระหว่างแหล่ง INMU/Anamai → canonical name |

DB file: `data/foodcheck.sqlite` (9.7 MB), `PRAGMA journal_mode=WAL`, `foreign_keys=ON`

### Static vs. user-generated

**ทั้งหมด static/reference เท่านั้น ไม่มี user-generated content เลย** — ไม่มี search log, ไม่มี analytics table, `user`/`recipe`/`recipe_ingredient` มี schema พร้อมแต่ไม่เคยถูกใช้จริง (0 แถวทั้งหมด, ไม่มี endpoint เขียนลงตารางเหล่านี้ในโค้ดปัจจุบัน) → **การ port ข้อมูลเป็นการ copy ครั้งเดียวได้เลย ไม่ต้องคิดเรื่อง sync ต่อเนื่อง**

### 3 Views

```sql
v_food_nutrient   -- ทุก nutrient ของ INMU + flag is_missing (per_100g เป็น NULL/'-'/deriv_by='Not analysed')
v_pd_nutrient     -- UNION ALL ของ INMU + Anamai, resolve เป็น 6 ตัว PD-critical, join USDA cache (ว่างเปล่าจริง)
                  -- food_uid = 'thaifcd_inmu:<id>' หรือ 'thaifcd_anamai:<fid>' (คนละ id space จงใจแยก)
v_recipe_nutrition -- SUM(per_100g * amount_g/100) ต่อ recipe — คำนวณสำเร็จรูปสำหรับ recipe builder ที่ยังไม่มี UI
```

`v_pd_nutrient` เป็น view ที่สำคัญที่สุด เพราะรวม 2 แหล่งข้อมูลที่ id-space และชื่อ nutrient ไม่ตรงกันให้เป็นรูปแบบเดียว ถ้า port เข้า Go ควรคง logic นี้ไว้ (เขียนเป็น SQL view เหมือนเดิมได้ใน MySQL หรือคำนวณใน Go ก็ได้ แต่ต้อง reproduce การ join นี้)

---

## 3. API Endpoints (`web/main.py`, 303 บรรทัด)

| Method | Path | ทำอะไร | ใช้จากหน้าไหน |
|---|---|---|---|
| `GET` | `/api/search?q=&group=&source=` | ค้นหา (ดูข้อ 3.1) | index.html (search box), compare.html (source=thaifcd_inmu เท่านั้น — ดู "ข้อจำกัด") |
| `GET` | `/api/food/{food_id}` | รายละเอียด nutrient ครบ + sanity check + unit conversion | detail.html, compare.html |
| `GET` | `/api/compare?ids=1,2,3,4` | เทียบ 2-4 รายการ (**เฉพาะ INMU เท่านั้น** — `int(x)` เท่านั้น ไม่รองรับ `thaifcd_anamai:` prefix) | compare.html |
| `GET` | `/api/groups` | รายชื่อกลุ่มอาหาร A-Z (สำหรับ dropdown filter) | index.html (`loadGroups()`) |
| `GET` | `/` `/food/{id}` `/compare` | serve HTML template ตรงๆ (ไม่มี templating engine, อ่านไฟล์ .html คืนทั้งไฟล์) | — |

ไม่มี internal-only endpoint, ไม่มี auth, ไม่มี admin panel UI (มีแค่ `admin_token`/`X-Admin-Token` query/header สำหรับปลดล็อก debug info ใน sanity check response — เทียบกับ `FOODCHECK_ADMIN_TOKEN` env var ซึ่งไม่ได้ตั้งไว้จริงบนเซิร์ฟเวอร์ตอนนี้)

### 3.1 Logic การค้นหา (`/api/search`)

ไม่มี full-text search, ไม่มี fuzzy matching — เป็น **`LIKE '%q%'` ธรรมดา** บน 3 คอลัมน์ (`name_th`, `name_en`, `food_code`) รันแยก 2 query (INMU + Anamai) แล้ว merge ผลใน Python:

```python
# INMU
WHERE (f.name_th LIKE ? OR f.name_en LIKE ? OR f.food_code LIKE ?) [AND f.status = ?]
ORDER BY f.name_th LIMIT 30

# Anamai (เฉพาะเมื่อไม่ได้กรอง group — Anamai ไม่มี A-Z taxonomy)
WHERE af.name_th LIKE ? OR af.name_en LIKE ? OR af.fid LIKE ?
ORDER BY af.name_th LIMIT 30
```

จากนั้น merge results จากทั้งสอง query, sort รวมด้วย `name_th`, ตัดเหลือ 40 รายการ — เป็น debounce ฝั่ง client (300ms, ดูข้อ 6) ไม่ใช่ full-text index ฝั่ง DB เลย ต้องมี query อย่างน้อย 2 ตัวอักษร (`min_length=0` แต่เช็ค `len(q.strip()) < 2` เอง) มิฉะนั้นคืนว่าง (ยกเว้นกรองด้วย `group` อย่างเดียวก็ค้นได้)

**ตัวอย่าง response** (`GET /api/search?q=หมู`):
```json
{
  "results": [
    {
      "id": 342, "food_code": "F42", "name_th": "หมู, สันนอก, ดิบ", "name_en": "Pork, loin, raw",
      "status": "F", "source": "thaifcd_inmu", "group_name": "Meat, other animals and their products",
      "energy": "143", "protein": "20.9", "phosphorus": "218", "potassium": "358", "sodium": "58"
    }
  ],
  "total": 1
}
```
(ตัวเลขจาก schema ตัวอย่าง ไม่ใช่ query จริงที่รันในการสำรวจนี้)

---

## 4. Business Logic สำคัญ (หัวใจของงาน port)

### 4.1 การไฮไลต์สารอาหาร PD — **ไม่มีการให้คะแนน/สี binary ตามที่คาดไว้**

จุดสำคัญที่สุดที่ต่างจากสมมติฐานในโจทย์: **ไม่มี threshold ตัดสิน "เกินเพดาน" หรือสีแดง/เขียวแบบ safe/unsafe เลยในโค้ดจริง** — นี่เป็นกฎที่บังคับไว้ใน `CLAUDE.md`/`PROJECT_INSTRUCTIONS.md` ของโปรเจกต์เอง ("ห้ามใช้สีเขียว/แดงแบบ binary", "ห้าม UI ตัดสินว่ากินได้/ห้ามกิน") และโค้ดจริงทำตามกฎนี้เป๊ะ มี 2 ระบบ highlight แยกกันคนละจุดประสงค์:

**(ก) PD-nutrient marker (สีเหลือง) — แค่บอกว่า "นี่คือ 1 ใน 6 ตัวที่ผู้ป่วย PD ต้องดู" ไม่มีนัยเรื่องค่าสูง/ต่ำ**
```js
// detail.html — pdNames เป็น array hardcode ไว้ในหน้าเว็บ (ไม่ได้ query จาก DB!)
pdNames: ['Energy, by calculation', 'Protein, total', 'Phosphorus', 'Potassium', 'Sodium', 'Moisture']
// แถวไหน nutrient_name อยู่ใน list นี้ → bg-yellow-50 + จุดสีเหลือง
```
ตาราง `pd_nutrient` ใน DB (6 rows, มี `risk_direction` เป็น `'high'`/`'low'`) **ถูกสร้างไว้แต่ไม่เคยถูกอ่านจากโค้ด backend หรือ frontend เลย** — เป็น config ที่ตั้งใจไว้แต่ implementation ไปทางอื่น (hardcode ในหน้าเว็บแทน) ต้องระวังเวลา port อย่ายึด schema/docs เป็นความจริงทั้งหมด ให้ยึดโค้ดที่รันจริง

**(ข) Nutrition sanity check (เหลือง/ม่วงแดง) — เป็น data-quality warning ไม่ใช่คำแนะนำสุขภาพ** (`web/nutrition_sanity.py`, มี unit test ใน `tests/test_nutrition_sanity.py`)

Threshold ตัวเลขจริงที่ใช้ (ต่อ 100 กรัม, ทุกตัว severity = `"warning"` ยกเว้นเคสโซเดียมด้านล่างเป็น `"severe"`):

| Nutrient | Rule | เงื่อนไข |
|---|---|---|
| ทุกตัว (energy/protein/phosphorus/potassium/sodium/water) | `<nutrient>_negative` | ค่า < 0 |
| Moisture | `water_over_100g` | > 100 g |
| Protein | `protein_over_100g` | > 100 g |
| Energy | `energy_over_900kcal` | > 900 kcal |
| Sodium | `sodium_over_40000mg` | > 40,000 mg |
| Potassium | `potassium_over_5000mg` | > 5,000 mg |
| Phosphorus | `phosphorus_over_3000mg` | > 3,000 mg |

เคสพิเศษ (severity = `"severe"`, สีม่วงแดง `fuchsia`) — ตรวจจับหน่วยผิด (g เขียนเป็น mg) ในเครื่องปรุงเค็ม โดย match keyword ในชื่อ (`น้ำปลา/เกลือ/ซีอิ๊ว/ซอสปรุงรส/กะปิ/น้ำปลาร้า/oyster sauce/miso/bouillon/stock cube`):
- ถ้าเป็น "fish_sauce" category และ sodium < 3,000 mg → severe (พร้อม hint "อาจเป็นกรณีบันทึกหน่วยกรัมเป็นมิลลิกรัม" ถ้า 1 ≤ sodium ≤ 50)
- ถ้าเป็น condiment เค็มอื่นๆ และ sodium < 500 mg → severe (hint เดียวกัน)

**สีที่ใช้จริงใน UI:** เหลือง (`amber-50/amber-200/amber-900`) = warning, ม่วงแดง (`fuchsia-50/fuchsia-200/fuchsia-900`) = severe — ไม่มีสีแดง/เขียวเลยทั้งระบบ, ทุกกล่องเตือนมีข้อความกำกับ "ระบบนี้เป็นการเตือนคุณภาพข้อมูล ไม่ใช่คำแนะนำทางการแพทย์"

### 4.2 แหล่งข้อมูล — INMU / Anamai / USDA

**USDA fallback: มีแค่ schema + คอมเมนต์ ไม่มี implementation จริง** — grep ทั้ง repo หา `usda`/`USDA` เจอแค่ใน `schema_complete.sql` (ตาราง `usda_food_mapping`, `usda_nutrient_cache`) และ comment เท่านั้น **ไม่มีไฟล์ Python ไหนเรียก USDA API เลย** ไม่มีใน `web/main.py`, ไม่มีใน `scraper/` ยืนยันด้วย row count = 0 ทั้งสองตาราง แปลว่า:
- "USDA ใช้เมื่อไหร่บ้าง" → **ไม่เคยถูกใช้ในระบบที่รันจริง ไม่มี env var `USDA_API_KEY` บนเซิร์ฟเวอร์ด้วย**
- คำว่า "single ingredients only" ในเอกสาร (`PROJECT_INSTRUCTIONS.md`) เป็นแค่ design intent ที่ไม่เคย build — **ไม่ต้อง port ส่วนนี้เลย** (เว้นแต่เจ้าของอยากได้ฟีเจอร์นี้จริงๆ ในเวอร์ชัน Go ซึ่งต้องเขียนใหม่หมด)

**INMU vs Anamai — ใช้งานจริงทั้งคู่ แยกกันเป็นคนละ id-space โดยตั้งใจ:**
- INMU: `food.id` (INTEGER), เข้าถึงด้วย URL `/food/129`
- Anamai: `anamai_food.fid` (TEXT, zero-padded 5 หลัก เช่น `'07034'`), เข้าถึงด้วย URL `/food/thaifcd_anamai:07034`
- `main.py::parse_food_id()` เช็ค prefix `thaifcd_anamai:` — ถ้าไม่มี prefix ถือเป็น INMU เสมอ (คอมเมนต์บอกเหตุผลชัดเจน: "เพื่อไม่ให้ลิงก์เดิมที่แจกไปแล้วพัง" — backward compat สำหรับ URL ที่แชร์ไปแล้ว)
- ชื่อ nutrient ของสองแหล่งไม่ตรงกัน ต้องผ่าน `nutrient_name_map` (เช่น Anamai ใช้ `'Water'`/`'Energy'`/`'Total Energy'` ส่วน INMU ใช้ `'Moisture'`/`'Energy, by calculation'`)
- **ข้อจำกัดที่พบ:** `/api/compare` รองรับเฉพาะ INMU (`int(x) for x in ids.split(',')`) — เปรียบเทียบอาหารจาก Anamai ไม่ได้เลยในหน้า compare ปัจจุบัน (ไม่ได้บั๊ก แต่เป็น known limitation ที่ยังไม่ implement)

### 4.3 หน่วยและการแปลงหน่วย

ข้อมูลทั้งหมดเก็บเป็น **per 100 กรัม** เท่านั้น ไม่มี normalize เป็นหน่วยอื่นใน DB การแปลงหน่วยทำ **on-the-fly ตอน request** (`web/unit_conversion.py`, มี client-side mirror ที่ `web/static/unit_conversion.js` — คอมเมนต์บอกตรงๆ ว่า "Mirrors web/unit_conversion.py")

- หน่วยมวล (g/kg/oz) แปลงตรงได้เสมอ (factor คงที่)
- หน่วยปริมาตร (mL/L/tsp/tbsp/cup) ต้องมี **density (g/mL)** — ลำดับการหา density:
  1. ถ้ามี nutrient ชื่อ `"Density"` ในข้อมูลอาหารนั้น → ใช้ค่านั้นตรงๆ
  2. ไม่มี → เช็ค fallback ตาม keyword ในชื่ออาหาร 3 กลุ่ม (เกลือ = 1.2 g/mL แบบ exact-start-of-name, น้ำตาลทราย = 0.8 g/mL แบบ contains, ซอส/น้ำปลา/ซีอิ๊ว = 1.0 g/mL เฉพาะกลุ่มอาหาร `status == 'N'` เท่านั้น)
  3. หาไม่ได้เลย → หน่วยปริมาตรทั้งหมด `available: false` (ปิดใน dropdown ฝั่ง UI)
- tsp = 5 mL, tbsp = 15 mL, cup = 240 mL, oz = 28.3495 g (ค่าคงที่ hardcode)

---

## 5. Scraper / Data Pipeline

**ทั้งสองชุด (INMU + Anamai) รันครั้งเดียวจบแล้ว ไม่มี cron ใดๆ อยู่** — ยืนยันจาก:
- `food.nutrient_fetched = 1` ครบทั้ง 1,781 แถว, `= -1` (error) = 0 แถว
- `anamai_food.nutrient_fetched = 1` ครบทั้ง 1,484 แถว, error = 0
- ไม่มี cron job หรือ systemd timer เรียก scraper (เช็คจาก git log: commit `3d2dd68`/`068fe12` เป็นการเพิ่ม anamai scraper "1,484 foods, complete nutrients" ครั้งเดียวจบ ไม่ใช่ scheduled job)

→ **ข้อมูลทั้งหมดคือ static snapshot ที่ freeze ได้เลย ไม่ต้อง port scraper logic เข้า Go เลยแม้แต่น้อย** เพียง export ข้อมูลจาก SQLite แล้ว import เข้า MySQL ของ pdlife ก็พอ

รายละเอียด pipeline (เผื่อในอนาคตต้องการข้อมูลเพิ่ม/อัปเดต):
- **INMU:** `build_food_list.py` (paginate ทีละกลุ่ม A-Z ผ่าน `food_group_result` endpoint) → `fetch_nutrients.py` (ทีละอาหาร ผ่าน `food_name_result` endpoint, resume ด้วย `nutrient_fetched` flag)
- **Anamai:** `anamai_build_list.py` (ดึงทั้งหมดในทีเดียวจาก `search.php` — server คืน HTML ครบทุก 1,484 แถวไม่มี pagination) → `anamai_fetch_nutrients.py` (ทีละอาหาร ผ่าน `view.php`/`view_branded.php` แยกตาม prefix `R`)
- ทั้งสองใช้ `fetcher.py` ร่วมกัน: session-based `requests`, delay **1.5 วินาทีบังคับ** (`REQUEST_DELAY`, มีกฎห้ามลดในเอกสารทุกที่), retry 3 ครั้ง + backoff, User-Agent ระบุตัวตน + contact email

USDA: ไม่มี local cache, ไม่มีการเรียก live API เลย (ดูข้อ 4.2)

---

## 6. Frontend

**3 หน้า** เท่านั้น ทำงานเป็น static HTML ที่ FastAPI อ่านไฟล์คืนตรงๆ (`read_template()` — ไม่มี Jinja2 หรือ template engine ใดๆ ทั้งที่ import `HTMLResponse` จาก FastAPI):

| หน้า | ไฟล์ | บรรทัด | ทำอะไร |
|---|---|---|---|
| หน้าแรก/ค้นหา | `templates/index.html` | 294 | search box + group filter + result list |
| รายละเอียดอาหาร | `templates/detail.html` | 437 | nutrient table เต็ม + PD summary + sanity check + unit converter |
| เปรียบเทียบ | `templates/compare.html` | 391 | เลือก 2-4 อาหารเทียบข้างกัน |

**Stack:** Tailwind CSS (โหลดจาก CDN `cdn.tailwindcss.com` — ไม่ build), Alpine.js (จาก jsdelivr CDN) สำหรับ reactivity, ฟอนต์ Noto Sans Thai จาก Google Fonts — **ไม่มี build step, ไม่มี bundler, ไม่มี framework (React/Vue) เลย** ทุกอย่างเป็น vanilla `<script>` ในไฟล์ HTML ตรงๆ

**ฟีเจอร์ client-side ทั้งหมดทำงานฝั่ง client (Alpine.js `x-data`), server ส่งแค่ JSON:**
- Autocomplete: `x-model="query"` + `@input.debounce.300ms="doSearch()"` → เรียก `/api/search` ทุกครั้งที่พิมพ์ (debounce 300ms) ไม่มี client-side cache
- PD highlight (สีเหลือง): เช็ค `pdNames.includes(...)` (hardcode array ในหน้า — ดูข้อ 4.1)
- Sanity check warning box: render จาก `sanity_check` field ที่ API ส่งมาตรงๆ (logic คำนวณอยู่ฝั่ง server ทั้งหมด client แค่ render)
- Unit converter: คำนวณกรัม↔หน่วยอื่นฝั่ง client (`unit_conversion.js`) โดยใช้ `grams_per_unit` ที่ API คำนวณมาให้แล้ว (server เป็น source of truth, client แค่ mirror สูตรคูณ/หารเพื่อ responsive UI ไม่ต้อง round-trip ทุกครั้งที่เปลี่ยนหน่วย)

Source attribution (INMU + กรมอนามัย) แสดงเป็นแถบสีส้มถาวรใต้ navbar ทุกหน้า (ไม่ใช่ footer) — ตรงตามกฎ "ต้องแสดง attribution เสมอ ห้ามซ่อนใน footer"

---

## 7. สิ่งที่ต้องระวัง/ข้อจำกัด

### Port ตรงๆ ได้ (low risk)
- ข้อมูล INMU + Anamai ทั้งหมด: static, ไม่มี user-generated content, export/import ครั้งเดียวจบ
- Business logic ทั้งหมดเป็น pure function ธรรมดา (sanity check, unit conversion) — ไม่มี Python-specific magic, แปลเป็น Go ตรงๆ ได้ (แค่ port ตาราง threshold/keyword ในข้อ 4.1/4.3)
- API shape เรียบง่าย มี 4 endpoint หลัก ไม่มี auth/session ต้องจัดการ
- Frontend เป็น static HTML + CDN libs ล้วนๆ — ย้ายไฟล์ตรงๆ หรือ rewrite เป็น Go template ก็ทำได้ไม่ยาก

### ต้องออกแบบใหม่ (medium risk)
- **การ merge ผลค้นหาจาก 2 แหล่ง** (`v_pd_nutrient` view + `/api/search` logic แยก query แล้ว merge ใน code) — ต้อง reproduce ให้ตรง หรือถือโอกาสรวม schema INMU/Anamai ให้เป็นตารางเดียวกันด้วย column `source` แทนการแยกตารางแบบเดิม (ตัดสินใจตอนออกแบบ schema Go)
- **ชื่อ nutrient TEXT ที่ปน `'-'`** ของ INMU (`per_100g TEXT`) ต้อง parse/cast ตอน query เสมอ (เหมือน `parse_number()` ใน Python) — ถ้า migrate เข้า MySQL แนะนำ cast เป็น `DECIMAL`/`NULL` ตอน import เลย จะได้ไม่ต้อง parse ซ้ำทุก query แบบเดิม
- `usda_food_mapping`/`usda_nutrient_cache`/`user`/`recipe`/`recipe_ingredient` — เป็น schema ที่ไม่เคยถูกใช้งานจริง **ต้องถามเจ้าของก่อนว่าจะ port มาเป็น "โครงไว้ก่อน" หรือตัดทิ้งไปเลยตอนนี้** (ดูคำถามด้านล่าง)

### ตัดทิ้งได้เพราะไม่ได้ใช้ (drop candidates)
- Scraper ทั้งหมด (`scraper/`) — ไม่ต้อง port เข้า pdlife เลย ข้อมูล freeze แล้ว
- `usda_food_mapping` / `usda_nutrient_cache` — 0 แถว ไม่เคยมี code เรียกใช้
- `pd_nutrient.risk_direction` — มีอยู่ใน DB แต่ไม่มีโค้ดไหนอ่านค่านี้เลย (frontend hardcode แทน)
- `data/thaifcd.sqlite` (legacy DB) — ไม่ได้ใช้แล้ว มีแค่ไว้เป็น auto-copy fallback ตอน deploy ครั้งแรก

### จุดเสี่ยงสูงสุด 3 อันดับ

1. **INMU nutrient value เป็น TEXT ปนขยะ** (`'-'`, `'Not analysed'` ใน `deriv_by`) — ถ้า migration script cast ผิดพลาด (เช่น cast `'-'` เป็น `0` แทน `NULL`) จะทำให้ nutrient ที่ "ไม่มีข้อมูล" กลายเป็น "มีค่า 0" ซึ่งอันตรายมากสำหรับผู้ป่วย PD ที่ดูค่าฟอสฟอรัส/โพแทสเซียม — ต้อง unit test การ cast ให้ตรงกับ `is_missing` logic ของ `v_food_nutrient` เป๊ะๆ ก่อน migrate

2. **Sanity-check เป็น data-quality warning ไม่ใช่ medical advice — ต้องคงกฎ "ห้ามฟันธงกินได้/ห้ามกิน" และห้ามใช้สีแดง/เขียวไว้ในเวอร์ชัน Go ด้วย** เพราะ pdlife.app เป็นแอปสำหรับผู้ป่วยจริง (ไม่ใช่ personal tool เหมือน foodcheck เดิม) ยิ่งต้องระวังเรื่องนี้มากกว่าเดิม ไม่ใช่น้อยกว่า — ต้องตัดสินใจว่าจะยกเกณฑ์ตัวเลขเดิมมาใช้เป๊ะๆ (energy>900kcal, sodium>40000mg ฯลฯ) หรือปรับใหม่ให้เหมาะกับ context การใช้งานจริงในแอป PD

3. **Attribution/licensing ของ INMU (non-commercial only)** — pdlife.app อาจมีสถานะทางการค้าต่างจาก foodcheck เดิมที่ระบุชัดว่า "personal/non-commercial" ต้องถามเจ้าของ/ตรวจสอบเงื่อนไขการใช้ Thai FCD ของ INMU ให้แน่ใจก่อนเอาไปใช้ใน pdlife.app (ถ้า pdlife มีโมเดลธุรกิจใดๆ แม้แต่ freemium อาจผิดเงื่อนไขจากคำว่า "non-commercial" ที่ INMU กำหนด) — ข้อมูลกรมอนามัยเป็นหน่วยงานรัฐ ความเสี่ยงด้าน licensing ต่ำกว่า

### คำถามที่ต้องถามเจ้าของโปรเจกต์ก่อนเริ่ม

1. **pdlife.app เป็น non-commercial เหมือน foodcheck เดิมหรือไม่?** ถ้าไม่ใช่ ต้องตรวจสอบเงื่อนไขการใช้ข้อมูล INMU ใหม่ก่อน (ดูจุดเสี่ยง #3)
2. **Recipe Builder / multi-user profile (schema มีแต่ไม่เคย implement) — อยากได้ฟีเจอร์นี้ใน pdlife จริงไหม หรือแค่ต้องการ ingredient lookup ล้วนๆ?** ถ้าไม่ต้องการ ตัด schema ส่วนนี้ทิ้งได้เลยตั้งแต่ต้น ลดงานได้เยอะ
3. **จะ merge food lookup ของ foodcheck เข้ากับ log book ของ pdlife อย่างไร** เช่น ผู้ป่วยบันทึกอาหารที่กินในแต่ละวันแล้วดึงค่าโภชนาการอัตโนมัติจากฐานนี้ไหม หรือแค่เป็นเครื่องมือค้นหาแยกต่างหากในแอปเดียวกัน (มีผลต่อการออกแบบ schema ว่าจะ link เข้ากับตาราง log book เดิมของ pdlife หรือไม่)
4. **ต้องการ sanity-check/threshold เดิมเป๊ะๆ หรือให้ปรึกษา nutritionist/แพทย์ก่อนกำหนดเกณฑ์ใหม่** เนื่องจาก context เปลี่ยนจาก personal tool เป็นแอปสำหรับผู้ป่วยจริง (ดูจุดเสี่ยง #2)

---

## คำแนะนำการ Port (ลำดับงานที่ควรทำ)

1. **Export ข้อมูลดิบจาก SQLite → CSV/JSON** (ใช้ scraper's `export.py` เป็นจุดเริ่มหรือ query ตรงจาก `v_pd_nutrient`/`v_food_nutrient`) — ทำตอนนี้ได้เลยแบบ read-only ไม่กระทบ production
2. **ออกแบบ schema MySQL ใหม่** ตาม pattern ของ pdlife (ทุกตารางมี `TableName()`, migration file แยก, ไม่ใช้ `AutoMigrate`) โดย:
   - ตัดสินใจเรื่อง Recipe/User schema ก่อน (คำถาม #2) — ถ้าตัด จะลดจาก 11 ตารางเหลือ ~6 ตาราง (food_group, food, nutrient, anamai_food, anamai_nutrient, nutrient_name_map — หรือรวม INMU/Anamai เป็นตารางเดียวด้วย `source` column)
   - cast `per_100g` เป็น `DECIMAL`/`NULL` ตอน import แทนการเก็บ TEXT (ดูจุดเสี่ยง #1)
3. **Port business logic เป็น Go** — `nutrition_sanity.go` (threshold ในข้อ 4.1) และ `unit_conversion.go` (ข้อ 4.3) ตรงไปตรงมา มี unit test ต้นฉบับให้ reference เกณฑ์การทดสอบได้เลย (`tests/test_nutrition_sanity.py`, `tests/test_unit_conversion.py`)
4. **เขียน Echo handler 4 endpoint หลัก** (`/api/search`, `/api/food/:id`, `/api/compare`, `/api/groups`) — logic ธรรมดา ไม่มี auth ต้องจัดการ (เว้นแต่จะ gate ไว้หลัง pdlife auth เดิม)
5. **Frontend** — เลือกระหว่าง (ก) คง vanilla Alpine.js + Tailwind CDN แบบเดิม ย้ายไฟล์ตรงๆ หรือ (ข) integrate เข้า design system ของ pdlife.app ถ้ามีอยู่แล้ว — แนะนำ (ข) เพื่อความสอดคล้องของ UX ทั้งแอป แต่เป็นงานที่ใช้เวลามากที่สุดในทั้งหมด
6. **Sanity check เกณฑ์** — คุยกับเจ้าของ/ที่ปรึกษาทางการแพทย์ก่อนตรึงเกณฑ์ (คำถาม #4) แล้วค่อย finalize
