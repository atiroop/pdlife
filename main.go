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
	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/handler"
	"github.com/atiroop/pdlife/internal/kpi"
	"github.com/atiroop/pdlife/internal/mailer"
)

//go:embed web/templates/*.html
var templateFS embed.FS

// csrfContextKey is where the CSRF middleware stores the token for the
// current request, and csrfTemplateKey is what templates read it as.
const (
	csrfContextKey  = "csrf"
	csrfTemplateKey = "csrf"
	csrfFormField   = "_csrf"
	csrfHeaderName  = "X-CSRF-Token"
)

type templateRenderer struct {
	templates *template.Template
}

// Render injects the request's CSRF token into the render data so that
// templates can write {{ .csrf }} without every handler having to thread
// the token through its own data map — there are ~70 Render calls, and one
// missed map would be a form that silently 403s.
func (r *templateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	token, _ := c.Get(csrfContextKey).(string)

	switch d := data.(type) {
	case map[string]interface{}:
		d[csrfTemplateKey] = token
	case map[string]string:
		d[csrfTemplateKey] = token
	case nil:
		data = map[string]interface{}{csrfTemplateKey: token}
	}

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

	e := newServer(cfg, db, m, true)

	addr := os.Getenv("APP_ADDR")
	if addr == "" {
		// nginx on the production server proxies pdlife.app to this port
		addr = "127.0.0.1:8085"
	}
	e.Logger.Fatal(e.Start(addr))
}

