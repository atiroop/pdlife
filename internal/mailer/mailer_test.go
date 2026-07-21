package mailer

import (
	"bytes"
	htemplate "html/template"
	"strings"
	"testing"
	ttemplate "text/template"

	"github.com/atiroop/pdlife/internal/config"
)

// TestNew guards the exact failure mode described in main_test.go's
// TestTemplatesParse for web/templates: a template syntax error anywhere
// under templates/*.html or *.txt fails ParseFS for the WHOLE set, and
// New (called once at process startup, in main() and every cmd/ tool)
// would abort the entire app rather than just the one broken email.
// cfg only ever gets stored, never dereferenced during parsing, so a
// zero-value Config needs no real SMTP credentials.
func TestNew(t *testing.T) {
	if _, err := New(&config.Config{}); err != nil {
		t.Fatalf("New() failed to parse embedded templates: %v", err)
	}
}

// TestAllAlertTemplatesExecute goes one step further than TestNew:
// parsing succeeds even if a template references a field the Data struct
// doesn't have (Go's html/template only catches that at Execute time,
// against a real value) - which is exactly the class of bug a
// hand-written {{.Filed}} typo produces. Every Send* method ends in
// m.send(), which opens a real SMTP connection, so this parses the
// embedded templates directly with the same stdlib calls New() uses and
// executes each pair without ever constructing a Mailer or touching the
// network.
func TestAllAlertTemplatesExecute(t *testing.T) {
	htmlTmpl, err := htemplate.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		t.Fatalf("parse html templates: %v", err)
	}
	textTmpl, err := ttemplate.ParseFS(templateFS, "templates/*.txt")
	if err != nil {
		t.Fatalf("parse text templates: %v", err)
	}

	cases := []struct {
		names []string // one Data value may back more than one template pair, e.g. UptimeAlertData
		data  interface{}
	}{
		{[]string{"verify_email"}, VerifyEmailData{Nickname: "ทดสอบ", VerifyURL: "https://pdlife.app/verify-email?token=x"}},
		{[]string{"reset_password"}, ResetPasswordData{Nickname: "ทดสอบ", ResetURL: "https://pdlife.app/reset-password?token=x"}},
		{[]string{"account_deletion"}, AccountDeletionData{Nickname: "ทดสอบ", Email: "test@pdlife.test", PurgeDate: "1 ม.ค. 2570", SupportEmail: "support@pdlife.app"}},
		{[]string{"foodcheck_diff_alert"}, FoodCheckDiffAlertData{SourceName: "Anamai", SourceURL: "https://example.com", OldCount: 100, NewCount: 105, PreviousCheckedAt: "1 ม.ค. 2569", CheckedAt: "1 ก.พ. 2569"}},
		{[]string{"backup_failure_alert"}, BackupFailureAlertData{Step: "mysqldump", Error: "boom", RanAt: "03:00:00 1 ม.ค. 2569", LogHint: "/var/log/pdlife-backup.log"}},
		{[]string{"uptime_down_alert", "uptime_recovered_alert"}, UptimeAlertData{URL: "https://pdlife.app/healthz", Detail: "timeout", Since: "10:00:00 1 ม.ค. 2569", CheckedAt: "10:05:00 1 ม.ค. 2569"}},
		// Full case (stack, user, suppressed count all set) and the empty
		// case (all three at zero value) both need covering: the
		// templates' {{if .Field}} guards are exactly what a typo'd field
		// name would silently skip over instead of erroring on, in either
		// direction.
		{[]string{"server_error_alert"}, ServerErrorAlertData{Status: 500, Method: "GET", Path: "/apd", Error: "boom", Stack: "goroutine 1 [running]:\nmain.main()", UserID: "42", OccurredAt: "10:00:00 1 ม.ค. 2569", SuppressedCount: 3}},
		{[]string{"server_error_alert"}, ServerErrorAlertData{Status: 404, Method: "GET", Path: "/x", Error: "not found", OccurredAt: "10:00:00 1 ม.ค. 2569"}},
	}

	seen := map[string]bool{}
	for _, tc := range cases {
		for _, base := range tc.names {
			seen[base] = true
			for _, ext := range []string{".html", ".txt"} {
				name := base + ext
				t.Run(name, func(t *testing.T) {
					var buf bytes.Buffer
					var execErr error
					if strings.HasSuffix(name, ".html") {
						execErr = htmlTmpl.ExecuteTemplate(&buf, name, tc.data)
					} else {
						execErr = textTmpl.ExecuteTemplate(&buf, name, tc.data)
					}
					if execErr != nil {
						t.Fatalf("execute %s with %#v: %v", name, tc.data, execErr)
					}
					if buf.Len() == 0 {
						t.Errorf("%s rendered to an empty string", name)
					}
				})
			}
		}
	}

	// Every template file on disk should have been exercised above - a
	// new alert added later without a matching test case here would
	// otherwise ship unexecuted, same gap this test exists to close.
	allDefined := htmlTmpl.Templates()
	for _, tmpl := range allDefined {
		name := tmpl.Name()
		if name == "" || !strings.HasSuffix(name, ".html") {
			continue
		}
		base := strings.TrimSuffix(name, ".html")
		if !seen[base] {
			t.Errorf("templates/%s exists but TestAllAlertTemplatesExecute has no case for it - add one", name)
		}
	}
}
