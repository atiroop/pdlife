package main

import (
	"html/template"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/atiroop/pdlife/internal/handler"
)

// TestTemplatesParse guards against the exact class of bug that took prod
// down on 2026-07-11: a literal "{{...}}" inside an HTML comment or CSS
// comment gets parsed as real template syntax, because every file under
// web/templates/*.html shares one template.Template instance (main.go's
// ParseFS call) — a syntax error in ANY file panics template.Must() at
// process startup for the WHOLE app, not just the page you edited. `go
// build`/`go vet` do not catch this since template parsing happens at
// runtime. This test mirrors main()'s FuncMap so it fails exactly where
// main() would panic, without needing a live DB connection.
func TestTemplatesParse(t *testing.T) {
	funcs := template.FuncMap{
		"logoURL":         func() string { return "" },
		"logoURLDark":     func() string { return "" },
		"dict":            func(pairs ...interface{}) (map[string]interface{}, error) { return nil, nil },
		"formatDate":      handler.FormatDateThai,
		"formatDateInput": func(t time.Time) string { return t.Format("2006-01-02") },
		"fmtNutrient": func(v *float64) string {
			if v == nil {
				return "-"
			}
			s := strconv.FormatFloat(*v, 'f', 1, 64)
			s = strings.TrimSuffix(s, ".0")
			return s
		},
		"appearanceLabel": handler.DialysateAppearanceLabel,
		"deref":           func(p interface{}) interface{} { return p },
		"labFlagLabel":      handler.LabFlagLabel,
		"labFlagShortLabel": handler.LabFlagShortLabel,
		"labFlagSelected":   handler.LabFlagSelected,
	}
	if _, err := template.New("").Funcs(funcs).ParseFS(templateFS, "web/templates/*.html"); err != nil {
		t.Fatalf("template parse failed (this would panic the whole app at startup): %v", err)
	}
}