// newServer wires the whole application: security/CSRF middleware,
// templates, and every route. main() calls it once at startup; tests call
// it directly against a real (local, throwaway) database, so they exercise
// the exact route table and guards that run in production instead of a
// hand-picked subset that could quietly drift from it.
//
// rateLimit is false only in tests. The per-IP limiters on
// register/login/etc. are process-lifetime and keyed by client IP, and
// every request from a test binary comes from the same loopback address —
// leaving them on would fail later subtests by exhausting the bucket, for
// reasons that have nothing to do with what those subtests check. The
// per-account login lockout (auth.LoginLimiter, keyed by email) is
// unaffected and still runs in tests.
func newServer(cfg *config.Config, db *gorm.DB, m *mailer.Mailer, rateLimit bool) *echo.Echo {
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

	// Session cookies are SameSite=Lax, which already stops a cross-site
	// POST from carrying them, so this is a second layer rather than the
	// only one. It is worth having anyway: Lax is a browser-side promise,
	// and this app writes health data.
	//
	// Both lookups are needed — most submissions are plain form posts, but
	// the admin content queue and the editorial editor POST via fetch() and
	// send the token as a header instead (see the templates' csrf-token
	// meta tag).
	//
	// The cookie can be HttpOnly because nothing reads the token from
	// JavaScript: the server renders it into the page.
	e.Use(echomw.CSRFWithConfig(echomw.CSRFConfig{
		TokenLookup:    "form:" + csrfFormField + ",header:" + csrfHeaderName,
		ContextKey:     csrfContextKey,
		CookieName:     "_csrf",
		CookiePath:     "/",
		CookieHTTPOnly: true,
		CookieSecure:   strings.HasPrefix(cfg.AppBaseURL, "https://"),
		CookieSameSite: http.SameSiteLaxMode,
		CookieMaxAge:   86400,
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

	// withLimiter returns the limiter as the route's only middleware, or
	// none at all when rateLimit is off (tests) — see newServer's doc
	// comment for why.
	withLimiter := func(mw echo.MiddlewareFunc) []echo.MiddlewareFunc {
		if !rateLimit {
			return nil
		}
		return []echo.MiddlewareFunc{mw}
	}

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

	// ---- public routes ----
	//
	// Everything not registered under one of the groups below is reachable
	// without a session, so this list is the security boundary: adding a
	// route here is a decision to make it public.

	e.GET("/register", authHandler.RegisterForm)
	e.POST("/register", authHandler.Register, withLimiter(registerLimiter)...)
	e.GET("/verify-email", authHandler.VerifyEmail)
	e.GET("/resend-verification", authHandler.ResendVerificationForm)
	e.POST("/resend-verification", authHandler.ResendVerification, withLimiter(resendLimiter)...)

	e.GET("/login", authHandler.LoginForm)
	e.POST("/login", authHandler.Login, withLimiter(loginLimiter)...)
	// Logout stays public on purpose: it clears the cookies, and gating it
	// on a valid session would leave someone whose session is already
	// broken with no way to clear the bad ones.
	e.POST("/logout", authHandler.Logout)
	e.GET("/forgot-password", authHandler.ForgotPasswordForm)
	e.POST("/forgot-password", authHandler.ForgotPassword, withLimiter(forgotPasswordLimiter)...)
	e.GET("/reset-password", authHandler.ResetPasswordForm)
	e.POST("/reset-password", authHandler.ResetPassword)

	// ---- routes that require a session ----
	//
	// RequireSession only checks that a session exists. Each handler's own
	// guard still applies the rest (verified email, completed onboarding,
	// health-data consent, matching treatment type) — this group is what
	// makes forgetting to call that guard fail closed instead of silently
	// exposing the route.
	//
	// Onboarding and consent live here too: they need a logged-in user but
	// deliberately not a completed profile, which is what they exist to
	// collect.
	authed := e.Group("", authHandler.RequireSession)

	authed.GET("/onboarding", authHandler.OnboardingForm)
	authed.POST("/onboarding", authHandler.OnboardingSubmit)
	authed.GET("/consent", authHandler.ConsentForm)
	authed.POST("/consent", authHandler.ConsentSubmit)
	authed.POST("/consent/withdraw", authHandler.WithdrawConsent)

	authed.GET("/dashboard", authHandler.Dashboard)
	authed.GET("/news", authHandler.NewsList)
	authed.GET("/profile", authHandler.ProfileForm)
	authed.POST("/profile/name", authHandler.ProfileUpdateName)
	authed.POST("/profile/password", authHandler.ProfileChangePassword)
	authed.POST("/profile/treatment", authHandler.ProfileUpdateTreatment)
	authed.GET("/profile/export-data", authHandler.ProfileExportData)
	authed.POST("/profile/delete-account", authHandler.ProfileDeleteAccount)

	authed.GET("/apd", authHandler.ApdDashboard)
	authed.GET("/apd/logs", authHandler.ApdLogsList)
	authed.GET("/apd/new", authHandler.ApdNewForm)
	authed.POST("/apd/new", authHandler.ApdCreate)
	authed.GET("/apd/:id/edit", authHandler.ApdEditForm)
	authed.POST("/apd/:id/edit", authHandler.ApdUpdate)
	authed.POST("/apd/:id/delete", authHandler.ApdDelete)
	authed.GET("/apd/export", authHandler.ApdExport)

	authed.GET("/capd", authHandler.CapdDashboard)
	authed.GET("/capd/logs", authHandler.CapdLogsList)
	authed.GET("/capd/new", authHandler.CapdNewForm)
	authed.POST("/capd/new", authHandler.CapdCreate)
	authed.GET("/capd/:id/edit", authHandler.CapdEditForm)
	authed.POST("/capd/:id/edit", authHandler.CapdUpdate)
	authed.POST("/capd/:id/delete", authHandler.CapdDelete)

	authed.GET("/hd", authHandler.HdDashboard)
	authed.GET("/hd/logs", authHandler.HdLogsList)
	authed.GET("/hd/new", authHandler.HdNewForm)
	authed.POST("/hd/new", authHandler.HdCreate)
	authed.GET("/hd/:id/edit", authHandler.HdEditForm)
	authed.POST("/hd/:id/edit", authHandler.HdUpdate)
	authed.POST("/hd/:id/delete", authHandler.HdDelete)

	authed.GET("/lab-results", authHandler.LabResultsList)
	authed.GET("/lab-results/new", authHandler.LabResultsNewForm)
	authed.POST("/lab-results/new", authHandler.LabResultsCreate)
	authed.GET("/lab-results/:id/edit", authHandler.LabResultsEditForm)
	authed.POST("/lab-results/:id/edit", authHandler.LabResultsUpdate)
	authed.POST("/lab-results/:id/delete", authHandler.LabResultsDelete)

	authed.GET("/food-check", authHandler.FoodCheckSearch)
	authed.GET("/food-check/results", authHandler.FoodCheckSearchResults)
	authed.GET("/food-check/food/:source/:ref", authHandler.FoodCheckDetail)
	authed.GET("/food-check/food/:source/:ref/nutrition", authHandler.FoodCheckNutrition)

	// ---- admin-only routes ----
	//
	// These act on other people's accounts and on what patients get shown,
	// so the role check belongs on the group rather than on each handler's
	// memory.
	adminOnly := e.Group("", authHandler.RequireAdminRole)

	adminOnly.GET("/admin/content-queue", authHandler.AdminContentQueue)
	adminOnly.POST("/admin/content-queue/:id/approve", authHandler.AdminApproveContent)
	adminOnly.POST("/admin/content-queue/:id/reject", authHandler.AdminRejectContent)
	adminOnly.POST("/admin/content-queue/:id/regenerate-image", authHandler.AdminRegenerateImage)

	adminOnly.GET("/admin/editorial", authHandler.EditorialList)
	adminOnly.GET("/admin/editorial/new", authHandler.EditorialNewForm)
	adminOnly.POST("/admin/editorial/new", authHandler.EditorialCreate)
	adminOnly.GET("/admin/editorial/:id/edit", authHandler.EditorialEditForm)
	adminOnly.POST("/admin/editorial/:id/edit", authHandler.EditorialUpdate)
	adminOnly.POST("/admin/editorial/:id/delete", authHandler.EditorialDelete)
	adminOnly.POST("/admin/editorial/upload-media", authHandler.EditorialUploadMedia)

	adminOnly.GET("/admin/users", authHandler.AdminUsersList)
	adminOnly.GET("/admin/users/:id", authHandler.AdminUserDetail)
	adminOnly.POST("/admin/users/:id/verify-email", authHandler.AdminVerifyEmail)
	adminOnly.POST("/admin/users/:id/unlock", authHandler.AdminUnlockAccount)
	adminOnly.POST("/admin/users/:id/suspend", authHandler.AdminSuspendAccount)
	adminOnly.POST("/admin/users/:id/unsuspend", authHandler.AdminUnsuspendAccount)

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

	return e
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
