package textgen

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
)

// A uniform tail has an analytic normalizer independent of the winner ID.
func TestTextgenLossRetainsTinyTail146(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: .01, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	if m.vocabSize != 259 {
		t.Fatalf("test vocabulary size = %d, want 259", m.vocabSize)
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
			if err != nil || count != 1 || len(gradient) != 2 {
				t.Fatalf("loss/upstream: loss=%g, count=%d, len=%d, err=%v", loss, count, len(gradient), err)
			}
			bits := make([]byte, 0, 16*m.vocabSize)
			for _, row := range gradient {
				for _, value := range row {
					bits = binary.LittleEndian.AppendUint64(bits, math.Float64bits(value))
				}
			}
			t.Logf("tail gradient sha256=%x", sha256.Sum256(bits))
			want := math.Log1p(258 * math.Exp(-40))
			if math.IsNaN(loss) || math.IsInf(loss, 0) || loss <= 0 || math.Abs(loss-want) > want*1e-12 {
				t.Fatalf("tiny-tail loss = %.17g; want %.17g", loss, want)
			}
		})
	}
}
