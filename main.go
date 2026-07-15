package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("startup aborted: %v", err)
	}

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

	// Security headers have to be set here rather than in nginx: the nginx
	// config on the server is managed separately and is off-limits to this
	// repo.
	//
	// No ContentSecurityPolicy on purpose. The templates rely on inline
	// <script>/<style>, so a policy strict enough to be worth having would
	// break the dashboards, and the deploy smoke test only fetches pages —
	// it would not notice. Adding CSP needs the inline blocks hashed or
	// nonced first, and a real browser pass over each module.
	e.Use(echomw.SecureWithConfig(echomw.SecureConfig{
		ContentTypeNosniff: "nosniff",
		XFrameOptions:      "SAMEORIGIN",
		// X-XSS-Protection is a legacy filter that modern browsers ignore
		// and older ones can be made to misbehave with; "0" disables it,
		// which is the current recommendation over Echo's "1; mode=block".
		XSSProtection: "0",
		// Patient URLs like /apd/123/edit are health data. Send the origin
		// only when leaving pdlife.app, never the path.
		ReferrerPolicy: "strict-origin-when-cross-origin",
		// One year. Not preloaded, and subdomains excluded: cdn.pdlife.app
		// is the only other host and it is not ours to commit to HTTPS-only
		// on the browser's behalf.
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: true,
	}))
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
		"deref": func(p interface{}) interface{} {
			switch v := p.(type) {
			case *int:
				if v == nil {
					return nil
				}
				return *v
			case *float64:
				if v == nil {
					return nil
				}
				return *v
			case *string:
				if v == nil {
					return nil
				}
				return *v
			case *time.Time:
				if v == nil {
					return nil
				}
				return *v
			default:
				return p
			}
		},
		"labFlagLabel":      handler.LabFlagLabel,
		"labFlagShortLabel": handler.LabFlagShortLabel,
		"labFlagSelected":   handler.LabFlagSelected,
	}
	e.Renderer = &templateRenderer{
		templates: template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "web/templates/*.html")),
	}

	e.GET("/", authHandler.LandingPage)
	e.GET("/dashboard-preview", func(c echo.Context) error {
		return c.Render(http.StatusOK, "dashboard_preview.html", mockDashboardData())
	})
	e.GET("/terms", func(c echo.Context) error {
		return c.Render(http.StatusOK, "legal_page.html", map[string]string{
			"Title":        "เงื่อนไขการใช้งาน",
			"ContentBlock": "legal-content-terms",
			"UpdatedDate":  handler.LegalContentUpdatedDate,
		})
	})
	e.GET("/privacy", func(c echo.Context) error {
		return c.Render(http.StatusOK, "legal_page.html", map[string]string{
			"Title":        "นโยบายความเป็นส่วนตัว",
			"ContentBlock": "legal-content-privacy",
			"UpdatedDate":  handler.LegalContentUpdatedDate,
		})
	})
	e.GET("/cookie-policy", func(c echo.Context) error {
		return c.Render(http.StatusOK, "legal_page.html", map[string]string{
			"Title":        "นโยบายคุกกี้",
			"ContentBlock": "legal-content-cookie",
			"UpdatedDate":  handler.LegalContentUpdatedDate,
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
	e.GET("/consent", authHandler.ConsentForm)
	e.POST("/consent", authHandler.ConsentSubmit)
	e.POST("/consent/withdraw", authHandler.WithdrawConsent)

	e.GET("/login", authHandler.LoginForm)
	e.POST("/login", authHandler.Login, loginLimiter)
	e.POST("/logout", authHandler.Logout)
	e.GET("/forgot-password", authHandler.ForgotPasswordForm)
	e.POST("/forgot-password", authHandler.ForgotPassword, forgotPasswordLimiter)
	e.GET("/reset-password", authHandler.ResetPasswordForm)
	e.POST("/reset-password", authHandler.ResetPassword)

	e.GET("/dashboard", authHandler.Dashboard)
	e.GET("/news", authHandler.NewsList)
	e.GET("/profile", authHandler.ProfileForm)
	e.POST("/profile/name", authHandler.ProfileUpdateName)
	e.POST("/profile/password", authHandler.ProfileChangePassword)
	e.POST("/profile/treatment", authHandler.ProfileUpdateTreatment)
	e.GET("/profile/export-data", authHandler.ProfileExportData)
	e.POST("/profile/delete-account", authHandler.ProfileDeleteAccount)

	e.GET("/apd", authHandler.ApdDashboard)
	e.GET("/apd/logs", authHandler.ApdLogsList)
	e.GET("/apd/new", authHandler.ApdNewForm)
	e.POST("/apd/new", authHandler.ApdCreate)
	e.GET("/apd/:id/edit", authHandler.ApdEditForm)
	e.POST("/apd/:id/edit", authHandler.ApdUpdate)
	e.POST("/apd/:id/delete", authHandler.ApdDelete)
	e.GET("/apd/export", authHandler.ApdExport)

	e.GET("/capd", authHandler.CapdDashboard)
	e.GET("/capd/logs", authHandler.CapdLogsList)
	e.GET("/capd/new", authHandler.CapdNewForm)
	e.POST("/capd/new", authHandler.CapdCreate)
	e.GET("/capd/:id/edit", authHandler.CapdEditForm)
	e.POST("/capd/:id/edit", authHandler.CapdUpdate)
	e.POST("/capd/:id/delete", authHandler.CapdDelete)

	e.GET("/hd", authHandler.HdDashboard)
	e.GET("/hd/logs", authHandler.HdLogsList)
	e.GET("/hd/new", authHandler.HdNewForm)
	e.POST("/hd/new", authHandler.HdCreate)
	e.GET("/hd/:id/edit", authHandler.HdEditForm)
	e.POST("/hd/:id/edit", authHandler.HdUpdate)
	e.POST("/hd/:id/delete", authHandler.HdDelete)

	e.GET("/lab-results", authHandler.LabResultsList)
	e.GET("/lab-results/new", authHandler.LabResultsNewForm)
	e.POST("/lab-results/new", authHandler.LabResultsCreate)
	e.GET("/lab-results/:id/edit", authHandler.LabResultsEditForm)
	e.POST("/lab-results/:id/edit", authHandler.LabResultsUpdate)
	e.POST("/lab-results/:id/delete", authHandler.LabResultsDelete)

	e.GET("/food-check", authHandler.FoodCheckSearch)
	e.GET("/food-check/results", authHandler.FoodCheckSearchResults)
	e.GET("/food-check/food/:source/:ref", authHandler.FoodCheckDetail)
	e.GET("/food-check/food/:source/:ref/nutrition", authHandler.FoodCheckNutrition)

	e.GET("/admin/content-queue", authHandler.AdminContentQueue)
	e.POST("/admin/content-queue/:id/approve", authHandler.AdminApproveContent)
	e.POST("/admin/content-queue/:id/reject", authHandler.AdminRejectContent)
	e.POST("/admin/content-queue/:id/regenerate-image", authHandler.AdminRegenerateImage)

	e.GET("/admin/editorial", authHandler.EditorialList)
	e.GET("/admin/editorial/new", authHandler.EditorialNewForm)
	e.POST("/admin/editorial/new", authHandler.EditorialCreate)
	e.GET("/admin/editorial/:id/edit", authHandler.EditorialEditForm)
	e.POST("/admin/editorial/:id/edit", authHandler.EditorialUpdate)
	e.POST("/admin/editorial/:id/delete", authHandler.EditorialDelete)
	e.POST("/admin/editorial/upload-media", authHandler.EditorialUploadMedia)

	e.GET("/admin/users", authHandler.AdminUsersList)
	e.GET("/admin/users/:id", authHandler.AdminUserDetail)
	e.POST("/admin/users/:id/verify-email", authHandler.AdminVerifyEmail)
	e.POST("/admin/users/:id/unlock", authHandler.AdminUnlockAccount)
	e.POST("/admin/users/:id/suspend", authHandler.AdminSuspendAccount)
	e.POST("/admin/users/:id/unsuspend", authHandler.AdminUnsuspendAccount)

	e.GET("/articles", authHandler.ArticlesList)
	e.GET("/articles/:slug", authHandler.ArticleDetail)

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
