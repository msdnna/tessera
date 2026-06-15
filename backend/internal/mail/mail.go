// Package mail sends transactional email (account verification, workspace
// invitations, password resets). U1 only scaffolds it — the real flows arrive
// in U2, when Tessera may run on a public web server rather than a pure
// self-host. When SMTP is unconfigured (the typical self-host today) the no-op
// mailer logs and drops the message, so callers never have to special-case it.
package mail

import (
	"fmt"
	"log"
	"net/smtp"
	"strings"
)

// Mailer sends a single plain-text email.
type Mailer interface {
	// Send delivers one message; a no-op mailer logs and returns nil.
	Send(to, subject, body string) error
	// Enabled reports whether a real transport is configured.
	Enabled() bool
}

// Config holds SMTP connection settings (all from env; see config.Config).
type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// New returns an SMTP mailer when Host and From are set, otherwise a no-op
// mailer that logs and drops (so self-host installs work without SMTP).
func New(cfg Config) Mailer {
	if cfg.Host == "" || cfg.From == "" {
		log.Println("mail: SMTP not configured — emails are logged and dropped (used by U2 features)")
		return noop{}
	}
	if cfg.Port == "" {
		cfg.Port = "587"
	}
	return &smtpMailer{cfg: cfg}
}

type noop struct{}

func (noop) Enabled() bool { return false }

func (noop) Send(to, subject, body string) error {
	// Log the full body too: on a self-host without SMTP this is the only place
	// the verification / reset / invite link surfaces (invites also return it in
	// the API response).
	log.Printf("mail (noop) to %s — %s\n%s", to, subject, body)
	return nil
}

type smtpMailer struct{ cfg Config }

func (m *smtpMailer) Enabled() bool { return true }

func (m *smtpMailer) Send(to, subject, body string) error {
	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}
	return smtp.SendMail(m.cfg.Host+":"+m.cfg.Port, auth, m.cfg.From, []string{to}, buildMessage(m.cfg.From, to, subject, body))
}

func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
