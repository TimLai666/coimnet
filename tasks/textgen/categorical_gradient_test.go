package textgen

import (
	"fmt"
	"math"
	"testing"
)

// These are the frozen float64 conversions of the independent Decimal160
// references in root-gradient-plan.json for the 259-class, count-one loss.
const (
	textgenGradientWinner147 = -0x1.3bec16e2e5052p-50
	textgenGradientOther147  = 0x1.39792499b1a1ep-58
)

func gradientULPDistance147(got, want float64) uint64 {
	gotBits, wantBits := math.Float64bits(got), math.Float64bits(want)
	if gotBits > wantBits {
		return gotBits - wantBits
	}
	return wantBits - gotBits
}

func checkTextgenGradientEntry147(t *testing.T, index int, got, want float64, target bool) {
	t.Helper()
	gotBits, wantBits := math.Float64bits(got), math.Float64bits(want)
	ulp := gradientULPDistance147(got, want)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("gradient[%d] target=%t = %.17g, want finite %.17g; got_bits=0x%016x want_bits=0x%016x ulp=%d", index, target, got, want, gotBits, wantBits, ulp)
		return
	}
	if target && got >= 0 {
		t.Errorf("gradient[%d] target=%t = %.17g, want negative; got_bits=0x%016x want_bits=0x%016x ulp=%d", index, target, got, gotBits, wantBits, ulp)
	}
	if !target && got <= 0 {
		t.Errorf("gradient[%d] target=%t = %.17g, want positive; got_bits=0x%016x want_bits=0x%016x ulp=%d", index, target, got, gotBits, wantBits, ulp)
	}
	if ulp > 4 {
		t.Errorf("gradient[%d] target=%t = %.17g, want %.17g within 4 ULP; got_bits=0x%016x want_bits=0x%016x ulp=%d", index, target, got, want, gotBits, wantBits, ulp)
	}
}

func TestTextgenLossGradientTail147(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: .01, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	if m.vocabSize != 259 {
		t.Fatalf("test vocabulary size=%d, want 259", m.vocabSize)
	}
	for _, winner := range []int{0, 27, 258} {
		t.Run(fmt.Sprintf("winner_%d", winner), func(t *testing.T) {
			rows := [][]float64{make([]float64, m.vocabSize), make([]float64, m.vocabSize)}
			for i := range rows[0] {
				rows[0][i] = -40
			}
			rows[0][winner] = 0
			ex := Example{IDs: []int{0, winner}, Scored: []bool{false, true}}
			loss, count, gradient, err := m.lossAndUpstream(ex, rows)
			if err != nil {
				t.Fatalf("loss/upstream error: loss=%.17g, count=%d, err=%v", loss, count, err)
			}
			if count != 1 {
				t.Errorf("count=%d, want 1", count)
			}
			if len(gradient) != len(rows) {
				t.Fatalf("gradient rows=%d, want %d", len(gradient), len(rows))
			}
			for row, values := range gradient {
				if len(values) != m.vocabSize {
					t.Fatalf("gradient row %d width=%d, want %d", row, len(values), m.vocabSize)
				}
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) || loss < 0 {
				t.Errorf("loss=%.17g, want finite nonnegative value", loss)
			}
			for i, got := range gradient[0] {
				target := i == winner
				want := textgenGradientOther147
				if target {
					want = textgenGradientWinner147
				}
				checkTextgenGradientEntry147(t, i, got, want, target)
			}
			for i, got := range gradient[1] {
				if got != 0 {
					t.Errorf("inactive gradient[1][%d] = %.17g, want 0; bits=0x%016x", i, got, math.Float64bits(got))
				}
			}
		})
	}
}
