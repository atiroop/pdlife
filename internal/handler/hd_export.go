package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
)

// hdExportColumns mirrors hd_logs.html's table columns.
var hdExportColumns = []exportColumn{
	{"วันที่", 70},
	{"น้ำหนักแห้ง (kg)", 65},
	{"ก่อนฟอก น้ำหนัก (kg)", 70},
	{"ก่อนฟอก ความดัน", 60},
	{"หลังฟอก น้ำหนัก (kg)", 70},
	{"หลังฟอก ความดัน", 60},
	{"UF (ml)", 55},
	{"หมายเหตุ", 120},
}

func hdExportRowValues(l models.HdLogEntry) []string {
	notes := ""
	if l.Notes != nil {
		notes = *l.Notes
	}
	return []string{
		formatEntryDate(l.LogDate),
		fmt.Sprintf("%.2f", l.DryWeightKG),
		fmt.Sprintf("%.2f", l.PreDialysisWeightKG),
		fmt.Sprintf("%d/%d", l.PreDialysisBPSystolic, l.PreDialysisBPDiastolic),
		fmt.Sprintf("%.2f", l.PostDialysisWeightKG),
		fmt.Sprintf("%d/%d", l.PostDialysisBPSystolic, l.PostDialysisBPDiastolic),
		fmt.Sprintf("%d", l.UFRemovedML),
		notes,
	}
}

// ---- GET /hd/export ----

func (h *AuthHandler) HdExport(c echo.Context) error {
	user, profile, err := h.requireHdPatient(c)
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
	var logs []models.HdLogEntry
	h.DB.Where("patient_profile_id = ? AND log_date >= ?", profile.ID, start).
		Order("log_date ASC").Find(&logs)

	rows := make([][]string, len(logs))
	for i, l := range logs {
		rows[i] = hdExportRowValues(l)
	}

	if format == "pdf" {
		title := fmt.Sprintf("สมุดบันทึก HD - ย้อนหลัง %d วัน", days)
		buf, err := buildTablePdf(title, hdExportColumns, rows, "ยังไม่มีข้อมูลในช่วงเวลานี้")
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="hd-log-%d-days.pdf"`, days))
		return c.Blob(http.StatusOK, "application/pdf", buf)
	}

	buf, err := buildTableXlsx("HD Log", hdExportColumns, rows)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="hd-log-%d-days.xlsx"`, days))
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf)
}
