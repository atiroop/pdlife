package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/handler"
	"github.com/atiroop/pdlife/internal/kpi"
	"github.com/atiroop/pdlife/internal/mailer"
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

	m, err := mailer.New(cfg)
	if err != nil {
		log.Fatalf("startup aborted: %v", err)
	}
	authHandler := handler.NewAuthHandler(db, cfg, m)

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
	e.GET("/login", func(c echo.Context) error {
		return c.Render(http.StatusOK, "placeholder.html", map[string]string{
			"Title":   "เข้าสู่ระบบ",
			"Message": "หน้าเข้าสู่ระบบกำลังอยู่ระหว่างพัฒนา เปิดให้ใช้เร็วๆ นี้",
		})
	})
	e.GET("/dashboard-preview", func(c echo.Context) error {
		return c.Render(http.StatusOK, "dashboard_preview.html", mockDashboardData())
	})

	registerLimiter := echomw.RateLimiterWithConfig(echomw.RateLimiterConfig{
		Store: echomw.NewRateLimiterMemoryStoreWithConfig(echomw.RateLimiterMemoryStoreConfig{
			Rate: rate.Limit(5.0 / 3600.0), Burst: 5, ExpiresIn: time.Hour,
		}),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return c.String(http.StatusForbidden, "forbidden")
		},
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			return c.String(http.StatusTooManyRequests, "สมัครบ่อยเกินไป กรุณาลองใหม่ภายหลัง")
		},
	})
	resendLimiter := echomw.RateLimiterWithConfig(echomw.RateLimiterConfig{
		Store: echomw.NewRateLimiterMemoryStoreWithConfig(echomw.RateLimiterMemoryStoreConfig{
			Rate: rate.Limit(3.0 / 3600.0), Burst: 3, ExpiresIn: time.Hour,
		}),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			email := c.FormValue("email")
			if email == "" {
				return c.RealIP(), nil
			}
			return email, nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return c.String(http.StatusForbidden, "forbidden")
		},
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			return c.String(http.StatusTooManyRequests, "ขอลิงก์บ่อยเกินไป กรุณาลองใหม่ภายหลัง")
		},
	})

	e.GET("/register", authHandler.RegisterForm)
	e.POST("/register", authHandler.Register, registerLimiter)
	e.GET("/verify-email", authHandler.VerifyEmail)
	e.GET("/resend-verification", authHandler.ResendVerificationForm)
	e.POST("/resend-verification", authHandler.ResendVerification, resendLimiter)
	e.GET("/onboarding", authHandler.OnboardingForm)
	e.POST("/onboarding", authHandler.OnboardingSubmit)

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
