package textgen

import (
	"fmt"
	"math"
	"testing"
)

// These constants are the binary64 conversions recorded in
// gradient-boundary-reference.json. The reference uses Decimal precision 500
// and never calls lossAndUpstream to obtain an expected value.
type textgenGradientBoundaryCase147 struct {
	name       string
	gap        float64
	count      int
	winner     int
	targets    []int
	targetBits []uint64
	winnerBits []uint64
	otherBits  []uint64
}

var textgenGradientBoundaryCases147 = []textgenGradientBoundaryCase147{
	{
		name: "count1_gap40_winner0_target0", gap: -40, count: 1, winner: 0, targets: []int{0},
		targetBits: []uint64{0xbcd3bec16e2e5052},
		winnerBits: []uint64{0xbcd3bec16e2e5052},
		otherBits:  []uint64{0x3c539792499b1a1e},
	},
	{
		name: "count1_gap740_winner129_target130", gap: -740, count: 1, winner: 129, targets: []int{130},
		targetBits: []uint64{0xbff0000000000000},
		winnerBits: []uint64{0x3ff0000000000000},
		otherBits:  []uint64{0x0000000000000055},
	},
	{
		name: "count1_gap744_winner258_target258", gap: -744, count: 1, winner: 258, targets: []int{258},
		targetBits: []uint64{0x8000000000000191},
		winnerBits: []uint64{0x8000000000000191},
		otherBits:  []uint64{0x0000000000000002},
	},
	{
		name: "count1_gap745_winner0_target1", gap: -745, count: 1, winner: 0, targets: []int{1},
		targetBits: []uint64{0xbff0000000000000},
		winnerBits: []uint64{0x3ff0000000000000},
		otherBits:  []uint64{0x0000000000000001},
	},
	{
		name: "count1_gap746_winner129_target129", gap: -746, count: 1, winner: 129, targets: []int{129},
		targetBits: []uint64{0x8000000000000036},
		winnerBits: []uint64{0x8000000000000036},
		otherBits:  []uint64{0x0000000000000000},
	},
	{
		name: "count1_gap750_winner258_target0", gap: -750, count: 1, winner: 258, targets: []int{0},
		targetBits: []uint64{0xbff0000000000000},
		winnerBits: []uint64{0x3ff0000000000000},
		otherBits:  []uint64{0x0000000000000000},
	},
	{
		name: "count1_gap750_winner258_target258", gap: -750, count: 1, winner: 258, targets: []int{258},
		targetBits: []uint64{0x8000000000000001},
		winnerBits: []uint64{0x8000000000000001},
		otherBits:  []uint64{0x0000000000000000},
	},
	{
		name: "count1_tie_winner129_target0", gap: 0, count: 1, winner: 129, targets: []int{0},
		targetBits: []uint64{0xbfefe05ee355fe06},
		winnerBits: []uint64{0x3f6fa11caa01fa12},
		otherBits:  []uint64{0x3f6fa11caa01fa12},
	},
	{
		name: "count3_gap745_winner0_targets01258", gap: -745, count: 3, winner: 0, targets: []int{0, 1, 258},
		targetBits: []uint64{0x8000000000000031, 0xbfd5555555555555, 0xbfd5555555555555},
		winnerBits: []uint64{0x8000000000000031, 0x3fd5555555555555, 0x3fd5555555555555},
		otherBits:  []uint64{0x0000000000000000, 0x0000000000000000, 0x0000000000000000},
	},
	{
		name: "count3_gap750_winner258_targets258257258", gap: -750, count: 3, winner: 258, targets: []int{258, 257, 258},
		targetBits: []uint64{0x8000000000000000, 0xbfd5555555555555, 0x8000000000000000},
		winnerBits: []uint64{0x8000000000000000, 0x3fd5555555555555, 0x8000000000000000},
		otherBits:  []uint64{0x0000000000000000, 0x0000000000000000, 0x0000000000000000},
	},
}

func textgenGradientBoundaryRows147(vocabSize, count int, gap float64, winner int) [][]float64 {
	rows := make([][]float64, count+1)
	for row := 0; row < count; row++ {
		rows[row] = make([]float64, vocabSize)
		for class := range rows[row] {
			rows[row][class] = gap
		}
		rows[row][winner] = 0
	}
	rows[count] = make([]float64, vocabSize)
	return rows
}

func textgenGradientBoundaryULP147(a, b float64) uint64 {
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

func checkTextgenGradientBoundary147(t *testing.T, got, want float64, label string) {
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
	if ulp := textgenGradientBoundaryULP147(got, want); ulp > 4 {
		t.Errorf("%s = %g (bits 0x%016x), want %g (bits 0x%016x), ULP=%d > 4", label, got, gotBits, want, wantBits, ulp)
	}
}

func sameTextgenFloat64Rows147(a, b [][]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if math.Float64bits(a[i][j]) != math.Float64bits(b[i][j]) {
				return false
			}
		}
	}
	return true
}

