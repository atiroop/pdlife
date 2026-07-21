package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/atiroop/pdlife/internal/auth"
	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/mailer"
	"github.com/atiroop/pdlife/internal/models"
	"gorm.io/gorm"
)

// This file drives the actual auth/session/CSRF code paths end to end
// against newServer() and a real (local, throwaway) database, rather than
// unit-testing pieces of it in isolation. It exists because every bug
// fixed on 2026-07-15 — missing CSRF protection, the concurrent
// refresh-rotation logout race, routes reachable without a session — was
// the kind that unit tests of individual functions would not have caught:
// each one only showed up in how the pieces behaved wired together over
// real HTTP.
//
// Requires a local dev database (scripts/setup-dev-db.ps1). Skips itself
// with a clear reason if one isn't reachable, rather than failing —
// see [[pdlife-project-context]] in project memory for why this
// environment usually can't reach one.

// ---- test harness ----

type testApp struct {
	srv *httptest.Server
	db  *gorm.DB
}

// newTestApp boots the real route table (newServer) against the local dev
// database over TLS. TLS, not plain HTTP, is not incidental: every
// session/CSRF cookie the app issues carries Secure (cookieSecure() is
// true because APP_BASE_URL defaults to https://pdlife.app even for local
// dev — see .env.example). A plain-HTTP httptest.Server would still SET
// those cookies but a spec-compliant client would then refuse to send them
// back on the next request, and every test in this file would fail on
// that mismatch instead of on anything the application actually does.
func newTestApp(t *testing.T) *testApp {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Skipf("skipping auth integration test: config.Load: %v", err)
	}
	// Same guard as cmd/seed_dev and scripts/setup-dev-db.ps1: this file
	// creates and deletes user rows outright, which must never be possible
	// against anything but a local throwaway database.
	if cfg.DBHost != "localhost" && cfg.DBHost != "127.0.0.1" {
		t.Fatalf("DB_HOST is %q, not localhost — refusing to run auth integration tests against a non-local database", cfg.DBHost)
	}

	db, err := config.NewDB(cfg)
	if err != nil {
		t.Skipf("skipping auth integration test: no local dev database reachable (%v) — run scripts/setup-dev-db.ps1", err)
	}

	m, err := mailer.New(cfg)
	if err != nil {
		t.Fatalf("mailer.New: %v", err)
	}

	// rateLimit=false: see newServer's doc comment. Every other piece of
	// the app — CSRF, security headers, session handling, route
	// grouping — is exactly what runs in production.
	e := newServer(cfg, db, m, false)
	srv := httptest.NewTLSServer(e)
	t.Cleanup(srv.Close)

	return &testApp{srv: srv, db: db}
}

// newClient returns a client with its own cookie jar (so each test's
// session is independent) that does not follow redirects, so tests can
// assert on the redirect itself (status + Location) rather than on
// whatever page it points to.
func (a *testApp) newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	return &http.Client{
		Transport: a.srv.Client().Transport, // trusts the test TLS cert
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}
}

// getBody GETs a URL and returns its body and status, so a test that only
// needs one of them doesn't have to manage response-body closing itself.
func getBody(t *testing.T, client *http.Client, rawURL string) (string, int) {
	t.Helper()
	resp, err := client.Get(rawURL)
	if err != nil {
		t.Fatalf("GET %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body of GET %s: %v", rawURL, err)
	}
	return string(b), resp.StatusCode
}

var csrfValueRe = regexp.MustCompile(`name="_csrf"\s+value="([^"]*)"`)

// extractCSRF pulls the token out of a rendered form the same way a real
// browser's submit would pick it up — from the page, not from the cookie
// jar directly, so this exercises templateRenderer's injection too.
func extractCSRF(t *testing.T, body string) string {
	t.Helper()
	m := csrfValueRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no _csrf field found in page:\n%.500s", body)
	}
	return m[1]
}

