package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
)

// exportColumn mirrors the legacy export/route.ts column list exactly, in
// the same order, so the exported file has the same shape as before.
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

	rows := make([][]string, len(logs))
	for i, l := range logs {
		rows[i] = exportRowValues(l, prescriptionNames)
	}

	if format == "pdf" {
		title := fmt.Sprintf("สมุดบันทึก APD - ย้อนหลัง %d วัน", days)
		buf, err := buildTablePdf(title, exportColumns, rows, "ยังไม่มีข้อมูลในช่วงเวลานี้")
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="apd-log-%d-days.pdf"`, days))
		return c.Blob(http.StatusOK, "application/pdf", buf)
	}

	buf, err := buildTableXlsx("APD Log", exportColumns, rows)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="apd-log-%d-days.xlsx"`, days))
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf)
}
