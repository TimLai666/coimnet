package categorical

import (
	"fmt"
	"math"
	"testing"
)

// Reference bits are generated independently in
// categorical-gradient-reference.json with Decimal precision 800. No
// expected value in this test is produced by Gradient.
type categoricalGradientCase147 struct {
	name         string
	classes      int
	winner       int
	target       int
	head         float64
	tail         float64
	explicit     []float64
	factor       float64
	divisor      float64
	maxULP       uint64
	targetBits   uint64
	winnerBits   uint64
	otherBits    uint64
	explicitBits []uint64
}

var categoricalGradientCases147 = []categoricalGradientCase147{
	{
		name: "uniform3_factor1_winner0_target0", classes: 3, winner: 0, target: 0, head: 0, tail: -40,
		factor: 1, divisor: 1, maxULP: 4,
		targetBits: 0xbc639792499b1a24, winnerBits: 0xbc639792499b1a24, otherBits: 0x3c539792499b1a24,
	},
	{
		name: "uniform3_factor1_winner1_target2", classes: 3, winner: 1, target: 2, head: 0, tail: -40,
		factor: 1, divisor: 1, maxULP: 4,
		targetBits: 0xbff0000000000000, winnerBits: 0x3ff0000000000000, otherBits: 0x3c539792499b1a24,
	},
	{
		name: "uniform259_factor1_winner0_target258", classes: 259, winner: 0, target: 258, head: 0, tail: -40,
		factor: 1, divisor: 1, maxULP: 4,
		targetBits: 0xbff0000000000000, winnerBits: 0x3feffffffffffff6, otherBits: 0x3c539792499b1a1e,
	},
	{
		name: "uniform259_factor1_divisor3_winner129_target0", classes: 259, winner: 129, target: 0, head: 0, tail: -40,
		factor: 1, divisor: 3, maxULP: 8,
		targetBits: 0xbfd5555555555555, winnerBits: 0x3fd555555555554f, otherBits: 0x3c3a1f6db7797828,
	},
	{
		name: "single_class_factor3", classes: 1, winner: 0, target: 0, head: 17, tail: 17,
		factor: 3, divisor: 1, maxULP: 8,
		targetBits: 0x0000000000000000, winnerBits: 0x0000000000000000, otherBits: 0x0000000000000000,
	},
	{
		name: "tie3_target1", classes: 3, winner: 0, target: 1, head: 0, tail: 0,
		factor: 1, divisor: 1, maxULP: 4,
		targetBits: 0xbfe5555555555555, winnerBits: 0x3fd5555555555555, otherBits: 0x3fd5555555555555,
	},
	{
		name: "tie259_divisor3_target258", classes: 259, winner: 0, target: 258, head: 0, tail: 0,
		factor: 1, divisor: 3, maxULP: 8,
		targetBits: 0xbfd5403f42395404, winnerBits: 0x3f5516131c015161, otherBits: 0x3f5516131c015161,
	},
	{
		name: "media3_mean2_winner0_target0", classes: 3, winner: 0, target: 0, head: 0, tail: -40,
		factor: 1, divisor: 2, maxULP: 8,
		targetBits: 0xbc539792499b1a24, winnerBits: 0xbc539792499b1a24, otherBits: 0x3c439792499b1a24,
	},
	{
		name: "subnormal740_target_winner", classes: 3, winner: 0, target: 0, head: 0, tail: -740,
		factor: 1, divisor: 1, maxULP: 4,
		targetBits: 0x80000000000000aa, winnerBits: 0x80000000000000aa, otherBits: 0x0000000000000055,
	},
	{
		name: "subnormal744_target_nonwinner", classes: 3, winner: 2, target: 0, head: 0, tail: -744,
		factor: 1, divisor: 1, maxULP: 4,
		targetBits: 0xbff0000000000000, winnerBits: 0x3ff0000000000000, otherBits: 0x0000000000000002,
	},
	{
		name: "subnormal745_target_winner", classes: 3, winner: 1, target: 1, head: 0, tail: -745,
		factor: 1, divisor: 1, maxULP: 4,
		targetBits: 0x8000000000000001, winnerBits: 0x8000000000000001, otherBits: 0x0000000000000001,
	},
	{
		name: "subnormal746_target_winner", classes: 3, winner: 0, target: 0, head: 0, tail: -746,
		factor: 1, divisor: 1, maxULP: 4,
		targetBits: 0x8000000000000000, winnerBits: 0x8000000000000000, otherBits: 0x0000000000000000,
	},
	{
		name: "subnormal750_target_winner", classes: 3, winner: 0, target: 0, head: 0, tail: -750,
		factor: 1, divisor: 1, maxULP: 4,
		targetBits: 0x8000000000000000, winnerBits: 0x8000000000000000, otherBits: 0x0000000000000000,
	},
	{
		name: "mixed_two_max_743_65_target_max", explicit: []float64{0, 0, -743.65}, target: 0,
		factor: 1, divisor: 1, maxULP: 4,
		explicitBits: []uint64{0xbfe0000000000000, 0x3fe0000000000000, 0x0000000000000001},
	},
	{
		name: "mixed_two_max_744_2_target_weak", explicit: []float64{0, 0, -744.2}, target: 2,
		factor: 1, divisor: 1, maxULP: 4,
		explicitBits: []uint64{0x3fe0000000000000, 0x3fe0000000000000, 0xbff0000000000000},
	},
	{
		name: "count2_749_8_large_factor", explicit: []float64{0, -749.8, -749.8}, target: 0,
		factor: 1e308, divisor: 2, maxULP: 8,
		explicitBits: []uint64{0xbc456c600f6541a3, 0x3c356c600f6541a3, 0x3c356c600f6541a3},
	},
	{
		name: "factor0_canonical", classes: 3, winner: 0, target: 1, head: 0, tail: -40,
		factor: 0, divisor: 1, maxULP: 0,
		targetBits: 0x0000000000000000, winnerBits: 0x0000000000000000, otherBits: 0x0000000000000000,
	},
	{
		name: "factor3_divisor2", classes: 3, winner: 0, target: 2, head: 0, tail: -40,
		factor: 3, divisor: 2, maxULP: 8,
		targetBits: 0xbff8000000000000, winnerBits: 0x3ff8000000000000, otherBits: 0x3c5d635b6e68a736,
	},
	{
		name: "factor1e6", classes: 3, winner: 0, target: 0, head: 0, tail: -40,
		factor: 1e6, divisor: 1, maxULP: 8,
		targetBits: 0xbda2af38beba2eba, winnerBits: 0xbda2af38beba2eba, otherBits: 0x3d92af38beba2eba,
	},
	{
		name: "factor1e_minus300", classes: 3, winner: 0, target: 0, head: 0, tail: -40,
		factor: 1e-300, divisor: 1, maxULP: 8,
		targetBits: 0x80000000001a3dc9, winnerBits: 0x80000000001a3dc9, otherBits: 0x00000000000d1ee4,
	},
	{
		name: "factor_min_subnormal_tie", classes: 3, winner: 0, target: 0, head: 0, tail: 0,
		factor: math.SmallestNonzeroFloat64, divisor: 1, maxULP: 8,
		targetBits: 0x8000000000000001, winnerBits: 0x8000000000000001, otherBits: 0x0000000000000000,
	},
	{
		name: "divisor_maxfloat_tie", classes: 3, winner: 0, target: 0, head: 0, tail: 0,
		factor: 1, divisor: math.MaxFloat64, maxULP: 8,
		targetBits: 0x8002aaaaaaaaaaab, winnerBits: 0x8002aaaaaaaaaaab, otherBits: 0x0001555555555555,
	},
	{
		name: "weighted_gap740_factor3_divisor3", classes: 3, winner: 0, target: 0, head: 0, tail: -740,
		factor: 3, divisor: 3, maxULP: 8,
		targetBits: 0x80000000000000aa, winnerBits: 0x80000000000000aa, otherBits: 0x0000000000000055,
	},
}

