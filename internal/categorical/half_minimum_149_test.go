package categorical

import (
	"math"
	"testing"
)

type HalfMinimum149Case struct {
	name          string
	logitsBits    []uint64
	target        int
	factorBits    uint64
	divisorBits   uint64
	publicDistill bool
	lossBits      uint64
	gradientBits  []uint64
}

func HalfMinimum149ULPDistance(got, want uint64) uint64 {
	if got >= want {
		return got - want
	}
	return want - got
}

func CheckHalfMinimum149Loss(t *testing.T, name string, got float64, wantBits uint64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s: nonfinite loss %v", name, got)
	}
	gotBits := math.Float64bits(got)
	if wantBits == 0 {
		if got != 0 {
			t.Errorf("%s: loss should be zero, got %016x", name, gotBits)
		}
		return
	}
	want := math.Float64frombits(wantBits)
	if got == 0 {
		t.Errorf("%s: positive loss became zero, want %016x", name, wantBits)
		return
	}
	if math.Signbit(got) != math.Signbit(want) {
		t.Errorf("%s: loss sign changed, got %016x want %016x", name, gotBits, wantBits)
		return
	}
	if distance := HalfMinimum149ULPDistance(gotBits, wantBits); distance > 4 {
		t.Errorf("%s: loss got %016x want %016x distance %d", name, gotBits, wantBits, distance)
	}
}

func CheckHalfMinimum149Gradient(t *testing.T, name string, got []float64, wantBits []uint64) {
	t.Helper()
	if len(got) != len(wantBits) {
		t.Fatalf("%s: gradient width got %d want %d", name, len(got), len(wantBits))
	}
	for i, wantBits := range wantBits {
		value := got[i]
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Errorf("%s: gradient[%d] nonfinite %v", name, i, value)
			continue
		}
		gotBits := math.Float64bits(value)
		if math.Float64frombits(wantBits) == 0 {
			if value != 0 {
				t.Errorf("%s: gradient[%d] should be zero, got %016x", name, i, gotBits)
			}
			continue
		}
		want := math.Float64frombits(wantBits)
		if value == 0 {
			t.Errorf("%s: gradient[%d] became zero, want %016x", name, i, wantBits)
			continue
		}
		if math.Signbit(value) != math.Signbit(want) {
			t.Errorf("%s: gradient[%d] sign changed, got %016x want %016x", name, i, gotBits, wantBits)
			continue
		}
		if distance := HalfMinimum149ULPDistance(gotBits, wantBits); distance > 4 {
			t.Errorf("%s: gradient[%d] got %016x want %016x distance %d", name, i, gotBits, wantBits, distance)
		}
	}
}

