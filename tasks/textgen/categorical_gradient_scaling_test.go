package textgen

import (
	"math"
	"testing"
)

func TestTextgenGradientScale147(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: .01, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	if m.vocabSize != 259 {
		t.Fatalf("vocabulary size=%d, want 259", m.vocabSize)
	}
	for _, tc := range []struct {
		name  string
		count int
		mixed bool
	}{
		{name: "count2_uniform", count: 2},
		{name: "mixed_tail", count: 1, mixed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := make([][]float64, tc.count+1)
			ids, scored := make([]int, len(rows)), make([]bool, len(rows))
			for row := range rows {
				rows[row] = make([]float64, m.vocabSize)
				if row == tc.count {
					continue
				}
				for class := range rows[row] {
					rows[row][class] = -749.8
					if tc.mixed {
						rows[row][class] = -1000
					}
				}
				rows[row][0] = 0
				if tc.mixed {
					rows[row][1], rows[row][2] = 0, -744.2
				}
				scored[row+1] = true
			}
			before := make([][]float64, len(rows))
			for row := range rows {
				before[row] = append([]float64(nil), rows[row]...)
			}
			loss, count, upstream, err := m.lossAndUpstream(Example{IDs: ids, Scored: scored}, rows)
			if err != nil || count != tc.count || len(upstream) != len(rows) || math.IsNaN(loss) || math.IsInf(loss, 0) || loss <= 0 {
				t.Fatalf("loss=%g count=%d upstream=%d err=%v", loss, count, len(upstream), err)
			}
			if !sameTextgenFloat64Rows147(rows, before) {
				t.Fatal("input changed")
			}
			for row, gradient := range upstream {
				if len(gradient) != m.vocabSize {
					t.Fatal("gradient width changed")
				}
				for class, got := range gradient {
					want := 0.0
					if row < tc.count {
						if tc.mixed {
							switch class {
							case 0:
								want = -.5
							case 1:
								want = .5
							case 2:
								want = math.SmallestNonzeroFloat64
							}
						} else if class == 0 {
							want = -math.SmallestNonzeroFloat64
						}
					}
					checkTextgenGradientBoundary147(t, got, want, "scaled gradient")
				}
			}
		})
	}
}
