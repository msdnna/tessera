package mail

import (
	"testing"
	"time"
)

// panickingMailer stands in for an SMTP transport that dies on a nil dereference
// mid-exchange. What it panics with doesn't matter; that it panics off the
// request goroutine does.
type panickingMailer struct{ started chan struct{} }

func (panickingMailer) Enabled() bool { return true }

func (m panickingMailer) Send(_, _, _ string) error {
	close(m.started)
	panic("smtp transport blew up")
}

// SendAsync must not let a panicking mailer take the process with it. The
// verification / invite / reset mails all go out through it, detached from the
// request that triggered them — so gin's Recovery is not on that stack and a
// panic there is a process death, not a failed email.
//
// The failure mode is why this test asserts nothing about the panic itself: an
// unrecovered panic in a goroutine kills the whole test binary, so without the
// guard this file's package fails to report at all.
func TestSendAsyncSurvivesPanickingMailer(t *testing.T) {
	m := panickingMailer{started: make(chan struct{})}
	SendAsync(m, "someone@example.com", "verify", "link")

	select {
	case <-m.started:
	case <-time.After(2 * time.Second):
		t.Fatal("SendAsync never called the mailer")
	}
	// Give the panic a moment to land: the goroutine is still unwinding when Send
	// returns control, and a test that ended here would pass before the crash.
	time.Sleep(100 * time.Millisecond)
}
