package learning_test

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// rangesFixture starts every core parameter outside the declared range, so one
// update is enough to make the projection do visible work.
func rangesFixture() *learning.ParameterRanges {
	return &learning.ParameterRanges{WeightMagnitudeMax: .1, BiasAbsMax: .05, LogTauMin: -.01, LogTauMax: .01}
}

// TestRangeProjectionClampsEveryGroupAndCountsIt pins root decision 3.
func TestRangeProjectionClampsEveryGroupAndCountsIt(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	o := learning.DefaultOptions()
	o.Ranges = rangesFixture()
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	x, target := maskInput()
	result, err := tr.Step(context.Background(), x, target)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"weights": 2, "bias": 2, "log_tau": 2}
	if !reflect.DeepEqual(result.Projected, want) {
		t.Fatalf("projected = %v, want %v", result.Projected, want)
	}
	if result.LearningRate != o.LearningRate {
		t.Fatalf("reported learning rate %g, want %g", result.LearningRate, o.LearningRate)
	}
	s := tr.Snapshot()
	for i, w := range s.Parameters.Core.Weights {
		if math.Abs(w) != o.Ranges.WeightMagnitudeMax {
			t.Fatalf("weight %d = %.17g, want magnitude %g", i, w, o.Ranges.WeightMagnitudeMax)
		}
	}
	for i, b := range s.Parameters.Core.Bias {
		if math.Abs(b) != o.Ranges.BiasAbsMax {
			t.Fatalf("bias %d = %.17g, want magnitude %g", i, b, o.Ranges.BiasAbsMax)
		}
	}
	for i, l := range s.Parameters.Core.LogTau {
		if l != o.Ranges.LogTauMax {
			t.Fatalf("log_tau %d = %.17g, want %g", i, l, o.Ranges.LogTauMax)
		}
	}
}

// TestRangeProjectionLeavesTheOptimizerStateAlone is the other half of root
// decision 3: the projection is a declared rule on parameters only.
func TestRangeProjectionLeavesTheOptimizerStateAlone(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	x, target := maskInput()
	plain, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	o := learning.DefaultOptions()
	o.Ranges = rangesFixture()
	projected, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	plainResult, err := plain.Step(context.Background(), x, target)
	if err != nil {
		t.Fatal(err)
	}
	projectedResult, err := projected.Step(context.Background(), x, target)
	if err != nil {
		t.Fatal(err)
	}
	a, b := plain.Snapshot(), projected.Snapshot()
	if !reflect.DeepEqual(a.Optimizer, b.Optimizer) {
		t.Fatalf("the projection changed optimizer state\n%+v\n%+v", a.Optimizer, b.Optimizer)
	}
	if plainResult.GradientNorm != projectedResult.GradientNorm {
		t.Fatalf("the projection changed the gradient norm: %g != %g", plainResult.GradientNorm, projectedResult.GradientNorm)
	}
	if reflect.DeepEqual(a.Parameters.Core, b.Parameters.Core) {
		t.Fatal("the projection did not change any parameter, the fixture proves nothing")
	}
	if len(plainResult.Projected) != 0 {
		t.Fatalf("a trainer without ranges reported projections: %v", plainResult.Projected)
	}
	// The update norm has to describe the committed change, projection included.
	var norm float64
	for i := range b.Parameters.Core.Weights {
		norm = math.Hypot(norm, b.Parameters.Core.Weights[i]-p.Core.Weights[i])
	}
	if projectedResult.UpdateNorm < norm {
		t.Fatalf("update norm %g is smaller than the committed weight change %g", projectedResult.UpdateNorm, norm)
	}
}

