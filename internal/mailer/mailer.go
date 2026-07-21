// Package mailer sends transactional email via Resend's SMTP relay.
package mailer

import (
	"bytes"
	"crypto/tls"
	"embed"
	"fmt"
	htemplate "html/template"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	ttemplate "text/template"
	"time"

	"github.com/atiroop/pdlife/internal/config"
)

// sendTimeout bounds the whole SMTP conversation so a slow or
// unreachable mail server can never hang a request indefinitely -
// net/smtp.SendMail has no built-in timeout.
const sendTimeout = 10 * time.Second

//go:embed templates/*.html templates/*.txt
var templateFS embed.FS

type Mailer struct {
	cfg      *config.Config
	htmlTmpl *htemplate.Template
	textTmpl *ttemplate.Template
}

func New(cfg *config.Config) (*Mailer, error) {
	htmlTmpl, err := htemplate.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse email html templates: %w", err)
	}
	textTmpl, err := ttemplate.ParseFS(templateFS, "templates/*.txt")
	if err != nil {
		return nil, fmt.Errorf("parse email text templates: %w", err)
	}
	return &Mailer{cfg: cfg, htmlTmpl: htmlTmpl, textTmpl: textTmpl}, nil
}

type VerifyEmailData struct {
	Nickname  string
	VerifyURL string
}

func (m *Mailer) SendVerificationEmail(to string, data VerifyEmailData) error {
	var htmlBuf, textBuf bytes.Buffer
	if err := m.htmlTmpl.ExecuteTemplate(&htmlBuf, "verify_email.html", data); err != nil {
		return fmt.Errorf("render verify email html: %w", err)
	}
	if err := m.textTmpl.ExecuteTemplate(&textBuf, "verify_email.txt", data); err != nil {
		return fmt.Errorf("render verify email text: %w", err)
	}
	return m.send(to, "ยืนยันอีเมลของคุณ - pdlife.app", htmlBuf.String(), textBuf.String())
}

type ResetPasswordData struct {
	Nickname string
	ResetURL string
}

func (m *Mailer) SendPasswordResetEmail(to string, data ResetPasswordData) error {
	var htmlBuf, textBuf bytes.Buffer
	if err := m.htmlTmpl.ExecuteTemplate(&htmlBuf, "reset_password.html", data); err != nil {
		return fmt.Errorf("render reset password html: %w", err)
	}
	if err := m.textTmpl.ExecuteTemplate(&textBuf, "reset_password.txt", data); err != nil {
		return fmt.Errorf("render reset password text: %w", err)
	}
	return m.send(to, "รีเซ็ตรหัสผ่านของคุณ - pdlife.app", htmlBuf.String(), textBuf.String())
}

type AccountDeletionData struct {
	Nickname     string
	Email        string
	PurgeDate    string // Thai-formatted date, ~90 days out — see handler.AccountDeletionGraceDays
	SupportEmail string
}

func (m *Mailer) SendAccountDeletionEmail(to string, data AccountDeletionData) error {
	var htmlBuf, textBuf bytes.Buffer
	if err := m.htmlTmpl.ExecuteTemplate(&htmlBuf, "account_deletion.html", data); err != nil {
		return fmt.Errorf("render account deletion html: %w", err)
	}
	if err := m.textTmpl.ExecuteTemplate(&textBuf, "account_deletion.txt", data); err != nil {
		return fmt.Errorf("render account deletion text: %w", err)
	}
	return m.send(to, "คำขอลบบัญชีของคุณ - pdlife.app", htmlBuf.String(), textBuf.String())
}

// FoodCheckDiffAlertData is used by cmd/foodcheck_diffcheck — see that
// tool's doc comment for the monthly drift-check this powers.
type FoodCheckDiffAlertData struct {
	SourceName        string
	SourceURL         string
	OldCount          int
	NewCount          int
	PreviousCheckedAt string
	CheckedAt         string
}

func (m *Mailer) SendFoodCheckDiffAlert(to string, data FoodCheckDiffAlertData) error {
	var htmlBuf, textBuf bytes.Buffer
	if err := m.htmlTmpl.ExecuteTemplate(&htmlBuf, "foodcheck_diff_alert.html", data); err != nil {
		return fmt.Errorf("render foodcheck diff alert html: %w", err)
	}
	if err := m.textTmpl.ExecuteTemplate(&textBuf, "foodcheck_diff_alert.txt", data); err != nil {
		return fmt.Errorf("render foodcheck diff alert text: %w", err)
	}
	return m.send(to, "Food Check: พบความเปลี่ยนแปลงที่แหล่งข้อมูล "+data.SourceName+" - pdlife.app", htmlBuf.String(), textBuf.String())
}

// ServerErrorAlertData is used by internal/handler's error reporter for a
// genuine 5xx or recovered panic from a live request — see that file's
// doc comment for why this is deliberately rate-limited to one email
// per cooldown window rather than one per error.
type ServerErrorAlertData struct {
	Status          int
	Method          string
	Path            string
	Error           string
	Stack           string // empty unless this came from a recovered panic
	UserID          string // empty if the request had no session
	OccurredAt      string
	SuppressedCount int // other 5xx/panics swallowed by the cooldown since the last alert
}

