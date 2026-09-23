package learning_test

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// sharingConfig is the three-node, two-edge chain (edge 0: 0->1, edge 1: 1->2)
// every sharing test drives.
func sharingConfig() learning.Config {
	return learning.Config{
		Dynamics:  dynamics.Config{Nodes: 3, Sources: []int{0, 1}, Targets: []int{1, 2}, Delays: []int{0, 0}, DT: .5, Activation: "tanh"},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2},
	}
}

// sharingParameters builds the parameter set for sharingConfig. Weights must
// supply the two edge weights; the core families are fixed so the fixture is
// deterministic.
func sharingParameters(weights ...float64) learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: weights, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}},
		Encoder: []float64{1, 0, 0},
		Readout: []float64{1},
	}
}

// sharedAdamWStep replicates one AdamW update of learning.Trainer under the
// DefaultOptions constants: the first bias-corrected step on parameter v with
// gradient d returns the new value. It mirrors the trainer's expression for
// expression, so the comparison is meaningful at 1e-15.
func sharedAdamWStep(v, d float64) float64 {
	first := .9*0 + (1-.9)*d
	second := .999*0 + (1-.999)*d*d
	m := first / (1 - math.Pow(.9, 1))
	vhat := second / (1 - math.Pow(.999, 1))
	return v*(1-.01*0) - .01*m/(math.Sqrt(vhat)+1e-8)
}

