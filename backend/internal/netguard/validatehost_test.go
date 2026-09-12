package netguard

import (
	"context"
	"testing"
	"time"
)

// TestValidateHost covers the check used where a guarded dialer cannot be
// installed: literal addresses judged directly, a name resolved before the
// verdict ("localhost" is loopback), and the opt-out.
func TestValidateHost(t *testing.T) {
	cases := []struct {
		host         string
		allowPrivate bool
		wantErr      bool
	}{
		{"127.0.0.1", false, true},
		{"169.254.169.254", false, true}, // cloud metadata
		{"10.1.2.3", false, true},
		{"::1", false, true},
		{"localhost", false, true}, // resolves to loopback
		{"8.8.8.8", false, false},
		{"127.0.0.1", true, false}, // opted in
		{"", false, false},         // nothing to judge
	}
	for _, tc := range cases {
		err := ValidateHost(context.Background(), tc.host, tc.allowPrivate)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateHost(%q, allowPrivate=%v) = %v, wantErr=%v",
				tc.host, tc.allowPrivate, err, tc.wantErr)
		}
	}
}

// TestValidateHostAllowsUnresolvable pins the placeholder-host case: a service
// DSL like telegram://token@telegram names a host no resolver can answer, and
// that must be allowed rather than turned into a permanent delivery failure —
// promptly, without waiting out a slow resolver.
func TestValidateHostAllowsUnresolvable(t *testing.T) {
	old := lookupTimeout
	lookupTimeout = 50 * time.Millisecond
	defer func() { lookupTimeout = old }()

	start := time.Now()
	for _, host := range []string{"telegram", "channel", "this-name-does-not-exist.invalid"} {
		if err := ValidateHost(context.Background(), host, false); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", host, err)
		}
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Errorf("three unresolvable hosts took %s — the lookup is not bounded", el)
	}
}
