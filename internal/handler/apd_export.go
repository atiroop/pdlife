package handler

import (
	"embed"
	"fmt"
	"net/http"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"github.com/atiroop/pdlife/internal/models"
)

//go:embed assets/fonts/NotoSansThai-Regular.ttf
var thaiFontFS embed.FS

// exportColumn mirrors the legacy export/route.ts column list exactly, in
// the same order, so the exported file has the same shape as before.
type exportColumn struct {
	header string
	width  float64 // relative width, mirrors the legacy "width" units
}

var exportColumns = []exportColumn{
	{"วันที่", 70},
	{"รอบที่", 35},
	{"เวลาเริ่ม", 50},
	{"น้ำหนัก (kg)", 55},
	{"ความดัน", 50},
	{"ชีพจร", 40},
	{"น้ำตาล (mg/dL)", 60},
	{"I-Drain (ml)", 55},
	{"Total UF (ml)", 60},
	{"ปัสสาวะ/วัน (ml)", 65},
	{"น้ำยาออก", 90},
	{"ใบสั่งฯ", 90},
	{"หมายเหตุ", 110},
}

func exportRowValues(l models.ApdLogEntry, prescriptionNames map[uint64]string) []string {
	glucose := "—"
	if l.BloodGlucoseMgDL != nil {
		glucose = fmt.Sprintf("%d", *l.BloodGlucoseMgDL)
	}
	drainage := formatDrainageAppearance(l.DrainageAppearance)
	prescriptionName := ""
	if l.PrescriptionID != nil {
		prescriptionName = prescriptionNames[*l.PrescriptionID]
	}
	remark := ""
	if l.Remark != nil {
		remark = *l.Remark
	}
	return []string{
		formatEntryDate(l.EntryDate),
		fmt.Sprintf("%d", l.CycleNumber),
		l.TreatmentStartTime,
		fmt.Sprintf("%.2f", l.WeightKG),
		fmt.Sprintf("%d/%d", l.BPSystolic, l.BPDiastolic),
		fmt.Sprintf("%d", l.Pulse),
		glucose,
		fmt.Sprintf("%d", l.IDrainVolumeML),
		fmt.Sprintf("%d", l.TotalUFML),
		fmt.Sprintf("%d", l.UrineAvgDayML),
		drainage,
		prescriptionName,
		remark,
	}
}

func formatDrainageAppearance(v *string) string {
	if v == nil || *v == "" {
		return "—"
	}
	return *v
}

// ---- GET /apd/export ----

func (h *AuthHandler) ApdExport(c echo.Context) error {
	user, profile, err := h.requireApdPatient(c)
	if user == nil {
		return err
	}

	days := 7
	if c.QueryParam("days") == "30" {
		days = 30
	}
	format := c.QueryParam("format")
	if format != "pdf" {
		format = "xlsx"
	}

	start := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
	var logs []models.ApdLogEntry
	h.DB.Where("patient_profile_id = ? AND entry_date >= ?", profile.ID, start).
		Order("entry_date ASC, cycle_number ASC").Find(&logs)

	prescriptionNames := map[uint64]string{}
	var prescriptions []models.ApdPrescription
	h.DB.Where("patient_profile_id = ?", profile.ID).Find(&prescriptions)
	for _, p := range prescriptions {
		prescriptionNames[p.ID] = p.Name
	}

	if format == "pdf" {
		buf, err := buildApdPdf(logs, prescriptionNames, days)
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="apd-log-%d-days.pdf"`, days))
		return c.Blob(http.StatusOK, "application/pdf", buf)
	}

	buf, err := buildApdXlsx(logs, prescriptionNames)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="apd-log-%d-days.xlsx"`, days))
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf)
}

func buildApdXlsx(logs []models.ApdLogEntry, prescriptionNames map[uint64]string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	const sheet = "APD Log"
	f.SetSheetName("Sheet1", sheet)

	for i, col := range exportColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, col.header)
		colName, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, colName, colName, col.width/4)
	}

	for r, l := range logs {
		row := r + 2
		values := exportRowValues(l, prescriptionNames)
		for i, v := range values {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildApdPdf(logs []models.ApdLogEntry, prescriptionNames map[uint64]string, days int) ([]byte, error) {
	fontBytes, err := thaiFontFS.ReadFile("assets/fonts/NotoSansThai-Regular.ttf")
	if err != nil {
		return nil, err
	}

	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "L",
		UnitStr:        "mm",
		SizeStr:        "A4",
	})
	pdf.AddUTF8FontFromBytes("thai", "", fontBytes)
	pdf.SetFont("thai", "", 10)
	pdf.SetMargins(8, 8, 8)
	pdf.AddPage()

	pdf.SetFont("thai", "", 14)
	pdf.CellFormat(0, 8, fmt.Sprintf("สมุดบันทึก APD - ย้อนหลัง %d วัน", days), "", 1, "C", false, 0, "")
	pdf.Ln(2)

	// mm widths scaled from the legacy pt-based column widths.
	totalWidth := 0.0
	for _, col := range exportColumns {
		totalWidth += col.width
	}
	pageWidth := 281.0 // A4 landscape usable width in mm at these margins
	colWidths := make([]float64, len(exportColumns))
	for i, col := range exportColumns {
		colWidths[i] = col.width / totalWidth * pageWidth
	}

	drawRow := func(values []string, isHeader bool) {
		if isHeader {
			pdf.SetFont("thai", "", 9)
		} else {
			pdf.SetFont("thai", "", 8.5)
		}
		_, pageHeight := pdf.GetPageSize()
		_, _, _, bottomMargin := pdf.GetMargins()
		if pdf.GetY()+6 > pageHeight-bottomMargin {
			pdf.AddPage()
			pdf.SetFont("thai", "", isHeaderFontSize(isHeader))
		}
		for i, v := range values {
			pdf.CellFormat(colWidths[i], 6, v, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}

	header := make([]string, len(exportColumns))
	for i, col := range exportColumns {
		header[i] = col.header
	}
	drawRow(header, true)

	if len(logs) == 0 {
		pdf.SetFont("thai", "", 10)
		pdf.CellFormat(0, 8, "ยังไม่มีข้อมูลในช่วงเวลานี้", "", 1, "L", false, 0, "")
	}

	for _, l := range logs {
		drawRow(exportRowValues(l, prescriptionNames), false)
	}

	var buf pdfBuffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isHeaderFontSize(isHeader bool) float64 {
	if isHeader {
		return 9
	}
	return 8.5
}

type pdfBuffer struct{ data []byte }

func (b *pdfBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *pdfBuffer) Bytes() []byte { return b.data }