func TestTextgenLossAndUpstreamGradientBoundary147(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 4, Settle: 2, LearningRate: .01, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	if m.vocabSize != 259 {
		t.Fatalf("test vocabulary size = %d, want 259", m.vocabSize)
	}

	for _, tc := range textgenGradientBoundaryCases147 {
		t.Run(tc.name, func(t *testing.T) {
			ids := make([]int, len(tc.targets)+1)
			copy(ids[1:], tc.targets)
			scored := make([]bool, len(ids))
			for i := 1; i < len(scored); i++ {
				scored[i] = true
			}
			ex := Example{IDs: ids, Scored: scored}
			rows := textgenGradientBoundaryRows147(m.vocabSize, tc.count, tc.gap, tc.winner)
			before := make([][]float64, len(rows))
			for i := range rows {
				before[i] = append([]float64(nil), rows[i]...)
			}
			loss, gotCount, upstream, callErr := m.lossAndUpstream(ex, rows)
			if callErr != nil {
				t.Fatalf("lossAndUpstream error: %v", callErr)
			}
			if gotCount != tc.count {
				t.Fatalf("count = %d, want %d", gotCount, tc.count)
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) || loss < 0 {
				t.Fatalf("loss = %g, want finite nonnegative value", loss)
			}
			if !sameTextgenFloat64Rows147(rows, before) {
				t.Fatalf("lossAndUpstream mutated input logits")
			}
			if len(upstream) != len(ex.IDs)*m.config.Settle {
				t.Fatalf("upstream rows = %d, want %d", len(upstream), len(ex.IDs)*m.config.Settle)
			}
			for row, values := range upstream {
				if len(values) != m.vocabSize {
					t.Fatalf("upstream row %d width = %d, want %d", row, len(values), m.vocabSize)
				}
			}

			scoredRows := make(map[int]bool, len(tc.targets))
			for position, target := range tc.targets {
				upstreamRow := (position+1)*m.config.Settle - 1
				scoredRows[upstreamRow] = true
				for class := 0; class < m.vocabSize; class++ {
					wantBits := tc.otherBits[position]
					if class == target {
						wantBits = tc.targetBits[position]
					} else if class == tc.winner {
						wantBits = tc.winnerBits[position]
					}
					want := math.Float64frombits(wantBits)
					checkTextgenGradientBoundary147(t, upstream[upstreamRow][class], want, fmt.Sprintf("upstream row%d class%d", upstreamRow, class))
				}
			}
			for row, values := range upstream {
				if scoredRows[row] {
					continue
				}
				for class, value := range values {
					if value != 0 {
						t.Errorf("unscored upstream row%d class%d = %g (bits 0x%016x), want exact zero", row, class, value, math.Float64bits(value))
					}
				}
			}
		})
	}

	t.Run("count2_gap40_internal_unscored_position", func(t *testing.T) {
		ex := Example{IDs: []int{0, 0, 2, 0}, Scored: []bool{false, true, false, true}}
		rows := make([][]float64, len(ex.IDs))
		for row := range rows {
			rows[row] = make([]float64, m.vocabSize)
		}
		for _, row := range []int{0, 2} {
			for class := range rows[row] {
				rows[row][class] = -40
			}
			rows[row][0] = 0
		}
		for class := range rows[1] {
			rows[1][class] = -17
		}
		before := make([][]float64, len(rows))
		for i := range rows {
			before[i] = append([]float64(nil), rows[i]...)
		}
		loss, count, upstream, callErr := m.lossAndUpstream(ex, rows)
		if callErr != nil {
			t.Fatalf("lossAndUpstream error: %v", callErr)
		}
		if count != 2 {
			t.Fatalf("count = %d, want 2", count)
		}
		if math.IsNaN(loss) || math.IsInf(loss, 0) || loss < 0 {
			t.Fatalf("loss = %g, want finite nonnegative value", loss)
		}
		if !sameTextgenFloat64Rows147(rows, before) {
			t.Fatalf("lossAndUpstream mutated input logits")
		}
		if len(upstream) != len(ex.IDs)*m.config.Settle {
			t.Fatalf("upstream rows = %d, want %d", len(upstream), len(ex.IDs)*m.config.Settle)
		}
		wantTarget := math.Float64frombits(0xbcc3bec16e2e5052)
		wantOther := math.Float64frombits(0x3c439792499b1a1e)
		for _, upstreamRow := range []int{1, 5} {
			for class := 0; class < m.vocabSize; class++ {
				want := wantOther
				if class == 0 {
					want = wantTarget
				}
				checkTextgenGradientBoundary147(t, upstream[upstreamRow][class], want, fmt.Sprintf("masked upstream row%d class%d", upstreamRow, class))
			}
		}
		for _, upstreamRow := range []int{0, 2, 3, 4, 6, 7} {
			for class, value := range upstream[upstreamRow] {
				if value != 0 {
					t.Errorf("masked unscored upstream row%d class%d = %g (bits 0x%016x), want exact zero", upstreamRow, class, value, math.Float64bits(value))
				}
			}
		}
	})

	for _, bad := range []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "positive_inf", value: math.Inf(1)},
		{name: "negative_inf", value: math.Inf(-1)},
	} {
		t.Run("reject_"+bad.name, func(t *testing.T) {
			ex := Example{IDs: []int{0, 0}, Scored: []bool{false, true}}
			rows := textgenGradientBoundaryRows147(m.vocabSize, 1, -40, 0)
			rows[0][1] = bad.value
			before := make([][]float64, len(rows))
			for i := range rows {
				before[i] = append([]float64(nil), rows[i]...)
			}
			_, _, _, callErr := m.lossAndUpstream(ex, rows)
			if callErr == nil {
				t.Fatalf("lossAndUpstream accepted non-finite logits")
			}
			if !sameTextgenFloat64Rows147(rows, before) {
				t.Fatalf("non-finite rejection mutated input logits")
			}
		})
	}
}
