# Known Issues

## PDF Export — Thai text layer ผิดพลาด (ไม่กระทบการใช้งานจริง)

ปัญหา: PDF ที่ export จาก APD Log Book มี text layer ภาษาไทยผิดพลาด
(ToUnicode CMap แมปผิดทุกตัวอักษร) ทำให้ copy ข้อความ/search ในไฟล์/screen reader ใช้ไม่ได้

ผลกระทบจริง: การแสดงผลและพิมพ์ PDF ถูกต้อง 100% (ยืนยันด้วย pdftoppm rasterize
เทียบกับข้อมูลจริงใน DB แล้ว) กระทบเฉพาะการเข้าถึงผ่าน screen reader หรือการ copy/search ข้อความ

สาเหตุ: บั๊กใน library go-pdf/fpdf (utf8fontfile.go) ทุกเวอร์ชัน v0.1.0–v0.9.0
เป็น unexported field เข้าถึงจากนอก package ไม่ได้ ต้อง fork ถึงจะแก้ได้

แนวทางแก้ระยะยาว (ยังไม่ทำ): เปลี่ยนวิธี generate PDF เป็น render HTML ผ่าน
headless Chrome หรือ wkhtmltopdf แทนการใช้ go-pdf/fpdf โดยตรง

Priority: Low — ไม่กระทบการใช้งานหลักของคนไข้/แพทย์ พิจารณาแก้ถ้ามีคนไข้ที่ใช้ screen reader จริง
วันที่บันทึก: 2026-07-08
