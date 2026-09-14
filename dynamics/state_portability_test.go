package dynamics_test

import (
	"context"
	"github.com/TimLai666/coimnet/dynamics"
	"math"
	"reflect"
	"testing"
)

func TestContinuousStateAcceptsBoundedCPURoundingWithoutRewritingHistory(t *testing.T) {
	// Values reproduced by loading a darwin/arm64 snapshot on linux/amd64,
	// both Go 1.26.5. The two math kernels differed by one or two ULPs.
	for _, tc := range []struct {
		activation string
		voltage    float64
	}{{"tanh", -2.6662666266626651}, {"softplus", -19.975997599759975}} {
		t.Run(tc.activation, func(t *testing.T) {
			m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{1}, DT: .5, Activation: tc.activation})
			if err != nil {
				t.Fatal(err)
			}
			for _, direction := range []float64{math.Inf(-1), math.Inf(1)} {
				s, err := m.NewState([]float64{tc.voltage})
				if err != nil {
					t.Fatal(err)
				}
				for range 4 {
					s.History[0][0] = math.Nextafter(s.History[0][0], direction)
				}
				original := cloneStateForTest(s)
				if err = m.ValidateState(s); err != nil {
					t.Fatalf("rejected CPU rounding: %v", err)
				}
				if !reflect.DeepEqual(s, original) {
					t.Fatal("validation rewrote persisted history")
				}
				next, _, err := m.Advance(context.Background(), dynamics.Parameters{Weights: []float64{.1}, Bias: []float64{0}, LogTau: []float64{0}}, s, [][]float64{{0}})
				if err != nil {
					t.Fatal(err)
				}
				if next.History[0][0] != original.History[0][0] {
					t.Fatal("advancing replaced historical output")
				}
				s.History[0][0] = math.Nextafter(s.History[0][0], direction)
				if err = m.ValidateState(s); err == nil {
					t.Fatal("accepted history beyond declared four-ULP comparison")
				}
			}
		})
	}
}
