package experiment

import (
	"math"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

func thresholdTailCheckReportFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s=%v, want finite %v", name, got, want)
	}
	if want == 0 {
		if got != 0 {
			t.Fatalf("%s=%g, want exactly zero", name, got)
		}
		return
	}
	if math.Abs((got-want)/want) > 1e-12 {
		t.Fatalf("%s=%g, want %g", name, got, want)
	}
}

func TestThresholdTailThetaBases(t *testing.T) {
	a := math.Ldexp(1, 1023)
	cases := []struct {
		name                  string
		minimum, maximum, raw float64
		want                  float64
	}{
		{name: "normal", maximum: 2, raw: 0, want: 1},
		{name: "positive-tail", maximum: 2, raw: 40, want: 2},
		{name: "negative-tail", maximum: 1e308, raw: -750, want: 1.9016849634750064e-18},
		{name: "negative-tail-underflow", maximum: 1e308, raw: -1000, want: 5.075958897549457e-127},
		{name: "wide-positive-tail", maximum: 1e308, raw: 740, want: 1e308},
		{name: "wide-negative-tail", maximum: 1e308, raw: -740, want: 4.188739880048049e-14},
		{name: "overflow-span-zero", minimum: -a, maximum: a, raw: 0, want: 0},
		{name: "overflow-span-positive", minimum: -a, maximum: a, raw: 40, want: a},
		{name: "overflow-span-negative", minimum: -a, maximum: a, raw: -40, want: -a},
		{name: "forward-cancellation", minimum: -1e308, raw: 40, want: -4.248354255291589e290},
		{name: "close-endpoints", minimum: 1e308, maximum: math.Nextafter(1e308, math.Inf(1)), raw: 40, want: math.Nextafter(1e308, math.Inf(1))},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			values := thetaBases([]float64{tc.raw}, dynamics.LIFConfig{
				Nodes: 1, ThetaMin: tc.minimum, ThetaMax: tc.maximum,
			})
			if len(values) != 1 {
				t.Fatalf("thetaBases length=%d, want 1", len(values))
			}
			thresholdTailCheckReportFloat(t, "theta_base", values[0], tc.want)
		})
	}
}
