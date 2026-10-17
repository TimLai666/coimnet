package textgen

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func textgenSnapshotSHA147(t *testing.T, snapshot learning.TrainingSnapshot) [32]byte {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func textgenAssertZeroOptimizer147(t *testing.T, snapshot learning.TrainingSnapshot) {
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

func textgenTargetReadout147(snapshot learning.TrainingSnapshot, targetColumn int) []float64 {
	values := make([]float64, 0)
	for i, value := range snapshot.Parameters.Readout {
		if i%snapshot.Config.OutputSize == targetColumn {
			values = append(values, value)
		}
	}
	return values
}

func textgenCompareTargetReadout147(t *testing.T, public, reference []float64) {
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

func TestTextgenPublicGradient147(t *testing.T) {
	ctx := context.Background()
	config := ModelConfig{Embed: 4, Hidden: 8, Settle: 3, LearningRate: .01, Seed: 17}
	vocab := testVocabulary(t)
	publicModel, err := NewModel(vocab, config)
	if err != nil {
		t.Fatal(err)
	}
	referenceModel, err := NewModel(vocab, config)
	if err != nil {
		t.Fatal(err)
	}
	if publicModel.vocabSize != 259 || referenceModel.vocabSize != 259 {
		t.Fatalf("test vocabulary sizes=%d/%d, want 259", publicModel.vocabSize, referenceModel.vocabSize)
	}
	ex := Example{IDs: []int{0, 1}, Scored: []bool{false, true}}

	baseline := publicModel.trainer.Snapshot()
	for i := range baseline.Parameters.Readout {
		baseline.Parameters.Readout[i] = 1
	}
	baseline.Options.Trainable = learning.Trainable{Readout: true}
	publicModel.trainer, err = learning.NewTrainer(baseline.Config, baseline.Parameters, baseline.Options)
	if err != nil {
		t.Fatal(err)
	}
	baselineLogits, err := publicModel.Logits(ctx, ex.IDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(baselineLogits) != len(ex.IDs) || len(baselineLogits) == 0 || len(baselineLogits[0]) != publicModel.vocabSize {
		gotWidth := 0
		if len(baselineLogits) > 0 {
			gotWidth = len(baselineLogits[0])
		}
		t.Fatalf("baseline logits shape=%d x %d, want %d x %d", len(baselineLogits), gotWidth, len(ex.IDs), publicModel.vocabSize)
	}
	a := baselineLogits[0][0]
	if math.IsNaN(a) || math.IsInf(a, 0) || a == 0 {
		t.Fatalf("baseline readout a=%.17g, want finite nonzero", a)
	}
	scaled := -40 / a
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
		t.Fatalf("tail readout=%.17g, want finite", scaled)
	}

	tail := publicModel.trainer.Snapshot()
	for i := range tail.Parameters.Readout {
		if i%tail.Config.OutputSize == 1 {
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
	publicModel.trainer = publicTrainer
	referenceModel.trainer = referenceTrainer
	beforePublic := publicModel.Snapshot()
	beforeReference := referenceModel.Snapshot()
	if !reflect.DeepEqual(beforePublic, beforeReference) {
		t.Fatalf("public/reference initial snapshots differ")
	}
	if beforePublic.Options.Trainable != (learning.Trainable{Readout: true}) {
		t.Fatalf("initial trainable groups=%+v, want readout only", beforePublic.Options.Trainable)
	}
	textgenAssertZeroOptimizer147(t, beforePublic)
	for i, value := range beforePublic.Parameters.Readout {
		if i%beforePublic.Config.OutputSize == 1 && value != 0 {
			t.Fatalf("initial target readout[%d]=%.17g, want 0", i, value)
		}
	}

	publicLogits, err := publicModel.Logits(ctx, ex.IDs)
	if err != nil {
		t.Fatal(err)
	}
	referenceLogits, err := referenceModel.Logits(ctx, ex.IDs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(publicLogits, referenceLogits) {
		t.Fatalf("public/reference initial logits differ")
	}
	if len(publicLogits) != len(ex.IDs) || len(publicLogits) == 0 {
		t.Fatalf("tail logits rows=%d, want %d", len(publicLogits), len(ex.IDs))
	}
	for row, logits := range publicLogits {
		if len(logits) != publicModel.vocabSize {
			t.Fatalf("tail logits row %d width=%d, want %d", row, len(logits), publicModel.vocabSize)
		}
	}
	row := publicLogits[0]
	if row[1] != 0 {
		t.Fatalf("winner logits[1]=%.17g, want exact 0; logits=%v", row[1], row)
	}
	d := row[0]
	if math.IsNaN(d) || math.IsInf(d, 0) || d >= 0 || math.Abs(d+40) > 1e-4 {
		t.Fatalf("nonwinner d=%.17g, want finite value near -40; logits=%v", d, row)
	}
	for i, value := range row {
		if i == 1 {
			continue
		}
		if value != d {
			t.Fatalf("logits[%d]=%.17g, want exactly d=%.17g; logits=%v", i, value, d, row)
		}
	}

	input, err := referenceModel.rows(ex.IDs)
	if err != nil {
		t.Fatal(err)
	}
	upstream := make([][]float64, len(input))
	for i := range upstream {
		upstream[i] = make([]float64, publicModel.vocabSize)
	}
	q := math.Exp(d)
	tailMass := float64(publicModel.vocabSize-1) * q
	denominator := 1 + tailMass
	targetDerivative := -tailMass / denominator
	otherDerivative := q / denominator
	if math.IsNaN(targetDerivative) || math.IsInf(targetDerivative, 0) || math.IsNaN(otherDerivative) || math.IsInf(otherDerivative, 0) || targetDerivative >= 0 || otherDerivative <= 0 {
		t.Fatalf("analytic derivatives invalid: d=%.17g target=%.17g other=%.17g; want target<0 and other>0", d, targetDerivative, otherDerivative)
	}
	for i := range upstream[config.Settle-1] {
		upstream[config.Settle-1][i] = otherDerivative
	}
	upstream[config.Settle-1][1] = targetDerivative

	publicResult, publicLoss, publicErr := publicModel.Step(ctx, ex)
	publicAfter := publicModel.Snapshot()
	referenceResult, referenceErr := referenceModel.trainer.StepFrom(ctx, input, upstream)
	referenceAfter := referenceModel.Snapshot()
	beforeHash := textgenSnapshotSHA147(t, beforePublic)
	publicAfterHash := textgenSnapshotSHA147(t, publicAfter)
	referenceAfterHash := textgenSnapshotSHA147(t, referenceAfter)
	publicTargets := textgenTargetReadout147(publicAfter, 1)
	referenceTargets := textgenTargetReadout147(referenceAfter, 1)
	t.Logf("d=%.17g reference_target_gradient=%.17g public_target_readout=%v reference_target_readout=%v before_snapshot_json_sha256=%x public_after_snapshot_json_sha256=%x reference_after_snapshot_json_sha256=%x public_result=%+v reference_result=%+v", d, targetDerivative, publicTargets, referenceTargets, beforeHash, publicAfterHash, referenceAfterHash, publicResult, referenceResult)

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
	if !finitePositiveTextgen147(publicLoss) {
		t.Errorf("public loss=%.17g, want finite positive", publicLoss)
	}
	if publicResult.LossKnown || referenceResult.LossKnown || publicResult.Loss != 0 || referenceResult.Loss != 0 {
		t.Errorf("StepResult loss fields public=(%.17g, known=%v) reference=(%.17g, known=%v), want zero and unknown", publicResult.Loss, publicResult.LossKnown, referenceResult.Loss, referenceResult.LossKnown)
	}
	if !finitePositiveTextgen147(publicResult.GradientNorm) || !finitePositiveTextgen147(referenceResult.GradientNorm) {
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
	textgenCompareTargetReadout147(t, publicTargets, referenceTargets)
}

func finitePositiveTextgen147(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}
