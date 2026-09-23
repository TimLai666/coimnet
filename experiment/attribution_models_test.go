package experiment

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment/nav2d"
)

// attributionTestSeed, inputs, hidden, recurrent and rate are the fixed
// protocol of ticket 28's attribution model construction tests.
const (
	attributionTestSeed      = uint64(1)
	attributionTestInputs    = 40
	attributionTestHidden    = 16
	attributionTestRecurrent = 4
	attributionTestRate      = 0.05
)

// TestAttributionGroupsParameterTable builds all seven groups with the fixed
// protocol parameters and pins the hand-computed parameter table the
// attribution claim is read from: E = 640 input->hidden + 64 recurrent +
// 64 hidden->readout + 160 input->readout = 928 edges, 60 nodes, a (16+4)x4
// = 80 readout and a 40x40 encoder. Every checked number is written out, so a
// change in any construction rule shows up here first.
func TestAttributionGroupsParameterTable(t *testing.T) {
	groupOrder := []string{
		AttributionNormal,
		AttributionFrozenCore,
		AttributionCoreOnly,
		AttributionGenericMatched,
		AttributionRewired,
		AttributionAblatedRetrained,
		AttributionCapacityMatchedModulator,
	}
	groups := AttributionGroups()
	if !reflect.DeepEqual(groups, groupOrder) {
		t.Fatalf("AttributionGroups() = %v, want %v", groups, groupOrder)
	}
	groups[0] = "tampered"
	if again := AttributionGroups(); again[0] != AttributionNormal || len(again) != len(groupOrder) {
		t.Fatalf("AttributionGroups() shared its backing array: a fresh call starts at %q", again[0])
	}

	type want struct {
		nodes, edges, parameters, free int
		trainable                      []string
		rewire                         bool
	}
	wantModel := map[string]want{
		AttributionNormal:                   {nodes: 60, edges: 928, parameters: 2728, free: 2608, trainable: []string{"weights", "encoder", "readout"}},
		AttributionFrozenCore:               {nodes: 60, edges: 928, parameters: 2728, free: 1680, trainable: []string{"encoder", "readout"}},
		AttributionCoreOnly:                 {nodes: 60, edges: 928, parameters: 2728, free: 928, trainable: []string{"weights"}},
		AttributionGenericMatched:           {nodes: 60, edges: 928, parameters: 2728, free: 2608, trainable: []string{"weights", "encoder", "readout"}},
		AttributionRewired:                  {nodes: 60, edges: 928, parameters: 2728, free: 2608, trainable: []string{"weights", "encoder", "readout"}, rewire: true},
		AttributionAblatedRetrained:         {nodes: 4, edges: 0, parameters: 184, free: 176, trainable: []string{"encoder", "readout"}},
		AttributionCapacityMatchedModulator: {nodes: 60, edges: 928, parameters: 2752, free: 2632, trainable: []string{"weights", "encoder", "readout"}},
	}

	policies := map[string]*nav2dPolicy{}
	models := map[string]AttributionModel{}
	for _, group := range groupOrder {
		p, model, err := newAttributionPolicy(group, attributionTestSeed, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate)
		if err != nil {
			t.Fatalf("newAttributionPolicy(%q): %v", group, err)
		}
		if p == nil || p.trainer == nil {
			t.Fatalf("newAttributionPolicy(%q) returned no trainer", group)
		}
		policies[group] = p
		models[group] = model
		w := wantModel[group]
		if model.Group != group {
			t.Errorf("%s: model.Group = %q", group, model.Group)
		}
		if model.Nodes != w.nodes {
			t.Errorf("%s: nodes %d, want %d", group, model.Nodes, w.nodes)
		}
		if model.Edges != w.edges {
			t.Errorf("%s: edges %d, want %d", group, model.Edges, w.edges)
		}
		if model.Parameters != w.parameters {
			t.Errorf("%s: parameters %d, want %d", group, model.Parameters, w.parameters)
		}
		if model.FreeParameters != w.free {
			t.Errorf("%s: free_parameters %d, want %d", group, model.FreeParameters, w.free)
		}
		if !reflect.DeepEqual(model.Trainable, w.trainable) {
			t.Errorf("%s: trainable %v, want %v", group, model.Trainable, w.trainable)
		}
		if (model.Rewire != nil) != w.rewire {
			t.Errorf("%s: rewire set = %v, want %v", group, model.Rewire != nil, w.rewire)
		}
		if p.edges != model.Edges {
			t.Errorf("%s: policy exposes %d edges, model declares %d", group, p.edges, model.Edges)
		}
		if gotParams, _ := p.parameters(); gotParams != model.Parameters {
			t.Errorf("%s: trainer holds %d parameters, model declares %d", group, gotParams, model.Parameters)
		}
	}

	capacity := models[AttributionCapacityMatchedModulator]
	if capacity.ModulatorParameters != 21 {
		t.Errorf("capacity: modulator_parameters %d, want 21 (= ablationController(1, 4).ParameterCount())", capacity.ModulatorParameters)
	}
	if capacity.AddOnParameters != 24 {
		t.Errorf("capacity: add_on_parameters %d, want 24 (k x nav2d.Actions, k = ceil(21/4) = 6)", capacity.AddOnParameters)
	}
	cfg := policies[AttributionCapacityMatchedModulator].trainer.Snapshot().Config
	if want := attributionTestHidden + nav2d.Actions + 6; len(cfg.ReadoutNodes) != want {
		t.Errorf("capacity: %d readout nodes, want %d (normal 16+4 plus the 6 input add-on rows)", len(cfg.ReadoutNodes), want)
	}

	rewired := models[AttributionRewired]
	if rewired.Rewire == nil {
		t.Fatalf("rewired group carries no rewire report")
	}
	if rewired.Rewire.Edges != 64 {
		t.Errorf("rewired report considers %d hidden->hidden edges, want 64", rewired.Rewire.Edges)
	}
	if rewired.Rewire.Duplicates != 0 || !rewired.Rewire.DegreesKept || !rewired.Rewire.DirectionKept || !rewired.Rewire.ReadoutReachable {
		t.Errorf("rewired report flags a check failure: %+v", *rewired.Rewire)
	}

	ablated := policies[AttributionAblatedRetrained].trainer.Snapshot().Config
	if !reflect.DeepEqual(ablated.InputNodes, []int{0, 1, 2, 3}) || !reflect.DeepEqual(ablated.ReadoutNodes, []int{0, 1, 2, 3}) {
		t.Errorf("ablated: input/readout nodes %v / %v, want the four action nodes", ablated.InputNodes, ablated.ReadoutNodes)
	}

	t.Logf("group\tnodes\tedges\tparams\tfree\tmod\taddon\t\ttrainable")
	for _, group := range groupOrder {
		m := models[group]
		t.Logf("%s\t%d\t%d\t%d\t%d\t%d\t%d\t\t%v", m.Group, m.Nodes, m.Edges, m.Parameters, m.FreeParameters, m.ModulatorParameters, m.AddOnParameters, m.Trainable)
	}
}

