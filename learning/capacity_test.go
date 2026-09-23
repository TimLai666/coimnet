package learning_test

import (
	"context"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// vectorC3Config declares two vector nodes of component width three joined by
// one matrix edge, the C = 3 fixture the COR-06 capacity constants are pinned
// against.
func vectorC3Config() learning.Config {
	return learning.Config{
		Dynamics: dynamics.Config{
			Nodes:          2,
			Sources:        []int{0},
			Targets:        []int{1},
			Delays:         []int{0},
			DT:             .5,
			Activation:     "tanh",
			StateDimension: 3,
			EdgeShape:      "matrix",
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
}

// vectorC3Parameters matches vectorC3Config: weights E*C*C = 9, bias N*C = 6,
// log_tau N = 2, encoder InputSize*(N*C) = 6 and readout len(Readout)*C*Output
// = 3, for 26 learnable values in total.
func vectorC3Parameters() learning.Parameters {
	return learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.3, -.1, .2, .15, .05, -.2, .1, .4, -.3},
			Bias:    []float64{.1, .2, .3, -.1, -.2, -.3},
			LogTau:  []float64{.05, -.05},
		},
		Encoder: []float64{.7, -.2, .3, .6, .1, -.4},
		Readout: []float64{.8, -.4, .2},
	}
}