// TestSharedGradientIsTheSumOfMembers pins Root decision 7 of ticket 25 on the
// cleanest fixture: a three-node, two-edge chain whose two edges share one
// group. The trainer's one step must move both members to the value a single
// AdamW step with the members' gradient sum reaches, the members must stay
// bit-identical, and the reported norm must count the group once.
func TestSharedGradientIsTheSumOfMembers(t *testing.T) {
	c := sharingConfig()
	p := sharingParameters(.3, .3)
	// Three rows: with two rows and delay-zero edges the chain sits exactly at
	// the propagation boundary and both weight gradients vanish, so the sum
	// would be vacuous. The third row lets node 2 see w*out and the reduced
	// gradient become genuinely nonzero.
	input := [][]float64{{1}, {0}, {0}}
	target := []float64{.5}

	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	_, g, err := n.LossGradient(context.Background(), p, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	g0, g1 := g.Core.Weights[0], g.Core.Weights[1]
	sum := g0 + g1

	c.Sharing = &learning.ParameterSharing{Weights: []int32{0, 0}, Groups: 1}
	o := learning.DefaultOptions()
	o.Trainable = learning.Trainable{Weights: true}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	res, err := tr.Step(context.Background(), input, target)
	if err != nil {
		t.Fatal(err)
	}
	s := tr.Snapshot()
	if s.Parameters.Core.Weights[0] != s.Parameters.Core.Weights[1] {
		t.Fatalf("shared members diverged after one step: %v vs %v", s.Parameters.Core.Weights[0], s.Parameters.Core.Weights[1])
	}
	norm := math.Hypot(0, sum)
	scale := 1.0
	if norm > 1 {
		scale = 1 / norm
	}
	want := sharedAdamWStep(.3, sum*scale)
	if math.Abs(s.Parameters.Core.Weights[0]-want) > 1e-15 {
		t.Fatalf("shared weight = %.17g, want the AdamW step with g0+g1 = %.17g", s.Parameters.Core.Weights[0], want)
	}
	if math.Abs(res.GradientNorm-norm) > 1e-15 {
		t.Fatalf("GradientNorm = %.17g, want |g0+g1| = %.17g", res.GradientNorm, norm)
	}
	t.Logf("g0=%.17g g1=%.17g g0+g1=%.17g norm=|sum|=%.17g scale=%g new=%.17g want=%.17g",
		g0, g1, sum, norm, scale, s.Parameters.Core.Weights[0], want)
}

// TestSharingNilIsBitIdentical pins the guarantee that a model without a
// Sharing declaration behaves exactly as before: two identical trainers take
// identical steps and the canonical Config JSON carries no sharing key.
func TestSharingNilIsBitIdentical(t *testing.T) {
	c := sharingConfig()
	p := sharingParameters(.3, .2)
	input := [][]float64{{.7}, {-.2}, {.1}}
	target := []float64{.4}
	trA, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	trB, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := trA.Step(context.Background(), input, target); err != nil {
			t.Fatal(err)
		}
		if _, err := trB.Step(context.Background(), input, target); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(trA.Snapshot(), trB.Snapshot()) {
		t.Fatalf("identical trainers without sharing diverged:\nA=%+v\nB=%+v", trA.Snapshot(), trB.Snapshot())
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sharing") {
		t.Fatalf("a config without sharing serializes a sharing key: %s", raw)
	}
}

// TestSharingRejects covers the validation contract: lengths, index range,
// missing members, unequal initial values, mixed edge signs and a group that
// spans two array families are all refused.
func TestSharingRejects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(c *learning.Config, p *learning.Parameters)
	}{
		{"weight-share length mismatch", func(c *learning.Config, p *learning.Parameters) {
			c.Sharing = &learning.ParameterSharing{Weights: []int32{0}, Groups: 1}
		}},
		{"bias-share length mismatch", func(c *learning.Config, p *learning.Parameters) {
			c.Sharing = &learning.ParameterSharing{Bias: []int32{0, 0}, Groups: 1}
		}},
		{"group index out of range", func(c *learning.Config, p *learning.Parameters) {
			c.Sharing = &learning.ParameterSharing{Weights: []int32{0, 5}, Groups: 2}
		}},
		{"unused group", func(c *learning.Config, p *learning.Parameters) {
			c.Sharing = &learning.ParameterSharing{Weights: []int32{0, 0}, Groups: 2}
		}},
		{"shared members start different", func(c *learning.Config, p *learning.Parameters) {
			p.Core.Weights = []float64{.3, .4}
			c.Sharing = &learning.ParameterSharing{Weights: []int32{0, 0}, Groups: 1}
		}},
		{"shared group mixes edge signs", func(c *learning.Config, p *learning.Parameters) {
			c.EdgeSigns = []int8{1, -1}
			c.Sharing = &learning.ParameterSharing{Weights: []int32{0, 0}, Groups: 1}
		}},
		{"a group spans two array families", func(c *learning.Config, p *learning.Parameters) {
			c.Sharing = &learning.ParameterSharing{Weights: []int32{0, 0}, Bias: []int32{0, 0, 0}, Groups: 1}
		}},
	} {
		c, p := sharingConfig(), sharingParameters(.3, .3)
		tc.mutate(&c, &p)
		if _, err := learning.NewTrainer(c, p, learning.DefaultOptions()); err == nil {
			t.Fatalf("%s: NewTrainer accepted the sharing declaration", tc.name)
		}
	}
}

// TestSharingRejectsOversizedGroups protects Validate against a declared
// Groups far larger than the shareable core values (weights + bias + log_tau),
// which could otherwise allocate a huge used slice when the declaration comes
// from an external model package or snapshot. The rejection must be immediate:
// the whole test has to finish well under a second.
func TestSharingRejectsOversizedGroups(t *testing.T) {
	c := sharingConfig()
	c.Sharing = &learning.ParameterSharing{Groups: 1 << 40}
	start := time.Now()
	_, err := learning.NewTrainer(c, sharingParameters(.3, .2), learning.DefaultOptions())
	if err == nil {
		t.Fatal("NewTrainer accepted a group count larger than the shareable values")
	}
	if !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("oversized-group error %q does not mention exceed", err)
	}
	if d := time.Since(start); d >= time.Second {
		t.Fatalf("oversized-group rejection took %v, want under a second", d)
	}
}

