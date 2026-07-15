package handler

import (
	"fmt"
	"strings"
)

// buildPolyline converts a series of (possibly nil) values into the SVG
// polyline points string every trend chart in this package shares —
// buildDailyTrendSVG (APD/CAPD daily aggregates), buildHdTrendSVG, and
// lab_results_chart.go all render through this one coordinate mapping.
// Nil values are skipped (the line simply connects across the gap).
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
