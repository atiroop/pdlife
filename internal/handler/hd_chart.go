package handler

import (
	"fmt"
	"html"
	"html/template"
	"math"

	"github.com/atiroop/pdlife/internal/models"
)

// buildHdTrendSVG renders a small inline SVG line chart from HD log
// entries (one point per session) — same hand-computed-polyline approach
// as buildDailyTrendSVG in capd_chart.go, reused via buildPolyline. Unlike
// buildDailyTrendSVG, this also accepts an optional tertiary line: HD's
// weight trend needs three series (pre/post/dry), one more than APD's
// two-line BP chart.
func buildHdTrendSVG(
	logs []models.HdLogEntry,
	title, unit string,
	primary func(models.HdLogEntry) (float64, bool),
	secondary func(models.HdLogEntry) (float64, bool),
	tertiary func(models.HdLogEntry) (float64, bool),
) template.HTML {
	if len(logs) == 0 {
		return template.HTML(fmt.Sprintf(`
<div class="card trend-card">
  <div class="trend-head"><h3>%s</h3><span class="trend-unit">%s</span></div>
  <div class="trend-empty">ยังไม่มีข้อมูลกราฟ</div>
</div>`, html.EscapeString(title), html.EscapeString(unit)))
	}

	primaryValues := make([]*float64, len(logs))
	var secondaryValues, tertiaryValues []*float64
	if secondary != nil {
		secondaryValues = make([]*float64, len(logs))
	}
	if tertiary != nil {
		tertiaryValues = make([]*float64, len(logs))
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
		if tertiary != nil {
			if v, ok := tertiary(l); ok {
				tertiaryValues[i] = floatPtr(v)
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
	var secondaryPath, tertiaryPath string
	if secondary != nil {
		secondaryPath = buildPolyline(secondaryValues, minV, maxV)
	}
	if tertiary != nil {
		tertiaryPath = buildPolyline(tertiaryValues, minV, maxV)
	}

	firstLabel := formatEntryDate(logs[0].LogDate)
	lastLabel := formatEntryDate(logs[len(logs)-1].LogDate)

	var secondaryPolyline, tertiaryPolyline string
	if secondaryPath != "" {
		secondaryPolyline = fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#06B6D4" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" />`, secondaryPath)
	}
	if tertiaryPath != "" {
		tertiaryPolyline = fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#22C55E" stroke-width="3" stroke-dasharray="6 5" stroke-linecap="round" stroke-linejoin="round" />`, tertiaryPath)
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
		tertiaryPolyline, secondaryPolyline, primaryPath,
		maxV, minV, html.EscapeString(firstLabel), html.EscapeString(lastLabel))

	return template.HTML(svg)
}
