package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A form that posts without a _csrf field gets a 403 from the CSRF
// middleware, and the failure is invisible from the outside: the page still
// renders, the deploy smoke test still sees 200, and only a real user
// pressing submit finds out. There are 30 such forms and any new one is a
// chance to forget, so this checks the whole template tree instead of
// trusting review.
//
// It reads the templates from disk rather than the embedded FS so a failure
// can name the file the developer has open.

var (
	formTagRe = regexp.MustCompile(`(?is)<form[^>]*>`)
	methodRe  = regexp.MustCompile(`(?i)method\s*=\s*"([^"]*)"`)
	csrfRe    = regexp.MustCompile(`(?i)name\s*=\s*"_csrf"`)
)

// formTagsWithMethod returns every <form> tag in src whose method matches
// want, paired with the byte offset just past the tag.
func formTagsWithMethod(src, want string) [][]int {
	var out [][]int
	for _, loc := range formTagRe.FindAllStringIndex(src, -1) {
		tag := src[loc[0]:loc[1]]
		m := methodRe.FindStringSubmatch(tag)
		method := "get" // HTML's default when the attribute is absent
		if m != nil {
			method = strings.ToLower(m[1])
		}
		if method == want {
			out = append(out, loc)
		}
	}
	return out
}

func templateFiles(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("web", "templates", "*.html"))
	if err != nil {
		t.Fatalf("glob templates: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no templates found — is the test running from the repo root?")
	}

	files := make(map[string]string, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		files[filepath.Base(p)] = string(b)
	}
	return files
}

func TestEveryPostFormCarriesACSRFField(t *testing.T) {
	var total int

	for name, src := range templateFiles(t) {
		for _, loc := range formTagsWithMethod(src, "post") {
			total++

			// The field has to be inside this form, so only look as far as
			// the next </form>.
			rest := src[loc[1]:]
			if end := strings.Index(strings.ToLower(rest), "</form>"); end >= 0 {
				rest = rest[:end]
			}
			if !csrfRe.MatchString(rest) {
				line := 1 + strings.Count(src[:loc[0]], "\n")
				t.Errorf("%s:%d: POST form has no _csrf field — it will 403 on submit.\n  %s\n  Add: <input type=\"hidden\" name=\"_csrf\" value=\"{{ $.csrf }}\">",
					name, line, strings.TrimSpace(src[loc[0]:loc[1]]))
			}
		}
	}

	if total == 0 {
		t.Fatal("found no POST forms at all — the scan is broken, not the templates")
	}
	t.Logf("checked %d POST forms", total)
}

// $.csrf resolves against the data passed to the enclosing template, while
// .csrf resolves against the current dot. Inside a {{range}} the dot is the
// loop element, so .csrf silently renders empty there and the form 403s.
// $.csrf is correct in both places, so require it everywhere rather than
// trying to work out which forms are in a range.
func TestCSRFFieldsUseRootScope(t *testing.T) {
	badValue := regexp.MustCompile(`(?i)name\s*=\s*"_csrf"[^>]*value\s*=\s*"\{\{\s*\.csrf`)

	for name, src := range templateFiles(t) {
		for _, loc := range badValue.FindAllStringIndex(src, -1) {
			line := 1 + strings.Count(src[:loc[0]], "\n")
			t.Errorf("%s:%d: uses {{ .csrf }}; use {{ $.csrf }} so it also resolves inside a {{range}}", name, line)
		}
	}
}

// GET forms put their fields in the URL, so a token there would leak into
// the query string, browser history and any Referer — and it buys nothing,
// since the middleware only checks unsafe methods.
func TestGetFormsDoNotCarryACSRFField(t *testing.T) {
	for name, src := range templateFiles(t) {
		for _, loc := range formTagsWithMethod(src, "get") {
			rest := src[loc[1]:]
			if end := strings.Index(strings.ToLower(rest), "</form>"); end >= 0 {
				rest = rest[:end]
			}
			if csrfRe.MatchString(rest) {
				line := 1 + strings.Count(src[:loc[0]], "\n")
				t.Errorf("%s:%d: GET form carries a _csrf field — it would end up in the query string", name, line)
			}
		}
	}
}

// The fetch() callers can't use a form field, so they read the token from a
// meta tag. If the tag goes missing the header is sent empty and the action
// 403s.
func TestTemplatesPostingViaFetchExposeTheTokenInAMetaTag(t *testing.T) {
	fetchPost := regexp.MustCompile(`(?is)fetch\([^)]*method:\s*'POST'`)
	metaTag := regexp.MustCompile(`(?i)<meta\s+name="csrf-token"\s+content="\{\{\s*\$\.csrf\s*\}\}"`)

	for name, src := range templateFiles(t) {
		if !fetchPost.MatchString(src) {
			continue
		}
		if !metaTag.MatchString(src) {
			t.Errorf(`%s: POSTs via fetch() but has no <meta name="csrf-token" content="{{ $.csrf }}">`, name)
			continue
		}
		for _, loc := range fetchPost.FindAllStringIndex(src, -1) {
			call := src[loc[0]:]
			if end := strings.Index(call, ")"); end >= 0 {
				call = call[:end]
			}
			if !strings.Contains(strings.ToLower(call), "x-csrf-token") {
				line := 1 + strings.Count(src[:loc[0]], "\n")
				t.Errorf("%s:%d: fetch POST does not send the X-CSRF-Token header:\n  %s",
					name, line, strings.TrimSpace(fmt.Sprint(call)))
			}
		}
	}
}
