package learning

import (
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

// The flat AdamW layout is weights, bias, log_tau, theta_raw, encoder, readout.
// A continuous model has no theta_raw entries, so its layout and every existing
// optimizer state stay byte-for-byte unchanged.
func TestFlatLayoutPlacesThetaAfterLogTau(t *testing.T) {
	p := Parameters{
		Core:     dynamics.Parameters{Weights: []float64{1, 2}, Bias: []float64{3, 4, 5}, LogTau: []float64{6, 7, 8}},
		ThetaRaw: []float64{9, 10, 11},
		Encoder:  []float64{12, 13, 14},
		Readout:  []float64{15},
	}
	want := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	if got := flatParameters(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("flatParameters = %v, want %v", got, want)
	}
	g := Gradient{
		Core:     dynamics.Gradient{Weights: []float64{1, 2}, Bias: []float64{3, 4, 5}, LogTau: []float64{6, 7, 8}},
		ThetaRaw: []float64{9, 10, 11},
		Encoder:  []float64{12, 13, 14},
		Readout:  []float64{15},
	}
	if got := flatGradient(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("flatGradient = %v, want %v", got, want)
	}
	flat := flatParameters(p)
	for i := range flat {
		flat[i] = -flat[i]
	}
	back := unflatten(flat, p)
	if !reflect.DeepEqual(flatParameters(back), flat) {
		t.Fatalf("unflatten round trip = %v, want %v", flatParameters(back), flat)
	}
	if len(back.ThetaRaw) != 3 || back.ThetaRaw[0] != -9 {
		t.Fatalf("unflatten lost theta_raw: %v", back.ThetaRaw)
	}
	mask := parameterMask(p, Options{Trainable: Trainable{Theta: true}})
	for i, enabled := range mask {
		if enabled != (i >= 8 && i < 11) {
			t.Fatalf("theta-only mask at %d = %v", i, enabled)
		}
	}
	full := parameterMask(p, Options{Trainable: Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Theta: true, Readout: true}})
	for i, enabled := range full {
		if !enabled {
			t.Fatalf("full mask disabled parameter %d", i)
		}
	}
	if len(mask) != len(want) || len(full) != len(want) {
		t.Fatalf("mask length %d/%d, want %d", len(mask), len(full), len(want))
	}
}

func TestContinuousFlatLayoutIsUnchanged(t *testing.T) {
	p := Parameters{
		Core:    dynamics.Parameters{Weights: []float64{1, 2}, Bias: []float64{3, 4}, LogTau: []float64{5, 6}},
		Encoder: []float64{7, 8},
		Readout: []float64{9},
	}
	want := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9}
	if got := flatParameters(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("continuous layout = %v, want %v", got, want)
	}
	if got := copyParameters(p); got.ThetaRaw != nil {
		t.Fatalf("copyParameters invented theta_raw: %v", got.ThetaRaw)
	}
	withTheta := p
	withTheta.ThetaRaw = []float64{1, 2}
	copied := copyParameters(withTheta)
	copied.ThetaRaw[0] = 99
	if withTheta.ThetaRaw[0] != 1 {
		t.Fatal("copyParameters aliases theta_raw")
	}
	mask := parameterMask(p, Options{Trainable: Trainable{Theta: true}})
	for i, enabled := range mask {
		if enabled {
			t.Fatalf("continuous mask enabled parameter %d with only the theta group trainable", i)
		}
	}
}
