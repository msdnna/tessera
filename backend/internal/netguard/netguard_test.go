package netguard

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestAllowedPublic covers the address classes an SSRF probe tries: the cloud
// metadata endpoint, loopback, every RFC 1918 range, ULA, link-local v4/v6,
// unspecified, multicast, and CGNAT — plus a real public v4 and v6.
func TestAllowedPublic(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"93.184.216.34", true},
		{"2606:4700:4700::1111", true}, // public IPv6

		{"127.0.0.1", false},            // loopback
		{"::1", false},                  // loopback v6
		{"10.0.0.1", false},             // private 10/8
		{"172.16.0.1", false},           // private 172.16/12
		{"172.31.255.255", false},       // private 172.16/12 upper edge
		{"192.168.1.1", false},          // private 192.168/16
		{"169.254.169.254", false},      // link-local → cloud metadata
		{"fe80::1", false},              // link-local v6
		{"fc00::1", false},              // IPv6 ULA (private)
		{"0.0.0.0", false},              // unspecified
		{"::", false},                   // unspecified v6
		{"224.0.0.1", false},            // multicast
		{"100.64.0.1", false},           // CGNAT 100.64/10 lower edge
		{"100.127.255.255", false},      // CGNAT 100.64/10 upper edge
		{"100.63.255.255", true},        // just below CGNAT — normal public ARIN space
		{"100.128.0.1", true},           // just above CGNAT — normal public space
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("ParseIP(%q) = nil", tc.ip)
			}
			if got := allowedPublic(ip); got != tc.want {
				t.Fatalf("allowedPublic(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

// TestControl exercises the dial hook directly: blocked addresses return
// ErrBlocked, public ones pass, and Control(true) is a nil no-op.
func TestControl(t *testing.T) {
	if Control(true) != nil {
		t.Fatal("Control(true) should be nil (a no-op)")
	}
	ctrl := Control(false)
	if ctrl == nil {
		t.Fatal("Control(false) returned nil — would block nothing")
	}

	blocked := []string{
		"127.0.0.1:80", "[::1]:443", "10.1.2.3:80", "192.168.0.1:80",
		"169.254.169.254:80", "0.0.0.0:80", "100.64.0.1:80",
	}
	for _, addr := range blocked {
		if err := ctrl("tcp", addr, nil); !errors.Is(err, ErrBlocked) {
			t.Fatalf("Control(%q) = %v, want ErrBlocked", addr, err)
		}
	}

	for _, addr := range []string{"93.184.216.34:443", "[2606:4700:4700::1111]:443"} {
		if err := ctrl("tcp", addr, nil); err != nil {
			t.Fatalf("Control(%q) = %v, want nil", addr, err)
		}
	}

	// A unix socket address is not host:port → passed through unchanged.
	if err := ctrl("unix", "/tmp/x.sock", nil); err != nil {
		t.Fatalf("Control on a non-IP network should be a no-op, got %v", err)
	}
}

// TestValidateURL pins the input-hygiene layer: scheme, userinfo, host, and the
// literal-private-IP early reject (which only applies when private targets are
// disallowed).
func TestValidateURL(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		allowPrivate bool
		wantErr      bool
	}{
		{"public https", "https://example.com/hook", false, false},
		{"public http", "http://example.com", false, false},
		{"public with port", "http://example.com:8080/x", false, false},
		{"public ipv6 host", "https://[2606:4700:4700::1111]/x", false, false},
		{"private allowed", "http://10.0.0.1/x", true, false},
		{"loopback allowed", "http://127.0.0.1:8080/x", true, false},
		{"private blocked", "http://10.0.0.1/x", false, true},
		{"loopback blocked", "http://127.0.0.1/x", false, true},
		{"metadata blocked", "http://169.254.169.254/latest/meta-data/", false, true},
		{"cgnat blocked", "http://100.64.0.1/x", false, true},
		{"file scheme", "file:///etc/passwd", false, true},
		{"javascript scheme", "javascript:alert(1)", false, true},
		{"gopher scheme", "gopher://x/", false, true},
		{"userinfo", "https://user:pass@example.com", false, true},
		{"missing host", "https:///path", false, true},
		{"garbage", "://nope", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateURL(tc.raw, tc.allowPrivate)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateURL(%q, allow=%v) err = %v, wantErr %v", tc.raw, tc.allowPrivate, err, tc.wantErr)
			}
		})
	}
}

// TestDialerBlocksLoopback is the integration proof: a client built with the
// guarded transport cannot reach a local (loopback) server, but can once private
// targets are allowed.
func TestDialerBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// srv.URL is http://127.0.0.1:<port> — a private target.

	reach := func(allowPrivate bool) error {
		d := Dialer(allowPrivate)
		d.Timeout = 2 * time.Second
		cl := &http.Client{Transport: &http.Transport{DialContext: d.DialContext}}
		resp, err := cl.Get(srv.URL)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		return nil
	}

	if err := reach(false); err == nil {
		t.Fatal("guarded client reached a loopback target (allowPrivate=false) — SSRF not blocked")
	}
	if err := reach(true); err != nil {
		t.Fatalf("client with allowPrivate=true should reach the loopback target, got %v", err)
	}
}
