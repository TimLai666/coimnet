package ocr

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func ocrPublicGradientSnapshotSHA147(t *testing.T, snapshot learning.TrainingSnapshot) [32]byte {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func ocrPublicGradientSameFloatBits147(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return false
		}
	}
	return true
}

func ocrPublicGradientSameRows147(a, b [][]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !ocrPublicGradientSameFloatBits147(a[i], b[i]) {
			return false
		}
	}
	return true
}

func ocrPublicGradientSameFrozen147(a, b learning.Parameters) bool {
	return ocrPublicGradientSameFloatBits147(a.Core.Weights, b.Core.Weights) &&
		ocrPublicGradientSameFloatBits147(a.Core.Bias, b.Core.Bias) &&
		ocrPublicGradientSameFloatBits147(a.Core.LogTau, b.Core.LogTau) &&
		ocrPublicGradientSameFloatBits147(a.ThetaRaw, b.ThetaRaw) &&
		ocrPublicGradientSameFloatBits147(a.Encoder, b.Encoder)
}

func ocrPublicGradientSameReadoutColumn147(a, b learning.Parameters, outputSize, column int) bool {
	if outputSize <= 0 || column < 0 || column >= outputSize || len(a.Readout) != len(b.Readout) {
		return false
	}
	for i := range a.Readout {
		if i%outputSize != column {
			continue
		}
		if math.Float64bits(a.Readout[i]) != math.Float64bits(b.Readout[i]) {
			return false
		}
	}
	return true
}

func ocrPublicGradientFiniteParameters147(parameters learning.Parameters) bool {
	groups := [][]float64{
		parameters.Core.Weights,
		parameters.Core.Bias,
		parameters.Core.LogTau,
		parameters.ThetaRaw,
		parameters.Encoder,
		parameters.Readout,
	}
	for _, group := range groups {
		for _, value := range group {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return false
			}
		}
	}
	return true
}

func ocrPublicGradientAssertZeroOptimizer147(t *testing.T, snapshot learning.TrainingSnapshot) {
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

func ocrPublicGradientTargetReadout147(snapshot learning.TrainingSnapshot, target int) []float64 {
	values := make([]float64, 0)
	for i, value := range snapshot.Parameters.Readout {
		if i%snapshot.Config.OutputSize == target {
			values = append(values, value)
		}
	}
	return values
}

func ocrPublicGradientCompareTarget147(t *testing.T, public, reference []float64) bool {
	t.Helper()
	if len(public) != len(reference) {
		t.Fatalf("target readout lengths=%d/%d, want equal", len(public), len(reference))
	}
	nonzeroReference := false
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
		nonzeroReference = true
		if got == 0 {
			t.Errorf("target readout[%d] became zero, reference=%.17g", i, want)
			continue
		}
		relative := math.Abs(got-want) / math.Abs(want)
		if relative > 2e-6 {
			t.Errorf("target readout[%d] public=%.17g reference=%.17g relative error=%.17g, want <=2e-6", i, got, want, relative)
		}
	}
	if !nonzeroReference {
		t.Errorf("reference target readout has no nonzero entry")
	}
	return nonzeroReference
}

