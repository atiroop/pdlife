package handler

import (
	"fmt"
	"net/http"

	"github.com/go-pdf/fpdf"
	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
)

// labExportFields is every numeric field in export order, HD-only fields
// included unconditionally: export reports whatever a patient actually
// has recorded rather than filtering by treatment type the way the
// dashboard's abnormal-summary does (labSummaryData/buildLabAbnormalItems
// gate on isHD to avoid flagging fields a PD patient was never tested on
// as noise; a report of what exists has no such row to avoid — a PD
// patient simply has nothing in the URR/nPCR columns).
var labExportFields = append(append([]labNumericField{}, labNumericFields...), labHDNumericFields...)

// labResultsExportColumns is a wide table: date, every numeric field,
// Kt/V, every enum flag, then the three free-text fields.
func labResultsExportColumns() []exportColumn {
	cols := []exportColumn{{"วันที่", 70}}
	for _, f := range labExportFields {
		cols = append(cols, exportColumn{fmt.Sprintf("%s (%s)", f.Label, f.Unit), 55})
	}
	cols = append(cols, exportColumn{"Kt/V", 45})
	for _, f := range labEnumFields {
		cols = append(cols, exportColumn{f.Label, 50})
	}
	cols = append(cols, exportColumn{"CXR", 90}, exportColumn{"EKG", 90}, exportColumn{"หมายเหตุ", 100})
	return cols
}

func labResultsExportRowValues(r models.LabResult) []string {
	values := []string{formatEntryDate(r.LogDate)}
	for _, f := range labExportFields {
		if v := f.Get(r); v != nil {
			values = append(values, formatLabValue(*v))
		} else {
			values = append(values, "-")
		}
	}
	if r.KtVValue != nil {
		values = append(values, formatLabValue(*r.KtVValue))
	} else {
		values = append(values, "-")
	}
	for _, f := range labEnumFields {
		values = append(values, LabFlagShortLabel(f.Get(r)))
	}
	cxr, ekg, notes := "", "", ""
	if r.CXRFinding != nil {
		cxr = *r.CXRFinding
	}
	if r.EKGFinding != nil {
		ekg = *r.EKGFinding
	}
	if r.Notes != nil {
		notes = *r.Notes
	}
	return append(values, cxr, ekg, notes)
}

// ---- GET /lab-results/export ----

func (h *AuthHandler) LabResultsExport(c echo.Context) error {
	user, profile, err := h.requireLabResultsPatient(c)
	if user == nil {
		return err
	}

	format := c.QueryParam("format")
	if format != "pdf" {
		format = "xlsx"
	}

	var rows []models.LabResult
	// Chronological (oldest first): unlike the daily/per-cycle log books,
	// this export has no days= window (lab panels run every 3-12 months,
	// so "everything" is never large) — reading top to bottom like a
	// report is more natural than the newest-first order the on-screen
	// list and dashboard summary use for "what changed most recently".
	h.DB.Where("patient_profile_id = ?", profile.ID).Order("log_date ASC, id ASC").Find(&rows)

	if format == "pdf" {
		buf, err := buildLabResultsPdf(rows)
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Disposition", `attachment; filename="lab-results.pdf"`)
		return c.Blob(http.StatusOK, "application/pdf", buf)
	}

	cols := labResultsExportColumns()
	xlsxRows := make([][]string, len(rows))
	for i, r := range rows {
		xlsxRows[i] = labResultsExportRowValues(r)
	}
	buf, err := buildTableXlsx("Lab Results", cols, xlsxRows)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Disposition", `attachment; filename="lab-results.xlsx"`)
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf)
}

// labExportItem is one field's value, rendered specially when out of
// range — see the red text below.
type labExportItem struct {
	label    string
	value    string
	abnormal bool
}

func labExportItemsForRow(r models.LabResult) []labExportItem {
	var items []labExportItem
	for _, f := range labExportFields {
		v := f.Get(r)
		if v == nil {
			continue
		}
		abnormal, _ := f.Range.Classify(*v)
		items = append(items, labExportItem{
			label:    fmt.Sprintf("%s (%s)", f.Label, f.Unit),
			value:    formatLabValue(*v),
			abnormal: abnormal,
		})
	}
	if r.KtVValue != nil {
		// Never auto-classified — no target range applies universally, see
		// labrange.KtVReferenceText.
		items = append(items, labExportItem{label: "Kt/V", value: formatLabValue(*r.KtVValue)})
	}
	for _, f := range labEnumFields {
		v := f.Get(r)
		if v == nil {
			continue
		}
		items = append(items, labExportItem{
			label:    f.Label,
			value:    labFlagPDFSafeLabel(v),
			abnormal: *v != f.NormalValue,
		})
	}
	return items
}