// TestSharingMaskFreezesTheWholeGroup pins the mask rule: any one member a
// per-item mask freezes freezes the entire group, counted as one conflict, and
// the frozen members stay bit-identical through a step.
func TestSharingMaskFreezesTheWholeGroup(t *testing.T) {
	c := sharingConfig()
	c.Sharing = &learning.ParameterSharing{Weights: []int32{0, 0}, Groups: 1}
	p := sharingParameters(.3, .3)
	o := controlOptions()
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	if r := tr.Sharing(); r.Groups != 1 || r.Members != 2 || r.Conflicts != 1 {
		t.Fatalf("sharing report = %+v, want groups 1 members 2 conflicts 1", r)
	}
	if _, err := tr.Step(context.Background(), [][]float64{{1}, {0}, {0}}, []float64{.5}); err != nil {
		t.Fatal(err)
	}
	got := tr.Snapshot().Parameters.Core.Weights
	if got[0] != p.Core.Weights[0] || got[1] != p.Core.Weights[1] {
		t.Fatalf("frozen shared members moved: %v -> %v", p.Core.Weights, got)
	}
	control, err := learning.NewTrainer(sharingConfig(), sharingParameters(.3, .3), controlOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.Step(context.Background(), [][]float64{{1}, {0}, {0}}, []float64{.5}); err != nil {
		t.Fatal(err)
	}
	if w := control.Snapshot().Parameters.Core.Weights[0]; w == p.Core.Weights[0] {
		t.Fatal("control unmasked weight did not move; the fixture gradient is zero")
	}
	if got[0] != got[1] {
		t.Fatalf("shared members diverged: %v", got)
	}
}

// controlOptions mirrors the mask test's options without a sharing declaration.
func controlOptions() learning.Options {
	o := learning.DefaultOptions()
	o.Trainable = learning.Trainable{Weights: true}
	o.Masks = &learning.UpdateMasks{Edges: []bool{true, false}}
	return o
}

// TestSharedMembersStayEqualOverSteps drives the shared array through repeated
// AdamW steps: the members of one group must stay bit-identical, in the weight
// and in the bias array, while individual entries may move independently.
func TestSharedMembersStayEqualOverSteps(t *testing.T) {
	t.Run("weights", func(t *testing.T) {
		c := sharingConfig()
		c.Sharing = &learning.ParameterSharing{Weights: []int32{0, 0}, Groups: 1}
		p := sharingParameters(.3, .3)
		o := learning.DefaultOptions()
		o.Trainable = learning.Trainable{Weights: true}
		tr, err := learning.NewTrainer(c, p, o)
		if err != nil {
			t.Fatal(err)
		}
		input := [][]float64{{1}, {0}, {1}}
		moved := false
		for step := range 5 {
			if _, err := tr.Step(context.Background(), input, []float64{.25}); err != nil {
				t.Fatalf("step %d: %v", step, err)
			}
			w := tr.Snapshot().Parameters.Core.Weights
			if w[0] != w[1] {
				t.Fatalf("step %d shared weights diverged: %v vs %v", step, w[0], w[1])
			}
			if w[0] != p.Core.Weights[0] {
				moved = true
			}
		}
		if !moved {
			t.Fatal("the shared weights never moved; the test proves nothing")
		}
	})
	t.Run("bias", func(t *testing.T) {
		c := sharingConfig()
		c.Sharing = &learning.ParameterSharing{Bias: []int32{0, 0, -1}, Groups: 1}
		p := sharingParameters(.3, .2)
		o := learning.DefaultOptions()
		o.Trainable = learning.Trainable{Weights: true, Bias: true}
		tr, err := learning.NewTrainer(c, p, o)
		if err != nil {
			t.Fatal(err)
		}
		input := [][]float64{{1}, {0}}
		moved := false
		for step := range 5 {
			if _, err := tr.Step(context.Background(), input, []float64{.25}); err != nil {
				t.Fatalf("step %d: %v", step, err)
			}
			b := tr.Snapshot().Parameters.Core.Bias
			if b[0] != b[1] {
				t.Fatalf("step %d shared bias diverged: %v vs %v", step, b[0], b[1])
			}
			if b[0] != p.Core.Bias[0] {
				moved = true
			}
		}
		if !moved {
			t.Fatal("the shared bias never moved; the test proves nothing")
		}
	})
}