func TestHalfMinimumCategorical149(t *testing.T) {
	cases := []HalfMinimum149Case{
		{name: "unit_-3_first", logitsBits: []uint64{0xbd104e7ce35362a1, 0xc0874910d52d3052}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "unit_-3_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce35362a1}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "unit_-2_first", logitsBits: []uint64{0xbd104e7ce35362a0, 0xc0874910d52d3052}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "unit_-2_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce35362a0}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "unit_-1_first", logitsBits: []uint64{0xbd104e7ce353629f, 0xc0874910d52d3052}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "unit_-1_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629f}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "unit_+0_first", logitsBits: []uint64{0xbd104e7ce353629e, 0xc0874910d52d3052}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "unit_+0_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629e}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "unit_+1_first", logitsBits: []uint64{0xbd104e7ce353629d, 0xc0874910d52d3052}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "unit_+1_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629d}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "unit_+2_first", logitsBits: []uint64{0xbd104e7ce353629c, 0xc0874910d52d3052}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "unit_+2_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629c}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "unit_+3_first", logitsBits: []uint64{0xbd104e7ce353629b, 0xc0874910d52d3052}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "unit_+3_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629b}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "factor2_-1_first", logitsBits: []uint64{0x3d26cc19221db5e0, 0xc0874e9c65eceee0}, target: 0, factorBits: 0x4000000000000000, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "factor2_-1_last", logitsBits: []uint64{0xc0874e9c65eceee0, 0x3d26cc19221db5e0}, target: 1, factorBits: 0x4000000000000000, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "factor2_+0_first", logitsBits: []uint64{0x3d26cc19221db5e1, 0xc0874e9c65eceee0}, target: 0, factorBits: 0x4000000000000000, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "factor2_+0_last", logitsBits: []uint64{0xc0874e9c65eceee0, 0x3d26cc19221db5e1}, target: 1, factorBits: 0x4000000000000000, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "factor2_+1_first", logitsBits: []uint64{0x3d26cc19221db5e2, 0xc0874e9c65eceee0}, target: 0, factorBits: 0x4000000000000000, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "factor2_+1_last", logitsBits: []uint64{0xc0874e9c65eceee0, 0x3d26cc19221db5e2}, target: 1, factorBits: 0x4000000000000000, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "divisor2_-1_first", logitsBits: []uint64{0x3d28e569fa8ee780, 0xc0874385446d71c3}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x4000000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "divisor2_-1_last", logitsBits: []uint64{0xc0874385446d71c3, 0x3d28e569fa8ee780}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x4000000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "divisor2_+0_first", logitsBits: []uint64{0x3d28e569fa8ee781, 0xc0874385446d71c3}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x4000000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "divisor2_+0_last", logitsBits: []uint64{0xc0874385446d71c3, 0x3d28e569fa8ee781}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x4000000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "divisor2_+1_first", logitsBits: []uint64{0x3d28e569fa8ee782, 0xc0874385446d71c3}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x4000000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "divisor2_+1_last", logitsBits: []uint64{0xc0874385446d71c3, 0x3d28e569fa8ee782}, target: 1, factorBits: 0x3ff0000000000000, divisorBits: 0x4000000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "max_ratio_-1_first", logitsBits: []uint64{0xbd104e7ce353629f, 0xc0874910d52d3052}, target: 0, factorBits: 0x7fefffffffffffff, divisorBits: 0x7fefffffffffffff, publicDistill: false, lossBits: 0x3cbfffffffffffff, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "max_ratio_-1_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629f}, target: 1, factorBits: 0x7fefffffffffffff, divisorBits: 0x7fefffffffffffff, publicDistill: false, lossBits: 0x3cbfffffffffffff, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "max_ratio_+0_first", logitsBits: []uint64{0xbd104e7ce353629e, 0xc0874910d52d3052}, target: 0, factorBits: 0x7fefffffffffffff, divisorBits: 0x7fefffffffffffff, publicDistill: false, lossBits: 0x3cbfffffffffffff, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "max_ratio_+0_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629e}, target: 1, factorBits: 0x7fefffffffffffff, divisorBits: 0x7fefffffffffffff, publicDistill: false, lossBits: 0x3cbfffffffffffff, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "max_ratio_+1_first", logitsBits: []uint64{0xbd104e7ce353629d, 0xc0874910d52d3052}, target: 0, factorBits: 0x7fefffffffffffff, divisorBits: 0x7fefffffffffffff, publicDistill: false, lossBits: 0x3cbfffffffffffff, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "max_ratio_+1_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629d}, target: 1, factorBits: 0x7fefffffffffffff, divisorBits: 0x7fefffffffffffff, publicDistill: false, lossBits: 0x3cbfffffffffffff, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "aggregate2_-1_first", logitsBits: []uint64{0x3d26cc19221db5e0, 0xc0874e9c65eceee0, 0xc0874e9c65eceee0}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000000, 0x0000000000000000}},
		{name: "aggregate2_-1_last", logitsBits: []uint64{0xc0874e9c65eceee0, 0xc0874e9c65eceee0, 0x3d26cc19221db5e0}, target: 2, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x8000000000000001}},
		{name: "aggregate2_+0_first", logitsBits: []uint64{0x3d26cc19221db5e1, 0xc0874e9c65eceee0, 0xc0874e9c65eceee0}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000000, 0x0000000000000000}},
		{name: "aggregate2_+0_last", logitsBits: []uint64{0xc0874e9c65eceee0, 0xc0874e9c65eceee0, 0x3d26cc19221db5e1}, target: 2, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x8000000000000001}},
		{name: "aggregate2_+1_first", logitsBits: []uint64{0x3d26cc19221db5e2, 0xc0874e9c65eceee0, 0xc0874e9c65eceee0}, target: 0, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000, 0x0000000000000000}},
		{name: "aggregate2_+1_last", logitsBits: []uint64{0xc0874e9c65eceee0, 0xc0874e9c65eceee0, 0x3d26cc19221db5e2}, target: 2, factorBits: 0x3ff0000000000000, divisorBits: 0x3ff0000000000000, publicDistill: true, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x8000000000000000}},
		{name: "minimum_factor_target0_first", logitsBits: []uint64{0x0000000000000000, 0x8000000000000001}, target: 0, factorBits: 0x0000000000000001, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "minimum_factor_target0_last", logitsBits: []uint64{0x8000000000000001, 0x0000000000000000}, target: 1, factorBits: 0x0000000000000001, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "minimum_factor_target1_first", logitsBits: []uint64{0x0000000000000000, 0x8000000000000001}, target: 1, factorBits: 0x0000000000000001, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "minimum_factor_target1_last", logitsBits: []uint64{0x8000000000000001, 0x0000000000000000}, target: 0, factorBits: 0x0000000000000001, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "exact_half_tie_target0", logitsBits: []uint64{0x0000000000000000, 0x0000000000000000}, target: 0, factorBits: 0x0000000000000001, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "exact_half_tie_target1", logitsBits: []uint64{0x0000000000000000, 0x0000000000000000}, target: 1, factorBits: 0x0000000000000001, divisorBits: 0x3ff0000000000000, publicDistill: false, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
	}
	if len(cases) != 44 {
		t.Fatalf("half-minimum case count got %d want 44", len(cases))
	}
	publicCount := 0
	for _, tc := range cases {
		if tc.publicDistill {
			publicCount++
		}
	}
	if publicCount != 20 {
		t.Fatalf("half-minimum public case count got %d want 20", publicCount)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.logitsBits) != len(tc.gradientBits) {
				t.Fatalf("oracle width got logits %d gradient %d", len(tc.logitsBits), len(tc.gradientBits))
			}
			logits := make([]float64, len(tc.logitsBits))
			for i, bits := range tc.logitsBits {
				logits[i] = math.Float64frombits(bits)
			}
			loss, err := Loss(logits, tc.target, math.Float64frombits(tc.factorBits))
			if err != nil {
				t.Fatalf("loss: %v", err)
			}
			dst := make([]float64, len(logits))
			if len(dst) != len(tc.gradientBits) {
				t.Fatalf("dst width got %d want %d", len(dst), len(tc.gradientBits))
			}
			if err := Gradient(dst, logits, tc.target, math.Float64frombits(tc.factorBits), math.Float64frombits(tc.divisorBits)); err != nil {
				t.Fatalf("gradient: %v", err)
			}
			CheckHalfMinimum149Loss(t, tc.name, loss, tc.lossBits)
			CheckHalfMinimum149Gradient(t, tc.name, dst, tc.gradientBits)
			for i, bits := range tc.logitsBits {
				if got := math.Float64bits(logits[i]); got != bits {
					t.Errorf("input logits[%d] changed got %016x want %016x", i, got, bits)
				}
			}
		})
	}
}
