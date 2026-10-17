package textgen

import (
	"math"
	"testing"
)

func TestTextgenLossAggregateAccuracy146(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: .01, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	// Decimal precision500 references for 2049 scored positions:
	// ln(1+exp(first)) + 2048*ln(1+exp(rest)). Padding tails at -1000
	// have no effect on the rounded reference.
	for _, tc := range []struct {
		name        string
		first, rest float64
		bits        uint64
	}{
		{"tiny", -0x1.62e42fefa39f0p+8, -0x1.87a0cee5f20efp+8, 0x1ff0000000000335},
		{"ordinary", 0, -40, 0x3fe62e42fefa3a3e},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := Example{IDs: make([]int, 2050), Scored: make([]bool, 2050)}
			rows := make([][]float64, len(ex.IDs))
			for i := range rows {
				rows[i] = make([]float64, m.vocabSize)
				for j := range rows[i] {
					rows[i][j] = -1000
				}
				rows[i][0], rows[i][1] = 0, tc.rest
				if i > 0 {
					ex.Scored[i] = true
				}
			}
			rows[0][1] = tc.first
			loss, count, gradient, err := m.lossAndUpstream(ex, rows)
			if err != nil || count != 2049 || len(gradient) != len(rows) || math.IsNaN(loss) || math.IsInf(loss, 0) || loss <= 0 {
				t.Fatalf("loss=%g count=%d gradient rows=%d err=%v", loss, count, len(gradient), err)
			}
			got := math.Float64bits(loss)
			var distance uint64
			if got >= tc.bits {
				distance = got - tc.bits
			} else {
				distance = tc.bits - got
			}
			if distance > 4 {
				t.Fatalf("loss bits=%#x; want %#x within 4 ULP, distance=%d", got, tc.bits, distance)
			}
		})
	}
}
