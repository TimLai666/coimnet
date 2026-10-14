package modulation_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/modulation"
)

// Sensitivity bounds include neutral gain 1 after applying zero-value defaults.
func TestNeutralGainRejectsExcludedRange(t *testing.T) {
	for _, mix := range []string{"", modulation.MixSum, modulation.MixMax} {
		for _, bounds := range []struct {
			name     string
			min, max float64
		}{
			{"lower_excludes", 1.25, 0},
			{"upper_excludes", 0, 0.75},
			{"point_excludes", 1.5, 1.5},
			{"next_above_one", math.Nextafter(1, math.Inf(1)), 2},
			{"next_below_one", .5, math.Nextafter(1, 0)},
		} {
			t.Run(mix+"/"+bounds.name, func(t *testing.T) {
				effects := []modulation.Effect{{Kind: modulation.EffectSensitivity, GammaScale: 0.5, GammaMin: bounds.min, GammaMax: bounds.max}}
				for _, occ := range [][]modulation.OccupancyRecord{nil, {{Receptor: 0, Cell: 0, Occupancy: 0}}, {{Receptor: 0, Cell: 0, Occupancy: .5}}} {
					gain, offset, threshold, report, err := modulation.ApplyEffects(effects, occ, mix, 1)
					if err == nil {
						t.Fatalf("accepted range [%g,%g] excluding neutral 1; gain=%v", bounds.min, bounds.max, gain)
					}
					if !strings.Contains(err.Error(), "gamma") || gain != nil || offset != nil || threshold != nil || report != (modulation.ClampReport{}) {
						t.Fatalf("bad refusal: gain=%v offset=%v threshold=%v report=%v err=%v", gain, offset, threshold, report, err)
					}
					gradient, err := modulation.BackwardEffects(effects, occ, mix, 1, []float64{1}, []float64{0}, []float64{0})
					if err == nil || gradient != nil {
						t.Fatalf("backward accepted excluded range: gradient=%v err=%v", gradient, err)
					}
				}
			})
		}
	}
}

func TestNeutralGainRetainsValidRanges(t *testing.T) {
	for _, mix := range []string{"", modulation.MixSum, modulation.MixMax} {
		for _, bounds := range [][2]float64{{0, 0}, {0.25, 1.5}, {1, 1}, {1, 2}, {0.5, 1}} {
			effects := []modulation.Effect{{Kind: modulation.EffectSensitivity, GammaScale: 0.5, GammaMin: bounds[0], GammaMax: bounds[1]}}
			zero := []modulation.OccupancyRecord{{Receptor: 0, Cell: 0}}
			gain, offset, theta, report, err := modulation.ApplyEffects(effects, zero, mix, 1)
			if err != nil || len(gain) != 1 || math.Float64bits(gain[0]) != math.Float64bits(1) || offset[0] != 0 || theta[0] != 0 || report != (modulation.ClampReport{}) {
				t.Fatalf("valid neutral range %v mix %q: %v %v %v %+v %v", bounds, mix, gain, offset, theta, report, err)
			}
		}
	}
}
