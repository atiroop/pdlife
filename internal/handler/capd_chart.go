package handler

import (
	"fmt"
	"html"
	"html/template"
	"math"
	"time"
)

// buildDailyTrendSVG renders a small inline SVG line chart from day-level
// log book aggregates (one point per calendar day, not per cycle) — same
// hand-computed-polyline approach as buildHdTrendSVG, reused via
// buildPolyline. Generic over the day-aggregate type so CAPD
// (capdDailyAgg) and APD (apdDailyAgg) share one implementation.
// secondary may be nil for a single-line chart; when set it draws a second
// cyan line (used for BP diastolic alongside systolic).
func buildDailyTrendSVG[T any](
	days []T,
	title, unit string,
	date func(T) time.Time,
	primary func(T) (float64, bool),
	secondary func(T) (float64, bool),
) template.HTML {
	emptyCard := template.HTML(fmt.Sprintf(`
<div class="card trend-card">
  <div class="trend-head"><h3>%s</h3><span class="trend-unit">%s</span></div>
  <div class="trend-empty">ยังไม่มีข้อมูลกราฟ</div>
</div>`, html.EscapeString(title), html.EscapeString(unit)))

	if len(days) == 0 {
		return emptyCard
	}

	primaryValues := make([]*float64, len(days))
	var secondaryValues []*float64
	if secondary != nil {
		secondaryValues = make([]*float64, len(days))
	}
	var allValues []float64
	for i, d := range days {
		if v, ok := primary(d); ok {
			primaryValues[i] = floatPtr(v)
			allValues = append(allValues, v)
		}
		if secondary != nil {
			if v, ok := secondary(d); ok {
				secondaryValues[i] = floatPtr(v)
				allValues = append(allValues, v)
			}
		}
	}
	if len(allValues) == 0 {
		return emptyCard
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
	var secondaryPolyline string
	if secondary != nil {
		if secondaryPath := buildPolyline(secondaryValues, minV, maxV); secondaryPath != "" {
			secondaryPolyline = fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#06B6D4" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" />`, secondaryPath)
		}
	}

	firstLabel := formatEntryDate(date(days[0]))
	lastLabel := formatEntryDate(date(days[len(days)-1]))

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
