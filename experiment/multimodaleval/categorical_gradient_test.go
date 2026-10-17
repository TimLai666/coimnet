package multimodaleval

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/multimodal/synthetic"
)

type multimodalGradientCase147 struct {
	name    string
	shape   int
	colour  int
	gap     float64
	highGap bool
}

func newMultimodalGradientTrainerPair147(t *testing.T, label synthetic.Label, gap float64) (*learning.Trainer, *learning.Trainer, example, example, []float64) {
	t.Helper()
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      1,
			Sources:    []int{0},
			Targets:    []int{0},
			DT:         1,
			Activation: "tanh",
		},
		InputSize:        1,
		OutputSize:       6,
		InputNodes:       []int{0},
		ReadoutNodes:     []int{0},
		ReadoutEveryStep: true,
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{0},
			Bias:    []float64{1},
			LogTau:  []float64{-10},
		},
		Encoder: []float64{0},
		Readout: make([]float64, 6),
	}
	for i := range parameters.Readout {
		parameters.Readout[i] = 1
	}
	options := learning.DefaultOptions()
	options.LearningRate = .01
	options.WeightDecay = 0
	options.ClipNorm = 0
	options.Trainable = learning.Trainable{Readout: true}
	baseline, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		t.Fatalf("new baseline trainer: %v", err)
	}
	input := [][]float64{{0}, {0}}
	baselineLogits, err := baseline.Predict(context.Background(), input)
	if err != nil {
		t.Fatalf("baseline predict: %v", err)
	}
	if len(baselineLogits) != config.OutputSize {
		t.Fatalf("baseline logits width=%d, want %d", len(baselineLogits), config.OutputSize)
	}
	z := baselineLogits[0]
	if !finiteMultimodal147(z) || z <= 0 {
		t.Fatalf("baseline final z=%.17g, want finite positive", z)
	}
	scaled := gap / z
	if !finiteMultimodal147(scaled) {
		t.Fatalf("tail readout=%.17g, want finite", scaled)
	}
	tail := baseline.Snapshot()
	shapeTarget := label.Shape
	colourTarget := synthetic.Shapes + label.Colour
	for i := range tail.Parameters.Readout {
		if i == shapeTarget || i == colourTarget {
			tail.Parameters.Readout[i] = 0
		} else {
			tail.Parameters.Readout[i] = scaled
		}
	}
	public, err := learning.NewTrainer(tail.Config, tail.Parameters, tail.Options)
	if err != nil {
		t.Fatalf("new public trainer: %v", err)
	}
	reference, err := learning.NewTrainer(tail.Config, tail.Parameters, tail.Options)
	if err != nil {
		t.Fatalf("new reference trainer: %v", err)
	}
	publicExample := example{Input: copyRowsMultimodal147(input), Label: label, Modality: "image"}
	referenceExample := example{Input: copyRowsMultimodal147(input), Label: label, Modality: "image"}
	logits, err := reference.Predict(context.Background(), referenceExample.Input)
	if err != nil {
		t.Fatalf("tail predict: %v", err)
	}
	if len(logits) != config.OutputSize {
		t.Fatalf("tail logits width=%d, want %d", len(logits), config.OutputSize)
	}
	return public, reference, publicExample, referenceExample, logits
}

func copyRowsMultimodal147(rows [][]float64) [][]float64 {
	copyOf := make([][]float64, len(rows))
	for i, row := range rows {
		copyOf[i] = append([]float64(nil), row...)
	}
	return copyOf
}

func analyticHeadGradientMultimodal147(logits []float64, target int) []float64 {
	numerator := make([]float64, len(logits))
	maximum := logits[0]
	for _, value := range logits[1:] {
		if value > maximum {
			maximum = value
		}
	}
	expSum := 0.0
	for i, value := range logits {
		numerator[i] = math.Exp(value - maximum)
		expSum += numerator[i]
	}
	otherMass := 0.0
	for i, value := range numerator {
		if i != target {
			otherMass += value
		}
	}
	gradient := make([]float64, len(logits))
	for i, value := range numerator {
		if i == target {
			value = -otherMass
		}
		gradient[i] = value / expSum
	}
	return gradient
}