// TestAttributionGenericMatchedShape pins the random-topology group: the same
// edge count as the normal group, distinct (source, target) pairs, sources
// drawn below the readout block and targets above the input block, bit-identical
// reproduction on the same seed and a different graph on a different seed.
func TestAttributionGenericMatchedShape(t *testing.T) {
	p, model, err := newAttributionPolicy(AttributionGenericMatched, attributionTestSeed, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate)
	if err != nil {
		t.Fatalf("newAttributionPolicy(generic_matched): %v", err)
	}
	cfg := p.trainer.Snapshot().Config
	sources, targets := cfg.Dynamics.Sources, cfg.Dynamics.Targets

	normal, _, err := newAttributionPolicy(AttributionNormal, attributionTestSeed, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate)
	if err != nil {
		t.Fatalf("newAttributionPolicy(normal): %v", err)
	}
	normSources, normTargets := normal.trainer.Snapshot().Config.Dynamics.Sources, normal.trainer.Snapshot().Config.Dynamics.Targets
	if len(sources) != len(normSources) || len(targets) != len(normTargets) {
		t.Fatalf("generic_matched has %d/%d edges, normal has %d/%d", len(sources), len(targets), len(normSources), len(normTargets))
	}
	if model.Edges != len(sources) {
		t.Errorf("generic_matched model declares %d edges, topology has %d", model.Edges, len(sources))
	}
	readoutFirst := attributionTestInputs + attributionTestHidden
	seen := map[[2]int]bool{}
	for i := range sources {
		key := [2]int{sources[i], targets[i]}
		if seen[key] {
			t.Fatalf("generic_matched pair %v repeats", key)
		}
		seen[key] = true
		if sources[i] >= readoutFirst {
			t.Errorf("generic_matched source %d is a readout node (>= %d)", sources[i], readoutFirst)
		}
		if targets[i] < attributionTestInputs {
			t.Errorf("generic_matched target %d is an input node (< %d)", targets[i], attributionTestInputs)
		}
	}

	twice, _, err := newAttributionPolicy(AttributionGenericMatched, attributionTestSeed, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate)
	if err != nil {
		t.Fatalf("second same-seed build: %v", err)
	}
	twiceCfg := twice.trainer.Snapshot().Config
	if !reflect.DeepEqual(sources, twiceCfg.Dynamics.Sources) || !reflect.DeepEqual(targets, twiceCfg.Dynamics.Targets) {
		t.Fatal("generic_matched differs between two builds of the same seed")
	}

	other, _, err := newAttributionPolicy(AttributionGenericMatched, attributionTestSeed+1, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate)
	if err != nil {
		t.Fatalf("different-seed build: %v", err)
	}
	otherCfg := other.trainer.Snapshot().Config
	if reflect.DeepEqual(sources, otherCfg.Dynamics.Sources) && reflect.DeepEqual(targets, otherCfg.Dynamics.Targets) {
		t.Fatal("generic_matched built the same graph for two different seeds")
	}
}

