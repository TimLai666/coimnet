package ocr

import (
	"math"
	"testing"
)

// Values are rounded once from an independent Decimal300 reference.
func TestRootOCRReportedLoss148(t *testing.T) {
	cases := []struct {
		name     string
		logits   []float64
		target   int
		wantBits uint64
	}{
		{"winner_first", []float64{1.0000000000000001e+300, 0, -40, -40}, 1, 0x3c639792499b1a24},
		{"winner_middle", []float64{1.0000000000000001e+300, -40, 0, -40}, 2, 0x3c639792499b1a24},
		{"winner_last", []float64{1.0000000000000001e+300, -40, -40, 0}, 3, 0x3c639792499b1a24},
		{"equal_classes", []float64{1.0000000000000001e+300, 0, 0, 0}, 1, 0x3ff193ea7aad030b},
		{"ordinary_gap", []float64{1.0000000000000001e+300, 0, -1}, 1, 0x3fd40c7abfbec124},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loss, gradient, err := singleCharacterLoss(tc.logits, tc.target)
			if err != nil {
				t.Fatalf("valid logits rejected: %v", err)
			}
			if len(gradient) != len(tc.logits) || gradient[0] != 0 {
				t.Fatalf("blank exclusion or gradient shape changed: %v", gradient)
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
				t.Fatalf("reported loss differs from independent reference: got %.17g (%016x), want %.17g (%016x), ULP=%d", loss, gotBits, math.Float64frombits(tc.wantBits), tc.wantBits, distance)
			}
		})
	}
}