func buildMultimodalUpstream147(input [][]float64, logits []float64, label synthetic.Label) [][]float64 {
	upstream := make([][]float64, len(input))
	for row := range upstream {
		upstream[row] = make([]float64, len(logits))
	}
	shapeGradient := analyticHeadGradientMultimodal147(logits[:synthetic.Shapes], label.Shape)
	colourGradient := analyticHeadGradientMultimodal147(logits[synthetic.Shapes:], label.Colour)
	last := upstream[len(upstream)-1]
	copy(last[:synthetic.Shapes], shapeGradient)
	copy(last[synthetic.Shapes:], colourGradient)
	return upstream
}

func snapshotSHAMultimodal147(t *testing.T, snapshot learning.TrainingSnapshot) [32]byte {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func assertZeroOptimizerMultimodal147(t *testing.T, snapshot learning.TrainingSnapshot) {
	t.Helper()
	if snapshot.Updates != 0 {
		t.Fatalf("initial updates=%d, want 0", snapshot.Updates)
	}
	if len(snapshot.Optimizer.First) != len(snapshot.Optimizer.Second) || len(snapshot.Optimizer.First) != len(snapshot.Optimizer.Steps) {
		t.Fatalf("initial optimizer lengths=%d/%d/%d, want equal", len(snapshot.Optimizer.First), len(snapshot.Optimizer.Second), len(snapshot.Optimizer.Steps))
	}
	for i := range snapshot.Optimizer.First {
		if snapshot.Optimizer.First[i] != 0 || snapshot.Optimizer.Second[i] != 0 || snapshot.Optimizer.Steps[i] != 0 {
			t.Fatalf("initial optimizer[%d]=first %.17g second %.17g steps %d, want zero", i, snapshot.Optimizer.First[i], snapshot.Optimizer.Second[i], snapshot.Optimizer.Steps[i])
		}
	}
}

func assertReadoutOptimizerStepsMultimodal147(t *testing.T, snapshot learning.TrainingSnapshot) {
	t.Helper()
	readout := len(snapshot.Parameters.Readout)
	if len(snapshot.Optimizer.Steps) < readout {
		t.Fatalf("optimizer steps=%d, want at least readout width %d", len(snapshot.Optimizer.Steps), readout)
	}
	readoutStart := len(snapshot.Optimizer.Steps) - readout
	for i, step := range snapshot.Optimizer.Steps {
		want := uint64(0)
		if i >= readoutStart {
			want = 1
		}
		if step != want {
			t.Errorf("optimizer steps[%d]=%d, want %d for readout-only update", i, step, want)
		}
	}
}

func targetReadoutMultimodal147(snapshot learning.TrainingSnapshot, label synthetic.Label) []float64 {
	values := make([]float64, 0, 2)
	shapeTarget := label.Shape
	colourTarget := synthetic.Shapes + label.Colour
	for i, value := range snapshot.Parameters.Readout {
		if i == shapeTarget || i == colourTarget {
			values = append(values, value)
		}
	}
	return values
}

func compareFloatSlicesMultimodal147(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s lengths=%d/%d, want equal", name, len(got), len(want))
	}
	for i, expected := range want {
		actual := got[i]
		if !finiteMultimodal147(actual) || !finiteMultimodal147(expected) {
			t.Errorf("%s[%d]=%.17g/%.17g, want finite", name, i, actual, expected)
			continue
		}
		if expected == 0 {
			if actual != 0 {
				t.Errorf("%s[%d]=%.17g, want exact zero", name, i, actual)
			}
			continue
		}
		relative := math.Abs(actual-expected) / math.Abs(expected)
		if relative > 2e-6 {
			t.Errorf("%s[%d]=%.17g want %.17g, relative error=%.17g want <=2e-6", name, i, actual, expected, relative)
		}
	}
}

