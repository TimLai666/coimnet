package distill_test

import (
	"github.com/TimLai666/coimnet/distill"
	"math"
	"reflect"
	"testing"
)

func TestRootDistillReportedLoss148(t *testing.T) {
	cases := []struct {
		name     string
		logits   []float64
		target   int
		wantBits uint64
	}{
		{"winner_first", []float64{0, -40, -40}, 0, 0x3c639792499b1a24},
		{"winner_middle", []float64{-40, 0, -40}, 1, 0x3c639792499b1a24},
		{"winner_last", []float64{-40, -40, 0}, 2, 0x3c639792499b1a24},
		{"equal_classes", []float64{0, 0, 0}, 0, 0x3ff193ea7aad030b},
		{"ordinary_gap", []float64{0, -1}, 0, 0x3fd40c7abfbec124},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := distill.DistributionDistiller{Temperature: 2, Scale: 3, Mix: 0, Alignment: distill.Alignment{Rule: distill.AlignmentIdentical, TeacherVocabHash: "audit148", StudentVocabHash: "audit148"}}
			teacher := distill.TeacherDistribution{Probabilities: make([]float64, len(tc.logits))}
			teacher.Probabilities[0] = 1
			before := append([]float64(nil), tc.logits...)
			loss, gradient, report, err := d.Loss(teacher, tc.logits, tc.target)
			if err != nil {
				t.Fatalf("valid public Loss rejected: %v", err)
			}
			if !reflect.DeepEqual(tc.logits, before) || report.Mix != 0 || len(gradient) != len(tc.logits) || !(gradient[tc.target] < 0) {
				t.Fatalf("input/report/gradient control changed: loss=%g gradient=%v report=%+v", loss, gradient, report)
			}
			if !(loss > 0) || math.IsNaN(loss) || math.IsInf(loss, 0) {
				t.Fatalf("representable positive loss erased: got %.17g (%016x), want %.17g (%016x)", loss, math.Float64bits(loss), math.Float64frombits(tc.wantBits), tc.wantBits)
			}
			gotBits := math.Float64bits(loss)
			distance := gotBits - tc.wantBits
			if gotBits < tc.wantBits {
				distance = tc.wantBits - gotBits
			}
			if distance > 4 {
				t.Fatalf("public Loss differs from independent reference: got %.17g, want %.17g, ULP=%d", loss, math.Float64frombits(tc.wantBits), distance)
			}
		})
	}
}
