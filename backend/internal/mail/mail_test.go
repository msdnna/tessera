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