// TestCapacityScalarCore pins the scalar report on the three-node, two-edge
// binding fixture: 2 weights + 3 bias + 3 log_tau + 4 encoder + 1 readout = 13
// parameters, E + N = 5 multiply-adds per step, and every parameter is free
// when no mask and no disabled group applies.
func TestCapacityScalarCore(t *testing.T) {
	c, p, candidates, input, output := bindingFixture(t)
	cfg, params, err := learning.BindProjections(c, p, candidates, input, output)
	if err != nil {
		t.Fatal(err)
	}
	network, err := learning.NewNetwork(cfg)
	if err != nil {
		t.Fatal(err)
	}
	report := network.Capacity(params)
	if report.Nodes != 3 || report.Edges != 2 {
		t.Fatalf("topology = %d nodes %d edges, want 3 and 2", report.Nodes, report.Edges)
	}
	if report.StateDimension != 1 {
		t.Fatalf("state dimension = %d, want 1", report.StateDimension)
	}
	if report.ParameterCount != 13 {
		t.Fatalf("parameter count = %d, want 13 (2 weights + 3 bias + 3 log_tau + 4 encoder + 1 readout)", report.ParameterCount)
	}
	if report.FreeParameterCount != report.ParameterCount {
		t.Fatalf("free = %d, want every parameter (%d)", report.FreeParameterCount, report.ParameterCount)
	}
	if report.MultAddsPerStep != 5 {
		t.Fatalf("multiply-adds = %d, want 5 (2 edges + 3 nodes)", report.MultAddsPerStep)
	}
	trainer, err := learning.NewTrainer(cfg, params, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	fromTrainer := trainer.Capacity()
	if fromTrainer != report {
		t.Fatalf("trainer capacity %+v differs from network capacity %+v", fromTrainer, report)
	}
	if fromTrainer.FreeParameterCount != fromTrainer.ParameterCount {
		t.Fatalf("trainer free = %d, want %d with no masks", fromTrainer.FreeParameterCount, fromTrainer.ParameterCount)
	}
}

// TestCapacityVectorMatrixC3 pins the C = 3 matrix report: 9 weights + 6 bias
// + 2 log_tau + 6 encoder + 3 readout = 26 parameters and one nine-multiply-add
// matrix edge plus a three-multiply-add leak per node, 1*9 + 2*3 = 15
// multiply-adds per step.
func TestCapacityVectorMatrixC3(t *testing.T) {
	cfg := vectorC3Config()
	params := vectorC3Parameters()
	if len(params.Encoder) != 6 || len(params.Readout) != 3 {
		t.Fatalf("fixture drifted: encoder %d readout %d, want 6 and 3", len(params.Encoder), len(params.Readout))
	}
	network, err := learning.NewNetwork(cfg)
	if err != nil {
		t.Fatal(err)
	}
	report := network.Capacity(params)
	if report.Nodes != 2 || report.Edges != 1 {
		t.Fatalf("topology = %d nodes %d edges, want 2 and 1", report.Nodes, report.Edges)
	}
	if report.StateDimension != 3 {
		t.Fatalf("state dimension = %d, want 3", report.StateDimension)
	}
	if report.ParameterCount != 26 {
		t.Fatalf("parameter count = %d, want 26 (9 + 6 + 2 + 6 + 3)", report.ParameterCount)
	}
	if report.FreeParameterCount != report.ParameterCount {
		t.Fatalf("free = %d, want every parameter (%d)", report.FreeParameterCount, report.ParameterCount)
	}
	if report.MultAddsPerStep != 15 {
		t.Fatalf("multiply-adds = %d, want 15 (1*9 + 2*3)", report.MultAddsPerStep)
	}
	trainer, err := learning.NewTrainer(cfg, params, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := trainer.Capacity(); got != report {
		t.Fatalf("trainer capacity %+v differs from network capacity %+v", got, report)
	}
}

// TestCapacityCountsMasks walks the free count down on the C = 3 matrix
// fixture: freezing the single edge removes its nine weight values, and
// masking node 1 removes that node's three bias components and its one
// log_tau. One Step then proves the mapping: the nine masked weights stay
// bit-identical while every unmasked group moves.
func TestCapacityCountsMasks(t *testing.T) {
	cfg := vectorC3Config()
	params := vectorC3Parameters()

	base := learning.DefaultOptions()
	plain, err := learning.NewTrainer(cfg, params, base)
	if err != nil {
		t.Fatal(err)
	}
	if free := plain.Capacity().FreeParameterCount; free != 26 {
		t.Fatalf("unmasked free = %d, want 26", free)
	}

	edgeFrozen := base
	edgeFrozen.Masks = &learning.UpdateMasks{Edges: []bool{false}}
	edgeTrainer, err := learning.NewTrainer(cfg, params, edgeFrozen)
	if err != nil {
		t.Fatal(err)
	}
	if free := edgeTrainer.Capacity().FreeParameterCount; free != 26-9 {
		t.Fatalf("edge-masked free = %d, want 17 (26 - 9 matrix weights)", free)
	}

	nodeMasked := base
	nodeMasked.Masks = &learning.UpdateMasks{Edges: []bool{false}, Nodes: []bool{true, false}}
	masked, err := learning.NewTrainer(cfg, params, nodeMasked)
	if err != nil {
		t.Fatal(err)
	}
	if free := masked.Capacity().FreeParameterCount; free != 26-9-3-1 {
		t.Fatalf("fully masked free = %d, want 13 (26 - 9 - 3 bias - 1 log_tau)", free)
	}

	before := masked.Snapshot().Parameters
	if _, err := masked.Step(context.Background(), [][]float64{{1}, {.5}}, []float64{.8}); err != nil {
		t.Fatal(err)
	}
	after := masked.Snapshot().Parameters
	for i := range before.Core.Weights {
		if after.Core.Weights[i] != before.Core.Weights[i] {
			t.Fatalf("masked matrix weight %d moved: %.17g != %.17g", i, after.Core.Weights[i], before.Core.Weights[i])
		}
	}
	for i := 3; i < 6; i++ {
		if after.Core.Bias[i] != before.Core.Bias[i] {
			t.Fatalf("masked node-1 bias %d moved: %.17g != %.17g", i, after.Core.Bias[i], before.Core.Bias[i])
		}
	}
	if after.Core.LogTau[1] != before.Core.LogTau[1] {
		t.Fatalf("masked node-1 log_tau moved: %.17g != %.17g", after.Core.LogTau[1], before.Core.LogTau[1])
	}
	moved := func(a, b []float64) bool {
		for i := range a {
			if a[i] != b[i] {
				return true
			}
		}
		return false
	}
	if !moved(before.Core.Bias[:3], after.Core.Bias[:3]) {
		t.Fatal("unmasked node-0 bias did not move in one step")
	}
	if before.Core.LogTau[0] == after.Core.LogTau[0] {
		t.Fatal("unmasked node-0 log_tau did not move in one step")
	}
	if !moved(before.Encoder, after.Encoder) {
		t.Fatal("encoder did not move in one step")
	}
	if !moved(before.Readout, after.Readout) {
		t.Fatal("readout did not move in one step")
	}
}

// TestVectorCoreRefusesEdgeSigns pins Root decision 6: a fixed-sign
// parametrisation is defined per scalar edge, so a vector-state model with a
// nonzero sign declaration is refused by name, while an all-zero declaration
// stays the same model as no declaration at all.
func TestVectorCoreRefusesEdgeSigns(t *testing.T) {
	signed := vectorC3Config()
	signed.EdgeSigns = []int8{1}
	_, err := learning.NewTrainer(signed, vectorC3Parameters(), learning.DefaultOptions())
	if err == nil {
		t.Fatal("NewTrainer accepted fixed edge signs on vector state edges")
	}
	if !strings.Contains(err.Error(), "fixed edge signs are not defined for vector state edges") {
		t.Fatalf("refusal %q does not name the rule", err)
	}
	unused := vectorC3Config()
	unused.EdgeSigns = []int8{0}
	if _, err := learning.NewTrainer(unused, vectorC3Parameters(), learning.DefaultOptions()); err != nil {
		t.Fatalf("an all-zero sign declaration must stay legal: %v", err)
	}
}
