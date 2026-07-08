package handler

import (
	"fmt"
	"html"
	"html/template"
	"math"
	"strings"

	"github.com/atiroop/pdlife/internal/models"
)

// buildTrendSVG renders a small inline SVG line chart (no JS, no external
// library — mirrors the legacy TrendChart.tsx approach of hand-computed
// polylines). secondary may be nil for a single-line chart.
func buildTrendSVG(
	logs []models.ApdLogEntry,
	title, unit string,
	primary func(models.ApdLogEntry) (float64, bool),
	secondary func(models.ApdLogEntry) (float64, bool),
) template.HTML {
	if len(logs) == 0 {
		return template.HTML(fmt.Sprintf(`
<div class="card trend-card">
  <div class="trend-head"><h3>%s</h3><span class="trend-unit">%s</span></div>
  <div class="trend-empty">ยังไม่มีข้อมูลกราฟ</div>
</div>`, html.EscapeString(title), html.EscapeString(unit)))
	}

	primaryValues := make([]*float64, len(logs))
	var secondaryValues []*float64
	if secondary != nil {
		secondaryValues = make([]*float64, len(logs))
	}
	var allValues []float64
	for i, l := range logs {
		if v, ok := primary(l); ok {
			primaryValues[i] = floatPtr(v)
			allValues = append(allValues, v)
		}
		if secondary != nil {
			if v, ok := secondary(l); ok {
				secondaryValues[i] = floatPtr(v)
				allValues = append(allValues, v)
			}
		}
	}
	if len(allValues) == 0 {
		return template.HTML(fmt.Sprintf(`
<div class="card trend-card">
  <div class="trend-head"><h3>%s</h3><span class="trend-unit">%s</span></div>
  <div class="trend-empty">ยังไม่มีข้อมูลกราฟ</div>
</div>`, html.EscapeString(title), html.EscapeString(unit)))
	}

	minV, maxV := allValues[0], allValues[0]
	for _, v := range allValues {
		minV = math.Min(minV, v)
		maxV = math.Max(maxV, v)
	}
	pad := math.Max((maxV-minV)*0.12, 1)
	minV -= pad
	maxV += pad

	primaryPath := buildPolyline(primaryValues, minV, maxV)
	var secondaryPath string
	if secondary != nil {
		secondaryPath = buildPolyline(secondaryValues, minV, maxV)
	}

	firstLabel := formatEntryDate(logs[0].EntryDate)
	lastLabel := formatEntryDate(logs[len(logs)-1].EntryDate)

	var secondaryPolyline string
	if secondaryPath != "" {
		secondaryPolyline = fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#06B6D4" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" />`, secondaryPath)
	}

	svg := fmt.Sprintf(`
<div class="card trend-card">
  <div class="trend-head"><h3>%s</h3><span class="trend-unit">%s</span></div>
  <div class="trend-svg-wrap">
    <svg viewBox="0 0 640 210" role="img" aria-label="%s" class="trend-svg">
      <line x1="18" y1="18" x2="18" y2="180" stroke="#151516" stroke-opacity="0.2" />
      <line x1="18" y1="180" x2="622" y2="180" stroke="#151516" stroke-opacity="0.2" />
      <line x1="18" y1="72" x2="622" y2="72" stroke="#151516" stroke-opacity="0.1" stroke-dasharray="4 6" />
      <line x1="18" y1="126" x2="622" y2="126" stroke="#151516" stroke-opacity="0.1" stroke-dasharray="4 6" />
      %s
      <polyline points="%s" fill="none" stroke="#FF3D6E" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" />
      <text x="24" y="30" fill="#151516" fill-opacity="0.6" font-size="11" font-weight="700">%.0f</text>
      <text x="24" y="174" fill="#151516" fill-opacity="0.6" font-size="11" font-weight="700">%.0f</text>
      <text x="18" y="202" fill="#151516" fill-opacity="0.6" font-size="11" font-weight="700">%s</text>
      <text x="622" y="202" fill="#151516" fill-opacity="0.6" font-size="11" font-weight="700" text-anchor="end">%s</text>
    </svg>
  </div>
</div>`,
		html.EscapeString(title), html.EscapeString(unit), html.EscapeString(title),
		secondaryPolyline, primaryPath,
		maxV, minV, html.EscapeString(firstLabel), html.EscapeString(lastLabel))

	return template.HTML(svg)
}

func buildPolyline(values []*float64, minV, maxV float64) string {
	const width, height, padX, padY = 640.0, 180.0, 18.0, 18.0
	usableWidth := width - padX*2
	usableHeight := height - padY*2
	rangeV := maxV - minV
	if rangeV == 0 {
		rangeV = 1
	}
	steps := len(values) - 1
	if steps < 1 {
		steps = 1
	}

	var points []string
	for i, v := range values {
		if v == nil {
			continue
		}
		x := padX + (float64(i)/float64(steps))*usableWidth
		y := padY + (1-(*v-minV)/rangeV)*usableHeight
		points = append(points, fmt.Sprintf("%.1f,%.1f", x, y))
	}
	return strings.Join(points, " ")
}
