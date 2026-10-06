package handlers

import (
	"math"
	"testing"
	"time"
)

// The recording length comes from the SFU, and the DB column is int32 seconds.
// A garbage length must clamp rather than wrap into a negative duration.
func TestDurationSec(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want int32
	}{
		{"zero", 0, 0},
		{"truncates to whole seconds", 90500 * time.Millisecond, 90},
		{"negative clamps to zero", -5 * time.Second, 0},
		{"overflow clamps to MaxInt32", math.MaxInt64, math.MaxInt32},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := durationSec(tc.in); got != tc.want {
				t.Errorf("durationSec(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
