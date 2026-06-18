// Package mail sends transactional email (account verification, workspace
// invitations, password resets). U1 only scaffolds it — the real flows arrive
// in U2, when Tessera may run on a public web server rather than a pure
// self-host. When SMTP is unconfigured (the typical self-host today) the no-op
// mailer logs and drops the message, so callers never have to special-case it.
package mail

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// smtpTimeout bounds the whole connect+send exchange so a misconfigured or
// unreachable server can't hang a request (or a background goroutine) forever.
const smtpTimeout = 20 * time.Second

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
	mode := "STARTTLS"
	if cfg.Port == "465" {
		mode = "implicit-TLS"
	}
	log.Printf("mail: SMTP enabled — %s:%s (%s), from=%s", cfg.Host, cfg.Port, mode, cfg.From)
	return &smtpMailer{cfg: cfg}
}

// SendAsync delivers a message in the background, logging any error. Use it for
// transactional mail (verification / reset / invite) so the HTTP request never
// blocks on — or fails because of — the SMTP exchange.
func SendAsync(m Mailer, to, subject, body string) {
	go func() {
		if err := m.Send(to, subject, body); err != nil {
			log.Printf("mail: send to %s (%q) failed: %v", to, subject, err)
		}
	}()
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

// Send delivers one message. Supports both implicit TLS (port 465, SMTPS) and
// STARTTLS (587/25) — stdlib smtp.SendMail only does the latter, which is why a
// 465 server would hang. Bounded by smtpTimeout end-to-end.
func (m *smtpMailer) Send(to, subject, body string) error {
	addr := m.cfg.Host + ":" + m.cfg.Port
	dialer := &net.Dialer{Timeout: smtpTimeout}
	tlsCfg := &tls.Config{ServerName: m.cfg.Host}

	var conn net.Conn
	var err error
	if m.cfg.Port == "465" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg) // implicit TLS
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	// On a plaintext connection (587/25), upgrade to TLS if offered.
	if m.cfg.Port != "465" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		}
	}

	if m.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := client.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("MAIL FROM %s: %w", m.cfg.From, err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO %s: %w", to, err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(buildMessage(m.cfg.From, to, subject, body)); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close body: %w", err)
	}
	return client.Quit()
}

func buildMessage(from, to, subject, body string) []byte {
	// A well-formed message: a Date and a unique Message-ID, the (Cyrillic)
	// Subject and From display name RFC 2047-encoded, and an explicit transfer
	// encoding. Missing these reads as spam to strict relays (e.g. Yandex 554).
	domain := from
	if i := strings.LastIndex(from, "@"); i >= 0 {
		domain = from[i+1:]
	}
	var idRaw [16]byte
	_, _ = rand.Read(idRaw[:])

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s <%s>\r\n", mime.QEncoding.Encode("utf-8", "Tessera"), from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(idRaw[:]), domain)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	// Quoted-printable, not raw 8bit: net/smtp never negotiates 8BITMIME, so a
	// body of raw UTF-8 bytes declared as 8bit reads as spam to strict relays
	// (Yandex 554). QP is 7-bit-safe and universally accepted.
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	var qp bytes.Buffer
	qw := quotedprintable.NewWriter(&qp)
	_, _ = qw.Write([]byte(body))
	_ = qw.Close()
	b.Write(qp.Bytes())
	return []byte(b.String())
}
