package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
)

// capdExportColumns mirrors capd_logs.html's table columns.
var capdExportColumns = []exportColumn{
	{"วันที่", 70},
	{"รอบ", 35},
	{"เดกซ์โทรส (%)", 55},
	{"น้ำยาเข้า (ml)", 60},
	{"น้ำยาออก (ml)", 60},
	{"UF สุทธิ (ml)", 60},
	{"ลักษณะน้ำยา", 65},
	{"น้ำหนัก (kg)", 55},
	{"ความดัน", 50},
	{"ปัสสาวะ (ml)", 60},
}

func capdExportRowValues(l models.CapdLogEntry) []string {
	urine := "—"
	if l.UrineOutputML != nil {
		urine = fmt.Sprintf("%d", *l.UrineOutputML)
	}
	return []string{
		formatEntryDate(l.LogDate),
		fmt.Sprintf("%d", l.CycleNumber),
		fmt.Sprintf("%.2f", l.DextroseConcentration),
		fmt.Sprintf("%d", l.FillVolumeML),
		fmt.Sprintf("%d", l.DrainVolumeML),
		fmt.Sprintf("%d", l.UFVolumeML),
		DialysateAppearanceLabel(l.DialysateAppearance),
		fmt.Sprintf("%.2f", l.WeightKG),
		fmt.Sprintf("%d/%d", l.BPSystolic, l.BPDiastolic),
		urine,
	}
}

// ---- GET /capd/export ----

func (h *AuthHandler) CapdExport(c echo.Context) error {
	user, profile, err := h.requireCapdPatient(c)
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
	var logs []models.CapdLogEntry
	h.DB.Where("patient_profile_id = ? AND log_date >= ?", profile.ID, start).
		Order("log_date ASC, cycle_number ASC").Find(&logs)

	rows := make([][]string, len(logs))
	for i, l := range logs {
		rows[i] = capdExportRowValues(l)
	}

	if format == "pdf" {
		title := fmt.Sprintf("สมุดบันทึก CAPD - ย้อนหลัง %d วัน", days)
		buf, err := buildTablePdf(title, capdExportColumns, rows, "ยังไม่มีข้อมูลในช่วงเวลานี้")
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="capd-log-%d-days.pdf"`, days))
		return c.Blob(http.StatusOK, "application/pdf", buf)
	}

	buf, err := buildTableXlsx("CAPD Log", capdExportColumns, rows)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="capd-log-%d-days.xlsx"`, days))
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf)
}