func finiteMultimodal147(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func positiveFiniteMultimodal147(value float64) bool {
	return finiteMultimodal147(value) && value > 0
}

func assertFiniteSnapshotMultimodal147(t *testing.T, snapshot learning.TrainingSnapshot) {
	t.Helper()
	values := append([]float64(nil), snapshot.Parameters.Core.Weights...)
	values = append(values, snapshot.Parameters.Core.Bias...)
	values = append(values, snapshot.Parameters.Core.LogTau...)
	values = append(values, snapshot.Parameters.Encoder...)
	values = append(values, snapshot.Parameters.Readout...)
	values = append(values, snapshot.Optimizer.First...)
	values = append(values, snapshot.Optimizer.Second...)
	for i, value := range values {
		if !finiteMultimodal147(value) {
			t.Errorf("snapshot float[%d]=%.17g, want finite", i, value)
		}
	}
}

func assertInputRowsUnchangedMultimodal147(t *testing.T, name string, before, after [][]float64) {
	t.Helper()
	if !reflect.DeepEqual(before, after) {
		t.Errorf("%s input changed: before=%v after=%v", name, before, after)
	}
}

func TestMultimodalCategoricalGradient147(t *testing.T) {
	cases := []multimodalGradientCase147{
		{name: "shape0-colour0-gap-40", shape: 0, colour: 0, gap: -40, highGap: true},
		{name: "shape1-colour2-gap-40", shape: 1, colour: 2, gap: -40, highGap: true},
		{name: "shape2-colour1-gap-40", shape: 2, colour: 1, gap: -40, highGap: true},
		{name: "shape0-colour1-gap-2-normal", shape: 0, colour: 1, gap: -2},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			label := synthetic.Label{Shape: testCase.shape, Colour: testCase.colour}
			public, reference, publicExample, referenceExample, logits := newMultimodalGradientTrainerPair147(t, label, testCase.gap)
			beforePublic := public.Snapshot()
			beforeReference := reference.Snapshot()
			if !reflect.DeepEqual(beforePublic, beforeReference) {
				t.Fatalf("public/reference initial snapshots differ")
			}
			if beforePublic.Options.Trainable != (learning.Trainable{Readout: true}) {
				t.Fatalf("initial trainable groups=%+v, want readout only", beforePublic.Options.Trainable)
			}
			if beforePublic.Options.ClipNorm != 0 || beforePublic.Options.WeightDecay != 0 || beforePublic.Options.LearningRate != .01 {
				t.Fatalf("initial optimizer options=%+v, want rate .01, clip 0, decay 0", beforePublic.Options)
			}
			assertZeroOptimizerMultimodal147(t, beforePublic)
			beforePublicInput := copyRowsMultimodal147(publicExample.Input)
			beforeReferenceInput := copyRowsMultimodal147(referenceExample.Input)
			for i, value := range beforePublic.Parameters.Readout {
				if (i == label.Shape || i == synthetic.Shapes+label.Colour) && value != 0 {
					t.Fatalf("initial target readout[%d]=%.17g, want 0", i, value)
				}
			}

			if len(logits) != synthetic.Shapes+synthetic.Colours {
				t.Fatalf("tail logits width=%d, want %d", len(logits), synthetic.Shapes+synthetic.Colours)
			}
			actualGaps := make([]float64, 0, 2)
			for head, target := range []int{label.Shape, synthetic.Shapes + label.Colour} {
				if logits[target] != 0 {
					t.Fatalf("head %d target logit[%d]=%.17g, want exact 0; logits=%v", head, target, logits[target], logits)
				}
				nonTarget := -1
				start := head * synthetic.Shapes
				for i := start; i < start+synthetic.Shapes; i++ {
					if i != target {
						nonTarget = i
						break
					}
				}
				if nonTarget < 0 {
					t.Fatalf("head %d has no non-target logit", head)
				}
				d := logits[nonTarget]
				if !finiteMultimodal147(d) || d >= 0 || math.Abs(d-testCase.gap) > 1e-4 {
					t.Fatalf("head %d actual gap=%.17g, want finite value near %.17g; logits=%v", head, d, testCase.gap, logits)
				}
				for i := start; i < start+synthetic.Shapes; i++ {
					if i != target && logits[i] != d {
						t.Fatalf("head %d logits[%d]=%.17g, want exactly d=%.17g; logits=%v", head, i, logits[i], d, logits)
					}
				}
				actualGaps = append(actualGaps, d)
			}
			upstream := buildMultimodalUpstream147(publicExample.Input, logits, label)
			referenceGradient := append([]float64(nil), upstream[len(upstream)-1]...)

			updates, publicErr := train(ctx, public, []example{publicExample}, 1)
			publicAfter := public.Snapshot()
			referenceResult, referenceErr := reference.StepFrom(ctx, referenceExample.Input, upstream)
			referenceAfter := reference.Snapshot()
			beforeHash := snapshotSHAMultimodal147(t, beforePublic)
			publicAfterHash := snapshotSHAMultimodal147(t, publicAfter)
			referenceAfterHash := snapshotSHAMultimodal147(t, referenceAfter)
			publicTargets := targetReadoutMultimodal147(publicAfter, label)
			referenceTargets := targetReadoutMultimodal147(referenceAfter, label)
			t.Logf("actual_gap=%v reference_gradient=%v public_target_updates=%v reference_target_updates=%v before_snapshot_json_sha256=%x public_after_snapshot_json_sha256=%x reference_after_snapshot_json_sha256=%x train_updates=%d reference_result=%+v", actualGaps, referenceGradient, publicTargets, referenceTargets, beforeHash, publicAfterHash, referenceAfterHash, updates, referenceResult)

			if publicErr != nil {
				t.Errorf("public train error: %v", publicErr)
			}
			if referenceErr != nil {
				t.Errorf("reference StepFrom error: %v", referenceErr)
			}
			if updates != beforePublic.Updates+1 || publicAfter.Updates != beforePublic.Updates+1 {
				t.Errorf("public updates returned=%d snapshot=%d, want %d", updates, publicAfter.Updates, beforePublic.Updates+1)
			}
			if !referenceResult.Applied || referenceResult.Updates != beforePublic.Updates+1 || referenceAfter.Updates != beforePublic.Updates+1 {
				t.Errorf("reference applied=%v result updates=%d snapshot updates=%d, want applied and %d", referenceResult.Applied, referenceResult.Updates, referenceAfter.Updates, beforePublic.Updates+1)
			}
			if referenceResult.LossKnown || referenceResult.Loss != 0 {
				t.Errorf("reference StepResult loss=(%.17g, known=%v), want zero and unknown", referenceResult.Loss, referenceResult.LossKnown)
			}
			if !positiveFiniteMultimodal147(referenceResult.GradientNorm) {
				t.Errorf("reference gradient norm=%.17g, want finite positive", referenceResult.GradientNorm)
			}
			if !reflect.DeepEqual(publicAfter.Optimizer.Steps, referenceAfter.Optimizer.Steps) {
				t.Errorf("optimizer steps differ: public=%v reference=%v", publicAfter.Optimizer.Steps, referenceAfter.Optimizer.Steps)
			}
			assertReadoutOptimizerStepsMultimodal147(t, publicAfter)
			assertReadoutOptimizerStepsMultimodal147(t, referenceAfter)
			if publicAfter.Updates != referenceAfter.Updates {
				t.Errorf("snapshot updates differ: public=%d reference=%d", publicAfter.Updates, referenceAfter.Updates)
			}
			compareFloatSlicesMultimodal147(t, "readout", publicAfter.Parameters.Readout, referenceAfter.Parameters.Readout)
			compareFloatSlicesMultimodal147(t, "optimizer.first", publicAfter.Optimizer.First, referenceAfter.Optimizer.First)
			compareFloatSlicesMultimodal147(t, "optimizer.second", publicAfter.Optimizer.Second, referenceAfter.Optimizer.Second)
			if !reflect.DeepEqual(beforePublic.Parameters.Core, publicAfter.Parameters.Core) || !reflect.DeepEqual(beforePublic.Parameters.Core, referenceAfter.Parameters.Core) {
				t.Errorf("core parameters changed")
			}
			if !reflect.DeepEqual(beforePublic.Parameters.Encoder, publicAfter.Parameters.Encoder) || !reflect.DeepEqual(beforePublic.Parameters.Encoder, referenceAfter.Parameters.Encoder) {
				t.Errorf("encoder parameters changed")
			}
			assertInputRowsUnchangedMultimodal147(t, "public", beforePublicInput, publicExample.Input)
			assertInputRowsUnchangedMultimodal147(t, "reference", beforeReferenceInput, referenceExample.Input)
			assertFiniteSnapshotMultimodal147(t, publicAfter)
			assertFiniteSnapshotMultimodal147(t, referenceAfter)
			if testCase.highGap {
				if len(publicTargets) != 2 || len(referenceTargets) != 2 {
					t.Fatalf("target readout lengths public=%d reference=%d, want 2", len(publicTargets), len(referenceTargets))
				}
				for i, value := range referenceTargets {
					if value == 0 {
						t.Errorf("reference target readout[%d]=%.17g, want nonzero", i, value)
					}
				}
			}
		})
	}
}
