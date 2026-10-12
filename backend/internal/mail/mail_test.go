package mail

import (
	"strings"
	"testing"
	"time"
)

func TestNewSelectsMailer(t *testing.T) {
	if m := New(Config{}); m.Enabled() {
		t.Fatal("no SMTP config must yield the no-op mailer")
	}
	if m := New(Config{Host: "smtp.example.com"}); m.Enabled() {
		t.Fatal("Host without From must still be no-op")
	}
	if m := New(Config{Host: "smtp.example.com", From: "t@example.com"}); !m.Enabled() {
		t.Fatal("Host+From must yield the SMTP mailer")
	}
}

func TestNoopSendAndAsync(t *testing.T) {
	m := New(Config{})
	if err := m.Send("user@example.com", "Subject", "body"); err != nil {
		t.Fatalf("noop send: %v", err)
	}
	// SendAsync must not panic or block; give the goroutine a beat to run.
	SendAsync(m, "user@example.com", "Subject", "body")
	time.Sleep(10 * time.Millisecond)
}

func TestBuildMessage(t *testing.T) {
	msg := string(buildMessage("from@example.com", "to@example.com", "Тема письма", "Привет, мир"))
	for _, want := range []string{
		"From: ", "<from@example.com>", "To: to@example.com",
		"Subject: =?utf-8?q?", "Message-ID: <", "@example.com>",
		"Content-Transfer-Encoding: quoted-printable",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message misses %q:\n%s", want, msg)
		}
	}
	if !strings.HasSuffix(msg, "\r\n") && !strings.Contains(msg, "=D0") {
		t.Errorf("body not quoted-printable encoded:\n%s", msg)
	}
}

// A line break in the recipient would end the To header and start one of the
// sender's choosing; it is refused before any connection is attempted (the host
// below does not exist — reaching the dial would fail differently).
func TestSendRefusesLineBreakInRecipient(t *testing.T) {
	m := &smtpMailer{cfg: Config{Host: "smtp.invalid", Port: "587", From: "from@example.com"}}
	for _, to := range []string{"a@example.com\r\nBcc: x@example.com", "a@example.com\nBcc: x@example.com"} {
		err := m.Send(to, "s", "b")
		if err == nil || !strings.Contains(err.Error(), "line break") {
			t.Errorf("Send(%q) err = %v; want the line-break refusal", to, err)
		}
	}
}

// The Subject is RFC 2047-encoded whenever it holds a control character, so a
// CRLF in it can never open a header line of its own.
func TestBuildMessageSubjectCannotAddHeader(t *testing.T) {
	msg := string(buildMessage("from@example.com", "to@example.com", "Hi\r\nBcc: x@example.com", "body"))
	if strings.Contains(msg, "\r\nBcc:") {
		t.Fatalf("subject injected a header:\n%s", msg)
	}
}
