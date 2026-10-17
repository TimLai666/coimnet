package media

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
)

// Each group has probabilities proportional to [1, exp(-40), exp(-40)].
func TestRequestLossRetainsTinyTail146(t *testing.T) {
	for winner := 0; winner < 3; winner++ {
		t.Run(fmt.Sprintf("winner_%d", winner), func(t *testing.T) {
			logits := []float64{-40, -40, -40, -40, -40, -40}
			logits[winner], logits[3+winner] = 0, 0
			loss, gradient, err := requestLossGradient(logits, winner, winner)
			if err != nil || len(gradient) != 6 {
				t.Fatalf("loss/gradient: loss=%g, len=%d, err=%v", loss, len(gradient), err)
			}
			bits := make([]byte, 8*len(gradient))
			for i, value := range gradient {
				binary.LittleEndian.PutUint64(bits[8*i:], math.Float64bits(value))
			}
			t.Logf("tail gradient sha256=%x", sha256.Sum256(bits))
			want := math.Log1p(2 * math.Exp(-40))
			if math.IsNaN(loss) || math.IsInf(loss, 0) || loss <= 0 || math.Abs(loss-want) > want*1e-12 {
				t.Fatalf("tiny-tail loss = %.17g; want %.17g", loss, want)
			}
		})
	}
}
