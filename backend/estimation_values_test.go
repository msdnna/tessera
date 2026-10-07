package main

import (
	"net/http"
	"testing"
)

// The picker's quick picks ride in the estimation config (#3029): kept positive,
// unique and sorted.
func TestEstimationValues(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	p := c.expect(t, c.put("/projects/"+s.Project+"/estimation",
		map[string]any{"unit": "time", "values": []any{480, 60, 60, -5, 120}}), http.StatusOK)
	est := p["estimation"].(map[string]any)
	vals := est["values"].([]any)
	if len(vals) != 3 || vals[0] != float64(60) || vals[2] != float64(480) {
		t.Fatalf("values = %v", vals)
	}
}
