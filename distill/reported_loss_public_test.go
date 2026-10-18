package distill_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/distill"
	"github.com/TimLai666/coimnet/internal/categorical"
)

func assertPositiveReported148(t *testing.T, got, want float64) {
	t.Helper()
	if !(got > 0) || math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("positive scalar erased: got %.17g, want %.17g", got, want)
		return
	}
	a, b := math.Float64bits(got), math.Float64bits(want)
	d := a - b
	if a < b {
		d = b - a
	}
	if d > 4 {
		t.Errorf("scalar %.17g, want %.17g within4 ULP; actual %016x expected %016x distance %d", got, want, a, b, d)
	}
}

func TestDistillPublicReportedLoss148(t *testing.T) {
	for _, mode := range []string{"label", "distribution"} {
		for target := 0; target < 3; target++ {
			t.Run(mode+string(rune('0'+target)), func(t *testing.T) {
				ctx := context.Background()
				public, reference, input, logits := newDistillPublicTrainerPair147(t, target, -40)
				before := copyRowsDistill147(input)
				tail := 0.0
				for j, v := range logits {
					if j != target {
						if math.Abs(v+40) > 1e-4 {
							t.Fatalf("unexpected tail %g", v)
						}
						tail += math.Exp(v - logits[target])
					}
				}
				want := math.Log1p(tail)
				upstream := make([][]float64, len(input))
				for i := range upstream {
					upstream[i] = make([]float64, len(logits))
				}
				if err := categorical.Gradient(upstream[len(input)-1], logits, target, 1, 1); err != nil {
					t.Fatal(err)
				}
				tracking := &trackingDistillStudent147{trainer: public}
				var report distill.StepReport
				var err error
				if mode == "label" {
					report, err = distill.StepLabel(ctx, tracking, input, target, 3)
				} else {
					report, err = distill.StepDistribution(ctx, tracking, distill.DistributionDistiller{Temperature: 2, Scale: 3, Mix: 0, Alignment: distillPublicAlignment147()}, input, distillPublicTeacher147(target), target)
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := reference.StepFrom(ctx, input, upstream); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(public.Snapshot(), reference.Snapshot()) {
					t.Fatal("scalar reporting changed accepted parameter/optimizer update")
				}
				if !reflect.DeepEqual(input, before) || tracking.predicts != 1 || tracking.steps != 1 || report.Partial {
					t.Fatalf("input/call/report control changed: %+v", report)
				}
				t.Logf("loss_bits=%016x reference_bits=%016x update_parity=true", math.Float64bits(report.Loss), math.Float64bits(want))
				assertPositiveReported148(t, report.Loss, want)
			})
		}
	}
}

func TestDistillWeightedReportedLoss148(t *testing.T) {
	cases := []struct {
		name   string
		logits []float64
		mix    float64
		bits   uint64
	}{
		{"factor_before_tiny_rounding", []float64{0, -744.1}, .6, 0x0000000000000001},
		{"smallest_public_mix_weight", []float64{0, -40, -40}, math.Nextafter(1, 0), 0x39139792499b1a24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := numericalDistiller(1, math.SmallestNonzeroFloat64, tc.mix)
			teacher := distill.TeacherDistribution{Probabilities: make([]float64, len(tc.logits))}
			teacher.Probabilities[0] = 1
			before := append([]float64(nil), tc.logits...)
			kl, kg, _, err := d.Loss(teacher, tc.logits, -1)
			if err != nil {
				t.Fatal(err)
			}
			if kl != 0 {
				t.Fatalf("KL isolation failed: %g", kl)
			}
			loss, gradient, report, err := d.Loss(teacher, tc.logits, 0)
			if err != nil {
				t.Fatal(err)
			}
			cg := make([]float64, len(tc.logits))
			if err := categorical.Gradient(cg, tc.logits, 0, 1-tc.mix, 1); err != nil {
				t.Fatal(err)
			}
			for i, g := range gradient {
				if math.Float64bits(g) != math.Float64bits(kg[i]+cg[i]) {
					t.Errorf("gradient[%d] differs from accepted KL+CE", i)
				}
			}
			if !reflect.DeepEqual(before, tc.logits) || report.Mix != tc.mix || report.Scale != math.SmallestNonzeroFloat64 {
				t.Fatal("input/report changed")
			}
			assertPositiveReported148(t, loss, math.Float64frombits(tc.bits))
		})
	}
}

func TestDistillLabelNumericalRefusal148(t *testing.T) {
	for _, target := range []int{0, 1} {
		t.Run(string(rune('0'+target)), func(t *testing.T) {
			d := numericalDistiller(2, 3, 0)
			input := []float64{math.MaxFloat64, -math.MaxFloat64}
			before := append([]float64(nil), input...)
			loss, gradient, report, err := d.Loss(distill.TeacherDistribution{Probabilities: []float64{1, 0}}, input, target)
			if err == nil || loss != 0 || gradient != nil || !reflect.DeepEqual(report, distill.DistributionReport{}) || !reflect.DeepEqual(input, before) {
				t.Fatalf("existing numerical refusal changed: loss %g, gradient %v, report %+v, error %v", loss, gradient, report, err)
			}
		})
	}
}
