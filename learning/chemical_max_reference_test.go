package learning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

// The table is a hand oracle for the private reference, not a production VJP.
func TestChemicalMaxReferencePrimitives(t *testing.T) {
	type result struct {
		v float64
		d [2]float64
	}
	cases := []struct {
		name, mix                  string
		occupancy                  [2]float64
		gs, bs, ts, lo, hi, bm, tm float64
		duplicate                  bool
		want                       [3]result
		saturated                  int
	}{
		{"max-negative", modulation.MixMax, [2]float64{.8, .2}, -.5, -.25, -.1, .5, 2, 1, 1, false, [3]result{{.6, [2]float64{-.5, 0}}, {-.2, [2]float64{-.25, 0}}, {-.08, [2]float64{-.1, 0}}}, 0},
		{"max-later", modulation.MixMax, [2]float64{.2, .8}, .5, .25, .1, .5, 2, 1, 1, false, [3]result{{1.4, [2]float64{0, .5}}, {.2, [2]float64{0, .25}}, {.08, [2]float64{0, .1}}}, 0},
		{"max-tie", modulation.MixMax, [2]float64{.5, .5}, .5, .25, .1, .5, 2, 1, 1, false, [3]result{{1.25, [2]float64{.5, 0}}, {.125, [2]float64{.25, 0}}, {.05, [2]float64{.1, 0}}}, 0},
		{"max-zero", modulation.MixMax, [2]float64{0, 0}, .5, .25, .1, .5, 2, 1, 1, false, [3]result{{1, [2]float64{}}, {0, [2]float64{}}, {0, [2]float64{}}}, 0},
		{"sum-positive", modulation.MixSum, [2]float64{.4, .2}, .5, .25, .1, .5, 2, 1, 1, false, [3]result{{1.3, [2]float64{.5, .5}}, {.15, [2]float64{.25, .25}}, {.06, [2]float64{.1, .1}}}, 0},
		{"sum-zero", modulation.MixSum, [2]float64{0, 0}, .5, .25, .1, .5, 2, 1, 1, false, [3]result{{1, [2]float64{.5, .5}}, {0, [2]float64{.25, .25}}, {0, [2]float64{.1, .1}}}, 0},
		{"sum-duplicate", modulation.MixSum, [2]float64{.4, .2}, .5, .25, .1, .5, 2, 1, 1, true, [3]result{{1.4, [2]float64{1, 0}}, {.2, [2]float64{.5, 0}}, {.08, [2]float64{.2, 0}}}, 0},
		{"max-duplicate", modulation.MixMax, [2]float64{.4, .2}, .5, .25, .1, .5, 2, 1, 1, true, [3]result{{1.2, [2]float64{.5, 0}}, {.1, [2]float64{.25, 0}}, {.04, [2]float64{.1, 0}}}, 0},
		{"upper", modulation.MixMax, [2]float64{.8, .2}, 1, .5, 1, .75, 1.1, .1, .1, false, [3]result{{1.1, [2]float64{}}, {.1, [2]float64{}}, {.1, [2]float64{}}}, 3},
		{"lower", modulation.MixMax, [2]float64{.8, .2}, -1, -.5, -1, .9, 1.25, .1, .1, false, [3]result{{.9, [2]float64{}}, {-.1, [2]float64{}}, {-.1, [2]float64{}}}, 3},
		{"endpoints", modulation.MixMax, [2]float64{.4, .2}, .5, .25, .25, .5, 1.2, .1, .1, false, [3]result{{1.2, [2]float64{}}, {.1, [2]float64{}}, {.1, [2]float64{}}}, 3},
	}
	if len(cases) != 11 {
		t.Fatal("hand oracle count changed")
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := mixedReferenceFixture("neural", "normal")
			cfg := &f.config.Chemistry.Part.Config
			cfg.Receptors.Mix = c.mix
			cfg.Receptors.Records = cfg.Receptors.Records[:2]
			for i := range cfg.Receptors.Records {
				cfg.Receptors.Records[i].Cells = []int{0}
			}
			cfg.Effects = nil
			for i := 0; i < 2; i++ {
				r := i
				if c.duplicate {
					r = 0
				}
				cfg.Effects = append(cfg.Effects,
					modulation.Effect{Kind: modulation.EffectSensitivity, Receptor: r, GammaScale: c.gs, BetaScale: c.bs, GammaMin: c.lo, GammaMax: c.hi, BetaAbsMax: c.bm},
					modulation.Effect{Kind: modulation.EffectThreshold, Receptor: r, ThetaScale: c.ts, ThetaAbsMax: c.tm})
			}
			g, b, th, s := referenceEffects(f.config.Chemistry, []dual{dVar(c.occupancy[0], 0, 2), dVar(c.occupancy[1], 1, 2)}, 2)
			if s != c.saturated {
				t.Fatalf("saturation=%d want%d", s, c.saturated)
			}
			for i, x := range []dual{g[0], b[0], th[0]} {
				if len(x.d) != 2 || !finiteReference(x.v) || math.Abs(x.v-c.want[i].v) > 1e-14 {
					t.Fatalf("output%d=%+v want%+v", i, x, c.want[i])
				}
				for j := range x.d {
					if !finiteReference(x.d[j]) || math.Abs(x.d[j]-c.want[i].d[j]) > 1e-14 {
						t.Fatalf("output%d derivative%d=%g want%g", i, j, x.d[j], c.want[i].d[j])
					}
				}
			}
			for i, x := range []dual{g[1], b[1], th[1]} {
				neutral := 0.
				if i == 0 {
					neutral = 1
				}
				if x.v != neutral || !reflect.DeepEqual(x.d, []float64{0, 0}) {
					t.Fatalf("unaddressed cell=%+v", x)
				}
			}
		})
	}
}

