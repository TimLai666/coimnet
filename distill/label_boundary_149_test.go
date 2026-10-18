package distill_test

import (
	"github.com/TimLai666/coimnet/distill"
	"math"
	"testing"
)

func TestDistillBoundary149(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  []uint64
		target int
		want   []uint64
		loss   uint64
	}{
		{"winner_first", []uint64{0x40774910d52d3051, 0xc0774910d52d3052}, 0, []uint64{0x8000000000000001, 0x0000000000000001}, 0x0000000000000001},
		{"winner_last", []uint64{0xc0774910d52d3052, 0x40774910d52d3051}, 1, []uint64{0x0000000000000001, 0x8000000000000001}, 0x0000000000000001},
		{"ordinary_gap", []uint64{0x0000000000000000, 0xc044000000000000}, 0, []uint64{0xbc539792499b1a24, 0x3c539792499b1a24}, 0x3c539792499b1a24},
		{"inexact40_first", []uint64{0x4034000000000001, 0xc034000000000000}, 0, []uint64{0xbc539792499b1a11, 0x3c539792499b1a11}, 0x3c539792499b1a11},
		{"inexact40_last", []uint64{0xc034000000000000, 0x4034000000000001}, 1, []uint64{0x3c539792499b1a11, 0xbc539792499b1a11}, 0x3c539792499b1a11},
		{"inexact16", []uint64{0x4020000000000001, 0xc020000000000000}, 0, []uint64{0xbe7e355b81e5bc39, 0x3e7e355b81e5bc39}, 0x3e7e355b9e6a20e8},
		{"near2_control", []uint64{0x3ff0000000000001, 0xbff0000000000000}, 0, []uint64{0xbfbe84152bac31ad, 0x3fbe84152bac31ad}, 0x3fc03f2d54301d48},
		{"inexact700", []uint64{0x4075e00000000001, 0xc075e00000000000}, 0, []uint64{0x80d14f2b0fb92f6a, 0x00d14f2b0fb92f6a}, 0x00d14f2b0fb92f6a},
		{"inexact701", []uint64{0x4075f00000000001, 0xc075e00000000000}, 0, []uint64{0x80b9788af0d60f4e, 0x00b9788af0d60f4e}, 0x00b9788af0d60f4e},
		{"exact700_control", []uint64{0x0000000000000000, 0xc085e00000000000}, 0, []uint64{0x80d14f2b0fb9307f, 0x00d14f2b0fb9307f}, 0x00d14f2b0fb9307f},
		{"exact701_control", []uint64{0x0000000000000000, 0xc085e80000000000}, 0, []uint64{0x80b9788af0d610e6, 0x00b9788af0d610e6}, 0x00b9788af0d610e6},
		{"three_classes", []uint64{0x40774910d52d3051, 0xc0774910d52d3052, 0xc0774910d52d3052}, 0, []uint64{0x8000000000000001, 0x0000000000000001, 0x0000000000000001}, 0x0000000000000001},
		{"three_classes_last", []uint64{0xc0774910d52d3052, 0xc0774910d52d3052, 0x40774910d52d3051}, 2, []uint64{0x0000000000000001, 0x0000000000000001, 0x8000000000000001}, 0x0000000000000001},
		{"target_not_winner", []uint64{0x40774910d52d3051, 0xc0774910d52d3052}, 1, []uint64{0x3ff0000000000000, 0xbff0000000000000}, 0x40874910d52d3052},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := make([]float64, len(tc.input))
			for i, b := range tc.input {
				x[i] = math.Float64frombits(b)
			}
			distiller := distill.DistributionDistiller{Temperature: 2, Scale: 3, Mix: 0, Alignment: distill.Alignment{Rule: distill.AlignmentIdentical, TeacherVocabHash: "boundary149", StudentVocabHash: "boundary149"}}
			teacher := make([]float64, len(x))
			teacher[0] = 1
			loss, g, report, err := distiller.Loss(distill.TeacherDistribution{Probabilities: teacher}, x, tc.target)
			if err != nil {
				t.Fatal(err)
			}
			if len(g) != len(tc.want) || report.Mix != 0 {
				t.Fatalf("public shape/report changed: %v %+v", g, report)
			}
			wantLoss := math.Float64frombits(tc.loss)
			lossBits := math.Float64bits(loss)
			lossDistance := lossBits - tc.loss
			if lossBits < tc.loss {
				lossDistance = tc.loss - lossBits
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) || (wantLoss > 0 && !(loss > 0)) || lossDistance > 4 {
				t.Errorf("public loss got%016x want%016x distance%d", lossBits, tc.loss, lossDistance)
			}

			for i, w := range tc.want {
				expected := math.Float64frombits(w)
				got := math.Float64bits(g[i])
				distance := got - w
				if got < w {
					distance = w - got
				}
				if math.IsNaN(g[i]) || math.IsInf(g[i], 0) {
					t.Errorf("nonfinite class%d: %v", i, g[i])
					continue
				}
				if expected == 0 {
					if g[i] != 0 {
						t.Errorf("class%d should underflow to zero: got%016x", i, got)
					}
				} else if g[i] == 0 || math.Signbit(g[i]) != math.Signbit(expected) || distance > 4 {
					t.Errorf("exact gradient class%d got%016x want%016x distance%d", i, got, w, distance)
				}
				if math.Float64bits(x[i]) != tc.input[i] {
					t.Error("input mutated")
				}
			}
		})
	}
}
