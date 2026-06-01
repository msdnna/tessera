package handlers

import "testing"

func f(v float64) *float64 { return &v }

func TestPositionBetween(t *testing.T) {
	cases := []struct {
		name       string
		prev, next *float64
		want       float64
	}{
		{"empty list", nil, nil, positionGap},
		{"append to end", f(100), nil, 100 + positionGap},
		{"prepend to top", nil, f(100), 50},
		{"between two", f(100), f(200), 150},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := positionBetween(c.prev, c.next); got != c.want {
				t.Fatalf("positionBetween: got %v want %v", got, c.want)
			}
		})
	}
}

// Inserting between two adjacent items must yield a strictly-between position
// so ordering is preserved without renumbering.
func TestPositionBetweenStrictlyBetween(t *testing.T) {
	a, b := 1.0, 2.0
	mid := positionBetween(&a, &b)
	if !(mid > a && mid < b) {
		t.Fatalf("midpoint %v not strictly between %v and %v", mid, a, b)
	}
}