func chemicalMaxReferenceFixture(t *testing.T, core, source, mix, profile string) mixedReferenceCase {
	t.Helper()
	f := mixedReferenceFixture(source, "normal")
	cfg := &f.config.Chemistry.Part.Config
	cfg.Receptors.Mix = mix
	r2, r3 := cfg.Receptors.Records[0], cfg.Receptors.Records[1]
	r2.Kd, r2.N = .15, 1.1
	r2.Location = cfg.Receptors.Records[1].Location
	r3.Kd, r3.N = .37, 1.3
	r3.Location = cfg.Receptors.Records[0].Location
	cfg.Receptors.Records = append(cfg.Receptors.Records, r2, r3)
	if core == "lif" {
		m := f.config.Mixed
		f.config.LIF = &dynamics.LIFConfig{Nodes: m.Nodes, Sources: m.Sources, Targets: m.Targets, Delays: m.Delays, DT: m.DT, TauSyn: m.LIF.TauSyn, ThetaMin: m.LIF.ThetaMin, ThetaMax: m.LIF.ThetaMax, VReset: m.LIF.VReset, RefractorySteps: m.LIF.RefractorySteps, Adaptation: m.LIF.Adaptation, Surrogate: m.LIF.Surrogate}
		f.config.Mixed = nil
		f.parameters.Core.Bias[0] = 1.60
		f.parameters.ThetaRaw = []float64{-.18, -.10}
		cfg.Receptors.Records[1].Cells = []int{0, 1}
		cfg.Receptors.Records[3].Cells = []int{0, 1}
	} else if core != "mixed" {
		t.Fatalf("unknown core%q", core)
	}
	spec, err := chemicalMaxProfile(profile)
	if profile == "floor" {
		spec, err = chemicalMaxProfile("open-positive")
		spec.thetaScale, spec.thetaMax = -4, 1
		f.variant = "floor"
	}
	if err != nil {
		t.Fatal(err)
	}
	sensitivity := modulation.Effect{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: spec.gammaScale, BetaScale: spec.betaScale, GammaMin: spec.gammaMin, GammaMax: spec.gammaMax, BetaAbsMax: spec.betaMax}
	threshold := modulation.Effect{Kind: modulation.EffectThreshold, Receptor: 1, ThetaScale: spec.thetaScale, ThetaAbsMax: spec.thetaMax}
	s2, t2 := sensitivity, threshold
	s2.Receptor, t2.Receptor = 2, 3
	cfg.Effects = []modulation.Effect{sensitivity, threshold, s2, t2}
	f.name = core + "/" + source + "/" + mix + "/" + profile
	return f
}

