package main

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// This file drives internal/handler.ErrorReporter through the exact same
// wiring newServer gives production (real templates, real Recover
// config), not a hand-assembled echo.Echo, so it fails if the pieces stop
// fitting together the way main.go actually connects them.
//
// newTestApp always passes DisableErrorAlerts — see its comment — so
// these can safely trigger a real 5xx without emailing
// cfg.AdminAlertEmail through the test machine's real SMTP credentials.

// TestErrorReporter_PanicRendersFriendlyPage is the regression test for
// why ErrorReporter exists at all: before it, a panic anywhere reached
// Echo's bare default error handler, so a real breakage looked identical
// in the browser to Echo's generic "Internal Server Error" - nothing
// telling the patient what happened, and nothing telling pdlife it had.
func TestErrorReporter_PanicRendersFriendlyPage(t *testing.T) {
	app := newTestApp(t)
	// Registered directly on the real *echo.Echo newServer already built
	// (app.e) - same renderer, same Recover/HTTPErrorHandler chain
	// production uses - never on main.go's own route table.
	app.e.GET("/__test_panic__", func(c echo.Context) error {
		panic("deliberate panic for TestErrorReporter_PanicRendersFriendlyPage")
	})

	client := app.newClient(t)
	resp, err := client.Get(app.srv.URL + "/__test_panic__")
	if err != nil {
		t.Fatalf("GET /__test_panic__: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if !strings.Contains(bodyStr, "เกิดข้อผิดพลาด") {
		t.Errorf("body does not contain the friendly error page's title:\n%.500s", bodyStr)
	}
	if !strings.Contains(bodyStr, "ระบบขัดข้องชั่วคราว") {
		t.Errorf("body does not contain the friendly error page's message:\n%.500s", bodyStr)
	}
	// Echo's bare default error handler renders {"message":"..."} - if
	// that's what's in the body, HTTPErrorHandler was never wired up at
	// all (this exact regression, silently, is what prompted checking).
	if strings.Contains(bodyStr, `"message"`) {
		t.Errorf("body looks like Echo's default JSON error handler, not placeholder.html:\n%.500s", bodyStr)
	}
}

// TestErrorReporter_NotFoundRendersFriendlyPage covers the other common
// path into the same handler - Echo's own routing, not a handler error -
// since 404 and the panic case share HTTPErrorHandler but take visibly
// different branches inside errorPageCopy.
func TestErrorReporter_NotFoundRendersFriendlyPage(t *testing.T) {
	app := newTestApp(t)
	client := app.newClient(t)

	resp, err := client.Get(app.srv.URL + "/this-route-does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if !strings.Contains(bodyStr, "ไม่พบหน้านี้") {
		t.Errorf("body does not contain the 404 page's title:\n%.500s", bodyStr)
	}
}
