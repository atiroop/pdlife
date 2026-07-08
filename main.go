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
	funcs := template.FuncMap{
		"logoURL":     func() string { return cfg.LogoURL },
		"logoURLDark": func() string { return cfg.LogoURLDark },
		"dict": func(pairs ...interface{}) (map[string]interface{}, error) {
			if len(pairs)%2 != 0 {
				return nil, fmt.Errorf("dict: odd number of arguments")
			}
			m := make(map[string]interface{}, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key at index %d is not a string", i)
				}
				m[key] = pairs[i+1]
			}
			return m, nil
		},
		"formatDate": func(t time.Time) string {
			thaiMonths := []string{"", "ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}
			return fmt.Sprintf("%d %s %d", t.Day(), thaiMonths[int(t.Month())], t.Year()+543)
		},
		"formatDateInput": func(t time.Time) string { return t.Format("2006-01-02") },
		"deref": func(p interface{}) interface{} {
			switch v := p.(type) {
			case *int:
				if v == nil {
					return nil
				}
				return *v
			case *string:
				if v == nil {
					return nil
				}
				return *v
			default:
				return p
			}
		},
	}
	e.Renderer = &templateRenderer{
		templates: template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "web/templates/*.html")),
	}

	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "index.html", nil)
	})
	e.GET("/dashboard-preview", func(c echo.Context) error {
		return c.Render(http.StatusOK, "dashboard_preview.html", mockDashboardData())
	})
	e.GET("/terms", func(c echo.Context) error {
		return c.Render(http.StatusOK, "legal_page.html", map[string]string{
			"Title":       "เงื่อนไขการใช้งาน",
			"Placeholder": "เนื้อหาเงื่อนไขการใช้งานฉบับเต็มอยู่ระหว่างจัดทำ จะเผยแพร่ที่นี่เมื่อพร้อม",
		})
	})
	e.GET("/privacy", func(c echo.Context) error {
		return c.Render(http.StatusOK, "legal_page.html", map[string]string{
			"Title":       "นโยบายความเป็นส่วนตัว",
			"Placeholder": "เนื้อหานโยบายความเป็นส่วนตัวฉบับเต็มอยู่ระหว่างจัดทำ จะเผยแพร่ที่นี่เมื่อพร้อม",
		})
	})
	e.GET("/cookie-policy", func(c echo.Context) error {
		return c.Render(http.StatusOK, "legal_page.html", map[string]string{
			"Title":       "นโยบายคุกกี้",
			"Placeholder": "เนื้อหานโยบายคุกกี้ฉบับเต็มอยู่ระหว่างจัดทำ จะเผยแพร่ที่นี่เมื่อพร้อม",
		})
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

	loginLimiter := echomw.RateLimiterWithConfig(echomw.RateLimiterConfig{
		Store: echomw.NewRateLimiterMemoryStoreWithConfig(echomw.RateLimiterMemoryStoreConfig{
			Rate: rate.Limit(10.0 / 3600.0), Burst: 10, ExpiresIn: time.Hour,
		}),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return c.String(http.StatusForbidden, "forbidden")
		},
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			return c.String(http.StatusTooManyRequests, "เข้าสู่ระบบบ่อยเกินไป กรุณาลองใหม่ภายหลัง")
		},
	})
	forgotPasswordLimiter := echomw.RateLimiterWithConfig(echomw.RateLimiterConfig{
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

	e.GET("/login", authHandler.LoginForm)
	e.POST("/login", authHandler.Login, loginLimiter)
	e.POST("/logout", authHandler.Logout)
	e.GET("/forgot-password", authHandler.ForgotPasswordForm)
	e.POST("/forgot-password", authHandler.ForgotPassword, forgotPasswordLimiter)
	e.GET("/reset-password", authHandler.ResetPasswordForm)
	e.POST("/reset-password", authHandler.ResetPassword)

	e.GET("/apd", authHandler.ApdDashboard)
	e.GET("/apd/logs", authHandler.ApdLogsList)
	e.GET("/apd/new", authHandler.ApdNewForm)
	e.POST("/apd/new", authHandler.ApdCreate)
	e.GET("/apd/:id/edit", authHandler.ApdEditForm)
	e.POST("/apd/:id/edit", authHandler.ApdUpdate)
	e.POST("/apd/:id/delete", authHandler.ApdDelete)
	e.GET("/apd/export", authHandler.ApdExport)

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