func TestChemicalMaxIndependentReference(t *testing.T) {
	profiles := append(chemicalMaxProfiles(true), "floor")
	cases, controls, segments := 0, 0, 0
	for _, core := range []string{"mixed", "lif"} {
		for _, source := range []string{"controller", "neural"} {
			for _, profile := range profiles {
				var previous learning.Gradient
				var previousPrediction [][]float64
				for _, mix := range []string{modulation.MixSum, modulation.MixMax} {
					f := chemicalMaxReferenceFixture(t, core, source, mix, profile)
					cases++
					t.Run(f.name, func(t *testing.T) {
						gradient, prediction := runMixedReference(t, f)
						want, _, coverage := mixedReferenceGradient(f)
						if profile != "open-positive" && profile != "open-negative" && profile != "floor" && coverage.effectSaturation == 0 {
							t.Fatal("capped profile did not reach an effect bound")
						}
						if mix == modulation.MixMax {
							controls++
							if reflect.DeepEqual(gradient, previous) && reflect.DeepEqual(prediction, previousPrediction) {
								t.Fatal("sum/max control was unobservable")
							}
						} else {
							previous, previousPrediction = gradient, prediction
						}
						if core != "lif" {
							return
						}
						expected := append([]float64(nil), want.coreWeights...)
						expected = append(expected, want.coreBias...)
						expected = append(expected, want.coreLogTau...)
						expected = append(expected, want.thetaRaw...)
						expected = append(expected, want.encoder...)
						expected = append(expected, want.readout...)
						if len(expected) != 18 {
							t.Fatalf("reference trainable coordinates%d want18", len(expected))
						}
						for _, segment := range []int{1, 3} {
							segments++
							t.Run(fmt.Sprintf("segment%d", segment), func(t *testing.T) {
								options := chemicalMaxOptions(true, segment, true)
								options.AccumulateSteps = 2
								options.Truncation = 0
								trainer, err := learning.NewTrainer(f.config, f.parameters, options)
								if err != nil {
									t.Fatal(err)
								}
								defer trainer.Close()
								if _, err = trainer.StepFrom(context.Background(), f.input, f.upstream); err != nil {
									t.Fatal(err)
								}
								snapshot := trainer.Snapshot()
								if snapshot.Updates != 0 || snapshot.Accumulator == nil || snapshot.Accumulator.Count != 1 {
									t.Fatalf("pending update state: updates%d accumulator%+v", snapshot.Updates, snapshot.Accumulator)
								}
								got := snapshot.Accumulator.Sum
								if len(got) != len(expected) {
									t.Fatalf("pending gradient width%d want%d", len(got), len(expected))
								}
								for i, value := range got {
									if !finiteReference(value) || math.Abs(value-expected[i]) > 1e-5+1e-4*math.Abs(expected[i]) {
										t.Fatalf("pending gradient%d=%g reference%g", i, value, expected[i])
									}
								}
								outputs, err := trainer.PredictAll(context.Background(), f.input)
								if err != nil {
									t.Fatal(err)
								}
								if !reflect.DeepEqual(outputs, prediction) {
									t.Fatal("pending trainer prediction differs")
								}
							})
						}
					})
				}
			}
		}
	}
	if cases != 72 || controls != 36 || segments != 72 {
		t.Fatalf("matrix cases%d controls%d segments%d want72/36/72", cases, controls, segments)
	}
}

// The evidence runner compares this deterministic seven-case record before and
// after extending the reference; it is not an expected-output training oracle.
func TestChemicalMaxReferenceLegacyRecord(t *testing.T) {
	for _, source := range []string{"controller", "neural"} {
		for _, variant := range []string{"normal", "floor", "saturation", "nil"} {
			if variant == "nil" && source == "controller" {
				continue
			}
			f := mixedReferenceFixture(source, variant)
			g, p, c := mixedReferenceGradient(f)
			public, outputs := runMixedReference(t, f)
			data, err := json.Marshal(struct {
				Name       string
				Expected   []interface{}
				Prediction [][]float64
				Coverage   []int
				Public     learning.Gradient
				Outputs    [][]float64
			}{f.name, []interface{}{g.coreWeights, g.coreBias, g.coreLogTau, g.thetaRaw, g.coreInputs, g.coreInitial, g.encoder, g.readout, g.inputs}, p, []int{c.floorSteps, c.refractorySteps, c.effectSaturation, c.expressionActive, c.expressionSaturation}, public, outputs})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("legacy-record:%s", data)
		}
	}
}
