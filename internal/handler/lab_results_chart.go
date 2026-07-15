package handler

import (
	"fmt"
	"html"
	"html/template"
	"math"

	"github.com/atiroop/pdlife/internal/models"
)

// buildLabTrendSVG renders a small inline SVG line chart for one lab
// value across every LabResult row that has it filled in — same
// hand-computed-polyline approach as capd_chart.go's buildDailyTrendSVG
// (reused via buildPolyline), but skipping rows where this particular value is
// nil rather than assuming every row has every field (lab results are
// filled in sparsely — see models.LabResult's doc comment).
func buildLabTrendSVG(
	rows []models.LabResult,
	title, unit string,
	value func(models.LabResult) (float64, bool),
) template.HTML {
	var points []*float64
	var labels []string
	var allValues []float64
	for _, r := range rows {
		if v, ok := value(r); ok {
			vv := v
			points = append(points, &vv)
			labels = append(labels, formatEntryDate(r.LogDate))
			allValues = append(allValues, v)
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

	path := buildPolyline(points, minV, maxV)
	firstLabel := labels[0]
	lastLabel := labels[len(labels)-1]

	svg := fmt.Sprintf(`
<div class="card trend-card">
  <div class="trend-head"><h3>%s</h3><span class="trend-unit">%s</span></div>
  <div class="trend-svg-wrap">
    <svg viewBox="0 0 640 210" role="img" aria-label="%s" class="trend-svg">
      <line x1="18" y1="18" x2="18" y2="180" stroke="#151516" stroke-opacity="0.2" />
      <line x1="18" y1="180" x2="622" y2="180" stroke="#151516" stroke-opacity="0.2" />
      <line x1="18" y1="72" x2="622" y2="72" stroke="#151516" stroke-opacity="0.1" stroke-dasharray="4 6" />
      <line x1="18" y1="126" x2="622" y2="126" stroke="#151516" stroke-opacity="0.1" stroke-dasharray="4 6" />
      <polyline points="%s" fill="none" stroke="#FF3D6E" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" />
      <text x="24" y="30" fill="#151516" fill-opacity="0.6" font-size="11" font-weight="700">%.1f</text>
      <text x="24" y="174" fill="#151516" fill-opacity="0.6" font-size="11" font-weight="700">%.1f</text>
      <text x="18" y="202" fill="#151516" fill-opacity="0.6" font-size="11" font-weight="700">%s</text>
      <text x="622" y="202" fill="#151516" fill-opacity="0.6" font-size="11" font-weight="700" text-anchor="end">%s</text>
    </svg>
  </div>
</div>`,
		html.EscapeString(title), html.EscapeString(unit), html.EscapeString(title),
		path,
		maxV, minV, html.EscapeString(firstLabel), html.EscapeString(lastLabel))

	return template.HTML(svg)
}