func categoricalGradientLogits147(tc categoricalGradientCase147) []float64 {
	if tc.explicit != nil {
		return append([]float64(nil), tc.explicit...)
	}
	logits := make([]float64, tc.classes)
	for i := range logits {
		logits[i] = tc.tail
	}
	logits[tc.winner] = tc.head
	return logits
}

func categoricalGradientULP147(a, b float64) uint64 {
	if a == b {
		return 0
	}
	ab, bb := math.Float64bits(a), math.Float64bits(b)
	if (ab >> 63) != (bb >> 63) {
		return ^uint64(0)
	}
	if ab > bb {
		return ab - bb
	}
	return bb - ab
}

func checkCategoricalGradientValue147(t *testing.T, got, want float64, maxULP uint64, label string) {
	t.Helper()
	gotBits, wantBits := math.Float64bits(got), math.Float64bits(want)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("%s = %g bits=0x%016x, want finite bits=0x%016x", label, got, gotBits, wantBits)
		return
	}
	if want == 0 {
		if got != 0 {
			t.Errorf("%s = %g bits=0x%016x, want signed zero bits=0x%016x", label, got, gotBits, wantBits)
		}
		return
	}
	if got == 0 {
		t.Errorf("%s became zero bits=0x%016x, want nonzero %g bits=0x%016x", label, gotBits, want, wantBits)
		return
	}
	if math.Signbit(got) != math.Signbit(want) {
		t.Errorf("%s sign got=%g bits=0x%016x, want=%g bits=0x%016x", label, got, gotBits, want, wantBits)
		return
	}
	if ulp := categoricalGradientULP147(got, want); ulp > maxULP {
		t.Errorf("%s got=%g bits=0x%016x, want=%g bits=0x%016x, ULP=%d > %d", label, got, gotBits, want, wantBits, ulp, maxULP)
	}
}

func sameCategoricalFloat64Slice147(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return false
		}
	}
	return true
}

func categoricalGradientSentinel147(n int) []float64 {
	dst := make([]float64, n)
	for i := range dst {
		dst[i] = math.Float64frombits(0x3ff0000000000000 + uint64(i))
	}
	return dst
}