// loginAs drives the real /login form (GET for the token, POST to submit)
// and fails the test if login doesn't succeed, so every test that needs an
// authenticated client can start from one line instead of repeating the
// flow.
func loginAs(t *testing.T, app *testApp, client *http.Client, email, password string) {
	t.Helper()
	body, _ := getBody(t, client, app.srv.URL+"/login")
	token := extractCSRF(t, body)

	resp, err := client.PostForm(app.srv.URL+"/login", url.Values{
		"_csrf":    {token},
		"email":    {email},
		"password": {password},
	})
	if err != nil {
		t.Fatalf("login POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("login as %s failed: status=%d body=%.300s", email, resp.StatusCode, b)
	}
}

// cookieValue reads a cookie back out of a client's jar without spending a
// request on it.
func cookieValue(t *testing.T, client *http.Client, rawURL, name string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %s: %v", rawURL, err)
	}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

var emailCounter int

// uniqueEmail returns an address no other test (or run) will reuse.
// Login's per-account lockout (auth.LoginLimiter) and per-account state
// (suspended, deletion-pending, ...) are both keyed by email, so tests
// sharing one would corrupt each other's results.
func uniqueEmail(t *testing.T) string {
	t.Helper()
	emailCounter++
	safe := strings.ToLower(strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()))
	return fmt.Sprintf("test-%s-%d-%d@pdlife.invalid", safe, time.Now().UnixNano(), emailCounter)
}

type testUserOpts struct {
	role            models.UserRole // "" defaults to Member
	suspended       bool
	deletionPending bool
	skipProfile     bool // when true, don't complete onboarding
}

// seedTestUser writes a user directly to the database — mirroring
// cmd/seed_dev — rather than going through /register, which would send a
// real verification email through the production Resend account (see
// cmd/seed_dev's own doc comment). By default the seeded user is a fully
// onboarded, consented APD patient, matching what a real logged-in patient
// looks like; opts narrow that down for the specific state a test needs.
func seedTestUser(t *testing.T, db *gorm.DB, email, password string, opts testUserOpts) *models.User {
	t.Helper()

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	role := opts.role
	if role == "" {
		role = models.RoleMember
	}

	now := time.Now()
	user := models.User{
		Email:        email,
		PasswordHash: hash,
		Nickname:     "ทดสอบ",
		Role:         role,
		IsActive:     true,
	}
	if role != models.RoleUnverified {
		user.EmailVerifiedAt = &now
	}
	if opts.suspended {
		reason := "ทดสอบ automated test"
		user.SuspendedAt = &now
		user.SuspendedReason = &reason
	}
	if opts.deletionPending {
		user.AccountDeletionRequestedAt = &now
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}
	t.Cleanup(func() {
		// Unscoped: a hard delete, not the soft delete gorm.DeletedAt would
		// do — this is throwaway test data, and refresh_tokens/
		// patient_profiles cascade off the FK so nothing else needs
		// cleaning up.
		db.Unscoped().Delete(&user)
	})

	if !opts.skipProfile {
		treatment := models.TreatmentAPD
		profile := models.PatientProfile{
			UserID:              user.ID,
			TreatmentType:       &treatment,
			ProfileCompletedAt:  &now,
			HealthDataConsentAt: &now,
		}
		if err := db.Create(&profile).Error; err != nil {
			t.Fatalf("create test patient profile: %v", err)
		}
	}

	return &user
}

const testPassword = "correcthorsebattery9"

// ---- login ----

func TestLogin_WrongPassword(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{})

	body, _ := getBody(t, client, app.srv.URL+"/login")
	token := extractCSRF(t, body)

	resp, err := client.PostForm(app.srv.URL+"/login", url.Values{
		"_csrf": {token}, "email": {email}, "password": {"the-wrong-password-1"},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if !strings.Contains(string(b), "อีเมลหรือรหัสผ่านไม่ถูกต้อง") {
		t.Errorf("response body does not contain the expected error message:\n%.400s", b)
	}
}

func TestLogin_CorrectPassword_EstablishesASessionThatReachesDashboard(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{})

	loginAs(t, app, client, email, testPassword)

	resp, err := client.Get(app.srv.URL + "/dashboard")
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/dashboard after login = %d, want 200", resp.StatusCode)
	}
}

func TestLogin_UnverifiedAccountBlocked(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{role: models.RoleUnverified})

	body, _ := getBody(t, client, app.srv.URL+"/login")
	token := extractCSRF(t, body)

	resp, err := client.PostForm(app.srv.URL+"/login", url.Values{
		"_csrf": {token}, "email": {email}, "password": {testPassword},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (login_unverified.html)", resp.StatusCode)
	}
}

