# News & Research Sources Survey (Phase 4, Step 1)

สำรวจ 4 แหล่งข้อมูลสำหรับฟีเจอร์ "สรุปภาษาไทย + ลิงก์กลับต้นฉบับ" — **ไม่แปลเต็ม ไม่ republish
เนื้อหาเต็ม**, เก็บแค่ title/date/URL/บทคัดย่อสั้นๆ เพื่อสร้างสรุปเองแล้วลิงก์กลับต้นฉบับเสมอ

สำรวจเมื่อ 2026-07-09 (ผลลัพธ์ ToS/robots.txt เปลี่ยนได้ตลอด — ควรเช็คซ้ำก่อน build จริง โดยเฉพาะ
ถ้าเว้นช่วงหลายเดือน)

---

## 1. สมาคมโรคไตแห่งประเทศไทย (nephrothai.org)

**Automated access: ✅ ได้ — ไม่มีข้อห้ามชัดเจน**

- `robots.txt` (https://www.nephrothai.org/robots.txt) อนุญาตกว้าง — บล็อกแค่ `/wp-admin/` กับไฟล์
  JSON ของปลั๊กอินตัวเดียว, มี sitemap ที่ `/wp-sitemap.xml`. ไม่มีการบล็อก AI bot รายชื่อ
  (ต่างจาก healio.com อย่างชัดเจน)
- ไม่พบหน้า Terms of Use/ToS ที่ระบุห้าม scrape หรือห้าม automated access โดยเฉพาะ (เท่าที่ค้นเจอ —
  ควรเช็คซ้ำให้ชัวร์ก่อน build เพราะเว็บไม่มีหน้า legal แยกที่หาเจอง่าย)
- เป็นเว็บ **WordPress** — มี RSS feed มาตรฐานที่ `https://www.nephrothai.org/feed/` ใช้งานได้จริง
  (ทดสอบแล้ว เป็น RSS 2.0 valid)
- เนื้อหาเป็นภาษาไทยอยู่แล้ว — ไม่ต้องแปล แค่สรุปย่อ

**⚠️ ข้อสังเกตสำคัญ:** หมวด "News and Events" (ภาษาอังกฤษ) ส่วนใหญ่เป็นประกาศงานประชุมวิชาการ
(APCN, KSN, WCN ฯลฯ) ไม่ใช่บทความความรู้สำหรับผู้ป่วย ส่วนหมวด "ข่าวประชาสัมพันธ์" (ภาษาไทย) ก็เป็น
ข่าวสมาคม/ประกาศฝึกอบรมเป็นหลัก ไม่ใช่บทความสุขภาพสำหรับผู้ป่วยโดยตรง — ถ้าต้องการเนื้อหาแนว
"ความรู้สำหรับผู้ป่วย PD" ต้องไปดูหมวด **"สาระความรู้"** (`/category/สาระความรู้/`) แทน ดูรายละเอียด
structure ที่สำรวจเพิ่มด้านล่าง

### บันทึกการอนุญาตใช้เนื้อหา (สำคัญ)

เจ้าของโปรเจกต์ได้โทรศัพท์สอบถามผู้ดูแลเว็บ nephrothai.org (สมาคมโรคไตแห่งประเทศไทย) โดยตรง
ได้รับแจ้งด้วยวาจาว่า: เนื้อหาเป็น public domain, สามารถ reuse ได้ โดยมีเงื่อนไขต้องให้เครดิต
ท้ายบทความเสมอ

**ข้อกำหนดที่ต้องบังคับใช้ในระบบ:** ทุกบทความที่ดึงจาก nephrothai.org ต้องมี credit block
ต่อท้ายเสมอ ระบุชื่อแหล่งที่มา + ลิงก์กลับไปบทความต้นฉบับ ห้าม publish ถ้าไม่มี credit

### สำรวจหมวด "สาระความรู้" โดยละเอียด

URL ที่ถูกต้อง: `https://www.nephrothai.org/category/สาระความรู้/` (percent-encoded:
`/category/%E0%B8%AA%E0%B8%B2%E0%B8%A3%E0%B8%B0%E0%B8%84%E0%B8%A7%E0%B8%B2%E0%B8%A1%E0%B8%A3%E0%B8%B9%E0%B9%89/`)
— **มีทั้งหมด 4 หน้า** (pagination มาตรฐาน WordPress `/page/2/`, `/page/3/`, `/page/4/`)

**RSS feed เฉพาะหมวดนี้ก็มี และใช้งานได้จริง:** `/category/สาระความรู้/feed/` — ทดสอบแล้ว entries
ตรงกับที่เห็นในหน้า category พอดี (คนละ feed กับ `/feed/` หลักที่รวมทุกหมวด)

**ตัวอย่างบทความ 15 ชิ้นล่าสุด (จากหน้า 1-2):**

| Title | Date | หมวดหมู่ย่อย (tag) |
|---|---|---|
| คำแนะนำสำหรับการดูแลและรักษาโรคไต | 7 มี.ค. 2569 | สาระความรู้ |
| คำแนะนำผู้ป่วยโรคไต ในช่วงสถานการณ์น้ำท่วม | 2 ธ.ค. 2568 | ข่าวประชาสัมพันธ์ |
| คำแนะนำมาตรฐานการฟอกเลือดนอกหน่วยไตเทียมโดยบุคลากรทางการแพทย์ | 28 ต.ค. 2568 | Hemodialysis |
| แนวทางการดูแลผู้ป่วยโรคไตเรื้อรัง โดยเฉพาะในระดับปฐมภูมิ (One page for CKD care) | 30 พ.ค. 2568 | CKD |
| Update คำแนะนำในการส่งต่อผู้ป่วยโรคไตเรื้อรังเพื่อผ่าตัดเตรียมหลอดเลือด | 7 พ.ค. 2568 | Hemodialysis |
| คำแนะนำสำหรับการดูแลรักษาสนับสนุนและประคับประคองผู้ป่วยโรคไตเรื้อรัง 2566 (infographics) | 6 ม.ค. 2568 | CKD |
| คำแนะนำสำหรับการดูแลรักษาสนับสนุนและประคับประคองผู้ป่วยโรคไตเรื้อรัง 2566 | 6 ม.ค. 2568 | CKD |
| คำแนะนำด้านอาหารสำหรับผู้ป่วยโรคไตในช่วงภัยพิบัติ | 26 ส.ค. 2567 | สาระความรู้ |
| Video แนะนำสำหรับการดูแลผู้ป่วยโรคไตเรื้อรังก่อนการบำบัดทดแทนไต 2565 | 7 ส.ค. 2567 | CKD |
| คำแนะนำการบริหารจัดการหน่วยฟอกเลือดในระหว่างการระบาดของโควิด-19 | 6 ก.ค. 2566 | Hemodialysis |
| การคัดกรองและดูแลรักษาโรคไตเรื้อรังในหน่วยบริการปฐมภูมิ | 23 มิ.ย. 2566 | CKD |
| คำแนะนำสำหรับการดูแลผู้ป่วยโรคไตเรื้อรังก่อนการบำบัดทดแทนไต 2565 (infographics) | 23 มิ.ย. 2566 | CKD |
| แนวทางเวชปฏิบัติโรคติดเชื้อทางเดินปัสสาวะในผู้ป่วยเด็กอายุ 2 เดือน-5 ปี 2565 | 22 มิ.ย. 2566 | สาระความรู้ |
| คำแนะนำสำหรับการดูแลผู้ป่วยโรคไตเรื้อรังก่อนการบำบัดทดแทนไต 2565 (ฉบับปรับปรุง) | 6 มิ.ย. 2566 | CKD |
| แนวทางการรักษาภาวะโลหิตจางในผู้ป่วยโรคไตเรื้อรัง 2564 | 24 มิ.ย. 2565 | สาระความรู้ |

⚠️ **ไม่มีบทความที่เจาะจง CAPD/APD/PD โดยตรงในตัวอย่างที่เจอ** — เนื้อหาหมวดนี้เน้น CKD ทั่วไปและ
Hemodialysis เป็นหลัก ต้องเช็คหน้า 3-4 เพิ่มถ้าต้องการเนื้อหาเจาะจง PD

**โครงสร้างหน้าบทความ (ตรวจจากตัวอย่างจริง 1 ชิ้น + ดู RSS `<content:encoded>` ดิบ):**

- เว็บสร้างด้วย **WordPress + Elementor page builder** — เนื้อหาอยู่ใน `<div data-elementor-type=
  "wp-post">` ซ้อน section/column/widget ตาม Elementor ปกติ ไม่ใช่ `<div class="entry-content">`
  แบบ WordPress theme ทั่วไป
- **ไม่มี author name แสดงบนหน้า**
- มี publish date แสดง (รูปแบบ "7 มีนาคม 2569") และมี category tag เดียว (คลิกได้)
- **ไม่มี featured image ในตัวอย่างที่เช็ค**
- **จุดสำคัญที่สุด: เนื้อหาส่วนใหญ่ในหมวดนี้ไม่ใช่ prose text แต่เป็นปุ่มดาวน์โหลด PDF, embed
  video, หรือ infographic image** — ตัวอย่างที่เช็คมีแค่ปุ่ม "ดาวน์โหลดคำแนะนำสำหรับการดูแล และ
  รักษาโรคไต" ลิงก์ไปที่ `wp-content/uploads/2026/03/Kidney-book-in-Thai.pdf` ไม่มี prose ให้อ่าน/
  สรุปบนหน้าเว็บเลย (ทั้งหน้ามีตัวอักษรแค่ ~15-20 คำ) — สอดคล้องกับรูปแบบชื่อบทความในตารางด้านบนที่
  ขึ้นต้นด้วย "คำแนะนำ...", "แนวทาง...", "Video แนะนำ...", "โบรชัวร์...", "คลิป Video..." ซึ่งบ่งชี้ว่า
  ส่วนใหญ่เป็นเอกสาร/สื่อดาวน์โหลด ไม่ใช่บทความ

**ผลต่อการออกแบบ:**

- ✅ **ไม่ต้อง scrape HTML แยก** — RSS `<content:encoded>` field มี HTML เต็มของ Elementor
  (รวมถึง href ของปุ่มดาวน์โหลด PDF) อยู่แล้ว ดึงจาก RSS parse อย่างเดียวพอ
- ⚠️ **แต่ AI สรุปเป็นภาษาไทยจาก "เนื้อหาบทความ" ตรงๆ ไม่ได้สำหรับหมวดนี้ส่วนใหญ่** เพราะไม่มี prose
  ให้สรุป มีแต่ลิงก์ PDF — ถ้าต้องการสรุปจริงๆ ต้องเพิ่มขั้นตอนแยกดึงข้อความจาก PDF (PDF text
  extraction) ซึ่งเป็นงานเพิ่มอีกชั้นที่ยังไม่ได้ประเมิน หรือไม่ก็แสดงแค่ title + category + ปุ่ม
  "ดาวน์โหลดเอกสารต้นฉบับ" โดยไม่มี AI summary สำหรับ item ประเภทนี้
- แนะนำให้ **แยก 2 กรณีตอน parse**: (1) item ที่มี prose จริงในหน้า (ถ้ามี — ต้องหาตัวอย่างเพิ่ม เพราะ
  ที่เช็คมาทั้งหมดเป็น PDF/video) → สรุปด้วย AI ได้ตามปกติ (2) item ที่เป็น PDF/video/infographic
  เท่านั้น → แสดง title + link กลับต้นฉบับ ไม่มี AI summary (หรือทำ PDF extraction แยกในเฟสถัดไป)

**Method แนะนำ:** RSS parse จาก `/feed/` (มาตรฐาน WordPress, เสถียร ไม่ต้อง scrape HTML)

**ความถี่:** วันละครั้งพอ — ความถี่โพสต์ของสมาคมต่ำ (มีข่าวใหม่ทุกไม่กี่วัน)

**ตัวอย่างข้อมูลจริง (จากหน้า ข่าวประชาสัมพันธ์):**

| Title | Date | URL |
|---|---|---|
| ประชาสัมพันธ์ งานประชุมใหญ่ประจำปี 2569 | 7 ก.ค. 2569 | nephrothai.org/ประชาสัมพันธ์-งานประชุ... |
| ตารางกิจกรรมวิชาการ สมาคมโรคไตฯ ปี 2569 | 7 ก.ค. 2569 | nephrothai.org/ตารางกิจกรรมวิชาการ... |
| ประกาศผลการเลือกตั้งคณะผู้บริหารฯ วาระ 2569-2571 | 6 ก.ค. 2569 | nephrothai.org/ประกาศผลการเลือกตั้ง... |
| ประกาศรายชื่อผู้มีสิทธิ์เข้าอบรม TRT Coordinator รอบ 2/2569 | 15 มิ.ย. 2569 | nephrothai.org/ประกาศรายชื่อผู้มีสิทธิ์... |
| ประกาศผลสอบการอบรมพยาบาล TRT Coordinator รอบ 1/2569 | 4 มิ.ย. 2569 | nephrothai.org/ประกาศผลสอบการอบรม... |

(URL เต็มเป็น percent-encoded ภาษาไทยยาว — ตัดไว้ในตารางเพื่อความอ่านง่าย ดูของจริงได้จาก RSS feed)

---

## 2. kidney.org (National Kidney Foundation)

**Automated access: ❌ ไม่ได้ — ToS ห้ามชัดเจน**

- **Terms of Use** (kidney.org/national-kidney-foundation-website-terms-use) เขียนตรงตัวว่า:
  > "use any scraper, crawler, spider, robot or other automated means of any kind to access or
  > copy data on our Website"
  ห้ามโดยไม่มีข้อยกเว้น และยังห้าม deep-linking ที่ข้าม navigation ปกติด้วย
- เรื่อง republish: > "you may not modify, copy, reproduce, republish, upload, post, transmit,
  hyperlink from, or distribute in any way Content" — ไม่มีข้อยกเว้นสำหรับสรุป/อ้างอิงแหล่งที่มา
  ต้องขอ permission เป็นลายลักษณ์อักษรเท่านั้น (ผ่านฟอร์มขอใช้เนื้อหา section 3.3)
- `robots.txt` เสริมอีกชั้น — บล็อก `/press-room?`, `/news-stories?`, `/kidney-topics?`,
  `/nutrition/recipes?` ทั้งเวอร์ชันอังกฤษและสเปน (หมายเหตุ: บล็อกเฉพาะ path ที่มี query string ต่อท้าย
  `?` ตาม syntax มาตรฐาน — หน้า listing เปล่าๆ อาจไม่ติด แต่ไม่สำคัญเพราะ ToS ห้ามอยู่แล้วไม่ว่า path ไหน)
- ไม่พบ RSS feed ที่ใช้งานได้ (ลอง `/news-stories/feed` → 404, ตรวจ HTML source ของหน้า
  news-stories ก็ไม่มี `<link rel="alternate" type="application/rss+xml">`)

**Method แนะนำ: ห้าม automate เด็ดขาด — ต้องทำมือ 100%** (เช่น ทีมงานอ่านเองแล้วเขียนสรุป+ลิงก์
กลับด้วยมือ ถ้าอยากใช้แหล่งนี้)

**ความถี่ (ถ้าทำมือ):** สัปดาห์ละครั้งก็เพียงพอ เพราะต้องใช้แรงคน

**ตัวอย่างข้อมูลจริง (สำหรับอ้างอิงออกแบบ schema เท่านั้น — ไม่ได้ดึงเนื้อหาเต็มมาเก็บ):**

| Title | Date | URL | หมวด |
|---|---|---|---|
| Rethinking Kidney Health: Advancing Combination Therapy in CKD | 7 ก.ค. 2026 | kidney.org/news-stories/rethinking-kidney-health-... | Professional Education |
| Cardiovascular-Kidney-Metabolic Syndrome (CKM) Has More Options Than Ever. Now What? | 7 ก.ค. 2026 | kidney.org/news-stories/cardiovascular-kidney-metabolic-... | Research, Patient Education |
| A Final Message from Kevin Longino as CEO | 30 มิ.ย. 2026 | kidney.org/news-stories/final-message-kevin-longino-ceo | News |
| Kidney Disease Education Access Expansion Act... | 30 มิ.ย. 2026 | kidney.org/news-stories/kidney-disease-education-access-... | Advocacy |
| Kidney-Friendly Fourth of July Recipes... | 25 มิ.ย. 2026 | kidney.org/news-stories/kidney-friendly-fourth-july-recipes-... | Patient Education |

⚠️ ไม่เจอบทความที่เจาะจง PD/CAPD/APD ตรงๆ ใน 5 อันดับแรก — เนื้อหาส่วนใหญ่เป็น CKD ทั่วไป/นโยบาย/
สูตรอาหาร ต้องเข้าไปหาในหมวดย่อยเพิ่มถ้าจะใช้จริง (แต่ทำไม่ได้แบบอัตโนมัติอยู่ดี)

---

## 3. healio.com/nephrology

**Automated access: ❌ ไม่ได้ — ทั้ง ToS และ robots.txt ห้ามชัดเจนกว่า kidney.org อีก**

- **robots.txt** (https://www.healio.com/robots.txt) มีรายชื่อ AI bot ที่ถูกบล็อกตรงๆ รวมถึง
  `ClaudeBot`, `Claude-Web`, `anthropic-ai`, `GPTBot`, `ChatGPT`, `ChatGPT-User`, `PerplexityBot`,
  `CCBot`, `Ai2Bot` และอื่นๆ อีกกว่า 20 ตัว ด้วย `Disallow: /` (บล็อกทั้งเว็บ) — **เครื่องมือที่ใช้สร้าง
  ฟีเจอร์นี้ (Claude) เป็นหนึ่งใน user-agent ที่ถูกระบุชื่อบล็อกตรงๆ**
  - ส่วน `User-agent: *` ทั่วไปไม่ได้บล็อกกว้าง และมีบรรทัด `Allow:/sws/feed/news/*` ที่บ่งชี้ว่ามี
    feed endpoint ที่ตั้งใจให้ crawl ได้ — ทดสอบแล้ว `https://www.healio.com/sws/feed/news/nephrology`
    เป็น RSS 2.0 ใช้งานได้จริง มีบทความ nephrology จริง
  - แต่การใช้ user-agent อื่นเพื่อเลี่ยงบล็อกเฉพาะของ AI bot ถือเป็นการหลบเลี่ยงเจตนาเว็บไซต์ชัดเจน
    ไม่แนะนำไม่ว่าทางเทคนิคจะทำได้หรือไม่
- **Terms and Conditions** (healio.com/terms-and-conditions) ห้ามชัดเจนยิ่งกว่า kidney.org:
  > "The use of content on Healio.com... may not be used in the development of any software and
  > expressly may not be used to train a machine learning or artificial intelligence system
  > without the express permission of The Wyanoke Group."

  และ: "No one has permission to reproduce, copy, display, transmit, distribute, republish, or
  create derivative works from such information in any form" — พร้อมประกาศสงวนสิทธิ์ "for text and
  data mining, AI training, and similar technologies" ตรงตัว
- สรุป: แม้ RSS feed จะเปิดให้ดึงได้ทางเทคนิค แต่ **ToS ห้ามใช้เนื้อหาไปพัฒนา/ฝึก AI หรือ software
  โดยไม่ขออนุญาตเป็นลายลักษณ์อักษร** ซึ่งตรงกับสิ่งที่เรากำลังจะทำเป๊ะๆ (ใช้ AI สรุปเนื้อหาลงแอป)

**Method แนะนำ: ห้าม automate เด็ดขาด เว้นแต่ติดต่อขอ permission จาก Wyanoke Group
(customerservice@slackinc.com) เป็นลายลักษณ์อักษรก่อน** — ถ้าไม่ขอ ต้องทำมือเหมือน kidney.org

**ความถี่ (ถ้าได้ permission แล้ว):** วันละครั้ง (feed อัปเดตบ่อย มีข่าวเกือบทุกวัน)

**ตัวอย่างข้อมูลจริง (จาก RSS feed — สำหรับอ้างอิงออกแบบเท่านั้น):**

| Title | Date | URL |
|---|---|---|
| What LEAD tells us about next era of kidney care | 8 ก.ค. 2026 | healio.com/news/nephrology/20260708/what-lead-tells-us-... |
| FDA approves Trutakna for IgA nephropathy | 7 ก.ค. 2026 | healio.com/news/nephrology/20260707/fda-approves-trutakna-... |

(ดึงมาแค่ 2 ตัวอย่างจาก feed จริง เพื่อยืนยันว่า feed ใช้งานได้ — ไม่ได้ดึงเก็บเพิ่มเพราะ ToS ห้าม)

---

## 4. PubMed / NCBI E-utilities API

**Automated access: ✅ ได้แน่นอน — เป็น public API ที่ NIH ออกแบบมาให้ใช้แบบนี้โดยเฉพาะ**

- ทดสอบจริงกับ endpoint `esearch.fcgi`, `esummary.fcgi`, `efetch.fcgi` — ใช้งานได้ฟรี ไม่ต้องสมัคร
  หรือขอ key สำหรับปริมาณใช้งานทั่วไป
- **Rate limit:** ไม่มี API key = สูงสุด 3 requests/วินาทีต่อ IP. มี API key (สมัครฟรีผ่าน NCBI
  account) = สูงสุด 10 requests/วินาที. เกินนี้ขอเพิ่มได้ผ่านติดต่อ NCBI โดยตรง
- **ต้องระบุตัวตน:** ควรส่ง parameter `tool=` (ชื่อแอป) และ `email=` (อีเมลติดต่อ) ทุก request — ถ้า
  IP ไหนไม่ระบุแล้วใช้งานเกิน policy อาจถูกบล็อก
- **ทดสอบ query จริง:** `esearch.fcgi?db=pubmed&term="peritoneal dialysis" AND CAPD` →
  พบ 12,201 บทความ, คืน PMID list ได้ปกติ (เช่น 42403152, 42377652, ...)
- **esummary** คืนแค่ metadata (title, authors, journal, pubdate, DOI, PMCID) — **ไม่มี abstract**
  แม้จะมี field `"attributes":["Has Abstract"]` บอกว่ามี abstract อยู่ในระบบก็ตาม
- **efetch** (`rettype=abstract&retmode=text`) คืน **abstract เต็ม** จริง (มี Objective/Methods/
  Results/Conclusion) — ถ้าจะปลอดภัยสุดสำหรับ fair use ควรใช้ **esummary (title+link+metadata)
  เป็นหลัก** แล้วใช้ abstract จาก efetch แค่เป็น *input ให้ AI เขียนสรุปภาษาไทยของเราเอง* — ไม่ควร
  เก็บ/แสดง abstract ภาษาอังกฤษเต็มๆ ตรงๆ ในแอป เพราะ "abstracts in PubMed may incorporate material
  that may be protected by U.S. and foreign copyright laws" (ตามเอกสาร NCBI เอง) — ตัว abstract เป็น
  ลิขสิทธิ์ของวารสารต้นทาง ไม่ใช่ของ NCBI แม้ NCBI จะให้ดึงได้ก็ตาม
- **ข้อกำหนดอื่น:** ต้องแสดง NCBI's Disclaimer and Copyright notice ในซอฟต์แวร์ที่ใช้ E-utilities.
  ถ้าจะโหลดข้อมูลจำนวนมาก NCBI แนะนำให้โหลด local copy ของ database แทนการ query ซ้ำๆ (ไม่ใช่ปัญหา
  สำหรับ use case เรา เพราะดึงแค่บทความใหม่ๆ เป็นระยะ)

**Method แนะนำ:** เรียก API ตรง — `esearch` หาบทความใหม่ตาม keyword (เช่น "peritoneal dialysis",
"CAPD", "APD" ร่วมกับ MeSH term ที่เกี่ยวข้อง) → `esummary` ดึง title/date/journal/link →
(ถ้าต้องการสรุปเนื้อหา) `efetch` ดึง abstract มาเป็น input ให้ AI สรุปเป็นภาษาไทยเอง ไม่ republish
abstract เต็ม → เก็บแค่ PMID+ลิงก์ https://pubmed.ncbi.nlm.nih.gov/{PMID}/ กลับไปต้นทาง

**ความถี่:** วันละครั้งพอ (งานวิจัยใหม่ไม่ได้ออกทุกชั่วโมง, sort by pubdate + filter เฉพาะที่ยังไม่เคย
เก็บ PMID ไว้)

**ตัวอย่างข้อมูลจริง (query: "peritoneal dialysis" AND CAPD, เรียงล่าสุดก่อน):**

| PMID | Title | Journal | Date |
|---|---|---|---|
| 42403152 | Clinical efficacy and safety of icodextrin dialysate for overnight dwell in continuous ambulatory peritoneal dialysis | J Int Med Res | ก.ค. 2026 |
| 42377652 | Denosumab increased bone mineral density but caused marked serum calcium fluctuations in a patient undergoing peritoneal dialysis | CEN Case Rep | 30 มิ.ย. 2026 |
| 42367183 | Pleuroperitoneal Fistula, an Unusual Case of Recurrent Unilateral Transudative Pleural Effusion | Respirol Case Rep | ก.ค. 2026 |
| 42351051 | Evaluation of the epidemiologic triad in the incidence of peritonitis among pediatric patients with chronic kidney disease undergoing peritoneal dialysis | BMC Pediatr | 26 มิ.ย. 2026 |
| 42317431 | Correction to 'Clostridium difficile Peritonitis Complicated by Splenic Rupture and Pelvic Abscess Formation...' | Case Rep Gastrointest Med | 2026 |

⚠️ หมายเหตุ: ผลลัพธ์ 5 อันดับแรกจาก query กว้างๆ นี้ส่วนใหญ่เป็น case report เฉพาะทาง (เหมาะสำหรับ
แพทย์มากกว่าผู้ป่วยทั่วไป) — ถ้าจะใช้กับผู้ป่วย pdlife.app ควรออกแบบ query ให้แคบลง/กรองด้วย
publication type (เช่น review, patient education) หรือใช้ MeSH term เจาะจงกว่านี้ ไม่ใช้ query
ตรงๆ แบบที่ทดสอบนี้

### ทดสอบเปรียบเทียบ query แบบกรอง (สำหรับเนื้อหาที่คนไข้อ่านง่ายขึ้น)

ทดสอบจริงผ่าน `esearch.fcgi` เทียบกับ query พื้นฐาน (baseline: `"peritoneal dialysis" AND CAPD` =
**12,201 ผลลัพธ์**, ส่วนใหญ่เป็น case report เฉพาะทาง):

**(A) เพิ่ม `AND "patient education"`** → **630 ผลลัพธ์**

| PMID | Title | Journal |
|---|---|---|
| 42045493 | Assessing quality and reliability of online videos on peritoneal dialysis: a Douyin video-based study | Scientific Reports |
| 41955295 | Results of a new model of videotraining for patients and caregivers for peritoneal dialysis: food for (AI) thought? | Journal of Nephrology |
| 41795759 | Facility-based educational systems and peritonitis incidence in peritoneal dialysis: findings from a nationwide survey in Japan | Clin Exp Nephrol |
| 41721291 | Baseline characteristics of the TEACH-PD trial participants... | BMC Nephrology |
| 41598631 | Artificial Intelligence Chatbots in Peritoneal Dialysis Education: A Cross-Sectional Comparative Study | J Clin Med |

ลดจำนวนลงเยอะ (12,201 → 630) และหัวข้อเกี่ยวกับการให้ความรู้ผู้ป่วยจริง แต่ตัวบทความเองยังเป็นงานวิจัย
เชิงวิชาการ (เช่น "ประเมินคุณภาพวิดีโอ", "เปรียบเทียบ AI chatbot") ไม่ใช่บทความสรุปความรู้แบบอ่านง่าย
โดยตรง — เหมาะเป็น "แหล่งไอเดียหัวข้อ" มากกว่าเนื้อหาที่เอาไปสรุปตรงๆ ได้ทันที

**(B) เพิ่ม `AND "Review"[Publication Type]`** → **1,173 ผลลัพธ์**

| PMID | Title | Journal |
|---|---|---|
| 42284506 | Current approaches to prescription and optimization of peritoneal dialysis: a practical review | Jornal brasileiro de nefrologia |
| 42232595 | Environmental footprint of peritoneal dialysis in Europe: a comparative life cycle assessment | Clinical kidney journal |
| 41630234 | Peritoneal dialysis-associated peritonitis caused by Eikenella corrodens... case report and literature review | Medicine |
| 41437224 | Successful treatment of peritonitis associated with Campylobacter fetus... case report and literature review | BMC Infectious Diseases |
| 41427443 | Optimizing CAPD Patient Monitoring Through Automated Vs Rule-Based AI: A Systematic Comparative Review | Int J Nephrol Renovasc Dis |

ลดจำนวนลงมากสุด (12,201 → 1,173) และได้ "practical review" ที่เป็นภาพรวมกว้างๆ อ่านง่ายกว่างานวิจัย
เดี่ยว — แต่สังเกตว่า filter นี้จับ "case report...and literature review" ติดมาด้วย (เพราะ NCBI มองว่า
มี component "review" อยู่) ทำให้ยังมี case report ปนอยู่บ้าง ไม่ได้กรองสะอาด 100%

**(C) เพิ่ม `NOT "case reports"[Publication Type]`** → **10,042 ผลลัพธ์**

ลดจากฐาน 12,201 แค่นิดเดียว (เหลือ 10,042) — **ไม่ค่อยได้ผล** เพราะบทความจำนวนมากที่เนื้อหาเป็น
"case report" ในทางปฏิบัติ (เช่น 2 ชิ้นที่เจอใน (B) ข้างต้น) ไม่ได้ถูกแท็ก publication type ว่า
"Case Reports" อย่างเป็นทางการใน PubMed metadata — กรองแบบนี้ไม่แม่นพอที่จะพึ่งอย่างเดียว

**คำแนะนำ query สุดท้าย:** ใช้ **(B) `"peritoneal dialysis" AND CAPD AND "Review"[Publication
Type]`** เป็นตัวหลัก เพราะลดปริมาณได้มากสุดและได้บทความภาพรวม/practical review ที่สรุปง่ายกว่า
case report เดี่ยวๆ แต่ **ต้องมีการกรอง/คัดอีกชั้นก่อน publish จริง** (เช่น ให้ AI หรือคนอ่าน title+
abstract แล้วตัดสินใจอีกทีว่าเหมาะกับผู้ป่วยทั่วไปไหม ไม่ใช่ auto-publish ทุกผลลัพธ์จาก query) เพราะ
แม้แต่ "review" ก็ยังมีทั้งแบบภาพรวมสำหรับผู้ป่วยและแบบเชิงเทคนิคสำหรับแพทย์ปนกันอยู่

---

## สรุป & คำแนะนำ

| แหล่ง | Automate ได้ไหม | ความเสี่ยงกฎหมาย | เริ่มก่อน/หลัง |
|---|---|---|---|
| **PubMed/NCBI E-utilities** | ✅ ได้เต็มที่ | ต่ำสุด — public API ที่ NIH ออกแบบมาให้ใช้แบบนี้ | **เริ่มก่อนสุด** |
| **nephrothai.org** | ✅ ได้ (RSS) | ต่ำ — ไม่มีข้อห้ามชัดเจนที่เจอ, ควรเช็ค ToS ซ้ำให้ชัวร์ | **เริ่มก่อนสุด** |
| **kidney.org** | ❌ ห้าม | ToS ห้าม automated access ตรงตัว | ทำมือเท่านั้น หรือข้ามไปก่อน |
| **healio.com** | ❌ ห้าม | ToS ห้าม + robots.txt บล็อก AI bot ชื่อ Claude ตรงๆ | ทำมือเท่านั้น หรือขอ permission เป็นทางการก่อน |

**แนะนำ:** เริ่ม build ด้วย **PubMed E-utilities + nephrothai.org RSS** ก่อน สองแหล่งนี้ไม่มีปัญหา
ด้านกฎหมาย/ToS ให้กังวล ครอบคลุมทั้งงานวิจัยเชิงลึก (PubMed) และข่าวสารภาษาไทยจากสมาคมวิชาชีพในประเทศ
(nephrothai) — เพียงพอสำหรับ MVP ของฟีเจอร์นี้แล้ว

**kidney.org และ healio.com ควรพักไว้ก่อน** ทั้งคู่มีเนื้อหาดีสำหรับผู้ป่วย (patient education,
research) แต่ automated pipeline ผิด ToS ชัดเจนทั้งคู่ — โดยเฉพาะ healio.com ที่ระบุชื่อบล็อก AI bot
ตรงๆ รวมถึง Claude ที่กำลังใช้เขียนโค้ดนี้อยู่ ถ้าอยากใช้สองแหล่งนี้ในอนาคตต้องเลือกทางใดทางหนึ่ง:
(1) ทำสรุปด้วยมือเป็นระยะ (ทีมงานอ่านเองแล้วเขียนสรุป+ลิงก์) หรือ (2) ติดต่อขอ permission เป็นทางการ
จากเจ้าของเว็บก่อน — **ไม่ควร build automated scraper สำหรับสองแหล่งนี้โดยไม่ได้รับอนุญาต**

**ยังไม่ได้สำรวจ/ต้องเช็คเพิ่มก่อน build จริง:**
- ✅ ~~structure ของหมวด "สาระความรู้"~~ — สำรวจแล้ว (ดูหัวข้อย่อยใน section 1) พบว่าเนื้อหาส่วนใหญ่
  เป็น PDF/video download ไม่ใช่ prose — ต้องออกแบบ 2 เส้นทางแยกกันสำหรับ item ที่มี prose จริง vs.
  item ที่เป็นแค่ลิงก์ดาวน์โหลด
- ✅ ~~MeSH term / publication type filter สำหรับ PubMed~~ — ทดสอบแล้ว แนะนำ `"Review"[Publication
  Type]` เป็นตัวหลัก (ดู section 4) แต่ยังต้องมี manual/AI curation อีกชั้นก่อน publish
- หน้า ToS/Terms ของ nephrothai.org แบบเจาะจง (หาไม่เจอหน้า legal แยก — ตอนนี้อ้างอิงจากการโทรถาม
  ผู้ดูแลเว็บโดยตรงแทน ตามที่บันทึกไว้ใน section 1 — ถ้าเป็นไปได้ควรขอให้ยืนยันเป็นลายลักษณ์อักษร
  (อีเมล) เก็บไว้เป็นหลักฐานเพิ่มเติมจากการโทรศัพท์)
- ยังไม่เจอตัวอย่างบทความในหมวด "สาระความรู้" ที่เป็น prose จริง (ทุกตัวอย่างที่เช็คมาเป็น PDF/video) —
  ควรเช็คหน้า 3-4 เพิ่ม หรือหมวดอื่นที่อาจมีบทความ prose (เช่น "ฐานข้อมูลโรคไต") ก่อนสรุปว่าทั้งเว็บไม่มี
  prose เลย
- ยังไม่พบบทความเจาะจง CAPD/APD/PD บนหมวด "สาระความรู้" โดยตรง (เจอแต่ CKD/Hemodialysis ทั่วไป) —
  ต้องเช็คหน้าที่เหลือหรือหมวดอื่นเพิ่ม
