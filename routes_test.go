package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Routes registered straight on `e` are public. That is the right default
// for Echo and the wrong default for this app, where all but a handful of
// pages are a patient's health data. A new route pasted next to its
// neighbours and left on `e` is served to anyone, with nothing to notice —
// no error, no failing test, just an open endpoint.
//
// So public is made the thing you have to say out loud: every path below
// is an explicit decision, and anything else must go through the `authed`
// or `adminOnly` group.
//
// This reads main.go as text rather than introspecting the router because
// Echo does not expose which middleware a route ended up with.
var publicPaths = map[string]string{
	"/":                    "landing page",
	"/dashboard-preview":   "static marketing demo of the dashboard, mock data only",
	"/terms":               "legal",
	"/privacy":             "legal",
	"/cookie-policy":       "legal",
	"/register":            "sign-up",
	"/verify-email":        "sign-up: reached from an email link, before any session exists",
	"/resend-verification": "sign-up",
	"/login":               "sign-in",
	"/logout":              "clears cookies; gating it would strand anyone whose session is already broken",
	"/forgot-password":     "password recovery, by definition pre-session",
	"/reset-password":      "password recovery, authorised by the emailed token",
	"/articles":            "public content marketing",
	"/articles/:slug":      "public content marketing",
	"/healthz":             "uptime check (cmd/uptime_check), no data",
}

var routeRe = regexp.MustCompile(`(?m)^\s*(e|authed|adminOnly)\.(GET|POST|PUT|DELETE|PATCH)\("([^"]*)"`)

func TestEveryNonPublicRouteIsInAnAuthGroup(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	matches := routeRe.FindAllStringSubmatch(string(src), -1)
	if len(matches) == 0 {
		t.Fatal("found no route registrations — the scan is broken, not main.go")
	}

	var public, gated int
	for _, m := range matches {
		receiver, method, path := m[1], m[2], m[3]

		if receiver != "e" {
			gated++
			continue
		}
		if _, ok := publicPaths[path]; !ok {
			t.Errorf("%s %s is registered on `e`, so it is public.\n"+
				"  If that is intended, add it to publicPaths in this test with the reason.\n"+
				"  If not, register it on `authed` or `adminOnly`.", method, path)
			continue
		}
		public++
	}

	t.Logf("%d gated routes, %d public", gated, public)
}

// A stale allowlist is how a route quietly goes from public-on-purpose to
// public-by-accident: someone gates it, the entry lingers, and the next
// person to add that path back gets a free pass.
func TestPublicAllowlistHasNoStaleEntries(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	registered := map[string]bool{}
	for _, m := range routeRe.FindAllStringSubmatch(string(src), -1) {
		if m[1] == "e" {
			registered[m[3]] = true
		}
	}

	for path := range publicPaths {
		if !registered[path] {
			t.Errorf("publicPaths lists %q, but no public route registers it — remove the entry", path)
		}
	}
}

// Every /admin path must be on adminOnly. Being merely logged in is not
// enough to suspend someone's account or publish to every patient.
func TestAdminPathsAreOnTheAdminGroup(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	for _, m := range routeRe.FindAllStringSubmatch(string(src), -1) {
		receiver, method, path := m[1], m[2], m[3]
		if strings.HasPrefix(path, "/admin") && receiver != "adminOnly" {
			t.Errorf("%s %s is on `%s`; /admin routes must be on `adminOnly`", method, path, receiver)
		}
	}
}