func TestLogin_SuspendedAccountBlocked(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{suspended: true})

	body, _ := getBody(t, client, app.srv.URL+"/login")
	token := extractCSRF(t, body)

	resp, err := client.PostForm(app.srv.URL+"/login", url.Values{
		"_csrf": {token}, "email": {email}, "password": {testPassword},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if !strings.Contains(string(b), "บัญชีถูกระงับชั่วคราว") {
		t.Errorf("response does not mention suspension:\n%.400s", b)
	}
}

func TestLogin_DeletionPendingAccountBlocked(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{deletionPending: true})

	body, _ := getBody(t, client, app.srv.URL+"/login")
	token := extractCSRF(t, body)

	resp, err := client.PostForm(app.srv.URL+"/login", url.Values{
		"_csrf": {token}, "email": {email}, "password": {testPassword},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if !strings.Contains(string(b), "อยู่ระหว่างกระบวนการลบ") {
		t.Errorf("response does not mention pending deletion:\n%.400s", b)
	}
}

// ---- CSRF ----

func TestCSRF_MissingTokenRejected(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)

	// No _csrf field, and Go's http.Client never sends Sec-Fetch-Site, so
	// this exercises the fallback token-check path (the one that protects
	// browsers Echo can't identify as same-origin — see
	// [[pdlife-project-context]] on the Sec-Fetch-Site verification trap).
	resp, err := client.PostForm(app.srv.URL+"/login", url.Values{
		"email": {"nobody@pdlife.invalid"}, "password": {"whatever12"},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (missing csrf token)", resp.StatusCode)
	}
}

func TestCSRF_WrongTokenRejected(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	// GET first so the client holds a real _csrf cookie to be checked
	// against — this confirms the middleware actually compares values
	// rather than merely requiring the field to be present.
	getBody(t, client, app.srv.URL+"/login")

	resp, err := client.PostForm(app.srv.URL+"/login", url.Values{
		"_csrf": {"totally-made-up-token-value"}, "email": {"nobody@pdlife.invalid"}, "password": {"whatever12"},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (wrong csrf token)", resp.StatusCode)
	}
}

func TestCSRF_CrossSiteRequestBlocked(t *testing.T) {
	app := newTestApp(t)

	// The actual attack this protects against: a request carrying
	// Sec-Fetch-Site: cross-site, which only a real cross-origin page load
	// can produce — no browser lets JavaScript set this header itself, so
	// this is the one check in this file that could not be reproduced by
	// clicking around the app manually. See [[pdlife-project-context]].
	req, err := http.NewRequest(http.MethodPost, app.srv.URL+"/login",
		strings.NewReader(url.Values{"email": {"nobody@pdlife.invalid"}, "password": {"whatever12"}}.Encode()))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	resp, err := app.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (cross-site request blocked by CSRF)", resp.StatusCode)
	}
}

func TestCSRF_RealLoginStillWorks(t *testing.T) {
	// Guards against the failure mode on the other side of CSRF: a change
	// here that blocks real submissions instead of just fake ones.
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{})

	loginAs(t, app, client, email, testPassword) // fails the test itself if this doesn't 302
}

// ---- route-group auth gating ----

func TestAuthGate_LoggedOutRedirectsToLogin(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)

	for _, p := range []string{
		"/dashboard", "/apd", "/capd", "/hd", "/lab-results",
		"/food-check", "/profile", "/news", "/onboarding", "/consent",
		"/admin/users", "/admin/content-queue",
	} {
		resp, err := client.Get(app.srv.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("%s (logged out) = %d, want 303", p, resp.StatusCode)
			continue
		}
		if loc := resp.Header.Get("Location"); loc != "/login" {
			t.Errorf("%s (logged out) redirected to %q, want /login", p, loc)
		}
	}
}

func TestAuthGate_PlainMemberCannotReachAdminRoutes(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{role: models.RoleMember})
	loginAs(t, app, client, email, testPassword)

	resp, err := client.Get(app.srv.URL + "/admin/users")
	if err != nil {
		t.Fatalf("GET /admin/users: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("/admin/users as Member = %d, want 403", resp.StatusCode)
	}
	if !strings.Contains(string(b), "ไม่มีสิทธิ์เข้าถึง") {
		t.Errorf("403 body missing the expected message:\n%.400s", b)
	}

	// And confirm the 403 didn't take the whole session down with it.
	resp2, err := client.Get(app.srv.URL + "/dashboard")
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("own /dashboard after a 403 elsewhere = %d, want 200", resp2.StatusCode)
	}
}

