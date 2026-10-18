package distill_test

import (
	"math"
	"testing"

	"github.com/TimLai666/coimnet/distill"
)

type HalfMinimum149DistillCase struct {
	name         string
	logitsBits   []uint64
	target       int
	factorBits   uint64
	divisorBits  uint64
	lossBits     uint64
	gradientBits []uint64
}

func HalfMinimum149DistillULPDistance(got, want uint64) uint64 {
	if got >= want {
		return got - want
	}
	return want - got
}

func CheckHalfMinimum149DistillLoss(t *testing.T, name string, got float64, wantBits uint64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s: nonfinite loss %v", name, got)
	}
	gotBits := math.Float64bits(got)
	if wantBits == 0 {
		if got != 0 {
			t.Errorf("%s: loss should be zero, got %016x", name, gotBits)
		}
		return
	}
	want := math.Float64frombits(wantBits)
	if got == 0 {
		t.Errorf("%s: positive loss became zero, want %016x", name, wantBits)
		return
	}
	if math.Signbit(got) != math.Signbit(want) {
		t.Errorf("%s: loss sign changed, got %016x want %016x", name, gotBits, wantBits)
		return
	}
	if distance := HalfMinimum149DistillULPDistance(gotBits, wantBits); distance > 4 {
		t.Errorf("%s: loss got %016x want %016x distance %d", name, gotBits, wantBits, distance)
	}
}

func CheckHalfMinimum149DistillGradient(t *testing.T, name string, got []float64, wantBits []uint64) {
	t.Helper()
	if len(got) != len(wantBits) {
		t.Fatalf("%s: gradient width got %d want %d", name, len(got), len(wantBits))
	}
	for i, wantBits := range wantBits {
		value := got[i]
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Errorf("%s: gradient[%d] nonfinite %v", name, i, value)
			continue
		}
		gotBits := math.Float64bits(value)
		if math.Float64frombits(wantBits) == 0 {
			if value != 0 {
				t.Errorf("%s: gradient[%d] should be zero, got %016x", name, i, gotBits)
			}
			continue
		}
		want := math.Float64frombits(wantBits)
		if value == 0 {
			t.Errorf("%s: gradient[%d] became zero, want %016x", name, i, wantBits)
			continue
		}
		if math.Signbit(value) != math.Signbit(want) {
			t.Errorf("%s: gradient[%d] sign changed, got %016x want %016x", name, i, gotBits, wantBits)
			continue
		}
		if distance := HalfMinimum149DistillULPDistance(gotBits, wantBits); distance > 4 {
			t.Errorf("%s: gradient[%d] got %016x want %016x distance %d", name, i, gotBits, wantBits, distance)
		}
	}
}

func TestHalfMinimumDistill149(t *testing.T) {
	const oneBits = uint64(0x3ff0000000000000)
	cases := []HalfMinimum149DistillCase{
		{name: "unit_-3_first", logitsBits: []uint64{0xbd104e7ce35362a1, 0xc0874910d52d3052}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "unit_-3_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce35362a1}, target: 1, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "unit_-2_first", logitsBits: []uint64{0xbd104e7ce35362a0, 0xc0874910d52d3052}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "unit_-2_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce35362a0}, target: 1, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "unit_-1_first", logitsBits: []uint64{0xbd104e7ce353629f, 0xc0874910d52d3052}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "unit_-1_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629f}, target: 1, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "unit_+0_first", logitsBits: []uint64{0xbd104e7ce353629e, 0xc0874910d52d3052}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "unit_+0_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629e}, target: 1, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "unit_+1_first", logitsBits: []uint64{0xbd104e7ce353629d, 0xc0874910d52d3052}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "unit_+1_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629d}, target: 1, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "unit_+2_first", logitsBits: []uint64{0xbd104e7ce353629c, 0xc0874910d52d3052}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "unit_+2_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629c}, target: 1, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "unit_+3_first", logitsBits: []uint64{0xbd104e7ce353629b, 0xc0874910d52d3052}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "unit_+3_last", logitsBits: []uint64{0xc0874910d52d3052, 0xbd104e7ce353629b}, target: 1, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
		{name: "aggregate2_-1_first", logitsBits: []uint64{0x3d26cc19221db5e0, 0xc0874e9c65eceee0, 0xc0874e9c65eceee0}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000000, 0x0000000000000000}},
		{name: "aggregate2_-1_last", logitsBits: []uint64{0xc0874e9c65eceee0, 0xc0874e9c65eceee0, 0x3d26cc19221db5e0}, target: 2, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x8000000000000001}},
		{name: "aggregate2_+0_first", logitsBits: []uint64{0x3d26cc19221db5e1, 0xc0874e9c65eceee0, 0xc0874e9c65eceee0}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000000, 0x0000000000000000}},
		{name: "aggregate2_+0_last", logitsBits: []uint64{0xc0874e9c65eceee0, 0xc0874e9c65eceee0, 0x3d26cc19221db5e1}, target: 2, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x8000000000000001}},
		{name: "aggregate2_+1_first", logitsBits: []uint64{0x3d26cc19221db5e2, 0xc0874e9c65eceee0, 0xc0874e9c65eceee0}, target: 0, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000, 0x0000000000000000}},
		{name: "aggregate2_+1_last", logitsBits: []uint64{0xc0874e9c65eceee0, 0xc0874e9c65eceee0, 0x3d26cc19221db5e2}, target: 2, factorBits: oneBits, divisorBits: oneBits, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x8000000000000000}},
	}
	if len(cases) != 20 {
		t.Fatalf("half-minimum public case count got %d want 20", len(cases))
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.factorBits != oneBits || tc.divisorBits != oneBits {
				t.Fatalf("public factor/divisor must be one, got %016x/%016x", tc.factorBits, tc.divisorBits)
			}
			if len(tc.logitsBits) != len(tc.gradientBits) {
				t.Fatalf("oracle width got logits %d gradient %d", len(tc.logitsBits), len(tc.gradientBits))
			}
			logits := make([]float64, len(tc.logitsBits))
			for i, bits := range tc.logitsBits {
				logits[i] = math.Float64frombits(bits)
			}
			teacher := make([]float64, len(logits))
			teacher[0] = 1
			distiller := distill.DistributionDistiller{
				Temperature: 2,
				Scale:       3,
				Mix:         0,
				Alignment: distill.Alignment{
					Rule:             distill.AlignmentIdentical,
					TeacherVocabHash: "half149",
					StudentVocabHash: "half149",
				},
			}
			loss, gradient, report, err := distiller.Loss(distill.TeacherDistribution{Probabilities: teacher}, logits, tc.target)
			if err != nil {
				t.Fatalf("public loss: %v", err)
			}
			if len(gradient) != len(logits) || len(gradient) != len(tc.gradientBits) {
				t.Fatalf("gradient width got %d want %d", len(gradient), len(tc.gradientBits))
			}
			if report.Mix != 0 {
				t.Fatalf("report mix got %v want 0", report.Mix)
			}
			CheckHalfMinimum149DistillLoss(t, tc.name, loss, tc.lossBits)
			CheckHalfMinimum149DistillGradient(t, tc.name, gradient, tc.gradientBits)
			for i, bits := range tc.logitsBits {
				if got := math.Float64bits(logits[i]); got != bits {
					t.Errorf("input logits[%d] changed got %016x want %016x", i, got, bits)
				}
			}
		})
	}
}