// TestAttributionTrainableGroupsHold trains every group for two episodes and
// proves the freeze contract on the trainer's own parameters: every array that
// is not in the group's Trainable list stays bit-identical (in particular
// bias and log_tau never move, and frozen_core's weights / core_only's
// encoder and readout hold), and every trainable array changes at least one
// value.
func TestAttributionTrainableGroupsHold(t *testing.T) {
	ctx := context.Background()
	env := nav2d.Config{Task: nav2d.TaskAvoidObstacles}
	for _, group := range AttributionGroups() {
		p, model, err := newAttributionPolicy(group, attributionTestSeed, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate)
		if err != nil {
			t.Fatalf("%s newAttributionPolicy: %v", group, err)
		}
		before := p.trainer.Snapshot().Parameters
		if _, err := trainNav2D(ctx, p, env, 1, 2); err != nil {
			t.Fatalf("%s trainNav2D: %v", group, err)
		}
		after := p.trainer.Snapshot().Parameters
		inTrainable := map[string]bool{}
		for _, name := range model.Trainable {
			inTrainable[name] = true
		}
		check := func(name string, oldV, newV []float64) {
			if len(oldV) != len(newV) {
				t.Errorf("%s %s length changed %d -> %d", group, name, len(oldV), len(newV))
				return
			}
			changed := false
			for i := range oldV {
				if oldV[i] != newV[i] {
					changed = true
					break
				}
			}
			if inTrainable[name] {
				if !changed && len(oldV) > 0 {
					t.Errorf("%s trainable %s is bit-identical after training", group, name)
				}
			} else if changed {
				t.Errorf("%s frozen %s changed values after training", group, name)
			}
		}
		check("weights", before.Core.Weights, after.Core.Weights)
		check("encoder", before.Encoder, after.Encoder)
		check("readout", before.Readout, after.Readout)
		check("bias", before.Core.Bias, after.Core.Bias)
		check("log_tau", before.Core.LogTau, after.Core.LogTau)
	}
}

