package media

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func mediaSnapshotSHA147(t *testing.T, snapshot learning.TrainingSnapshot) [32]byte {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func mediaAssertZeroOptimizer147(t *testing.T, snapshot learning.TrainingSnapshot) {
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

func mediaTargetReadout147(snapshot learning.TrainingSnapshot, targetColumns map[int]bool) []float64 {
	values := make([]float64, 0)
	for i, value := range snapshot.Parameters.Readout {
		if targetColumns[i%snapshot.Config.OutputSize] {
			values = append(values, value)
		}
	}
	return values
}

func mediaCompareTargetReadout147(t *testing.T, public, reference []float64) {
	t.Helper()
	if len(public) != len(reference) {
		t.Fatalf("target readout lengths=%d/%d, want equal", len(public), len(reference))
	}
	for i, want := range reference {
		got := public[i]
		if math.IsNaN(got) || math.IsInf(got, 0) || math.IsNaN(want) || math.IsInf(want, 0) {
			t.Errorf("target readout[%d] public=%.17g reference=%.17g, want finite values", i, got, want)
			continue
		}
		if want == 0 {
			if got != 0 {
				t.Errorf("target readout[%d] public=%.17g reference=0, want exact zero", i, got)
			}
			continue
		}
		relative := math.Abs(got-want) / math.Abs(want)
		if relative > 2e-6 {
			t.Errorf("target readout[%d] public=%.17g reference=%.17g, relative error=%.17g want <=2e-6", i, got, want, relative)
		}
	}
}

func TestRequestHeadPublicGradient147(t *testing.T) {
	ctx := context.Background()
	config := DefaultGeneratorConfig(ModalityImage)
	config.Embed, config.Hidden, config.Settle, config.LearningRate, config.Seed = 4, 8, 3, .01, 17
	publicHead, err := NewRequestHead(config)
	if err != nil {
		t.Fatal(err)
	}
	referenceHead, err := NewRequestHead(config)
	if err != nil {
		t.Fatal(err)
	}
	input, colourWinner, shapeIndex, err := publicHead.requestInput("red square")
	if err != nil {
		t.Fatal(err)
	}
	shapeWinner := len(ImageColours) + shapeIndex
	outputSize := len(ImageColours) + len(ImageShapes)
	targetColumns := map[int]bool{colourWinner: true, shapeWinner: true}

	baseline := publicHead.trainer.Snapshot()
	for i := range baseline.Parameters.Readout {
		baseline.Parameters.Readout[i] = 1
	}
	baseline.Options.Trainable = learning.Trainable{Readout: true}
	publicHead.trainer, err = learning.NewTrainer(baseline.Config, baseline.Parameters, baseline.Options)
	if err != nil {
		t.Fatal(err)
	}
	baselineOutputs, err := publicHead.trainer.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(baselineOutputs) != len(input) || len(baselineOutputs) == 0 || len(baselineOutputs[len(baselineOutputs)-1]) != outputSize {
		gotWidth := 0
		if len(baselineOutputs) > 0 {
			gotWidth = len(baselineOutputs[len(baselineOutputs)-1])
		}
		t.Fatalf("baseline output shape=%d x %d, want %d x %d", len(baselineOutputs), gotWidth, len(input), outputSize)
	}
	a := baselineOutputs[len(baselineOutputs)-1][0]
	if math.IsNaN(a) || math.IsInf(a, 0) || a == 0 {
		t.Fatalf("baseline readout a=%.17g, want finite nonzero", a)
	}
	scaled := -40 / a
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
		t.Fatalf("tail readout=%.17g, want finite", scaled)
	}

	tail := publicHead.trainer.Snapshot()
	for i := range tail.Parameters.Readout {
		if targetColumns[i%tail.Config.OutputSize] {
			tail.Parameters.Readout[i] = 0
		} else {
			tail.Parameters.Readout[i] = scaled
		}
	}
	publicTrainer, err := learning.NewTrainer(tail.Config, tail.Parameters, tail.Options)
	if err != nil {
		t.Fatal(err)
	}
	referenceTrainer, err := learning.NewTrainer(tail.Config, tail.Parameters, tail.Options)
	if err != nil {
		t.Fatal(err)
	}
	publicHead.trainer = publicTrainer
	referenceHead.trainer = referenceTrainer
	beforePublic := publicHead.trainer.Snapshot()
	beforeReference := referenceHead.trainer.Snapshot()
	if !reflect.DeepEqual(beforePublic, beforeReference) {
		t.Fatalf("public/reference initial snapshots differ")
	}
	if beforePublic.Options.Trainable != (learning.Trainable{Readout: true}) {
		t.Fatalf("initial trainable groups=%+v, want readout only", beforePublic.Options.Trainable)
	}
	mediaAssertZeroOptimizer147(t, beforePublic)
	for i, value := range beforePublic.Parameters.Readout {
		if targetColumns[i%beforePublic.Config.OutputSize] && value != 0 {
			t.Fatalf("initial target readout[%d]=%.17g, want 0", i, value)
		}
	}

	publicOutputs, err := publicHead.trainer.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	referenceOutputs, err := referenceHead.trainer.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(publicOutputs, referenceOutputs) {
		t.Fatalf("public/reference initial outputs differ")
	}
	if len(publicOutputs) != len(input) || len(publicOutputs) == 0 {
		t.Fatalf("tail output rows=%d, want %d", len(publicOutputs), len(input))
	}
	for row, logits := range publicOutputs {
		if len(logits) != outputSize {
			t.Fatalf("tail output row %d width=%d, want %d", row, len(logits), outputSize)
		}
	}
	last := len(publicOutputs) - 1
	logits := publicOutputs[last]
	groupStarts := []int{0, len(ImageColours)}
	winners := []int{colourWinner, shapeWinner}
	ds := make([]float64, 0, len(groupStarts))
	referenceTargetGradients := make([]float64, 0, len(groupStarts))
	upstream := make([][]float64, len(publicOutputs))
	for row := range upstream {
		upstream[row] = make([]float64, outputSize)
	}
	for group, start := range groupStarts {
		winner := winners[group]
		if logits[winner] != 0 {
			t.Fatalf("winner logits[%d]=%.17g, want exact 0; logits=%v", winner, logits[winner], logits)
		}
		nonWinner := start
		if nonWinner == winner {
			nonWinner++
		}
		d := logits[nonWinner]
		if math.IsNaN(d) || math.IsInf(d, 0) || d >= 0 || math.Abs(d+40) > 1e-4 {
			t.Fatalf("group %d nonwinner d=%.17g, want finite value near -40; logits=%v", group, d, logits)
		}
		for i := start; i < start+len(ImageColours); i++ {
			if i == winner {
				continue
			}
			if logits[i] != d {
				t.Fatalf("group %d logits[%d]=%.17g, want exactly d=%.17g; logits=%v", group, i, logits[i], d, logits)
			}
		}
		ds = append(ds, d)
		q := math.Exp(d)
		tailMass := float64(len(ImageColours)-1) * q
		denominator := 1 + tailMass
		targetDerivative := -tailMass / denominator / 2
		otherDerivative := q / denominator / 2
		if math.IsNaN(targetDerivative) || math.IsInf(targetDerivative, 0) || math.IsNaN(otherDerivative) || math.IsInf(otherDerivative, 0) || targetDerivative >= 0 || otherDerivative <= 0 {
			t.Fatalf("group %d analytic derivatives invalid: d=%.17g target=%.17g other=%.17g; want target<0 and other>0", group, d, targetDerivative, otherDerivative)
		}
		referenceTargetGradients = append(referenceTargetGradients, targetDerivative)
		for i := start; i < start+len(ImageColours); i++ {
			upstream[last][i] = otherDerivative
		}
		upstream[last][winner] = targetDerivative
	}

	publicResult, publicLoss, publicErr := publicHead.Step(ctx, "red square")
	publicAfter := publicHead.trainer.Snapshot()
	referenceResult, referenceErr := referenceHead.trainer.StepFrom(ctx, input, upstream)
	referenceAfter := referenceHead.trainer.Snapshot()
	beforeHash := mediaSnapshotSHA147(t, beforePublic)
	publicAfterHash := mediaSnapshotSHA147(t, publicAfter)
	referenceAfterHash := mediaSnapshotSHA147(t, referenceAfter)
	publicTargets := mediaTargetReadout147(publicAfter, targetColumns)
	referenceTargets := mediaTargetReadout147(referenceAfter, targetColumns)
	t.Logf("d=%v reference_target_gradient=%v public_target_readout=%v reference_target_readout=%v before_snapshot_json_sha256=%x public_after_snapshot_json_sha256=%x reference_after_snapshot_json_sha256=%x public_result=%+v reference_result=%+v", ds, referenceTargetGradients, publicTargets, referenceTargets, beforeHash, publicAfterHash, referenceAfterHash, publicResult, referenceResult)

	if publicErr != nil {
		t.Errorf("public Step error: %v", publicErr)
	}
	if referenceErr != nil {
		t.Errorf("reference StepFrom error: %v", referenceErr)
	}
	if !publicResult.Applied || !referenceResult.Applied {
		t.Errorf("Applied public=%v reference=%v, want both true", publicResult.Applied, referenceResult.Applied)
	}
	if publicAfter.Updates != beforePublic.Updates+1 || referenceAfter.Updates != beforePublic.Updates+1 {
		t.Errorf("updates public=%d reference=%d, want %d", publicAfter.Updates, referenceAfter.Updates, beforePublic.Updates+1)
	}
	if publicResult.Updates != beforePublic.Updates+1 || referenceResult.Updates != beforePublic.Updates+1 {
		t.Errorf("result updates public=%d reference=%d, want %d", publicResult.Updates, referenceResult.Updates, beforePublic.Updates+1)
	}
	if !finitePositiveMedia147(publicLoss) {
		t.Errorf("public loss=%.17g, want finite positive", publicLoss)
	}
	if publicResult.LossKnown || referenceResult.LossKnown || publicResult.Loss != 0 || referenceResult.Loss != 0 {
		t.Errorf("StepResult loss fields public=(%.17g, known=%v) reference=(%.17g, known=%v), want zero and unknown", publicResult.Loss, publicResult.LossKnown, referenceResult.Loss, referenceResult.LossKnown)
	}
	if !finitePositiveMedia147(publicResult.GradientNorm) || !finitePositiveMedia147(referenceResult.GradientNorm) {
		t.Errorf("gradient norms public=%.17g reference=%.17g, want both finite positive", publicResult.GradientNorm, referenceResult.GradientNorm)
	}
	if !reflect.DeepEqual(beforePublic.Parameters.Core, publicAfter.Parameters.Core) || !reflect.DeepEqual(beforePublic.Parameters.Core, referenceAfter.Parameters.Core) {
		t.Errorf("core parameters changed")
	}
	if !reflect.DeepEqual(beforePublic.Parameters.Encoder, publicAfter.Parameters.Encoder) || !reflect.DeepEqual(beforePublic.Parameters.Encoder, referenceAfter.Parameters.Encoder) {
		t.Errorf("encoder parameters changed")
	}
	if len(referenceTargets) == 0 {
		t.Fatalf("reference target readout is empty")
	}
	nonzeroReference := false
	for _, value := range referenceTargets {
		if !math.IsNaN(value) && !math.IsInf(value, 0) && value != 0 {
			nonzeroReference = true
			break
		}
	}
	if !nonzeroReference {
		t.Errorf("reference target readout has no nonzero entry")
	}
	mediaCompareTargetReadout147(t, publicTargets, referenceTargets)
}

func finitePositiveMedia147(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}
