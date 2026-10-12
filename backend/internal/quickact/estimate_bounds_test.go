package quickact

import (
	"math"
	"strings"
	"testing"
)

// The estimate parser used to have two silent ways of handing back a number no
// column can hold — "nan"/"inf" spelled out on a points board, and float64
// overflow on a time one. Both returned a nil error, and both ended up clearing
// the task's estimate instead of refusing the command (#2932).

func TestParseEstimateRefusesNonFinitePoints(t *testing.T) {
	cfg := Estimation{Unit: "points", PointsScale: "linear"}
	// strconv.ParseFloat accepts every one of these as a float literal.
	for _, arg := range []string{"nan", "NaN", "inf", "Inf", "infinity", "+inf", "-inf"} {
		v, err := ParseEstimate(arg, cfg)
		if err == nil {
			t.Errorf("ParseEstimate(%q) = %v, want an error", arg, v)
		}
		if v != 0 {
			t.Errorf("ParseEstimate(%q) returned %v alongside its error, want 0", arg, v)
		}
	}
}

func TestParseEstimateRefusesOverflowingTime(t *testing.T) {
	// 3e306 hours: the number itself parses, but ×60 leaves float64 range.
	arg := "3" + strings.Repeat("0", 306) + "h"
	v, err := ParseEstimate(arg, Estimation{})
	if err == nil {
		t.Fatalf("ParseEstimate(3e306h) = %v, want an error", v)
	}
	if math.IsInf(v, 0) || math.IsNaN(v) {
		t.Fatalf("ParseEstimate(3e306h) = %v, want a finite zero", v)
	}
}

func TestParseEstimateCeiling(t *testing.T) {
	// A value exactly on the ceiling is still an estimate; one minute past it is
	// not. Checked in minutes so the bound does not depend on hours_per_day.
	atCap := "1000000000m" // MaxEstimate
	if v, err := ParseEstimate(atCap, Estimation{}); err != nil || v != MaxEstimate {
		t.Errorf("ParseEstimate(%s) = %v, %v; want %v, nil", atCap, v, err, MaxEstimate)
	}
	overCap := "1000000001m"
	if v, err := ParseEstimate(overCap, Estimation{}); err == nil {
		t.Errorf("ParseEstimate(%s) = %v, want an error", overCap, v)
	}
	// Points count something else, but the same ceiling applies to the stored
	// number — a billion story points is not an estimate either.
	pts := Estimation{Unit: "points", PointsScale: "linear"}
	if v, err := ParseEstimate("1000000001", pts); err == nil {
		t.Errorf("ParseEstimate(1000000001 points) = %v, want an error", v)
	}
}

func TestParseEstimateStillAcceptsOrdinaryValues(t *testing.T) {
	cases := []struct {
		arg  string
		cfg  Estimation
		want float64
	}{
		{"2h", Estimation{}, 120},
		{"1ч 30", Estimation{}, 90},
		{"1d", Estimation{}, 480},
		{"1w", Estimation{}, 2400},
		{"3", Estimation{}, 180},
		{"5", Estimation{Unit: "points", PointsScale: "linear"}, 5},
		{"m", Estimation{Unit: "points", PointsScale: "tshirt"}, 3},
	}
	for _, tc := range cases {
		got, err := ParseEstimate(tc.arg, tc.cfg)
		if err != nil || got != tc.want {
			t.Errorf("ParseEstimate(%q) = %v, %v; want %v, nil", tc.arg, got, err, tc.want)
		}
	}
}
