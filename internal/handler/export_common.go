package handler

import (
	"embed"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

// Shared table-shaped Excel/PDF export builders for APD/CAPD/HD (Lab
// Results uses its own card-layout PDF — see lab_results_export.go — since
// its ~30 nullable columns don't fit a table even on landscape A4, but
// still shares buildTableXlsx for its own wide-table Excel export).

//go:embed assets/fonts/NotoSansThai-Regular.ttf
var thaiFontFS embed.FS

// exportColumn is one column's header and its relative width (mirrors the
// legacy export/route.ts "width" units this was ported from).
type exportColumn struct {
	header string
	width  float64
}

type pdfBuffer struct{ data []byte }

func (b *pdfBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *pdfBuffer) Bytes() []byte { return b.data }

func loadThaiFont() ([]byte, error) {
	return thaiFontFS.ReadFile("assets/fonts/NotoSansThai-Regular.ttf")
}

// buildTableXlsx writes cols as the header row and rows below it, one
// sheet, column widths scaled from exportColumn.width.
func buildTableXlsx(sheetName string, cols []exportColumn, rows [][]string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", sheetName)

	for i, col := range cols {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, col.header)
		colName, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheetName, colName, colName, col.width/4)
	}

	for r, values := range rows {
		row := r + 2
		for i, v := range values {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			f.SetCellValue(sheetName, cell, v)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildTablePdf renders cols/rows as a bordered table on landscape A4,
// paginating automatically when a row would cross the bottom margin.
// emptyMessage is shown in place of the table body when rows is empty.
func buildTablePdf(title string, cols []exportColumn, rows [][]string, emptyMessage string) ([]byte, error) {
	fontBytes, err := loadThaiFont()
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
	pdf.CellFormat(0, 8, title, "", 1, "C", false, 0, "")
	pdf.Ln(2)

	totalWidth := 0.0
	for _, col := range cols {
		totalWidth += col.width
	}
	pageWidth := 281.0 // A4 landscape usable width in mm at these margins
	colWidths := make([]float64, len(cols))
	for i, col := range cols {
		colWidths[i] = col.width / totalWidth * pageWidth
	}

	drawRow := func(values []string, isHeader bool) {
		size := 8.5
		if isHeader {
			size = 9
		}
		pdf.SetFont("thai", "", size)
		_, pageHeight := pdf.GetPageSize()
		_, _, _, bottomMargin := pdf.GetMargins()
		if pdf.GetY()+6 > pageHeight-bottomMargin {
			pdf.AddPage()
			pdf.SetFont("thai", "", size)
		}
		for i, v := range values {
			pdf.CellFormat(colWidths[i], 6, v, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}

	header := make([]string, len(cols))
	for i, col := range cols {
		header[i] = col.header
	}
	drawRow(header, true)

	if len(rows) == 0 {
		pdf.SetFont("thai", "", 10)
		pdf.CellFormat(0, 8, emptyMessage, "", 1, "L", false, 0, "")
	}
	for _, r := range rows {
		drawRow(r, false)
	}

	var buf pdfBuffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
