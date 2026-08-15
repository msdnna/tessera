package converter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDisabledClientDoesNotCallOut(t *testing.T) {
	c := New("")
	if c.Enabled() {
		t.Fatal("client with no URL reports itself enabled")
	}
	if _, err := c.Convert(context.Background(), []byte("x"), "docx", "html"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Convert on a disabled client = %v, want ErrDisabled", err)
	}
	if _, err := c.Health(context.Background()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Health on a disabled client = %v, want ErrDisabled", err)
	}
}

func TestConvertPassesFormatsAndBody(t *testing.T) {
	var gotFrom, gotTo string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFrom = r.URL.Query().Get("from")
		gotTo = r.URL.Query().Get("to")
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()

	out, err := New(srv.URL+"/").Convert(context.Background(), []byte("source"), ".DOCX", "HTML")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if string(out) != "<html>ok</html>" {
		t.Fatalf("body = %q", out)
	}
	// The dot and the case come from a filename extension at the call site; the
	// sidecar matches on a bare lowercase token, so normalising here is what
	// keeps ".DOCX" from being reported as an unsupported format.
	if gotFrom != "docx" || gotTo != "html" {
		t.Fatalf("from=%q to=%q, want docx/html", gotFrom, gotTo)
	}
	if string(gotBody) != "source" {
		t.Fatalf("sidecar received %q", gotBody)
	}
}

func TestConvertSurfacesSidecarRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"LibreOffice produced no output"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL).Convert(context.Background(), []byte("junk"), "docx", "html")
	var convErr *Error
	if !errors.As(err, &convErr) {
		t.Fatalf("err = %v, want *converter.Error", err)
	}
	if convErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", convErr.Status)
	}
	// The message has to survive: the handler turns a 4xx into "could not
	// convert this file" and shows the reason, and a generic string there would
	// leave the user with no idea whether to try a different file.
	if convErr.Message != "LibreOffice produced no output" {
		t.Fatalf("message = %q", convErr.Message)
	}
}

func TestConvertRefusesEmptySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// A 200 with no bytes would otherwise be handed to the user as a zero-length
	// .pdf — a download that looks like it worked and opens as a corrupt file.
	if _, err := New(srv.URL).Convert(context.Background(), []byte("x"), "html", "pdf"); err == nil {
		t.Fatal("empty 200 accepted as a successful conversion")
	}
}

func TestConvertRejectsEmptyInputWithoutCallingOut(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	if _, err := New(srv.URL).Convert(context.Background(), nil, "docx", "html"); err == nil {
		t.Fatal("empty source accepted")
	}
	if called {
		t.Fatal("empty source was still sent to the sidecar")
	}
}

func TestHealthReportsFormats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("Health hit %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"sources":["docx","odt"],"targets":["html","pdf"]}`))
	}))
	defer srv.Close()

	info, err := New(srv.URL).Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !info.OK || len(info.Targets) != 2 {
		t.Fatalf("info = %+v", info)
	}
}

func TestHealthOnUnreachableSidecarFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close()

	if _, err := New(url).Health(context.Background()); err == nil {
		t.Fatal("Health against a closed sidecar reported success")
	}
}

func TestSidecarMessageFallsBackToRawBody(t *testing.T) {
	if got := sidecarMessage([]byte("plain failure")); got != "plain failure" {
		t.Fatalf("got %q", got)
	}
	if got := sidecarMessage(nil); got != "conversion failed" {
		t.Fatalf("empty body message = %q", got)
	}
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'a'
	}
	if got := sidecarMessage(long); len([]rune(got)) != 301 {
		t.Fatalf("long message not trimmed: %d runes", len([]rune(got)))
	}
}
