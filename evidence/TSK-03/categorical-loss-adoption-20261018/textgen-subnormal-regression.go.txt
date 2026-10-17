package textgen

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"testing"
)

func TestTextgenLossSubnormal146(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: .01, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	if m.vocabSize != 259 {
		t.Fatalf("test vocabulary size = %d, want 259", m.vocabSize)
	}

	tests := []struct {
		name      string
		winner    int
		rows      int
		gap       float64
		wantBits  uint64
		wantCount int
	}{
		{name: "one_position_winner_0", winner: 0, rows: 2, gap: 746, wantBits: 54, wantCount: 1},
		{name: "one_position_winner_258", winner: 258, rows: 2, gap: 746, wantBits: 54, wantCount: 1},
		{name: "two_positions_winner_0", winner: 0, rows: 3, gap: 751, wantBits: 1, wantCount: 2},
		{name: "two_positions_winner_258", winner: 258, rows: 3, gap: 751, wantBits: 1, wantCount: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids := []int{0, tt.winner}
			scored := []bool{false, true}
			rows := make([][]float64, 2)
			if tt.rows == 3 {
				ids = []int{0, tt.winner, tt.winner}
				scored = []bool{false, true, true}
			}
			for row := range rows {
				rows[row] = make([]float64, m.vocabSize)
				if row >= tt.rows-1 {
					continue
				}
				for i := range rows[row] {
					rows[row][i] = -tt.gap
				}
				rows[row][tt.winner] = 0
			}
			if tt.rows == 3 {
				rows = append(rows, make([]float64, m.vocabSize))
			}

			loss, count, gradient, err := m.lossAndUpstream(Example{IDs: ids, Scored: scored}, rows)
			if err != nil {
				t.Fatalf("lossAndUpstream: %v", err)
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) {
				t.Fatalf("loss = %g, want finite", loss)
			}
			if count != tt.wantCount {
				t.Fatalf("count = %d, want %d", count, tt.wantCount)
			}
			if len(gradient) != len(rows) {
				t.Fatalf("gradient rows = %d, want %d", len(gradient), len(rows))
			}
			bits := make([]byte, 0, 8*len(gradient)*m.vocabSize)
			for row, values := range gradient {
				if len(values) != m.vocabSize {
					t.Fatalf("gradient row %d width = %d, want %d", row, len(values), m.vocabSize)
				}
				for column, value := range values {
					if math.IsNaN(value) || math.IsInf(value, 0) {
						t.Fatalf("gradient[%d][%d] = %g, want finite", row, column, value)
					}
					bits = binary.LittleEndian.AppendUint64(bits, math.Float64bits(value))
				}
			}
			t.Logf("gradient Float64bits sha256=%x", sha256.Sum256(bits))
			if got := math.Float64bits(loss); got != tt.wantBits {
				t.Fatalf("loss bits = %#x (%g), want %#x", got, loss, tt.wantBits)
			}
		})
	}
}
