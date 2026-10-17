package media

import (
	"fmt"
	"math"
	"testing"
)

// These constants are the binary64 conversions recorded in
// gradient-boundary-reference.json. The reference uses Decimal precision 500
// and never calls requestLossGradient to obtain an expected value.
type mediaGradientBoundaryCase147 struct {
	name       string
	gap        float64
	winner     int
	targets    [2]int
	targetBits [2]uint64
	winnerBits [2]uint64
	otherBits  [2]uint64
}

var mediaGradientBoundaryCases147 = []mediaGradientBoundaryCase147{
	{
		name: "gap40_winner0_targets00", gap: -40, winner: 0, targets: [2]int{0, 0},
		targetBits: [2]uint64{0xbc539792499b1a24, 0xbc539792499b1a24},
		winnerBits: [2]uint64{0xbc539792499b1a24, 0xbc539792499b1a24},
		otherBits:  [2]uint64{0x3c439792499b1a24, 0x3c439792499b1a24},
	},
	{
		name: "gap740_winner1_targets12", gap: -740, winner: 1, targets: [2]int{1, 2},
		targetBits: [2]uint64{0x8000000000000055, 0xbfe0000000000000},
		winnerBits: [2]uint64{0x8000000000000055, 0x3fe0000000000000},
		otherBits:  [2]uint64{0x000000000000002a, 0x000000000000002a},
	},
	{
		name: "gap744_winner2_targets02", gap: -744, winner: 2, targets: [2]int{0, 2},
		targetBits: [2]uint64{0xbfe0000000000000, 0x8000000000000002},
		winnerBits: [2]uint64{0x3fe0000000000000, 0x8000000000000002},
		otherBits:  [2]uint64{0x0000000000000001, 0x0000000000000001},
	},
	{
		name: "gap745_winner0_targets10", gap: -745, winner: 0, targets: [2]int{1, 0},
		targetBits: [2]uint64{0xbfe0000000000000, 0x8000000000000001},
		winnerBits: [2]uint64{0x3fe0000000000000, 0x8000000000000001},
		otherBits:  [2]uint64{0x0000000000000000, 0x0000000000000000},
	},
	{
		name: "gap746_winner1_targets21", gap: -746, winner: 1, targets: [2]int{2, 1},
		targetBits: [2]uint64{0xbfe0000000000000, 0x8000000000000000},
		winnerBits: [2]uint64{0x3fe0000000000000, 0x8000000000000000},
		otherBits:  [2]uint64{0x0000000000000000, 0x0000000000000000},
	},
	{
		name: "gap750_winner2_targets20", gap: -750, winner: 2, targets: [2]int{2, 0},
		targetBits: [2]uint64{0x8000000000000000, 0xbfe0000000000000},
		winnerBits: [2]uint64{0x8000000000000000, 0x3fe0000000000000},
		otherBits:  [2]uint64{0x0000000000000000, 0x0000000000000000},
	},
	{
		name: "tie_winner1_targets10", gap: 0, winner: 1, targets: [2]int{1, 0},
		targetBits: [2]uint64{0xbfd5555555555555, 0xbfd5555555555555},
		winnerBits: [2]uint64{0xbfd5555555555555, 0x3fc5555555555555},
		otherBits:  [2]uint64{0x3fc5555555555555, 0x3fc5555555555555},
	},
}

func mediaGradientBoundaryLogits147(gap float64, winner int) []float64 {
	logits := make([]float64, len(ImageColours)+len(ImageShapes))
	for group := 0; group < 2; group++ {
		start := group * len(ImageColours)
		for i := 0; i < len(ImageColours); i++ {
			logits[start+i] = gap
		}
		logits[start+winner] = 0
	}
	return logits
}

func mediaGradientBoundaryULP147(a, b float64) uint64 {
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

func checkMediaGradientBoundary147(t *testing.T, got, want float64, label string) {
	t.Helper()
	gotBits, wantBits := math.Float64bits(got), math.Float64bits(want)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("%s = %g (bits 0x%016x), want finite reference bits 0x%016x", label, got, gotBits, wantBits)
		return
	}
	if want == 0 {
		if got != 0 {
			t.Errorf("%s = %g (bits 0x%016x), want signed zero reference bits 0x%016x", label, got, gotBits, wantBits)
		}
		return
	}
	if got == 0 {
		t.Errorf("%s became zero (bits 0x%016x), want nonzero reference %g (bits 0x%016x)", label, gotBits, want, wantBits)
		return
	}
	if math.Signbit(got) != math.Signbit(want) {
		t.Errorf("%s sign = %g (bits 0x%016x), want %g (bits 0x%016x)", label, got, gotBits, want, wantBits)
		return
	}
	if ulp := mediaGradientBoundaryULP147(got, want); ulp > 4 {
		t.Errorf("%s = %g (bits 0x%016x), want %g (bits 0x%016x), ULP=%d > 4", label, got, gotBits, want, wantBits, ulp)
	}
}

func sameMediaFloat64Slice147(a, b []float64) bool {
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

func TestRequestLossGradientBoundary147(t *testing.T) {
	for _, tc := range mediaGradientBoundaryCases147 {
		t.Run(tc.name, func(t *testing.T) {
			logits := mediaGradientBoundaryLogits147(tc.gap, tc.winner)
			before := append([]float64(nil), logits...)
			loss, gradient, err := requestLossGradient(logits, tc.targets[0], tc.targets[1])
			if err != nil {
				t.Fatalf("requestLossGradient error: %v", err)
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) || loss < 0 {
				t.Fatalf("loss = %g, want finite nonnegative value", loss)
			}
			if len(gradient) != len(logits) {
				t.Fatalf("gradient length = %d, want %d", len(gradient), len(logits))
			}
			if !sameMediaFloat64Slice147(logits, before) {
				t.Fatalf("requestLossGradient mutated input logits")
			}
			for group := 0; group < 2; group++ {
				start := group * len(ImageColours)
				for class := 0; class < len(ImageColours); class++ {
					wantBits := tc.otherBits[group]
					if class == tc.targets[group] {
						wantBits = tc.targetBits[group]
					} else if class == tc.winner {
						wantBits = tc.winnerBits[group]
					}
					want := math.Float64frombits(wantBits)
					checkMediaGradientBoundary147(t, gradient[start+class], want, fmt.Sprintf("group%d class%d", group, class))
				}
			}
		})
	}

	for _, bad := range []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "positive_inf", value: math.Inf(1)},
		{name: "negative_inf", value: math.Inf(-1)},
	} {
		t.Run("reject_"+bad.name, func(t *testing.T) {
			logits := mediaGradientBoundaryLogits147(-40, 0)
			logits[2] = bad.value
			before := append([]float64(nil), logits...)
			_, _, err := requestLossGradient(logits, 0, 0)
			if err == nil {
				t.Fatalf("requestLossGradient accepted non-finite logits")
			}
			if !sameMediaFloat64Slice147(logits, before) {
				t.Fatalf("non-finite rejection mutated input logits")
			}
		})
	}
}
