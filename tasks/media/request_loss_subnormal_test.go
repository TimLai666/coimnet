package media

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"testing"
)

func TestRequestLossSubnormal146(t *testing.T) {
	tests := []struct {
		name   string
		logits []float64
		want   uint64
	}{
		{
			name:   "both_groups_gap_745_and_1000",
			logits: []float64{0, -745, -1000, 0, -745, -1000},
			want:   1,
		},
		{
			name:   "both_groups_equal_gap_745_5",
			logits: []float64{0, -745.5, -745.5, 0, -745.5, -745.5},
			want:   1,
		},
		{
			name:   "both_groups_equal_gap_746_round_to_zero",
			logits: []float64{0, -746, -746, 0, -746, -746},
			want:   0,
		},
		{
			name:   "one_group_gap_745_and_1000_other_gap_1000",
			logits: []float64{0, -745, -1000, 0, -1000, -1000},
			want:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loss, gradient, err := requestLossGradient(tt.logits, 0, 0)
			if err != nil {
				t.Fatalf("requestLossGradient: %v", err)
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) {
				t.Fatalf("loss = %g, want finite", loss)
			}
			if len(gradient) != len(tt.logits) {
				t.Fatalf("gradient length = %d, want %d", len(gradient), len(tt.logits))
			}
			bits := make([]byte, 8*len(gradient))
			for i, value := range gradient {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					t.Fatalf("gradient[%d] = %g, want finite", i, value)
				}
				binary.LittleEndian.PutUint64(bits[8*i:], math.Float64bits(value))
			}
			t.Logf("gradient Float64bits sha256=%x", sha256.Sum256(bits))
			if got := math.Float64bits(loss); got != tt.want {
				t.Fatalf("loss bits = %#x (%g), want %#x", got, loss, tt.want)
			}
		})
	}
}
