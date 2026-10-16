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
	"github.com/TimLai666/coimnet/plasticity"
)

func interventionNormIndividual(t *testing.T, core, source string, regions int, pair bool, aplus, decayPre float64) *learning.Individual {
	t.Helper()
	c := learning.Config{InputSize: 3, OutputSize: 3, InputNodes: []int{0, 1, 2}, ReadoutNodes: []int{0, 1, 2}, ReadoutEveryStep: true}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{0, 0}, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}}, Encoder: []float64{1, 0, 0, 0, 1, 0, 0, 0, 1}, Readout: []float64{1, 0, 0, 0, 1, 0, 0, 0, 1}}
	rule := dynamics.LIFRule{TauSyn: 1, ThetaMin: .1, ThetaMax: 1.9, VReset: 0, Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}}
	switch core {
	case "continuous":
		c.Dynamics = dynamics.Config{Nodes: 3, Sources: []int{0, 0}, Targets: []int{1, 2}, Delays: []int{1, 2}, DT: math.Ln2, Activation: "tanh"}
	case "lif":
		c.LIF = &dynamics.LIFConfig{Nodes: 3, Sources: []int{0, 0}, Targets: []int{1, 2}, Delays: []int{1, 2}, DT: math.Ln2, TauSyn: rule.TauSyn, ThetaMin: rule.ThetaMin, ThetaMax: rule.ThetaMax, VReset: rule.VReset, Surrogate: rule.Surrogate}
		p.ThetaRaw = []float64{0, 0, 0}
	case "mixed":
		c.Mixed = &dynamics.MixedConfig{Nodes: 3, Sources: []int{0, 0}, Targets: []int{1, 2}, Delays: []int{1, 2}, DT: math.Ln2, NodeRule: []uint8{dynamics.RuleContinuous, dynamics.RuleLIF, dynamics.RuleContinuous}, Continuous: dynamics.ContinuousRule{Activation: "tanh"}, LIF: rule}
		p.ThetaRaw = []float64{0}
	default:
		t.Fatal("unknown core")
	}
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), []float64{.2, .1, .3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Advance(context.Background(), [][]float64{{0, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	pr := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 2, WMin: .01}
	if pair {
		pr.Kind = plasticity.RuleSTDPPair
		pr.DecayPre = decayPre
		pr.DecayPost = .5
		pr.APlus = aplus
	}
	if err := a.EnablePlasticity(plasticity.Config{Rule: pr, Edges: []int{0, 1}}); err != nil {
		t.Fatal(err)
	}
	spec := modulation.SourceSpec{Kind: modulation.SourceExternalTimeline, Timeline: &modulation.ExternalTimeline{ChannelCount: 1}}
	switch source {
	case "timeline":
	case "neural":
		spec = modulation.SourceSpec{Kind: modulation.SourceNeuralActivity, Neural: &modulation.NeuralActivity{Nodes: []int{0}, Gain: .2}}
	case "controller":
		spec = modulation.SourceSpec{Kind: modulation.SourceController, Controller: &modulation.Controller{Inputs: modulation.ControllerInputs{SummaryWindow: 2, UseFeedback: true, Resources: []string{"energy"}}, Nodes: 3, Hidden: 1, Parameters: []float64{.1, .2, .3, .4, .05, .2, -.1}}}
	default:
		t.Fatal("unknown source")
	}
	chemistry := modulation.ChemistryConfig{Chemistry: modulation.Chemistry{Regions: regions, Channels: 1, DT: math.Ln2, Tau: []float64{1}}, Sources: []modulation.SourceSpec{spec}, Regions: modulation.RegionAssignment{NodeRegion: []int{0, regions - 1, 0}}, Receptors: modulation.Receptors{Records: []modulation.Receptor{{Cells: []int{0}, Signal: "engineering", Status: modulation.StatusHypothesized, Kd: 1, N: 1, Evidence: "numerical boundary", MeasurementKind: "engineering boundary", MappingVersion: "intervention-norm/v1"}}}}
	if err := a.EnableChemistry(chemistry); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	s.Plastic.State.Eligibility = []float64{.6, .4}
	s.Plastic.State.Plastic = []float64{.02, .03}
	s.Plastic.Slow = &learning.SlowState{Values: []float64{.01, .02}, Budget: 4}
	if pair {
		s.Plastic.State.PreTrace = []float64{1, 1}
		s.Plastic.State.PostTrace = []float64{.25, .25}
	}
	a, err = learning.RestoreIndividual(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.FreezePlasticity(true); err != nil {
		t.Fatal(err)
	}
	if err := a.SetResource("energy", .25); err != nil {
		t.Fatal(err)
	}
	if err := a.OfferFeedback(stepFeedbackAt(t, 0, 10)); err != nil {
		t.Fatal(err)
	}
	if err := a.SetExpressionGain(learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: .2, Min: .5, Max: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Advance(context.Background(), [][]float64{{0, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	if err := a.FreezePlasticity(false); err != nil {
		t.Fatal(err)
	}
	return a
}

func interventionNormClone(t *testing.T, a *learning.Individual, plasticFrozen, chemicalFrozen bool) *learning.Individual {
	t.Helper()
	b, err := learning.RestoreIndividual(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if err := b.FreezePlasticity(plasticFrozen); err != nil {
		t.Fatal(err)
	}
	if err := b.FreezeChemistry(chemicalFrozen); err != nil {
		t.Fatal(err)
	}
	return b
}

func interventionNormPlan(kind string, value float64, shuffle bool) learning.InterventionPlan {
	item := learning.Intervention{Kind: kind, Value: value, Start: 0, End: 1, Restore: true}
	if kind == learning.InterventionForceSpike {
		item.Targets = []int{1, 2}
	}
	p := learning.InterventionPlan{Authorized: true, Reason: "independent numerical report control", Items: []learning.Intervention{item}}
	if shuffle {
		p.Items = append(p.Items, learning.Intervention{Kind: learning.InterventionShuffleDelays, Seed: 3, Start: 0, End: 1, Restore: true})
	}
	return p
}

func interventionNormCheck(t *testing.T, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || (want != 0 && got == 0) || (want == 0 && got != 0) || (want != 0 && math.Abs(got/want-1) > 3e-15) {
		t.Fatalf("delta got %.17g want %.17g", got, want)
	}
}

func TestInterveneNormFiniteConcentration(t *testing.T) {
	for _, core := range []string{"continuous", "lif", "mixed"} {
		for _, tc := range []struct {
			name    string
			value   float64
			regions int
		}{
			{"large", 5e307, 1}, {"small", 2.5e-201, 1}, {"sum_overflow", 1e154, 2}, {"normal", .25, 2},
		} {
			t.Run(core+"/"+tc.name, func(t *testing.T) {
				a := interventionNormIndividual(t, core, "timeline", tc.regions, false, 0, 0)
				before := a.Snapshot()
				control := interventionNormClone(t, a, false, false)
				if _, err := control.Advance(context.Background(), [][]float64{{0, 0, 0}}); err != nil {
					t.Fatal(err)
				}
				_, log, err := a.Intervene(context.Background(), interventionNormPlan(learning.InterventionFixConcentration, tc.value, false), [][]float64{{0, 0, 0}})
				if err != nil {
					t.Fatal(err)
				}
				if len(log.Entries) != 1 || log.Entries[0].AppliedRows != 1 {
					t.Fatal(log)
				}
				want := tc.value
				if tc.regions == 2 {
					want = math.Hypot(tc.value, tc.value)
				}
				interventionNormCheck(t, log.Entries[0].ConcentrationDelta, want)
				if _, err := json.Marshal(log); err != nil {
					t.Fatal(err)
				}
				if reflect.DeepEqual(a.Snapshot(), before) {
					t.Fatal("successful operation did not commit")
				}
				if !reflect.DeepEqual(a.Snapshot().Parameters, before.Parameters) {
					t.Fatal("intervention changed base parameters")
				}
				interventionNormCheck(t, log.Entries[0].BaseParameterDelta, 0)
			})
		}
	}
}

func TestInterveneNormFinitePlastic(t *testing.T) {
	t.Run("large", func(t *testing.T) {
		a := interventionNormIndividual(t, "lif", "timeline", 1, true, 1e308, .5)
		_, log, err := a.Intervene(context.Background(), interventionNormPlan(learning.InterventionForceSpike, 1, false), [][]float64{{0, 0, 0}})
		if err != nil {
			t.Fatal(err)
		}
		interventionNormCheck(t, log.Entries[0].PlasticDelta, math.Hypot(5e307, 5e307))
		if _, err := json.Marshal(log); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("small", func(t *testing.T) {
		c := learning.Config{Dynamics: dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: math.Ln2, Activation: "tanh"}, InputSize: 2, OutputSize: 2, InputNodes: []int{0, 1}, ReadoutNodes: []int{0, 1}, ReadoutEveryStep: true}
		p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{0}, Bias: []float64{1e-100, 1e-100}, LogTau: []float64{0, 0}}, Encoder: []float64{1, 0, 0, 1}, Readout: []float64{1, 0, 0, 1}}
		a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), []float64{0, 0})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.EnablePlasticity(plasticity.Config{Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .01}, Edges: []int{0}}); err != nil {
			t.Fatal(err)
		}
		plan := learning.InterventionPlan{Authorized: true, Reason: "hand-calculated tiny eligibility", Items: []learning.Intervention{{Kind: learning.InterventionClampVoltage, Targets: []int{1}, Value: 0, Start: 0, End: 1, Restore: true}}}
		_, log, err := a.Intervene(context.Background(), plan, [][]float64{{0, 0}})
		if err != nil {
			t.Fatal(err)
		}
		interventionNormCheck(t, log.Entries[0].PlasticDelta, 2.5e-201)
		if _, err := json.Marshal(log); err != nil {
			t.Fatal(err)
		}
	})
}

func interventionNormRefusal(t *testing.T, a *learning.Individual, plan learning.InterventionPlan, plasticFrozen, chemicalFrozen bool) {
	t.Helper()
	if err := a.FreezePlasticity(plasticFrozen); err != nil {
		t.Fatal(err)
	}
	if err := a.FreezeChemistry(chemicalFrozen); err != nil {
		t.Fatal(err)
	}
	before, report := a.Snapshot(), a.ChemistryReport()
	if report.Steps == 0 {
		t.Fatal("fixture has no prior completed chemical report")
	}
	control := interventionNormClone(t, a, plasticFrozen, chemicalFrozen)
	out, log, err := a.Intervene(context.Background(), plan, [][]float64{{0, 0, 0}})
	if err == nil || out != nil || !reflect.DeepEqual(log, learning.InterventionLog{}) {
		t.Fatalf("numeric refusal got out=%v log=%+v err=%v", out, log, err)
	}
	if !reflect.DeepEqual(a.Snapshot(), before) || !reflect.DeepEqual(a.ChemistryReport(), report) {
		t.Fatal("numeric failure committed state or replaced the previous report")
	}
	next := [][]float64{{.1, .2, .3}, {0, 0, 0}}
	got, err := a.Advance(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	want, err := control.Advance(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(a.Snapshot(), control.Snapshot()) || !reflect.DeepEqual(a.ChemistryReport(), control.ChemistryReport()) {
		t.Fatal("continuation after refusal differs from untouched control")
	}
}

func TestInterveneNormConcentrationRefusalAtomic(t *testing.T) {
	for _, core := range []string{"continuous", "lif", "mixed"} {
		for _, source := range []string{"timeline", "neural", "controller"} {
			for _, shuffle := range []bool{false, true} {
				for _, frozen := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/shuffle%t/plasticFrozen%t", core, source, shuffle, frozen), func(t *testing.T) {
						a := interventionNormIndividual(t, core, source, 2, false, 0, 0)
						interventionNormRefusal(t, a, interventionNormPlan(learning.InterventionFixConcentration, 1.3e308, shuffle), frozen, false)
					})
				}
			}
		}
	}
}

func TestInterveneNormPlasticRefusalAtomic(t *testing.T) {
	for _, source := range []string{"timeline", "neural", "controller"} {
		for _, shuffle := range []bool{false, true} {
			for _, frozen := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/shuffle%t/chemicalFrozen%t", source, shuffle, frozen), func(t *testing.T) {
					a := interventionNormIndividual(t, "lif", source, 2, true, 1.3e308, .999)
					interventionNormRefusal(t, a, interventionNormPlan(learning.InterventionForceSpike, 1, shuffle), false, frozen)
				})
			}
		}
	}
}
