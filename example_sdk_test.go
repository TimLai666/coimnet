package coimnet_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

// exConfig is the two-node continuous core every example builds on: one edge
// 0->1, scalar input and output, node 1 read out.
var exConfig = learning.Config{
	Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: .5, Activation: "tanh"},
	InputSize:    1,
	OutputSize:   1,
	ReadoutNodes: []int{1},
}

// exParameters is the fixed parameter set each example starts from.
var exParameters = learning.Parameters{
	Core:    dynamics.Parameters{Weights: []float64{.3}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
	Encoder: []float64{.5, .1},
	Readout: []float64{.8},
}

// Example_newContinuousCore demonstrates the 16.1 "build a core" capability: a
// two-node continuous core rolls two steps of its own dynamics.
func Example_newContinuousCore() {
	core, err := dynamics.NewContinuous(dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: .5, Activation: "tanh"})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	trace, err := core.Forward(context.Background(), dynamics.Parameters{Weights: []float64{.3}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}, []float64{0, 0}, [][]float64{{1, 0}, {1, 0}})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	out := trace.Outputs()
	fmt.Println(len(out), len(out[0]))
	fmt.Printf("%.6f\n", out[0][0])
	// Output:
	// 2 2
	// 0.374347
}

// Example_newIndividual demonstrates the 16.1 "build an individual and step it"
// capability: one two-node individual advances three rows and exposes the last
// readout.
func Example_newIndividual() {
	ind, err := learning.NewIndividual(exConfig, exParameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	out, err := ind.Advance(context.Background(), [][]float64{{1}, {0}, {1}})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%.6f\n", out[len(out)-1][0])
	// Output:
	// 0.065252
}

// Example_trainEpisode demonstrates the 16.1 "train" capability: five episodes
// of the same example, reading the first and the fifth pre-update losses. The
// loss falls as the episodes repeat.
func Example_trainEpisode() {
	options := learning.DefaultOptions()
	options.LearningRate = .1
	ind, err := learning.NewIndividual(exConfig, exParameters, options, []float64{0, 0})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	var first, final float64
	for i := 0; i < 5; i++ {
		result, err := ind.TrainEpisode(context.Background(), [][]float64{{1}, {0}, {1}}, []float64{.5})
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		if i == 0 {
			first = result.Loss
		}
		final = result.Loss
	}
	fmt.Printf("first=%.6f final=%.6f\n", first, final)
	// Output:
	// first=0.189006 final=0.048553
}

// Example_enablePlasticity demonstrates the 16.1 "register a learning rule"
// capability: a local hebbian-rate rule runs one gated advance.
func Example_enablePlasticity() {
	ind, err := learning.NewIndividual(exConfig, exParameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	rule := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .01}
	if err := ind.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		fmt.Println("error:", err)
		return
	}
	_, report, err := ind.AdvanceGated(context.Background(), [][]float64{{1}, {0}}, []float64{1, 1})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(report.Steps, ind.Snapshot().Plastic != nil)
	// Output:
	// 2 true
}

// Example_enableChemistry demonstrates the 16.1 "register modulation"
// capability: the minimal chemistry declaration from learning/chemistry_test.go
// releases one pulse and every advance row ticks the chemical clock.
func Example_enableChemistry() {
	ind, err := learning.NewIndividual(exConfig, exParameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	decl := modulation.ChemistryConfig{Chemistry: modulation.Chemistry{Regions: 2, Channels: 1, DT: 1, Tau: []float64{2}}}
	decl.Sources = []modulation.SourceSpec{{Kind: modulation.SourceExternalTimeline, Channel: 0, Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 0, Channel: 0, Rate: 1}}}}}
	decl.Receptors = modulation.Receptors{Records: []modulation.Receptor{{Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: .5, N: 1, Evidence: "example", MeasurementKind: "declared", MappingVersion: "example/v1"}}}
	decl.Regions = modulation.RegionAssignment{NodeRegion: []int{0, 1}}
	if err := ind.EnableChemistry(decl); err != nil {
		fmt.Println("error:", err)
		return
	}
	if _, err := ind.Advance(context.Background(), [][]float64{{1}, {0}}); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(ind.ChemistryReport().Steps)
	// Output:
	// 2
}

// Example_intervene demonstrates the 16.1 "intervene" capability: an authorized
// plan clamps one node's voltage and leaves base parameters untouched.
func Example_intervene() {
	ind, err := learning.NewIndividual(exConfig, exParameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	plan := learning.InterventionPlan{Authorized: true, Reason: "example probe", Items: []learning.Intervention{{Kind: learning.InterventionClampVoltage, Targets: []int{1}, Start: 0, End: 2}}}
	_, log, err := ind.Intervene(context.Background(), plan, [][]float64{{1}, {0}})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	entry := log.Entries[0]
	fmt.Println(entry.AppliedRows, entry.BaseParameterDelta)
	// Output:
	// 2 0
}

// Example_snapshotRestore demonstrates the 16.1 "save and restore" capability:
// the snapshot and its restore advance the same rows with identical readouts.
func Example_snapshotRestore() {
	ind, err := learning.NewIndividual(exConfig, exParameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	restored, err := learning.RestoreIndividual(ind.Snapshot())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	a, err := ind.Advance(context.Background(), [][]float64{{1}, {0}})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	b, err := restored.Advance(context.Background(), [][]float64{{1}, {0}})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(reflect.DeepEqual(a, b))
	// Output:
	// true
}

// Example_observeLocalState demonstrates the 16.1 "observe local state"
// capability: the snapshot names the neural core, its node count and the total
// parameter count of the continuous model.
func Example_observeLocalState() {
	ind, err := learning.NewIndividual(exConfig, exParameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	snap := ind.Snapshot()
	fmt.Println(snap.Neural.Core, len(snap.Neural.Continuous.Voltage))
	p := snap.Parameters
	fmt.Println(len(p.Core.Weights) + len(p.Core.Bias) + len(p.Core.LogTau) + len(p.Encoder) + len(p.Readout))
	// Output:
	// continuous 2
	// 8
}