func TestGradientBoundary147(t *testing.T) {
	for _, tc := range categoricalGradientCases147 {
		t.Run(tc.name, func(t *testing.T) {
			logits := categoricalGradientLogits147(tc)
			before := append([]float64(nil), logits...)
			dst := categoricalGradientSentinel147(len(logits))
			if err := Gradient(dst, logits, tc.target, tc.factor, tc.divisor); err != nil {
				t.Fatalf("Gradient error: %v", err)
			}
			if !sameCategoricalFloat64Slice147(logits, before) {
				t.Fatalf("Gradient mutated logits")
			}
			if len(dst) != len(logits) {
				t.Fatalf("dst length = %d, want %d", len(dst), len(logits))
			}
			for i, got := range dst {
				var wantBits uint64
				if tc.explicitBits != nil {
					wantBits = tc.explicitBits[i]
				} else if i == tc.target {
					wantBits = tc.targetBits
				} else if i == tc.winner {
					wantBits = tc.winnerBits
				} else {
					wantBits = tc.otherBits
				}
				want := math.Float64frombits(wantBits)
				checkCategoricalGradientValue147(t, got, want, tc.maxULP, fmt.Sprintf("gradient[%d]", i))
				if tc.factor == 0 && math.Float64bits(got) != 0 {
					t.Errorf("gradient[%d] bits=0x%016x, factor zero requires canonical +0", i, math.Float64bits(got))
				}
			}
		})
	}

	tests := []struct {
		name    string
		dst     []float64
		logits  []float64
		target  int
		factor  float64
		divisor float64
	}{
		{name: "nil_logits", dst: categoricalGradientSentinel147(2), logits: nil, target: 0, factor: 1, divisor: 1},
		{name: "empty_logits", dst: categoricalGradientSentinel147(2), logits: []float64{}, target: 0, factor: 1, divisor: 1},
		{name: "nil_dst", dst: nil, logits: []float64{0, -1}, target: 0, factor: 1, divisor: 1},
		{name: "short_dst", dst: categoricalGradientSentinel147(1), logits: []float64{0, -1}, target: 0, factor: 1, divisor: 1},
		{name: "long_dst", dst: categoricalGradientSentinel147(3), logits: []float64{0, -1}, target: 0, factor: 1, divisor: 1},
		{name: "negative_target", dst: categoricalGradientSentinel147(2), logits: []float64{0, -1}, target: -1, factor: 1, divisor: 1},
		{name: "target_out_of_range", dst: categoricalGradientSentinel147(2), logits: []float64{0, -1}, target: 2, factor: 1, divisor: 1},
		{name: "negative_factor", dst: categoricalGradientSentinel147(2), logits: []float64{0, -1}, target: 0, factor: -1, divisor: 1},
		{name: "nan_factor", dst: categoricalGradientSentinel147(2), logits: []float64{0, -1}, target: 0, factor: math.NaN(), divisor: 1},
		{name: "infinite_factor", dst: categoricalGradientSentinel147(2), logits: []float64{0, -1}, target: 0, factor: math.Inf(1), divisor: 1},
		{name: "zero_divisor", dst: categoricalGradientSentinel147(2), logits: []float64{0, -1}, target: 0, factor: 1, divisor: 0},
		{name: "fractional_divisor", dst: categoricalGradientSentinel147(2), logits: []float64{0, -1}, target: 0, factor: 1, divisor: .5},
		{name: "nan_divisor", dst: categoricalGradientSentinel147(2), logits: []float64{0, -1}, target: 0, factor: 1, divisor: math.NaN()},
		{name: "infinite_divisor", dst: categoricalGradientSentinel147(2), logits: []float64{0, -1}, target: 0, factor: 1, divisor: math.Inf(1)},
	}
	for _, bad := range []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "positive_inf", value: math.Inf(1)},
		{name: "negative_inf", value: math.Inf(-1)},
	} {
		tests = append(tests, struct {
			name    string
			dst     []float64
			logits  []float64
			target  int
			factor  float64
			divisor float64
		}{name: "nonfinite_logits_" + bad.name, dst: categoricalGradientSentinel147(3), logits: []float64{bad.value, 0, -1}, target: 0, factor: 0, divisor: 1})
	}
	for _, tc := range tests {
		t.Run("reject_"+tc.name, func(t *testing.T) {
			beforeDst := append([]float64(nil), tc.dst...)
			beforeLogits := append([]float64(nil), tc.logits...)
			if err := Gradient(tc.dst, tc.logits, tc.target, tc.factor, tc.divisor); err == nil {
				t.Fatalf("Gradient accepted invalid input")
			}
			if !sameCategoricalFloat64Slice147(tc.dst, beforeDst) {
				t.Fatalf("invalid Gradient call mutated dst")
			}
			if !sameCategoricalFloat64Slice147(tc.logits, beforeLogits) {
				t.Fatalf("invalid Gradient call mutated logits")
			}
		})
	}
}
