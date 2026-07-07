package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/kpi"
)

//go:embed web/templates/*.html
var templateFS embed.FS

type templateRenderer struct {
	templates *template.Template
}

func (r *templateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

func main() {
	cfg := config.Load()

	db, err := config.NewDB(cfg)
	if err != nil {
		log.Fatalf("startup aborted: %v", err)
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(echomw.Logger())
	e.Use(echomw.Recover())
	e.Renderer = &templateRenderer{
		templates: template.Must(template.ParseFS(templateFS, "web/templates/*.html")),
	}

	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "index.html", nil)
	})
	e.GET("/register", func(c echo.Context) error {
		return c.Render(http.StatusOK, "placeholder.html", map[string]string{
			"Title":   "สมัครใช้งาน",
			"Message": "หน้าสมัครใช้งานกำลังอยู่ระหว่างพัฒนา เปิดให้ใช้เร็วๆ นี้",
		})
	})
	e.GET("/login", func(c echo.Context) error {
		return c.Render(http.StatusOK, "placeholder.html", map[string]string{
			"Title":   "เข้าสู่ระบบ",
			"Message": "หน้าเข้าสู่ระบบกำลังอยู่ระหว่างพัฒนา เปิดให้ใช้เร็วๆ นี้",
		})
	})
	e.GET("/dashboard-preview", func(c echo.Context) error {
		return c.Render(http.StatusOK, "dashboard_preview.html", mockDashboardData())
	})

	e.GET("/healthz", func(c echo.Context) error {
		sqlDB, err := db.DB()
		if err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status": "error",
				"error":  "database unreachable",
			})
		}
		if err := sqlDB.Ping(); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status": "error",
				"error":  "database unreachable",
			})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	addr := os.Getenv("APP_ADDR")
	if addr == "" {
		// nginx on the production server proxies pdlife.app to this port
		addr = "127.0.0.1:8085"
	}
	e.Logger.Fatal(e.Start(addr))
}

type dashboardCard struct {
	Title       string
	Value       string
	Unit        string
	Meta        string
	Status      kpi.Status
	StatusLabel string
}

// mockDashboardData produces example KPI cards to demo the neo-brutalist
// status-driven dashboard layout ahead of real Log Book data. Values are
// fixed examples chosen to show all three status colors.
func mockDashboardData() map[string]interface{} {
	ufML := 1450.0
	weightDeltaKg := 1.4
	systolic, diastolic := 142, 88

	cards := []dashboardCard{
		{
			Title:       "Total UF วันนี้",
			Value:       fmt.Sprintf("%.0f", ufML),
			Unit:        "ml",
			Meta:        "ค่าปกติ 800–2000 ml/วัน",
			Status:      kpi.TotalUF(ufML),
			StatusLabel: kpi.TotalUF(ufML).Label(),
		},
		{
			Title:       "น้ำหนักตัว",
			Value:       fmt.Sprintf("+%.1f", weightDeltaKg),
			Unit:        "กก. จากค่าเฉลี่ย 7 วัน",
			Meta:        "เปลี่ยน >1 กก./วัน = เฝ้าระวัง, >2 กก. = ผิดปกติ",
			Status:      kpi.WeightChange(weightDeltaKg),
			StatusLabel: kpi.WeightChange(weightDeltaKg).Label(),
		},
		{
			Title:       "ความดันโลหิต",
			Value:       fmt.Sprintf("%d/%d", systolic, diastolic),
			Unit:        "mmHg",
			Meta:        "ค่าปกติ <130/80 mmHg",
			Status:      kpi.BloodPressure(systolic, diastolic),
			StatusLabel: kpi.BloodPressure(systolic, diastolic).Label(),
		},
	}

	return map[string]interface{}{"Cards": cards}
}
