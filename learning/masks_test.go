package learning_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// maskFixture is the two-node, two-edge continuous model every mask test
// drives. The flat optimizer layout is weights[0:2], bias[2:4], log_tau[4:6],
// encoder[6:8], readout[8:9]; the constants below name the two halves of each
// core group so a test never hard-codes an offset twice.
const (
	maskEdges = 2
	maskNodes = 2
)

func maskIndex(group string, item int) int {
	switch group {
	case "weights":
		return item
	case "bias":
		return maskEdges + item
	case "log_tau":
		return maskEdges + maskNodes + item
	}
	panic("unknown group " + group)
}

func maskInput() ([][]float64, []float64) {
	return [][]float64{{.7}, {-.2}, {.1}}, []float64{.4}
}

// TestPerItemMasksFreezeOneEdgeAndOneNodeThroughWeightDecay pins root decision
// 1: the effective mask is the group flag AND the per-item mask, and a masked
// parameter keeps its value, its Adam moments and its step count bit-identical
// across many updates even with a nonzero weight decay.
func TestPerItemMasksFreezeOneEdgeAndOneNodeThroughWeightDecay(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	o := learning.DefaultOptions()
	o.WeightDecay = .1
	o.Masks = &learning.UpdateMasks{Edges: []bool{false, true}, Nodes: []bool{false, true}}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	x, target := maskInput()
	for step := range 50 {
		if _, err := tr.Step(context.Background(), x, target); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
	}
	s := tr.Snapshot()
	frozen := map[string]struct {
		got, want float64
		index     int
	}{
		"weights[0]": {s.Parameters.Core.Weights[0], p.Core.Weights[0], maskIndex("weights", 0)},
		"bias[0]":    {s.Parameters.Core.Bias[0], p.Core.Bias[0], maskIndex("bias", 0)},
		"log_tau[0]": {s.Parameters.Core.LogTau[0], p.Core.LogTau[0], maskIndex("log_tau", 0)},
	}
	for name, f := range frozen {
		if f.got != f.want {
			t.Fatalf("%s moved: %.17g != %.17g", name, f.got, f.want)
		}
		if s.Optimizer.First[f.index] != 0 || s.Optimizer.Second[f.index] != 0 || s.Optimizer.Steps[f.index] != 0 {
			t.Fatalf("%s accrued optimizer state: first=%g second=%g steps=%d",
				name, s.Optimizer.First[f.index], s.Optimizer.Second[f.index], s.Optimizer.Steps[f.index])
		}
	}
	moving := map[string]struct {
		got, want float64
		index     int
	}{
		"weights[1]": {s.Parameters.Core.Weights[1], p.Core.Weights[1], maskIndex("weights", 1)},
		"bias[1]":    {s.Parameters.Core.Bias[1], p.Core.Bias[1], maskIndex("bias", 1)},
		"log_tau[1]": {s.Parameters.Core.LogTau[1], p.Core.LogTau[1], maskIndex("log_tau", 1)},
	}
	for name, m := range moving {
		if m.got == m.want {
			t.Fatalf("%s did not move from %.17g", name, m.want)
		}
		if s.Optimizer.Steps[m.index] != 50 {
			t.Fatalf("%s took %d steps, want 50", name, s.Optimizer.Steps[m.index])
		}
	}
	if s.Updates != 50 {
		t.Fatalf("trainer recorded %d updates", s.Updates)
	}
}

// TestGroupFlagAndPerItemMaskBothHaveToAllowAnUpdate keeps the conjunction
// explicit in both directions.
func TestGroupFlagAndPerItemMaskBothHaveToAllowAnUpdate(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	x, target := maskInput()
	for _, tc := range []struct {
		name    string
		mutate  func(o *learning.Options)
		updates bool
	}{
		{"group off, item on", func(o *learning.Options) {
			o.Trainable.Weights = false
			o.Masks = &learning.UpdateMasks{Edges: []bool{true, true}}
		}, false},
		{"group on, item off", func(o *learning.Options) {
			o.Masks = &learning.UpdateMasks{Edges: []bool{false, false}}
		}, false},
		{"group on, item on", func(o *learning.Options) {
			o.Masks = &learning.UpdateMasks{Edges: []bool{true, true}}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := learning.DefaultOptions()
			tc.mutate(&o)
			tr, err := learning.NewTrainer(c, p, o)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tr.Step(context.Background(), x, target); err != nil {
				t.Fatal(err)
			}
			s := tr.Snapshot()
			moved := s.Parameters.Core.Weights[0] != p.Core.Weights[0] || s.Parameters.Core.Weights[1] != p.Core.Weights[1]
			if moved != tc.updates {
				t.Fatalf("weights moved=%v, want %v (%v -> %v)", moved, tc.updates, p.Core.Weights, s.Parameters.Core.Weights)
			}
		})
	}
}

