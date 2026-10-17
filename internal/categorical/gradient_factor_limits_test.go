package categorical

import (
	"fmt"
	"math"
	"testing"
)

// References use Decimal1000 and the exact binary64 inputs in
// categorical-factor-limits-reference.json; no expected value uses Gradient.
func TestGradientFactorLimits147(t *testing.T) {
	cases := []struct {
		name                              string
		classes                           int
		gap                               float64
		target                            int
		divisor                           float64
		targetBits, winnerBits, otherBits uint64
	}{
		{name: "gap1300_target0", classes: 3, gap: -1300.0, target: 0, divisor: 1.0, targetBits: 0x8ac6925eb1948275, winnerBits: 0x8ac6925eb1948275, otherBits: 0x0ab6925eb1948275},
		{name: "gap1300_target1", classes: 3, gap: -1300.0, target: 1, divisor: 1.0, targetBits: 0xffefffffffffffff, winnerBits: 0x7fefffffffffffff, otherBits: 0x0ab6925eb1948275},
		{name: "gap1400_target0", classes: 3, gap: -1400.0, target: 0, divisor: 1.0, targetBits: 0x81c2b9c33b18b05f, winnerBits: 0x81c2b9c33b18b05f, otherBits: 0x01b2b9c33b18b05f},
		{name: "gap1400_target1", classes: 3, gap: -1400.0, target: 1, divisor: 1.0, targetBits: 0xffefffffffffffff, winnerBits: 0x7fefffffffffffff, otherBits: 0x01b2b9c33b18b05f},
		{name: "gap1450_target0", classes: 3, gap: -1450.0, target: 0, divisor: 1.0, targetBits: 0x8000000000000088, winnerBits: 0x8000000000000088, otherBits: 0x0000000000000044},
		{name: "gap1450_target1", classes: 3, gap: -1450.0, target: 1, divisor: 1.0, targetBits: 0xffefffffffffffff, winnerBits: 0x7fefffffffffffff, otherBits: 0x0000000000000044},
		{name: "gap1490_target0", classes: 3, gap: -1490.0, target: 0, divisor: 1.0, targetBits: 0x8000000000000000, winnerBits: 0x8000000000000000, otherBits: 0x0000000000000000},
		{name: "gap1490_target1", classes: 3, gap: -1490.0, target: 1, divisor: 1.0, targetBits: 0xffefffffffffffff, winnerBits: 0x7fefffffffffffff, otherBits: 0x0000000000000000},
		{name: "gap1400_div3_target0", classes: 3, gap: -1400.0, target: 0, divisor: 3.0, targetBits: 0x81a8f7aef976407f, winnerBits: 0x81a8f7aef976407f, otherBits: 0x0198f7aef976407f},
		{name: "gap1400_div3_target1", classes: 3, gap: -1400.0, target: 1, divisor: 3.0, targetBits: 0xffd5555555555555, winnerBits: 0x7fd5555555555555, otherBits: 0x0198f7aef976407f},
		{name: "tie3_maxfactor", classes: 3, gap: 0.0, target: 0, divisor: 1.0, targetBits: 0xffe5555555555555, winnerBits: 0xffe5555555555555, otherBits: 0x7fd5555555555555},
		{name: "259_winner_maxfactor", classes: 259, gap: -40.0, target: 0, divisor: 1.0, targetBits: 0xfcd3bec16e2e5052, winnerBits: 0xfcd3bec16e2e5052, otherBits: 0x7c539792499b1a1e},
		{name: "259_weak_target_maxfactor", classes: 259, gap: -40.0, target: 1, divisor: 1.0, targetBits: 0xffefffffffffffff, winnerBits: 0x7feffffffffffff5, otherBits: 0x7c539792499b1a1e},
		{name: "maxfactor_maxdivisor", classes: 3, gap: -40.0, target: 0, divisor: math.MaxFloat64, targetBits: 0xbc639792499b1a24, winnerBits: 0xbc639792499b1a24, otherBits: 0x3c539792499b1a24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logits := make([]float64, tc.classes)
			for i := 1; i < len(logits); i++ {
				logits[i] = tc.gap
			}
			dst := make([]float64, len(logits))
			if err := Gradient(dst, logits, tc.target, math.MaxFloat64, tc.divisor); err != nil {
				t.Fatal(err)
			}
			for i, got := range dst {
				wantBits := tc.otherBits
				if i == tc.target {
					wantBits = tc.targetBits
				} else if i == 0 {
					wantBits = tc.winnerBits
				}
				checkCategoricalGradientValue147(t, got, math.Float64frombits(wantBits), 8, fmt.Sprintf("factor limit gradient[%d]", i))
			}
		})
	}
}
