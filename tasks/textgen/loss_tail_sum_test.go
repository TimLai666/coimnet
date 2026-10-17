package textgen

import (
	"fmt"
	"math"
	"testing"
)

func TestTextgenLossTailSumAccuracy146(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: .01, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	if m.vocabSize != 259 {
		t.Fatalf("vocabulary size = %d, want 259", m.vocabSize)
	}
	// Nearest float64 references from Decimal precision500, not the candidate:
	// ln(1 + exp(-gap)*(1 + 257*exp(-37))).
	for _, tc := range []struct {
		gap  float64
		bits uint64
	}{
		{0, 0x3fe62e42fefa3a52},
		{300, 0x24e245639c3a4a68},
		{400, 0x1bde50c483c04e88},
	} {
		for _, runnerUp := range []int{1, 258} {
			t.Run(fmt.Sprintf("gap_%g_runnerup_%d", tc.gap, runnerUp), func(t *testing.T) {
				rows := [][]float64{make([]float64, m.vocabSize), make([]float64, m.vocabSize)}
				for i := range rows[0] {
					rows[0][i] = -tc.gap - 37
				}
				rows[0][0], rows[0][runnerUp] = 0, -tc.gap
				loss, count, gradient, err := m.lossAndUpstream(Example{IDs: []int{0, 0}, Scored: []bool{false, true}}, rows)
				if err != nil || count != 1 || len(gradient) != 2 || math.IsNaN(loss) || math.IsInf(loss, 0) || loss <= 0 {
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
					t.Fatalf("loss=%.17g bits=%#x; want %#x within 4 ULP, distance=%d", loss, got, tc.bits, distance)
				}
			})
		}
	}
}