func TestAuthGate_AdminReachesAdminRoutes(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{role: models.RoleAdmin})
	loginAs(t, app, client, email, testPassword)

	resp, err := client.Get(app.srv.URL + "/admin/users")
	if err != nil {
		t.Fatalf("GET /admin/users: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/admin/users as Admin = %d, want 200", resp.StatusCode)
	}
}

// ---- logout ----

func TestLogout_KillsSessionImmediately(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{})
	loginAs(t, app, client, email, testPassword)

	if _, status := getBody(t, client, app.srv.URL+"/dashboard"); status != http.StatusOK {
		t.Fatalf("pre-logout /dashboard = %d, want 200", status)
	}

	profileBody, _ := getBody(t, client, app.srv.URL+"/profile")
	token := extractCSRF(t, profileBody)

	resp, err := client.PostForm(app.srv.URL+"/logout", url.Values{"_csrf": {token}})
	if err != nil {
		t.Fatalf("POST /logout: %v", err)
	}
	resp.Body.Close()

	resp2, err := client.Get(app.srv.URL + "/dashboard")
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusSeeOther {
		t.Errorf("/dashboard after logout = %d, want 303 (session must not survive logout, even briefly)", resp2.StatusCode)
	}
}

// ---- session rotation ----

// TestSessionRotation_ConcurrentRequestsDoNotLogOut is a permanent
// regression test for the bug fixed on 2026-07-15
// (internal/handler/session.go): resolving a session with an expired
// access token rotates the refresh token, and sibling requests already in
// flight with the same (about-to-be-superseded) refresh cookie were being
// treated as invalid — three concurrent requests reliably logged out one
// or two of them. Reproduced and fixed with a rotation grace window keyed
// off a new rotated_at column, distinct from a token revoked for a real
// reason (logout, suspension, ...), which must still bite immediately —
// see TestLogout_KillsSessionImmediately above and session.go's comments.
func TestSessionRotation_ConcurrentRequestsDoNotLogOut(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)
	email := uniqueEmail(t)
	seedTestUser(t, app.db, email, testPassword, testUserOpts{})
	loginAs(t, app, client, email, testPassword)

	refreshToken := cookieValue(t, client, app.srv.URL, "pdlife_refresh")
	if refreshToken == "" {
		t.Fatal("no pdlife_refresh cookie after login")
	}

	// A client that does NOT follow redirects: a rejected session redirects
	// to /login, which itself renders 200. A client that followed the
	// redirect would report that 200 and this test would pass no matter
	// what currentSession decided — silently checking nothing. Confirmed
	// by mutation: with this fixed, shrinking refreshRotationGrace to ~0
	// made this test fail as expected; with app.srv.Client()'s default
	// redirect-following used here originally, it did not.
	noRedirectClient := &http.Client{
		Transport: app.srv.Client().Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	// Simulates an expired access token by sending only the refresh
	// cookie — exactly what currentSession sees once the 1-hour access
	// token has expired and only the 30-day refresh token remains.
	const concurrency = 6
	statuses := make([]int, concurrency)
	errs := make([]error, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, app.srv.URL+"/dashboard", nil)
			if err != nil {
				errs[i] = err
				return
			}
			req.AddCookie(&http.Cookie{Name: "pdlife_refresh", Value: refreshToken})
			resp, err := noRedirectClient.Do(req)
			if err != nil {
				errs[i] = err
				return
			}
			defer resp.Body.Close()
			statuses[i] = resp.StatusCode
		}(i)
	}
	wg.Wait()

	for i := range statuses {
		if errs[i] != nil {
			t.Errorf("request %d: %v", i, errs[i])
			continue
		}
		if statuses[i] != http.StatusOK {
			t.Errorf("request %d (concurrent refresh-token rotation) = %d, want 200 — a patient with an expired access token and two tabs open would have been logged out of one of them", i, statuses[i])
		}
	}
}