func (m *Mailer) SendServerErrorAlert(to string, data ServerErrorAlertData) error {
	var htmlBuf, textBuf bytes.Buffer
	if err := m.htmlTmpl.ExecuteTemplate(&htmlBuf, "server_error_alert.html", data); err != nil {
		return fmt.Errorf("render server error alert html: %w", err)
	}
	if err := m.textTmpl.ExecuteTemplate(&textBuf, "server_error_alert.txt", data); err != nil {
		return fmt.Errorf("render server error alert text: %w", err)
	}
	subject := fmt.Sprintf("🔴 pdlife.app: %d บน %s %s", data.Status, data.Method, data.Path)
	return m.send(to, subject, htmlBuf.String(), textBuf.String())
}

// BackupFailureAlertData is used by cmd/db_backup on any failed step
// (mysqldump, gzip, R2 upload, or retention pruning) — see that tool's doc
// comment. A silently-failing backup is more dangerous than no backup at
// all, since it looks safe while not being one.
type BackupFailureAlertData struct {
	Step    string // which step failed, e.g. "mysqldump", "gzip", "r2 upload"
	Error   string
	RanAt   string
	LogHint string // where to find full logs on the VPS
}

func (m *Mailer) SendBackupFailureAlert(to string, data BackupFailureAlertData) error {
	var htmlBuf, textBuf bytes.Buffer
	if err := m.htmlTmpl.ExecuteTemplate(&htmlBuf, "backup_failure_alert.html", data); err != nil {
		return fmt.Errorf("render backup failure alert html: %w", err)
	}
	if err := m.textTmpl.ExecuteTemplate(&textBuf, "backup_failure_alert.txt", data); err != nil {
		return fmt.Errorf("render backup failure alert text: %w", err)
	}
	return m.send(to, "⚠ Database backup ล้มเหลว - pdlife.app", htmlBuf.String(), textBuf.String())
}

// UptimeAlertData is used by cmd/uptime_check for both the "down" and
// "recovered" emails — the two share the same fields.
type UptimeAlertData struct {
	URL       string
	Detail    string // status code or timeout/connection error
	Since     string // when this state was first observed
	CheckedAt string
}

func (m *Mailer) SendUptimeDownAlert(to string, data UptimeAlertData) error {
	var htmlBuf, textBuf bytes.Buffer
	if err := m.htmlTmpl.ExecuteTemplate(&htmlBuf, "uptime_down_alert.html", data); err != nil {
		return fmt.Errorf("render uptime down alert html: %w", err)
	}
	if err := m.textTmpl.ExecuteTemplate(&textBuf, "uptime_down_alert.txt", data); err != nil {
		return fmt.Errorf("render uptime down alert text: %w", err)
	}
	return m.send(to, "🔴 pdlife.app ล่ม (down)", htmlBuf.String(), textBuf.String())
}

func (m *Mailer) SendUptimeRecoveredAlert(to string, data UptimeAlertData) error {
	var htmlBuf, textBuf bytes.Buffer
	if err := m.htmlTmpl.ExecuteTemplate(&htmlBuf, "uptime_recovered_alert.html", data); err != nil {
		return fmt.Errorf("render uptime recovered alert html: %w", err)
	}
	if err := m.textTmpl.ExecuteTemplate(&textBuf, "uptime_recovered_alert.txt", data); err != nil {
		return fmt.Errorf("render uptime recovered alert text: %w", err)
	}
	return m.send(to, "✅ pdlife.app กลับมาทำงานปกติแล้ว", htmlBuf.String(), textBuf.String())
}

const boundary = "pdlife-boundary-7f3a9c"

func (m *Mailer) send(to, subject, htmlBody, textBody string) error {
	fromAddr, err := mail.ParseAddress(m.cfg.SMTPFrom)
	if err != nil {
		return fmt.Errorf("invalid SMTP_FROM config: %w", err)
	}

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", m.cfg.SMTPFrom)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary)

	fmt.Fprintf(&msg, "--%s\r\n", boundary)
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	msg.WriteString(textBody)
	msg.WriteString("\r\n\r\n")

	fmt.Fprintf(&msg, "--%s\r\n", boundary)
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n\r\n")

	fmt.Fprintf(&msg, "--%s--\r\n", boundary)

	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPassword, m.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%s", m.cfg.SMTPHost, m.cfg.SMTPPort)
	return sendMailWithTimeout(addr, m.cfg.SMTPHost, auth, fromAddr.Address, []string{to}, msg.Bytes())
}

// sendMailWithTimeout mirrors net/smtp.SendMail but with a bounded dial
// and overall deadline, since the standard library version can block
// forever if the server accepts the TCP connection but never responds.
func sendMailWithTimeout(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, sendTimeout)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	if err := conn.SetDeadline(time.Now().Add(sendTimeout)); err != nil {
		conn.Close()
		return err
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return fmt.Errorf("smtp rcpt to: %w", err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return client.Quit()
}
