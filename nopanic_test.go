package coimnet_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/distill"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/experiment/nav2d"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/multimodal"
	"github.com/TimLai666/coimnet/plasticity"
	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/simulate"
	"github.com/TimLai666/coimnet/teacher"
)

// nopanicContinuousConfig is the two-node continuous fixture the learning
// cases build on, matching learning/lif_test.go's continuousConfig.
func nopanicContinuousConfig() learning.Config {
	return learning.Config{
		Dynamics:  dynamics.Config{Nodes: 2, Sources: []int{0, 1}, Targets: []int{1, 0}, DT: .5, Activation: "tanh"},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1},
	}
}

func nopanicContinuousParams() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.3, -.2}, Bias: []float64{.1, -.1}, LogTau: []float64{.1, .2}},
		Encoder: []float64{.5, .1},
		Readout: []float64{.8},
	}
}

func nopanicContinuousCore() dynamics.Config {
	return dynamics.Config{Nodes: 2, Sources: []int{0, 1}, Targets: []int{1, 0}, DT: .5, Activation: "tanh"}
}

func nopanicContinuousRates() dynamics.Parameters {
	return dynamics.Parameters{Weights: []float64{.3, -.2}, Bias: []float64{.1, -.1}, LogTau: []float64{.1, .2}}
}