// labFlagPDFSafeLabel is deliberately English ("Negative"/"Positive"),
// not LabFlagShortLabel's Thai "ลบ"/"บวก" used on-screen. go-pdf/fpdf's
// font subsetting has a known bug (see docs/known_issues.md) that was
// believed to corrupt only the invisible copy/search text layer — but the
// Thai string "ลบ" was found to visibly render as the Latin "au" in this
// PDF's card layout, reproducibly, depending on what other characters
// share the page. For a serology result (HBsAg/HIV/HCV), a doctor
// misreading a silently-swapped negative/positive is a real safety risk,
// so this sidesteps the bug for this one field rather than trusting the
// library. The on-screen /lab-results page is unaffected — it renders
// through the browser's own font engine, not fpdf.
func labFlagPDFSafeLabel(p *models.LabResultFlag) string {
	if p == nil {
		return "-"
	}
	switch *p {
	case models.LabResultNegative:
		return "Negative"
	case models.LabResultPositive:
		return "Positive"
	default:
		return "-"
	}
}

// buildLabResultsPdf renders one card per visit date rather than a table:
// ~30 possible columns don't fit a table even on landscape A4 in readable
// print, and most cells would be blank anyway since a lab panel doesn't
// test everything on the same visit (see models.LabResult's doc comment).
// A card of only the fields actually recorded, laid out two per line, is
// both narrower and closer to what a printed lab report looks like —
// which is the point, since the purpose of this export is handing it to a
// doctor.
func buildLabResultsPdf(rows []models.LabResult) ([]byte, error) {
	fontBytes, err := loadThaiFont()
	if err != nil {
		return nil, err
	}

	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "P",
		UnitStr:        "mm",
		SizeStr:        "A4",
	})
	pdf.AddUTF8FontFromBytes("thai", "", fontBytes)
	pdf.SetMargins(12, 12, 12)
	pdf.AddPage()

	pdf.SetFont("thai", "", 16)
	pdf.CellFormat(0, 9, "ผลตรวจเลือด", "", 1, "C", false, 0, "")
	pdf.Ln(2)

	if len(rows) == 0 {
		pdf.SetFont("thai", "", 11)
		pdf.CellFormat(0, 8, "ยังไม่มีข้อมูลผลตรวจ", "", 1, "L", false, 0, "")
	}

	_, pageHeight := pdf.GetPageSize()
	_, _, _, bottomMargin := pdf.GetMargins()
	const (
		leftMargin  = 12.0
		usableWidth = 186.0 // A4 portrait (210mm) minus 2x12mm margin
		lineHeight  = 6.0
	)
	colWidth := usableWidth / 2

	renderItem := func(it labExportItem, w float64, newLine bool) {
		ln := 0
		if newLine {
			ln = 1
		}
		if it.abnormal {
			pdf.SetTextColor(190, 30, 30)
		}
		pdf.CellFormat(w, lineHeight, it.label+": "+it.value, "", ln, "L", false, 0, "")
		if it.abnormal {
			pdf.SetTextColor(0, 0, 0)
		}
	}

	for vi, r := range rows {
		items := labExportItemsForRow(r)

		extraLines := 0
		if r.CXRFinding != nil {
			extraLines++
		}
		if r.EKGFinding != nil {
			extraLines++
		}
		if r.Notes != nil {
			extraLines++
		}
		blockHeight := 11.0 + float64((len(items)+1)/2)*lineHeight + float64(extraLines)*lineHeight

		if pdf.GetY()+blockHeight > pageHeight-bottomMargin {
			pdf.AddPage()
		}

		pdf.SetFont("thai", "", 12)
		pdf.SetFillColor(240, 240, 245)
		pdf.CellFormat(0, 8, "ผลตรวจวันที่ "+formatEntryDate(r.LogDate), "", 1, "L", true, 0, "")
		pdf.Ln(1)

		pdf.SetFont("thai", "", 9.5)
		if len(items) == 0 {
			pdf.CellFormat(0, lineHeight, "ไม่มีค่าที่บันทึกในการตรวจครั้งนี้", "", 1, "L", false, 0, "")
		}
		for i := 0; i < len(items); i += 2 {
			if i+1 < len(items) {
				renderItem(items[i], colWidth, false)
				renderItem(items[i+1], colWidth, true)
			} else {
				renderItem(items[i], colWidth, true)
			}
		}

		if r.CXRFinding != nil {
			pdf.MultiCell(0, lineHeight, "CXR: "+*r.CXRFinding, "", "L", false)
		}
		if r.EKGFinding != nil {
			pdf.MultiCell(0, lineHeight, "EKG: "+*r.EKGFinding, "", "L", false)
		}
		if r.Notes != nil {
			pdf.MultiCell(0, lineHeight, "หมายเหตุ: "+*r.Notes, "", "L", false)
		}

		pdf.Ln(3)
		if vi < len(rows)-1 {
			y := pdf.GetY()
			pdf.Line(leftMargin, y, leftMargin+usableWidth, y)
			pdf.Ln(3)
		}
	}

	var buf pdfBuffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