// TestAttributionAddOnStartsSilent proves the capacity-matched add-on is inert
// on construction: the added readout rows are zeros, so before any training
// the normal and the capacity_matched_modulator policies of the same seed
// produce the same logits on one observation sequence and the same argmax at
// every step.
func TestAttributionAddOnStartsSilent(t *testing.T) {
	normal, _, err := newAttributionPolicy(AttributionNormal, attributionTestSeed, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate)
	if err != nil {
		t.Fatalf("newAttributionPolicy(normal): %v", err)
	}
	capacity, cm, err := newAttributionPolicy(AttributionCapacityMatchedModulator, attributionTestSeed, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate)
	if err != nil {
		t.Fatalf("newAttributionPolicy(capacity_matched_modulator): %v", err)
	}
	if cm.AddOnParameters == 0 {
		t.Fatal("capacity-matched model declares no add-on")
	}
	flat := nav2dUniform(3, 11, 6*attributionTestInputs)
	input := make([][]float64, 6)
	for r := range input {
		input[r] = flat[r*attributionTestInputs : (r+1)*attributionTestInputs]
	}
	ctx := context.Background()
	normalOut, err := normal.trainer.PredictAll(ctx, input)
	if err != nil {
		t.Fatalf("normal PredictAll: %v", err)
	}
	capacityOut, err := capacity.trainer.PredictAll(ctx, input)
	if err != nil {
		t.Fatalf("capacity PredictAll: %v", err)
	}
	if len(normalOut) != len(capacityOut) {
		t.Fatalf("logit row counts differ: %d vs %d", len(normalOut), len(capacityOut))
	}
	for r := range normalOut {
		for j := range normalOut[r] {
			if d := math.Abs(normalOut[r][j] - capacityOut[r][j]); d >= 1e-6 {
				t.Errorf("row %d action %d logit %v vs %v (diff %g)", r, j, normalOut[r][j], capacityOut[r][j], d)
			}
		}
		if nav2dArgmax(normalOut[r]) != nav2dArgmax(capacityOut[r]) {
			t.Errorf("row %d argmax %d vs %d", r, nav2dArgmax(normalOut[r]), nav2dArgmax(capacityOut[r]))
		}
	}
}

// TestAttributionGroupsShareTheLearningRate builds all seven groups with the
// fixed protocol rate 0.05 and checks that every trainer's snapshot carries
// that rate, so no group silently trains at DefaultOptions' 0.01 instead.
func TestAttributionGroupsShareTheLearningRate(t *testing.T) {
	for _, group := range AttributionGroups() {
		p, _, err := newAttributionPolicy(group, attributionTestSeed, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate)
		if err != nil {
			t.Fatalf("newAttributionPolicy(%q): %v", group, err)
		}
		got := p.trainer.Snapshot().Options.LearningRate
		if got != attributionTestRate {
			t.Errorf("%s: LearningRate = %v, want %v", group, got, attributionTestRate)
		}
	}
}

// TestAttributionRejects checks the construction errors: an unknown group, a
// missing hidden block, a recurrent count above the hidden block, a non-positive
// learning rate and a capacity-matched add-on wider than the inputs.
func TestAttributionRejects(t *testing.T) {
	cases := []struct {
		name                   string
		group                  string
		inputs, hidden, recurr int
		rate                   float64
	}{
		{"unknown group", "dream_team", attributionTestInputs, attributionTestHidden, attributionTestRecurrent, attributionTestRate},
		{"hidden zero", AttributionNormal, attributionTestInputs, 0, attributionTestRecurrent, attributionTestRate},
		{"recurrent above hidden", AttributionNormal, attributionTestInputs, attributionTestHidden, attributionTestHidden + 1, attributionTestRate},
		{"rate zero", AttributionNormal, attributionTestInputs, attributionTestHidden, attributionTestRecurrent, 0},
		{"add-on wider than inputs", AttributionCapacityMatchedModulator, 5, attributionTestHidden, attributionTestRecurrent, attributionTestRate},
	}
	for _, tc := range cases {
		p, model, err := newAttributionPolicy(tc.group, attributionTestSeed, tc.inputs, tc.hidden, tc.recurr, tc.rate)
		if err == nil {
			t.Errorf("%s: got policy %v and model %+v, want an error", tc.name, p != nil, model)
		}
		if p != nil {
			t.Errorf("%s: returned a policy alongside the error", tc.name)
		}
	}
}
