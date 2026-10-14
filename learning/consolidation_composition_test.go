package learning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

func compositionIndividual(t *testing.T, profile string, sign int8, explicit bool, base, slow, fast float64) *learning.Individual {
	t.Helper()
	c := learning.Config{InputSize: 1, OutputSize: 1, InputNodes: []int{0}, ReadoutNodes: []int{1}}
	if explicit {
		c.EdgeSigns = []int8{sign}
	}
	switch profile {
	case "continuous":
		c.Dynamics = dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1, Activation: "tanh"}
	case "lif":
		c.LIF = &dynamics.LIFConfig{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1, TauSyn: 1, ThetaMin: .1, ThetaMax: 1, VReset: -.5, Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2}}
	case "mixed":
		c.Mixed = &dynamics.MixedConfig{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1, NodeRule: []uint8{dynamics.RuleContinuous, dynamics.RuleLIF}, Continuous: dynamics.ContinuousRule{Activation: "tanh"}, LIF: dynamics.LIFRule{TauSyn: 1, ThetaMin: .1, ThetaMax: 1, VReset: -.5, Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2}}}
	default:
		t.Fatalf("unknown profile %q", profile)
	}
	rawBase := base
	if sign != 0 {
		rawBase = math.Log(base)
	}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{rawBase}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}, Encoder: []float64{1}, Readout: []float64{1}}
	if profile == "lif" {
		p.ThetaRaw = []float64{0, 0}
	} else if profile == "mixed" {
		p.ThetaRaw = []float64{0}
	}
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, 2))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.EnablePlasticity(plasticity.Config{Edges: []int{0}, Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1e308, WMin: .0625}}); err != nil {
		t.Fatal(err)
	}
	if err := a.EnableConsolidation(1); err != nil {
		t.Fatal(err)
	}
	cc := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{1}},
		Sources:   []modulation.SourceSpec{{Kind: modulation.SourceExternalTimeline, Channel: 0, Timeline: &modulation.ExternalTimeline{ChannelCount: 1}}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{Cells: []int{0}, Signal: "fixture", Channel: 0, Status: modulation.StatusHypothesized, Kd: 1, N: 1, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "slow-composition/v1"}}},
		Regions:   modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
	if err := a.EnableChemistry(cc); err != nil {
		t.Fatal(err)
	}
	snapshot := a.Snapshot()
	snapshot.Plastic.Slow.Values[0] = slow
	snapshot.Plastic.State.Plastic[0] = fast
	a, err = learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestConsolidateRejectsUnrepresentableEffectiveComposition(t *testing.T) {
	for _, profile := range []string{"continuous", "lif", "mixed"} {
		for _, mode := range []struct {
			name     string
			sign     int8
			explicit bool
		}{{"free", 0, false}, {"free-zero", 0, true}, {"excitatory", 1, true}, {"inhibitory", -1, true}} {
			t.Run(profile+"/"+mode.name, func(t *testing.T) {
				a := compositionIndividual(t, profile, mode.sign, mode.explicit, 1e308, -1e308, 1e308)
				before := a.Snapshot()
				control, err := learning.RestoreIndividual(before)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := control.Advance(context.Background(), [][]float64{{0}}); err != nil {
					t.Fatalf("pre-consolidation state cannot advance: %v", err)
				}
				report, err := a.Consolidate(context.Background(), learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: 1, Retain: 1})
				if err == nil || report.Applied || !strings.Contains(err.Error(), "effective weight") {
					t.Fatalf("Consolidate() = %+v, %v; must reject combined effective overflow", report, err)
				}
				if after := a.Snapshot(); !reflect.DeepEqual(before, after) {
					t.Fatal("combined overflow changed persistent state or consumed budget")
				}
				if _, err := json.Marshal(a.Snapshot()); err != nil {
					t.Fatal(err)
				}
				if _, err := a.Advance(context.Background(), [][]float64{{0}}); err != nil {
					t.Fatalf("refused consolidation left the individual unusable: %v", err)
				}
			})
		}
	}
}

// This reference follows the published two-neuron equations directly. It does
// not call the effective-weight or dynamics implementations for expected values.
func compositionReference(profile string, sign int8, slow, fast float64) []float64 {
	leak, alpha := math.Exp(-1), -math.Expm1(-1)
	var voltage, trace, output [2]float64
	var result []float64
	for step := 0; step < 3; step++ {
		weight := (.75 + slow) + fast
		if sign != 0 {
			weight *= float64(sign)
		}
		drive := [2]float64{1.25, weight * output[0]}
		for node := 0; node < 2; node++ {
			voltage[node] = leak*voltage[node] + alpha*drive[node]
			spiking := profile == "lif" || (profile == "mixed" && node == 1)
			if spiking {
				event := 0.0
				if voltage[node] >= .55 {
					event, voltage[node] = 1, -.5
				}
				trace[node] = leak*trace[node] + event
				output[node] = trace[node]
			} else {
				output[node] = math.Tanh(voltage[node])
			}
		}
		result = append(result, float64(float32(output[1])))
		fast *= .5
	}
	return result
}

func TestConsolidateFiniteCompositionMatchesIndependentReference(t *testing.T) {
	for _, profile := range []string{"continuous", "lif", "mixed"} {
		for _, sign := range []int8{0, 1, -1} {
			for _, rate := range []float64{.25, 1} {
				for _, retain := range []float64{0, .5, 1} {
					t.Run(fmt.Sprintf("%s/sign%d/rate%g/retain%g", profile, sign, rate, retain), func(t *testing.T) {
						a := compositionIndividual(t, profile, sign, true, .75, .125, .25)
						report, err := a.Consolidate(context.Background(), learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: rate, Retain: retain})
						if err != nil || !report.Applied {
							t.Fatalf("Consolidate() = %+v, %v", report, err)
						}
						slow, fast := .125+rate*.25, retain*.25
						snapshot := a.Snapshot()
						if snapshot.Plastic.Slow.Values[0] != slow || snapshot.Plastic.State.Plastic[0] != fast || report.Used != 1 {
							t.Fatal("finite write or counters differ")
						}
						got, err := a.Advance(context.Background(), [][]float64{{1.25}, {1.25}, {1.25}})
						if err != nil {
							t.Fatal(err)
						}
						want := compositionReference(profile, sign, slow, fast)
						for step := range want {
							if math.Abs(got[step][0]-want[step]) > 1e-6 {
								t.Fatalf("row %d = %.9g, want %.9g", step, got[step][0], want[step])
							}
						}
						if sign >= 0 && want[2] == 0 {
							t.Fatal("positive control produced no readout")
						}
					})
				}
			}
		}
	}
}

func TestConsolidateFixedSignFloorRetainsFiniteIntegration(t *testing.T) {
	for _, profile := range []string{"continuous", "lif", "mixed"} {
		for _, sign := range []int8{1, -1} {
			t.Run(fmt.Sprintf("%s/sign%d", profile, sign), func(t *testing.T) {
				a := compositionIndividual(t, profile, sign, true, 1, 0, -1e308)
				report, err := a.Consolidate(context.Background(), learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: 1, Retain: 1})
				if err != nil || !report.Applied {
					t.Fatalf("floor-bounded composition = %+v, %v", report, err)
				}
				if _, err := a.Advance(context.Background(), [][]float64{{1.25}, {1.25}}); err != nil {
					t.Fatalf("fixed-sign floor no longer integrates: %v", err)
				}
			})
		}
	}
}
