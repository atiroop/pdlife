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