func TestOCRPublicGradient147(t *testing.T) {
	targets := []struct {
		name  string
		class int
		text  string
	}{
		{name: "target_a", class: 1, text: "a"},
		{name: "target_b", class: 2, text: "b"},
		{name: "target_c", class: 3, text: "c"},
	}
	for _, tc := range targets {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			config := RecognizerConfig{
				Height: 2, Hidden: 4, Alphabet: []rune{'a', 'b', 'c'},
				LearningRate: .01, Seed: 17, InputProfile: "single-character", CharacterWidth: 2,
			}
			image := glyphs.Image{Width: 2, Height: 2, Pixels: []float64{0.1, 0.2, 0.3, 0.4}}
			imageBefore := append([]float64(nil), image.Pixels...)

			publicRecognizer, err := NewRecognizer(config)
			if err != nil {
				t.Fatal(err)
			}
			referenceRecognizer, err := NewRecognizer(config)
			if err != nil {
				t.Fatal(err)
			}

			baseline := publicRecognizer.Snapshot()
			for i := range baseline.Parameters.Readout {
				baseline.Parameters.Readout[i] = 1
			}
			baseline.Options.Trainable = learning.Trainable{Readout: true}
			baselineTrainer, err := learning.NewTrainer(baseline.Config, baseline.Parameters, baseline.Options)
			if err != nil {
				t.Fatal(err)
			}
			publicRecognizer.trainer = baselineTrainer
			baselineInputs, err := publicRecognizer.Inputs(image)
			if err != nil {
				t.Fatal(err)
			}
			baselineLogits, err := publicRecognizer.trainer.PredictAll(ctx, baselineInputs)
			if err != nil {
				t.Fatal(err)
			}
			if len(baselineInputs) != 2 || len(baselineInputs[0]) != 4 || len(baselineInputs[1]) != 4 {
				t.Fatalf("baseline input shape=%d rows, widths=%d/%d, want 2 rows of width 4", len(baselineInputs), len(baselineInputs[0]), len(baselineInputs[1]))
			}
			if len(baselineLogits) != 2 || len(baselineLogits[0]) != 4 || len(baselineLogits[1]) != 4 {
				t.Fatalf("baseline logits shape=%d rows, widths=%d/%d, want 2 rows of width 4", len(baselineLogits), len(baselineLogits[0]), len(baselineLogits[1]))
			}
			a := baselineLogits[len(baselineLogits)-1][1]
			if math.IsNaN(a) || math.IsInf(a, 0) || a == 0 {
				t.Fatalf("baseline final z=%.17g, want finite nonzero", a)
			}
			for class, value := range baselineLogits[len(baselineLogits)-1] {
				if value != a {
					t.Fatalf("baseline final logits[%d]=%.17g, want equal all-readout value %.17g", class, value, a)
				}
			}
			blankReadout := 1000 / a
			nonblankReadout := -40 / a
			if math.IsNaN(blankReadout) || math.IsInf(blankReadout, 0) || math.IsNaN(nonblankReadout) || math.IsInf(nonblankReadout, 0) {
				t.Fatalf("scaled readout values blank=%.17g nonblank=%.17g, want finite", blankReadout, nonblankReadout)
			}

			tail := publicRecognizer.trainer.Snapshot()
			ocrPublicGradientAssertZeroOptimizer147(t, tail)
			if tail.Config.OutputSize != 4 {
				t.Fatalf("output size=%d, want blank plus three alphabet classes", tail.Config.OutputSize)
			}
			for i := range tail.Parameters.Readout {
				switch column := i % tail.Config.OutputSize; {
				case column == 0:
					tail.Parameters.Readout[i] = blankReadout
				case column == tc.class:
					tail.Parameters.Readout[i] = 0
				default:
					tail.Parameters.Readout[i] = nonblankReadout
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
			publicRecognizer.trainer = publicTrainer
			referenceRecognizer.trainer = referenceTrainer

			beforePublic := publicRecognizer.Snapshot()
			beforeReference := referenceRecognizer.Snapshot()
			if !reflect.DeepEqual(beforePublic, beforeReference) {
				t.Fatalf("public/reference initial snapshots differ")
			}
			if beforePublic.Options.Trainable != (learning.Trainable{Readout: true}) {
				t.Fatalf("initial trainable groups=%+v, want readout only", beforePublic.Options.Trainable)
			}
			ocrPublicGradientAssertZeroOptimizer147(t, beforePublic)
			for i, value := range beforePublic.Parameters.Readout {
				if i%beforePublic.Config.OutputSize == tc.class && value != 0 {
					t.Fatalf("initial target readout[%d]=%.17g, want exact 0", i, value)
				}
			}

			input, err := publicRecognizer.Inputs(image)
			if err != nil {
				t.Fatal(err)
			}
			referenceInput, err := referenceRecognizer.Inputs(image)
			if err != nil {
				t.Fatal(err)
			}
			inputBefore := make([][]float64, len(input))
			for i := range input {
				inputBefore[i] = append([]float64(nil), input[i]...)
			}
			referenceInputBefore := make([][]float64, len(referenceInput))
			for i := range referenceInput {
				referenceInputBefore[i] = append([]float64(nil), referenceInput[i]...)
			}
			if !ocrPublicGradientSameRows147(input, referenceInput) {
				t.Fatalf("public/reference inputs differ")
			}
			publicLogits, err := publicRecognizer.trainer.PredictAll(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			referenceLogits, err := referenceRecognizer.trainer.PredictAll(ctx, referenceInput)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(publicLogits, referenceLogits) {
				t.Fatalf("public/reference initial logits differ")
			}
			if len(publicLogits) != 2 || len(publicLogits[0]) != 4 || len(publicLogits[1]) != 4 {
				t.Fatalf("tail logits shape=%d rows, widths=%d/%d, want 2 rows of width 4", len(publicLogits), len(publicLogits[0]), len(publicLogits[1]))
			}
			finalLogits := publicLogits[len(publicLogits)-1]
			if finalLogits[tc.class] != 0 {
				t.Fatalf("target logits[%d]=%.17g, want exact 0; logits=%v", tc.class, finalLogits[tc.class], finalLogits)
			}
			nonTarget := 1
			if nonTarget == tc.class {
				nonTarget = 2
			}
			d := finalLogits[nonTarget]
			if math.IsNaN(d) || math.IsInf(d, 0) || d >= 0 || math.Abs(d+40) > 1e-4 {
				t.Fatalf("target %d nonwinner d=%.17g, want finite value near -40; logits=%v", tc.class, d, finalLogits)
			}
			blank := finalLogits[0]
			if math.IsNaN(blank) || math.IsInf(blank, 0) || math.Abs(blank-1000) > 1e-3 {
				t.Fatalf("blank logit=%.17g, want finite value near 1000; logits=%v", blank, finalLogits)
			}
			for class := 1; class < len(finalLogits); class++ {
				if class != tc.class && finalLogits[class] != d {
					t.Fatalf("nonwinner logits[%d]=%.17g, want exactly d=%.17g; logits=%v", class, finalLogits[class], d, finalLogits)
				}
			}

			q := math.Exp(d)
			denominator := 1 + 2*q
			targetDerivative := -2 * q / denominator
			otherDerivative := q / denominator
			if math.IsNaN(targetDerivative) || math.IsInf(targetDerivative, 0) || math.IsNaN(otherDerivative) || math.IsInf(otherDerivative, 0) || targetDerivative >= 0 || otherDerivative <= 0 {
				t.Fatalf("analytic derivatives invalid: d=%.17g target=%.17g other=%.17g; want target<0 and other>0", d, targetDerivative, otherDerivative)
			}
			upstream := make([][]float64, len(input))
			for row := range upstream {
				upstream[row] = make([]float64, beforePublic.Config.OutputSize)
			}
			last := len(upstream) - 1
			for class := 1; class < beforePublic.Config.OutputSize; class++ {
				upstream[last][class] = otherDerivative
			}
			upstream[last][tc.class] = targetDerivative
			if upstream[last][0] != 0 {
				t.Fatalf("blank upstream=%.17g, want exact 0", upstream[last][0])
			}
			for class, value := range upstream[0] {
				if value != 0 {
					t.Fatalf("first-row upstream[%d]=%.17g, want exact 0", class, value)
				}
			}

			publicReport, publicErr := publicRecognizer.TrainLine(ctx, image, tc.text)
			publicAfter := publicRecognizer.Snapshot()
			referenceResult, referenceErr := referenceRecognizer.trainer.StepFrom(ctx, referenceInput, upstream)
			referenceAfter := referenceRecognizer.Snapshot()
			beforeHash := ocrPublicGradientSnapshotSHA147(t, beforePublic)
			publicAfterHash := ocrPublicGradientSnapshotSHA147(t, publicAfter)
			referenceAfterHash := ocrPublicGradientSnapshotSHA147(t, referenceAfter)
			publicTargets := ocrPublicGradientTargetReadout147(publicAfter, tc.class)
			referenceTargets := ocrPublicGradientTargetReadout147(referenceAfter, tc.class)
			t.Logf("target=%d d=%.17g target_gradient=%.17g other_gradient=%.17g public_target_readout=%v reference_target_readout=%v before_snapshot_json_sha256=%x public_after_snapshot_json_sha256=%x reference_after_snapshot_json_sha256=%x public_report=%+v reference_result=%+v", tc.class, d, targetDerivative, otherDerivative, publicTargets, referenceTargets, beforeHash, publicAfterHash, referenceAfterHash, publicReport, referenceResult)

			if publicErr != nil {
				t.Errorf("public TrainLine error: %v", publicErr)
			}
			if referenceErr != nil {
				t.Errorf("reference StepFrom error: %v", referenceErr)
			}
			if publicErr != nil || referenceErr != nil {
				return
			}
			if publicAfter.Updates != beforePublic.Updates+1 || referenceAfter.Updates != beforePublic.Updates+1 {
				t.Errorf("updates public=%d reference=%d, want %d", publicAfter.Updates, referenceAfter.Updates, beforePublic.Updates+1)
			}
			if !referenceResult.Applied {
				t.Errorf("reference StepFrom Applied=%v, want true", referenceResult.Applied)
			}
			if referenceResult.Updates != beforePublic.Updates+1 {
				t.Errorf("reference StepFrom updates=%d, want %d", referenceResult.Updates, beforePublic.Updates+1)
			}
			if referenceResult.LossKnown || referenceResult.Loss != 0 {
				t.Errorf("reference StepFrom loss fields=(%.17g, known=%v), want zero and unknown", referenceResult.Loss, referenceResult.LossKnown)
			}
			if math.IsNaN(publicReport.Loss) || math.IsInf(publicReport.Loss, 0) || publicReport.Loss < 0 {
				t.Errorf("public report loss=%.17g, want finite >=0", publicReport.Loss)
			}
			if publicReport.Frames != 1 || publicReport.Labels != 1 || publicReport.Impossible {
				t.Errorf("public report=%+v, want Frames=1 Labels=1 Impossible=false", publicReport)
			}
			if math.IsNaN(referenceResult.GradientNorm) || math.IsInf(referenceResult.GradientNorm, 0) || referenceResult.GradientNorm <= 0 {
				t.Errorf("reference gradient norm=%.17g, want finite positive", referenceResult.GradientNorm)
			}
			if !ocrPublicGradientFiniteParameters147(publicAfter.Parameters) || !ocrPublicGradientFiniteParameters147(referenceAfter.Parameters) {
				t.Errorf("public/reference after parameters contain non-finite values")
			}
			if !ocrPublicGradientSameFrozen147(beforePublic.Parameters, publicAfter.Parameters) || !ocrPublicGradientSameFrozen147(beforePublic.Parameters, referenceAfter.Parameters) {
				t.Errorf("core or encoder parameters changed")
			}
			if !ocrPublicGradientSameReadoutColumn147(beforePublic.Parameters, publicAfter.Parameters, beforePublic.Config.OutputSize, 0) || !ocrPublicGradientSameReadoutColumn147(beforePublic.Parameters, referenceAfter.Parameters, beforePublic.Config.OutputSize, 0) {
				t.Errorf("blank readout changed")
			}
			if !ocrPublicGradientSameFloatBits147(image.Pixels, imageBefore) {
				t.Errorf("TrainLine mutated image pixels")
			}
			if !ocrPublicGradientSameRows147(input, inputBefore) || !ocrPublicGradientSameRows147(referenceInput, referenceInputBefore) {
				t.Errorf("public/reference trainer mutated supplied input rows")
			}
			ocrPublicGradientCompareTarget147(t, publicTargets, referenceTargets)
		})
	}
}