// TestMaskedParametersAreNotProjected keeps the two mechanisms consistent: a
// frozen parameter stays bit-identical, even when it sits outside the range.
func TestMaskedParametersAreNotProjected(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	o := learning.DefaultOptions()
	o.Ranges = rangesFixture()
	o.Masks = &learning.UpdateMasks{Edges: []bool{false, false}}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	x, target := maskInput()
	result, err := tr.Step(context.Background(), x, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Projected["weights"]; ok {
		t.Fatalf("a frozen group was projected: %v", result.Projected)
	}
	s := tr.Snapshot()
	for i, w := range s.Parameters.Core.Weights {
		if w != p.Core.Weights[i] {
			t.Fatalf("frozen weight %d moved: %.17g != %.17g", i, w, p.Core.Weights[i])
		}
	}
}

// TestFixedSignEdgesAreCappedInLogSpace covers the interaction between root
// decisions 2 and 3: WeightMagnitudeMax bounds the effective weight, so a
// fixed-sign edge is capped at log(max) in its raw parametrization.
func TestFixedSignEdgesAreCappedInLogSpace(t *testing.T) {
	c, p := signsConfig(), signsParameters()
	c.EdgeSigns = []int8{1, 0, -1, 0}
	o := learning.DefaultOptions()
	o.Ranges = &learning.ParameterRanges{WeightMagnitudeMax: .2}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	x, target := signsInput()
	result, err := tr.Step(context.Background(), x, target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Projected["weights"] == 0 {
		t.Fatalf("no weight was capped: %v", result.Projected)
	}
	s := tr.Snapshot()
	effective, err := learning.EffectiveWeights(s.Config, s.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	for i, w := range effective {
		if math.Abs(w) > o.Ranges.WeightMagnitudeMax*(1+1e-15) {
			t.Fatalf("effective weight %d = %.17g exceeds the cap %g", i, w, o.Ranges.WeightMagnitudeMax)
		}
	}
	for _, edge := range []int{0, 2} {
		if math.Abs(effective[edge]) != o.Ranges.WeightMagnitudeMax {
			t.Fatalf("fixed edge %d = %.17g, want the cap %g", edge, effective[edge], o.Ranges.WeightMagnitudeMax)
		}
	}
}

// TestThetaRawRangeAppliesOnTheSpikingCore covers the fourth group.
func TestThetaRawRangeAppliesOnTheSpikingCore(t *testing.T) {
	c, p := lifConfig(), lifParameters()
	o := learning.DefaultOptions()
	o.Trainable = learning.Trainable{Theta: true, Readout: true}
	o.Ranges = &learning.ParameterRanges{ThetaRawAbsMax: .5}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tr.Step(context.Background(), lifInput(), []float64{.5})
	if err != nil {
		t.Fatal(err)
	}
	if result.Projected["theta_raw"] != 3 {
		t.Fatalf("projected = %v, want three thresholds", result.Projected)
	}
	for i, v := range tr.Snapshot().Parameters.ThetaRaw {
		if v != -o.Ranges.ThetaRawAbsMax {
			t.Fatalf("theta_raw %d = %.17g, want %g", i, v, -o.Ranges.ThetaRawAbsMax)
		}
	}
}

// TestInvalidRangesAreRejected keeps a meaningless declaration out of a trainer.
func TestInvalidRangesAreRejected(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	for name, ranges := range map[string]*learning.ParameterRanges{
		"negative weight cap": {WeightMagnitudeMax: -1},
		"negative bias cap":   {BiasAbsMax: -1},
		"negative theta cap":  {ThetaRawAbsMax: -1},
		"inverted log tau":    {LogTauMin: 1, LogTauMax: -1},
		"non-finite cap":      {WeightMagnitudeMax: math.Inf(1)},
		"nan bound":           {LogTauMin: math.NaN()},
	} {
		o := learning.DefaultOptions()
		o.Ranges = ranges
		if _, err := learning.NewTrainer(c, p, o); err == nil {
			t.Fatalf("NewTrainer accepted %s", name)
		}
	}
	// A cap below the log-magnitude floor could never be satisfied.
	signs := signsConfig()
	signs.EdgeSigns = []int8{1, 0, -1, 0}
	signs.MinLogMagnitude = -1
	conflict := learning.DefaultOptions()
	conflict.Ranges = &learning.ParameterRanges{WeightMagnitudeMax: .01}
	if _, err := learning.NewTrainer(signs, signsParameters(), conflict); err == nil {
		t.Fatal("NewTrainer accepted a weight cap below the log-magnitude floor")
	}
}

// TestRangesTravelWithTheSnapshotAndAreOwned mirrors the mask ownership test.
func TestRangesTravelWithTheSnapshotAndAreOwned(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	o := learning.DefaultOptions()
	o.Ranges = rangesFixture()
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	o.Ranges.BiasAbsMax = 99
	s := tr.Snapshot()
	if s.Options.Ranges == nil || s.Options.Ranges.BiasAbsMax != .05 {
		t.Fatalf("trainer aliased the caller's ranges: %+v", s.Options.Ranges)
	}
	s.Options.Ranges.BiasAbsMax = 42
	if tr.Snapshot().Options.Ranges.BiasAbsMax != .05 {
		t.Fatal("snapshot aliased the trainer's ranges")
	}
	encoded, err := json.Marshal(tr.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var decoded learning.TrainingSnapshot
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Options.Ranges, rangesFixture()) {
		t.Fatalf("ranges did not round trip: %s", encoded)
	}
	if _, err := learning.RestoreTrainer(decoded); err != nil {
		t.Fatalf("restore a snapshot carrying ranges: %v", err)
	}
}