func nopanicLIFCore() dynamics.LIFConfig {
	return dynamics.LIFConfig{
		Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, TauSyn: 1,
		ThetaMin: .05, ThetaMax: 1, VReset: -.5, Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
}

func nopanicMixedCore() dynamics.MixedConfig {
	return dynamics.MixedConfig{
		Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, NodeRule: []uint8{0, 1},
		Continuous: dynamics.ContinuousRule{Activation: "tanh"},
		LIF: dynamics.LIFRule{
			TauSyn: 1, ThetaMin: .05, ThetaMax: 1, VReset: -.5,
			Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
	}
}

func nopanicVectorCore() dynamics.Config {
	return dynamics.Config{
		Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: .5, Activation: "tanh",
		StateDimension: 2, EdgeShape: "scalar",
	}
}

// nopanicProtocol is the validateable simulate fixture (an empty-topology LIF
// core, one injection, one probe and a uniform parameter source).
func nopanicProtocol() simulate.Protocol {
	return simulate.Protocol{
		SchemaVersion: simulate.ProtocolSchemaVersion,
		Core:          simulate.CoreLIF,
		LIF: &dynamics.LIFConfig{
			DT: 1, TauSyn: 1, ThetaMin: .5, ThetaMax: 1.5, VReset: -.5,
			Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
		Injections: []simulate.Injection{{Channel: 0, Node: 0, Gain: 1}},
		Probes: []simulate.Probe{
			{Name: "first", Nodes: []int{0}, Reduce: simulate.ReduceMeanOutput},
		},
		Stimulus:        simulate.StimulusSpec{Pulse: &simulate.Pulse{Channels: 1, Steps: 2, Channel: 0, Onset: 0, Duration: 1, Amplitude: 1}},
		ParameterSource: simulate.ParameterSourceUniform,
		Uniform:         &simulate.UniformParameters{Gain: 1, Bias: 0, LogTau: 0, ThetaRaw: 0},
	}
}

func TestPublicAPINeverPanics(t *testing.T) {
	tmp := t.TempDir()

	// Dynamics constructors: nil-or-zero, negative nodes, mismatched
	// sources/targets, out-of-range index, NaN dt, unknown activation/surrogate.
	continuous := []dynamics.Config{
		{},
		{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, Activation: "bogus"},
		{Nodes: 2, Sources: []int{0, 0}, Targets: []int{1}, DT: 1, Activation: "tanh"},
		{Nodes: 2, Sources: []int{9}, Targets: []int{1}, DT: 1, Activation: "tanh"},
		{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: math.NaN(), Activation: "tanh"},
	}
	lif := []dynamics.LIFConfig{
		{},
		func() dynamics.LIFConfig { c := nopanicLIFCore(); c.Nodes = 0; return c }(),
		func() dynamics.LIFConfig { c := nopanicLIFCore(); c.Nodes = -2; return c }(),
		func() dynamics.LIFConfig {
			c := nopanicLIFCore()
			c.Sources = []int{0, 0}
			c.Targets = []int{1}
			return c
		}(),
		func() dynamics.LIFConfig { c := nopanicLIFCore(); c.Sources = []int{9}; return c }(),
		func() dynamics.LIFConfig { c := nopanicLIFCore(); c.DT = math.NaN(); return c }(),
		func() dynamics.LIFConfig { c := nopanicLIFCore(); c.Surrogate.Kind = "bogus"; return c }(),
	}
	mixed := []dynamics.MixedConfig{
		{},
		func() dynamics.MixedConfig { c := nopanicMixedCore(); c.Nodes = -1; c.NodeRule = []uint8{0}; return c }(),
		func() dynamics.MixedConfig {
			c := nopanicMixedCore()
			c.Sources = []int{0, 0}
			c.Targets = []int{1}
			return c
		}(),
		func() dynamics.MixedConfig { c := nopanicMixedCore(); c.Sources = []int{9}; return c }(),
		func() dynamics.MixedConfig { c := nopanicMixedCore(); c.DT = math.NaN(); return c }(),
		func() dynamics.MixedConfig {
			c := nopanicMixedCore()
			c.NodeRule = []uint8{0, 0}
			c.Continuous.Activation = "bogus"
			c.LIF = dynamics.LIFRule{}
			return c
		}(),
	}
	vector := []dynamics.Config{
		{},
		func() dynamics.Config { c := nopanicVectorCore(); c.Nodes = 0; return c }(),
		func() dynamics.Config { c := nopanicVectorCore(); c.Nodes = -2; return c }(),
		func() dynamics.Config { c := nopanicVectorCore(); c.Targets = []int{1, 0}; return c }(),
		func() dynamics.Config { c := nopanicVectorCore(); c.Sources = []int{9}; return c }(),
		func() dynamics.Config { c := nopanicVectorCore(); c.DT = math.NaN(); return c }(),
		func() dynamics.Config { c := nopanicVectorCore(); c.Activation = "bogus"; return c }(),
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"dynamics/NewContinuous:zero-value config", func() error {
			_, err := dynamics.NewContinuous(continuous[0])
			return err
		}},
		{"dynamics/NewContinuous:Unknown activation", func() error {
			_, err := dynamics.NewContinuous(continuous[1])
			return err
		}},
		{"dynamics/NewContinuous:SourceTarget length mismatch", func() error {
			_, err := dynamics.NewContinuous(continuous[2])
			return err
		}},
		{"dynamics/NewContinuous:out-of-range index", func() error {
			_, err := dynamics.NewContinuous(continuous[3])
			return err
		}},
		{"dynamics/NewContinuous:NaN dt", func() error {
			_, err := dynamics.NewContinuous(continuous[4])
			return err
		}},
		{"dynamics/NewLIF:zero-value config", func() error {
			_, err := dynamics.NewLIF(lif[0])
			return err
		}},
		{"dynamics/NewLIF:zero nodes", func() error {
			_, err := dynamics.NewLIF(lif[1])
			return err
		}},
		{"dynamics/NewLIF:negative nodes", func() error {
			_, err := dynamics.NewLIF(lif[2])
			return err
		}},
		{"dynamics/NewLIF:SourceTarget length mismatch", func() error {
			_, err := dynamics.NewLIF(lif[3])
			return err
		}},
		{"dynamics/NewLIF:out-of-range index", func() error {
			_, err := dynamics.NewLIF(lif[4])
			return err
		}},
		{"dynamics/NewLIF:NaN dt", func() error {
			_, err := dynamics.NewLIF(lif[5])
			return err
		}},
		{"dynamics/NewLIF:unknown surrogate", func() error {
			_, err := dynamics.NewLIF(lif[6])
			return err
		}},
		{"dynamics/NewMixed:zero-value config", func() error {
			_, err := dynamics.NewMixed(mixed[0])
			return err
		}},
		{"dynamics/NewMixed:negative nodes", func() error {
			_, err := dynamics.NewMixed(mixed[1])
			return err
		}},
		{"dynamics/NewMixed:SourceTarget length mismatch", func() error {
			_, err := dynamics.NewMixed(mixed[2])
			return err
		}},
		{"dynamics/NewMixed:out-of-range index", func() error {
			_, err := dynamics.NewMixed(mixed[3])
			return err
		}},
		{"dynamics/NewMixed:NaN dt", func() error {
			_, err := dynamics.NewMixed(mixed[4])
			return err
		}},
		{"dynamics/NewMixed:unknown activation", func() error {
			_, err := dynamics.NewMixed(mixed[5])
			return err
		}},
		{"dynamics/NewVectorContinuous:zero-value config", func() error {
			_, err := dynamics.NewVectorContinuous(vector[0])
			return err
		}},
		{"dynamics/NewVectorContinuous:zero nodes", func() error {
			_, err := dynamics.NewVectorContinuous(vector[1])
			return err
		}},
		{"dynamics/NewVectorContinuous:negative nodes", func() error {
			_, err := dynamics.NewVectorContinuous(vector[2])
			return err
		}},
		{"dynamics/NewVectorContinuous:SourceTarget length mismatch", func() error {
			_, err := dynamics.NewVectorContinuous(vector[3])
			return err
		}},
		{"dynamics/NewVectorContinuous:out-of-range index", func() error {
			_, err := dynamics.NewVectorContinuous(vector[4])
			return err
		}},
		{"dynamics/NewVectorContinuous:NaN dt", func() error {
			_, err := dynamics.NewVectorContinuous(vector[5])
			return err
		}},
		{"dynamics/NewVectorContinuous:unknown activation", func() error {
			_, err := dynamics.NewVectorContinuous(vector[6])
			return err
		}},

		// dynamics Forward: nil params, wrong-length initial, empty inputs,
		// NaN inputs; Backward: nil trace, wrong upstream shape.
		{"dynamics/Continuous.Forward:nil parameters", func() error {
			m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"})
			if err != nil {
				return err
			}
			_, err = m.Forward(context.Background(), dynamics.Parameters{}, []float64{0}, [][]float64{{1}})
			return err
		}},
		{"dynamics/Continuous.Forward:wrong-length initial", func() error {
			m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"})
			if err != nil {
				return err
			}
			_, err = m.Forward(context.Background(), dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}}, []float64{0, 0}, [][]float64{{1}})
			return err
		}},
		{"dynamics/Continuous.Forward:empty inputs", func() error {
			m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"})
			if err != nil {
				return err
			}
			_, err = m.Forward(context.Background(), dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}}, []float64{0}, nil)
			return err
		}},
		{"dynamics/Continuous.Forward:NaN input", func() error {
			m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"})
			if err != nil {
				return err
			}
			_, err = m.Forward(context.Background(), dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}}, []float64{0}, [][]float64{{math.NaN()}})
			return err
		}},
		{"dynamics/Continuous.Backward:nil trace", func() error {
			m, err := dynamics.NewContinuous(nopanicContinuousCore())
			if err != nil {
				return err
			}
			_, err = m.Backward(context.Background(), nil, [][]float64{{.1, .2}, {.3, .4}}, 1)
			return err
		}},
		{"dynamics/Continuous.Backward:wrong upstream shape", func() error {
			m, err := dynamics.NewContinuous(nopanicContinuousCore())
			if err != nil {
				return err
			}
			tr, err := m.Forward(context.Background(), nopanicContinuousRates(), []float64{0, 0}, [][]float64{{0, 0}, {0, 0}})
			if err != nil {
				return err
			}
			_, err = m.Backward(context.Background(), tr, [][]float64{{.1}}, 1)
			return err
		}},

		// learning: constructors with invalid config/parameters, then the
		// invalid-input methods on valid fixtures.
		{"learning/NewNetwork:no core config", func() error {
			_, err := learning.NewNetwork(learning.Config{})
			return err
		}},
		{"learning/NewTrainer:wrong parameter shape", func() error {
			p := nopanicContinuousParams()
			p.Encoder = []float64{1, 2, 3}
			_, err := learning.NewTrainer(nopanicContinuousConfig(), p, learning.DefaultOptions())
			return err
		}},
		{"learning/NewTrainer:NaN parameter", func() error {
			p := nopanicContinuousParams()
			p.Core.LogTau = []float64{math.NaN(), 0}
			_, err := learning.NewTrainer(nopanicContinuousConfig(), p, learning.DefaultOptions())
			return err
		}},
		{"learning/NewTrainer:negative learning rate", func() error {
			_, err := learning.NewTrainer(nopanicContinuousConfig(), nopanicContinuousParams(), learning.Options{LearningRate: -1})
			return err
		}},
		{"learning/NewIndividual:wrong parameter shape", func() error {
			p := nopanicContinuousParams()
			p.Readout = []float64{1, 2}
			_, err := learning.NewIndividual(nopanicContinuousConfig(), p, learning.DefaultOptions(), []float64{0, 0})
			return err
		}},
		{"learning/NewIndividual:NaN parameter", func() error {
			p := nopanicContinuousParams()
			p.Core.Weights = []float64{math.NaN(), .2}
			_, err := learning.NewIndividual(nopanicContinuousConfig(), p, learning.DefaultOptions(), []float64{0, 0})
			return err
		}},
		{"learning/NewIndividual:negative learning rate", func() error {
			_, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.Options{LearningRate: -1}, []float64{0, 0})
			return err
		}},
		{"learning/NewIndividual:nil initial state", func() error {
			_, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), nil)
			return err
		}},
		{"learning/NewIndividual:wrong-length initial state", func() error {
			_, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0})
			return err
		}},
		{"learning/Trainer.Step:empty input", func() error {
			tr, err := learning.NewTrainer(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions())
			if err != nil {
				return err
			}
			_, err = tr.Step(context.Background(), nil, []float64{0})
			return err
		}},
		{"learning/Trainer.Step:wrong input width", func() error {
			tr, err := learning.NewTrainer(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions())
			if err != nil {
				return err
			}
			_, err = tr.Step(context.Background(), [][]float64{{0.5, 0.5}}, []float64{0})
			return err
		}},
		{"learning/Trainer.Step:NaN input", func() error {
			tr, err := learning.NewTrainer(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions())
			if err != nil {
				return err
			}
			_, err = tr.Step(context.Background(), [][]float64{{math.NaN()}}, []float64{0})
			return err
		}},
		{"learning/Trainer.Predict:empty input", func() error {
			tr, err := learning.NewTrainer(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions())
			if err != nil {
				return err
			}
			_, err = tr.Predict(context.Background(), nil)
			return err
		}},
		{"learning/Trainer.Predict:wrong input width", func() error {
			tr, err := learning.NewTrainer(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions())
			if err != nil {
				return err
			}
			_, err = tr.Predict(context.Background(), [][]float64{{0.5, 0.5}})
			return err
		}},
		{"learning/Trainer.Predict:NaN input", func() error {
			tr, err := learning.NewTrainer(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions())
			if err != nil {
				return err
			}
			_, err = tr.Predict(context.Background(), [][]float64{{math.NaN()}})
			return err
		}},
		{"learning/Individual.Advance:empty input", func() error {
			ind, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0, 0})
			if err != nil {
				return err
			}
			_, err = ind.Advance(context.Background(), nil)
			return err
		}},
		{"learning/Individual.AdvanceGated:empty input", func() error {
			ind, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0, 0})
			if err != nil {
				return err
			}
			_, _, err = ind.AdvanceGated(context.Background(), nil, []float64{0, 0})
			return err
		}},
		{"learning/Individual.TrainEpisode:empty input", func() error {
			ind, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0, 0})
			if err != nil {
				return err
			}
			_, err = ind.TrainEpisode(context.Background(), nil, []float64{0})
			return err
		}},
		{"learning/Individual.ResetNeural:nil initial", func() error {
			ind, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0, 0})
			if err != nil {
				return err
			}
			return ind.ResetNeural(context.Background(), nil)
		}},
		{"learning/Individual.ResetNeural:wrong length initial", func() error {
			ind, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0, 0})
			if err != nil {
				return err
			}
			return ind.ResetNeural(context.Background(), []float64{0})
		}},
		{"learning/Individual.Intervene:unauthorized plan", func() error {
			ind, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0, 0})
			if err != nil {
				return err
			}
			_, _, err = ind.Intervene(context.Background(), learning.InterventionPlan{}, nil)
			return err
		}},
		{"learning/Individual.EnablePlasticity:bad rule", func() error {
			ind, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0, 0})
			if err != nil {
				return err
			}
			return ind.EnablePlasticity(plasticity.Config{Rule: plasticity.Rule{Kind: "bogus"}, Edges: []int{0}})
		}},
		{"learning/Individual.EnableChemistry:bad config", func() error {
			ind, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0, 0})
			if err != nil {
				return err
			}
			return ind.EnableChemistry(modulation.ChemistryConfig{})
		}},
		{"learning/Individual.FreezeChemistry:not enabled", func() error {
			ind, err := learning.NewIndividual(nopanicContinuousConfig(), nopanicContinuousParams(), learning.DefaultOptions(), []float64{0, 0})
			if err != nil {
				return err
			}
			return ind.FreezeChemistry(true)
		}},
		{"learning/RestoreIndividual:empty snapshot", func() error {
			_, err := learning.RestoreIndividual(learning.IndividualSnapshot{})
			return err
		}},
		{"learning/RestoreIndividual:bad schema", func() error {
			_, err := learning.RestoreIndividual(learning.IndividualSnapshot{SchemaVersion: "bogus/v1"})
			return err
		}},

		// checkpoint.
		{"checkpoint/Load:nonexistent path", func() error {
			_, err := checkpoint.Load(context.Background(), filepath.Join(tmp, "missing.json"))
			return err
		}},
		{"checkpoint/LoadIndividual:nonexistent path", func() error {
			_, err := checkpoint.LoadIndividual(context.Background(), filepath.Join(tmp, "missing.json"))
			return err
		}},
		{"checkpoint/LoadIndividual:empty file", func() error {
			p := filepath.Join(tmp, "empty.json")
			if err := writeFile(p, ""); err != nil {
				return err
			}
			_, err := checkpoint.LoadIndividual(context.Background(), p)
			return err
		}},
		{"checkpoint/LoadIndividual:bad JSON", func() error {
			p := filepath.Join(tmp, "bad.json")
			if err := writeFile(p, "{not json"); err != nil {
				return err
			}
			_, err := checkpoint.LoadIndividual(context.Background(), p)
			return err
		}},
		{"checkpoint/LoadModelPackage:nonexistent path", func() error {
			_, err := checkpoint.LoadModelPackage(context.Background(), filepath.Join(tmp, "missing.json"))
			return err
		}},
		{"checkpoint/LoadModelPackage:empty file", func() error {
			p := filepath.Join(tmp, "empty-pkg.json")
			if err := writeFile(p, ""); err != nil {
				return err
			}
			_, err := checkpoint.LoadModelPackage(context.Background(), p)
			return err
		}},
		{"checkpoint/Migrate:same source and target", func() error {
			p := filepath.Join(tmp, "migrate-src.json")
			if err := writeFile(p, "{}"); err != nil {
				return err
			}
			_, err := checkpoint.Migrate(context.Background(), p, p, checkpoint.IndividualSchemaVersion)
			return err
		}},
		{"checkpoint/SaveIndividual:empty snapshot", func() error {
			return checkpoint.SaveIndividual(context.Background(), filepath.Join(tmp, "snap.json"), learning.IndividualSnapshot{})
		}},

		// simulate.
		{"simulate/Protocol.Validate:empty protocol", func() error {
			return simulate.Protocol{}.Validate()
		}},
		{"simulate/Protocol.Validate:negative pulse steps", func() error {
			p := nopanicProtocol()
			p.Stimulus.Pulse.Steps = -1
			return p.Validate()
		}},
		{"simulate/Protocol.Validate:NaN threshold", func() error {
			p := nopanicProtocol()
			p.Thresholds.MaxPopulationRate = math.NaN()
			return p.Validate()
		}},
		{"simulate/DecodeProtocol:bad JSON", func() error {
			_, err := simulate.DecodeProtocol(strings.NewReader("{bad"))
			return err
		}},

		// modulation.
		{"modulation/SourceSpec.Build:unknown kind", func() error {
			_, err := modulation.SourceSpec{Kind: "bogus"}.Build()
			return err
		}},
		{"modulation/SourceSpec.Build:two bodies", func() error {
			_, err := modulation.SourceSpec{
				Kind:     modulation.SourceExternalTimeline,
				Timeline: &modulation.ExternalTimeline{},
				Neural:   &modulation.NeuralActivity{},
			}.Build()
			return err
		}},
		{"modulation/ChemistryConfig.Validate:zero regions", func() error {
			c := modulation.ChemistryConfig{Chemistry: modulation.Chemistry{Regions: 0, Channels: 1, DT: 1, Tau: []float64{1}}}
			return c.Validate(1)
		}},
		{"modulation/Receptors.Validate:bad record", func() error {
			r := &modulation.Receptors{Records: []modulation.Receptor{{}}, Mix: modulation.MixSum}
			return r.Validate(2, 1)
		}},

		// plasticity (the rule validator is only reachable through New).
		{"plasticity/New:unknown rule kind", func() error {
			_, err := plasticity.New(plasticity.Config{Rule: plasticity.Rule{Kind: "bogus"}, Edges: []int{0}}, 2)
			return err
		}},
		{"plasticity/New:negative decay", func() error {
			_, err := plasticity.New(plasticity.Config{Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: -.5, DecayP: .9, PlasticMax: 1, WMin: .1}, Edges: []int{0}}, 2)
			return err
		}},
		{"plasticity/New:edge out of range", func() error {
			_, err := plasticity.New(plasticity.Config{Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .9, DecayP: .9, PlasticMax: 1, WMin: .1}, Edges: []int{5}}, 2)
			return err
		}},

		// experiment, nav2d, gridnav.
		{"experiment/ContinualProtocol.Validate:empty", func() error {
			return experiment.ContinualProtocol{}.Validate()
		}},
		{"experiment/RunContinualMatrix:nil build", func() error {
			_, err := experiment.RunContinualMatrix(context.Background(), experiment.ContinualProtocol{}, nil)
			return err
		}},
		{"experiment/AblationConfig.Validate:empty", func() error {
			return experiment.AblationConfig{}.Validate()
		}},
		{"nav2d.New:even width", func() error {
			_, err := nav2d.New(nav2d.Config{Width: 8, Height: 9, ViewDepth: 2, TimeLimit: 10, StepPenalty: .01, CollisionPenalty: .05, GoalReward: 1})
			return err
		}},
		{"gridnav.New:even length", func() error {
			_, err := gridnav.New(gridnav.Config{Length: 8, TimeLimit: 10, StepPenalty: 0, GoalReward: 1})
			return err
		}},

		// teacher.
		{"teacher/OfflineJSONL.Ask:nonexistent store path", func() error {
			store, err := teacher.OpenRecordStore(filepath.Join(tmp, "missing.jsonl"), nil)
			if err != nil {
				return err
			}
			req := teacher.Request{
				RequestID: "req-1", InputHash: strings.Repeat("a", 64),
				Kind: teacher.KindLabel, ModelVersion: "v1",
			}
			_, err = teacher.OfflineJSONL{Store: store, ID: "offline", Version: "v1"}.Ask(context.Background(), req)
			return err
		}},
		{"teacher/HTTP.Validate:empty endpoint", func() error {
			h := teacher.HTTP{}
			return h.Validate()
		}},
		{"teacher/OpenRecordStore:bad path", func() error {
			blocker := filepath.Join(tmp, "blocker")
			if err := writeFile(blocker, "x"); err != nil {
				return err
			}
			_, err := teacher.OpenRecordStore(filepath.Join(blocker, "records.jsonl"), nil)
			return err
		}},

		// distill.
		{"distill/Split.Validate:empty train", func() error {
			return distill.Split{}.Validate()
		}},
		{"distill/DistributionDistiller.Loss:empty student", func() error {
			d := distill.DistributionDistiller{Temperature: 1, Scale: 1, Mix: .5, TopK: 0}
			_, _, _, err := d.Loss(distill.TeacherDistribution{Probabilities: []float64{1}}, nil, 0)
			return err
		}},

		// multimodal.
		{"multimodal/Sample.Validate:empty sample", func() error {
			return multimodal.Sample{}.Validate()
		}},
		{"multimodal/Vector:missing modality", func() error {
			s := multimodal.Sample{
				EntityID: "e", EventID: "v",
				Modalities: map[string]multimodal.Modality{
					"a": {Present: true, Values: []float64{1}, Shape: []int{1}},
				},
				Timestamps: map[string]signal.Timestamp{"a": {Value: 0, Unit: signal.TimeUnitModelStep}},
			}
			l := multimodal.Layout{Order: []string{"a", "b"}, Widths: map[string]int{"a": 1, "b": 1}}
			_, err := multimodal.Vector(s, l)
			return err
		}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s panicked: %v", tc.name, r)
				}
			}()
			if err := tc.call(); err == nil {
				t.Errorf("%s accepted invalid input with a nil error", tc.name)
			}
		})
	}
}

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}