// TestMaskLengthsAreValidatedAgainstTheTopology covers construction and
// restore, because a snapshot carries the masks with the options.
func TestMaskLengthsAreValidatedAgainstTheTopology(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	for name, masks := range map[string]*learning.UpdateMasks{
		"short edges": {Edges: []bool{true}},
		"long edges":  {Edges: []bool{true, true, true}},
		"short nodes": {Nodes: []bool{true}},
		"long nodes":  {Nodes: []bool{true, true, true}},
	} {
		o := learning.DefaultOptions()
		o.Masks = masks
		if _, err := learning.NewTrainer(c, p, o); err == nil {
			t.Fatalf("NewTrainer accepted %s", name)
		}
	}
	good := learning.DefaultOptions()
	good.Masks = &learning.UpdateMasks{Edges: []bool{true, false}, Nodes: []bool{false, true}}
	tr, err := learning.NewTrainer(c, p, good)
	if err != nil {
		t.Fatal(err)
	}
	s := tr.Snapshot()
	s.Options.Masks = &learning.UpdateMasks{Edges: []bool{true}}
	if _, err := learning.RestoreTrainer(s); err == nil {
		t.Fatal("RestoreTrainer accepted a mask that does not match the topology")
	}
	// A nil half means "every item of that group is open" and must stay legal.
	half := learning.DefaultOptions()
	half.Masks = &learning.UpdateMasks{Edges: []bool{true, false}}
	if _, err := learning.NewTrainer(c, p, half); err != nil {
		t.Fatalf("rejected a mask with no node half: %v", err)
	}
}

// TestMasksTravelWithTheSnapshotAndAreOwned pins the serialization and the
// deep copy: a snapshot must not alias the trainer's mask buffers.
func TestMasksTravelWithTheSnapshotAndAreOwned(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	o := learning.DefaultOptions()
	o.Masks = &learning.UpdateMasks{Edges: []bool{false, true}, Nodes: []bool{true, false}}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	o.Masks.Edges[0] = true // caller-owned mutation after construction
	s := tr.Snapshot()
	if s.Options.Masks == nil || s.Options.Masks.Edges[0] {
		t.Fatalf("trainer aliased the caller's mask: %+v", s.Options.Masks)
	}
	s.Options.Masks.Nodes[0] = false
	if !tr.Snapshot().Options.Masks.Nodes[0] {
		t.Fatal("snapshot aliased the trainer's mask")
	}
	encoded, err := json.Marshal(tr.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var decoded learning.TrainingSnapshot
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Options.Masks == nil || decoded.Options.Masks.Edges[0] || !decoded.Options.Masks.Edges[1] ||
		!decoded.Options.Masks.Nodes[0] || decoded.Options.Masks.Nodes[1] {
		t.Fatalf("masks did not round trip: %s", encoded)
	}
	if _, err := learning.RestoreTrainer(decoded); err != nil {
		t.Fatalf("restore a snapshot carrying masks: %v", err)
	}
}

// TestOptionsWithoutMasksStaySerializedExactlyAsBefore keeps every existing
// snapshot readable: the new fields are omitted when unused.
func TestOptionsWithoutMasksStaySerializedExactlyAsBefore(t *testing.T) {
	encoded, err := json.Marshal(learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"learning_rate":0.01,"beta1":0.9,"beta2":0.999,"epsilon":1e-8,"weight_decay":0,"clip_norm":1,"truncation":0,"trainable":{"encoder":true,"weights":true,"bias":true,"tau":true,"readout":true}}`
	if string(encoded) != want {
		t.Fatalf("default options serialize as\n%s\nwant\n%s", encoded, want)
	}
}

// TestNodeMaskFreezesThetaRawOnTheSpikingCore covers the third node group,
// which only a LIF core owns.
func TestNodeMaskFreezesThetaRawOnTheSpikingCore(t *testing.T) {
	c, p := lifConfig(), lifParameters()
	o := learning.DefaultOptions()
	o.Trainable = learning.Trainable{Theta: true}
	o.LearningRate = .1
	o.Masks = &learning.UpdateMasks{Nodes: []bool{false, true, true}}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	for step := range 20 {
		if _, err := tr.Step(context.Background(), lifInput(), []float64{.5}); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
	}
	s := tr.Snapshot()
	if s.Parameters.ThetaRaw[0] != p.ThetaRaw[0] {
		t.Fatalf("masked theta_raw[0] moved: %.17g != %.17g", s.Parameters.ThetaRaw[0], p.ThetaRaw[0])
	}
	// theta_raw is the fourth group: weights(3) bias(3) log_tau(3) theta_raw(3).
	if s.Optimizer.Steps[9] != 0 || s.Optimizer.First[9] != 0 || s.Optimizer.Second[9] != 0 {
		t.Fatalf("masked theta_raw[0] accrued optimizer state: %+v", s.Optimizer)
	}
	moved := false
	for i := 1; i < len(s.Parameters.ThetaRaw); i++ {
		if s.Parameters.ThetaRaw[i] != p.ThetaRaw[i] {
			moved = true
		}
	}
	if !moved {
		t.Fatal("no unmasked threshold moved, the fixture proves nothing")
	}
}
