package distill_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/distill"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const distillPublicVocabHash147 = "coimnet-distill-public-gradient-147/v1"

type distillPublicCase147 struct {
	name   string
	mode   string
	target int
	gap    float64
}

type trackingDistillStudent147 struct {
	trainer      *learning.Trainer
	predicts     int
	steps        int
	lastInput    [][]float64
	lastUpstream [][]float64
	result       learning.StepResult
}

func (s *trackingDistillStudent147) Predict(ctx context.Context, input [][]float64) ([]float64, error) {
	s.predicts++
	s.lastInput = copyRowsDistill147(input)
	return s.trainer.Predict(ctx, input)
}

func (s *trackingDistillStudent147) StepFrom(ctx context.Context, input [][]float64, upstream [][]float64) (learning.StepResult, error) {
	s.steps++
	s.lastInput = copyRowsDistill147(input)
	s.lastUpstream = copyRowsDistill147(upstream)
	result, err := s.trainer.StepFrom(ctx, input, upstream)
	s.result = result
	return result, err
}

func distillPublicAlignment147() distill.Alignment {
	return distill.Alignment{
		Rule:             distill.AlignmentIdentical,
		TeacherVocabHash: distillPublicVocabHash147,
		StudentVocabHash: distillPublicVocabHash147,
	}
}

func distillPublicTeacher147(target int) distill.TeacherDistribution {
	p := []float64{.25, .25, .25}
	p[target] = .5
	return distill.TeacherDistribution{Probabilities: p}
}

func newDistillPublicTrainerPair147(t *testing.T, target int, gap float64) (*learning.Trainer, *learning.Trainer, [][]float64, []float64) {
	t.Helper()
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      3,
			Sources:    []int{0, 1, 1},
			Targets:    []int{1, 1, 2},
			DT:         1,
			Activation: "tanh",
		},
		InputSize:    1,
		OutputSize:   3,
		ReadoutNodes: []int{2},
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.237, .2, .2},
			Bias:    []float64{0, 0, 0},
			LogTau:  []float64{math.Log(2), math.Log(2), math.Log(2)},
		},
		Encoder: make([]float64, 3),
		Readout: make([]float64, len(config.ReadoutNodes)*config.OutputSize),
	}
	parameters.Encoder[0] = 1
	for i := range parameters.Readout {
		parameters.Readout[i] = 1
	}
	options := learning.DefaultOptions()
	options.Trainable = learning.Trainable{Readout: true}
	baseline, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		t.Fatalf("new baseline trainer: %v", err)
	}
	input := [][]float64{{1}, {0}, {0}, {0}}
	baselineLogits, err := baseline.Predict(context.Background(), input)
	if err != nil {
		t.Fatalf("baseline predict: %v", err)
	}
	if len(baselineLogits) != config.OutputSize {
		t.Fatalf("baseline logits width=%d, want %d", len(baselineLogits), config.OutputSize)
	}
	a := baselineLogits[0]
	if !finiteDistill147(a) || a == 0 {
		t.Fatalf("baseline final z=%.17g, want finite nonzero", a)
	}
	for i, value := range baselineLogits {
		if value != a {
			t.Fatalf("baseline logits[%d]=%.17g, want same constant z=%.17g; logits=%v", i, value, a, baselineLogits)
		}
	}
	scaled := gap / a
	if !finiteDistill147(scaled) {
		t.Fatalf("scaled tail readout=%.17g, want finite", scaled)
	}
	tail := baseline.Snapshot()
	for i := range tail.Parameters.Readout {
		if i%tail.Config.OutputSize == target {
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
	logits, err := reference.Predict(context.Background(), input)
	if err != nil {
		t.Fatalf("tail predict: %v", err)
	}
	if len(logits) != config.OutputSize {
		t.Fatalf("tail logits width=%d, want %d", len(logits), config.OutputSize)
	}
	return public, reference, input, logits
}

func copyRowsDistill147(rows [][]float64) [][]float64 {
	copyOf := make([][]float64, len(rows))
	for i, row := range rows {
		copyOf[i] = append([]float64(nil), row...)
	}
	return copyOf
}

func distillPublicSnapshotSHA147(t *testing.T, snapshot learning.TrainingSnapshot) [32]byte {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func assertZeroOptimizerDistill147(t *testing.T, snapshot learning.TrainingSnapshot) {
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

func targetReadoutDistill147(snapshot learning.TrainingSnapshot, target int) []float64 {
	values := make([]float64, 0)
	for i, value := range snapshot.Parameters.Readout {
		if i%snapshot.Config.OutputSize == target {
			values = append(values, value)
		}
	}
	return values
}

func compareTargetReadoutDistill147(t *testing.T, public, reference []float64) {
	t.Helper()
	if len(public) != len(reference) {
		t.Fatalf("target readout lengths=%d/%d, want equal", len(public), len(reference))
	}
	for i, want := range reference {
		got := public[i]
		if !finiteDistill147(got) || !finiteDistill147(want) {
			t.Errorf("target readout[%d] public=%.17g reference=%.17g, want finite", i, got, want)
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

func assertDistillUpstream147(t *testing.T, actual, want [][]float64) {
	t.Helper()
	if len(actual) != len(want) {
		t.Fatalf("upstream rows=%d, want %d", len(actual), len(want))
	}
	for row := range want {
		if len(actual[row]) != len(want[row]) {
			t.Fatalf("upstream row %d width=%d, want %d", row, len(actual[row]), len(want[row]))
		}
		for column, expected := range want[row] {
			got := actual[row][column]
			if !finiteDistill147(got) {
				t.Errorf("upstream[%d][%d]=%.17g, want finite", row, column, got)
				continue
			}
			if expected == 0 {
				if got != 0 {
					t.Errorf("upstream[%d][%d]=%.17g, want exact zero", row, column, got)
				}
				continue
			}
			relative := math.Abs(got-expected) / math.Abs(expected)
			if relative > 2e-6 {
				t.Errorf("upstream[%d][%d]=%.17g want %.17g, relative error=%.17g want <=2e-6", row, column, got, expected, relative)
			}
		}
	}
}

func finiteDistill147(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func positiveFiniteDistill147(value float64) bool {
	return finiteDistill147(value) && value > 0
}

func ulpDistill147(a, b float64) uint64 {
	if a == b {
		return 0
	}
	if math.IsNaN(a) || math.IsNaN(b) || math.Signbit(a) != math.Signbit(b) {
		return ^uint64(0)
	}
	ordered := func(value float64) uint64 {
		bits := math.Float64bits(value)
		if bits&(uint64(1)<<63) != 0 {
			return ^bits
		}
		return bits | uint64(1)<<63
	}
	left, right := ordered(a), ordered(b)
	if left > right {
		return left - right
	}
	return right - left
}

func TestDistillPublicGradient147(t *testing.T) {
	cases := []distillPublicCase147{
		{name: "StepLabel-target0-gap-40", mode: "label", target: 0, gap: -40},
		{name: "StepLabel-target1-gap-40", mode: "label", target: 1, gap: -40},
		{name: "StepLabel-target2-gap-40", mode: "label", target: 2, gap: -40},
		{name: "StepLabel-target0-gap-2-normal", mode: "label", target: 0, gap: -2},
		{name: "StepDistribution-target0-gap-40", mode: "distribution", target: 0, gap: -40},
		{name: "StepDistribution-target1-gap-40", mode: "distribution", target: 1, gap: -40},
		{name: "StepDistribution-target2-gap-40", mode: "distribution", target: 2, gap: -40},
		{name: "StepDistribution-target0-gap-2-normal", mode: "distribution", target: 0, gap: -2},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			public, reference, input, logits := newDistillPublicTrainerPair147(t, testCase.target, testCase.gap)
			beforeInput := copyRowsDistill147(input)
			beforePublic := public.Snapshot()
			beforeReference := reference.Snapshot()
			if !reflect.DeepEqual(beforePublic, beforeReference) {
				t.Fatalf("public/reference initial snapshots differ")
			}
			if beforePublic.Options.Trainable != (learning.Trainable{Readout: true}) {
				t.Fatalf("initial trainable groups=%+v, want readout only", beforePublic.Options.Trainable)
			}
			assertZeroOptimizerDistill147(t, beforePublic)
			for i, value := range beforePublic.Parameters.Readout {
				if i%beforePublic.Config.OutputSize == testCase.target && value != 0 {
					t.Fatalf("initial target readout[%d]=%.17g, want 0", i, value)
				}
			}

			if logits[testCase.target] != 0 {
				t.Fatalf("target logit[%d]=%.17g, want exact 0; logits=%v", testCase.target, logits[testCase.target], logits)
			}
			nonTarget := -1
			for i := range logits {
				if i != testCase.target {
					nonTarget = i
					break
				}
			}
			if nonTarget < 0 {
				t.Fatalf("no non-target logit in %v", logits)
			}
			d := logits[nonTarget]
			if !finiteDistill147(d) || d >= 0 || math.Abs(d-testCase.gap) > 1e-4 {
				t.Fatalf("actual d=%.17g, want finite value near %.17g; logits=%v", d, testCase.gap, logits)
			}
			for i, value := range logits {
				if i != testCase.target && value != d {
					t.Fatalf("logits[%d]=%.17g, want exactly d=%.17g; logits=%v", i, value, d, logits)
				}
			}
			q := math.Exp(d)
			denominator := 1 + 2*q
			targetGradient := -2 * q / denominator
			otherGradient := q / denominator
			if !positiveFiniteDistill147(-targetGradient) || !positiveFiniteDistill147(otherGradient) {
				t.Fatalf("analytic gradient invalid: d=%.17g target=%.17g other=%.17g", d, targetGradient, otherGradient)
			}
			upstream := make([][]float64, len(input))
			for row := range upstream {
				upstream[row] = make([]float64, len(logits))
			}
			last := len(upstream) - 1
			for class := range upstream[last] {
				if class == testCase.target {
					upstream[last][class] = targetGradient
				} else {
					upstream[last][class] = otherGradient
				}
			}

			tracking := &trackingDistillStudent147{trainer: public}
			var publicReport distill.StepReport
			var publicErr error
			if testCase.mode == "label" {
				publicReport, publicErr = distill.StepLabel(ctx, tracking, input, testCase.target, 3)
			} else {
				publicReport, publicErr = distill.StepDistribution(ctx, tracking, distill.DistributionDistiller{
					Temperature: 2,
					Scale:       3,
					Mix:         0,
					Alignment:   distillPublicAlignment147(),
				}, input, distillPublicTeacher147(testCase.target), testCase.target)
			}
			publicAfter := public.Snapshot()
			referenceResult, referenceErr := reference.StepFrom(ctx, input, upstream)
			referenceAfter := reference.Snapshot()
			beforeHash := distillPublicSnapshotSHA147(t, beforePublic)
			publicAfterHash := distillPublicSnapshotSHA147(t, publicAfter)
			referenceAfterHash := distillPublicSnapshotSHA147(t, referenceAfter)
			publicTargets := targetReadoutDistill147(publicAfter, testCase.target)
			referenceTargets := targetReadoutDistill147(referenceAfter, testCase.target)
			t.Logf("actual_d=%.17g reference_gradient=%v public_target_readout=%v reference_target_readout=%v before_snapshot_json_sha256=%x public_after_snapshot_json_sha256=%x reference_after_snapshot_json_sha256=%x public_result=%+v reference_result=%+v predict_count=%d step_count=%d actual_upstream=%v", d, upstream[last], publicTargets, referenceTargets, beforeHash, publicAfterHash, referenceAfterHash, tracking.result, referenceResult, tracking.predicts, tracking.steps, tracking.lastUpstream)

			if publicErr != nil {
				t.Errorf("public distill step error: %v", publicErr)
			}
			if referenceErr != nil {
				t.Errorf("reference StepFrom error: %v", referenceErr)
			}
			if tracking.predicts != 1 || tracking.steps != 1 {
				t.Errorf("tracking calls predict=%d step=%d, want 1/1", tracking.predicts, tracking.steps)
			}
			assertDistillUpstream147(t, tracking.lastUpstream, upstream)
			if publicReport.Partial {
				t.Errorf("public report partial=%v, want false", publicReport.Partial)
			}
			if !finiteDistill147(publicReport.Loss) || publicReport.Loss < 0 {
				t.Errorf("public loss=%.17g, want finite and non-negative", publicReport.Loss)
			}
			if !tracking.result.Applied || !referenceResult.Applied {
				t.Errorf("Applied public=%v reference=%v, want both true", tracking.result.Applied, referenceResult.Applied)
			}
			if publicAfter.Updates != beforePublic.Updates+1 || referenceAfter.Updates != beforePublic.Updates+1 {
				t.Errorf("updates public=%d reference=%d, want %d", publicAfter.Updates, referenceAfter.Updates, beforePublic.Updates+1)
			}
			if tracking.result.Updates != beforePublic.Updates+1 || referenceResult.Updates != beforePublic.Updates+1 {
				t.Errorf("result updates public=%d reference=%d, want %d", tracking.result.Updates, referenceResult.Updates, beforePublic.Updates+1)
			}
			if tracking.result.LossKnown || referenceResult.LossKnown || tracking.result.Loss != 0 || referenceResult.Loss != 0 {
				t.Errorf("StepResult loss fields public=(%.17g, known=%v) reference=(%.17g, known=%v), want zero and unknown", tracking.result.Loss, tracking.result.LossKnown, referenceResult.Loss, referenceResult.LossKnown)
			}
			if !positiveFiniteDistill147(tracking.result.GradientNorm) || !positiveFiniteDistill147(referenceResult.GradientNorm) {
				t.Errorf("gradient norms public=%.17g reference=%.17g, want finite positive", tracking.result.GradientNorm, referenceResult.GradientNorm)
			}
			if !reflect.DeepEqual(beforePublic.Parameters.Core, publicAfter.Parameters.Core) || !reflect.DeepEqual(beforePublic.Parameters.Core, referenceAfter.Parameters.Core) {
				t.Errorf("core parameters changed")
			}
			if !reflect.DeepEqual(beforePublic.Parameters.Encoder, publicAfter.Parameters.Encoder) || !reflect.DeepEqual(beforePublic.Parameters.Encoder, referenceAfter.Parameters.Encoder) {
				t.Errorf("encoder parameters changed")
			}
			if !reflect.DeepEqual(beforeInput, input) {
				t.Errorf("input mutated by public/reference step: before=%v after=%v", beforeInput, input)
			}
			if len(referenceTargets) == 0 {
				t.Fatalf("reference target readout is empty")
			}
			nonzeroReference := false
			for _, value := range referenceTargets {
				if finiteDistill147(value) && value != 0 {
					nonzeroReference = true
					break
				}
			}
			if !nonzeroReference {
				t.Errorf("reference target readout has no nonzero entry")
			}
			compareTargetReadoutDistill147(t, publicTargets, referenceTargets)
		})
	}
}

func TestDistillMixedGradient147(t *testing.T) {
	student := []float64{0, 1, -1}
	teacher := distill.TeacherDistribution{Probabilities: []float64{.3, .2, .5}}
	studentBefore := append([]float64(nil), student...)
	teacherBefore := append([]float64(nil), teacher.Probabilities...)
	d := distill.DistributionDistiller{
		Temperature: 2,
		Scale:       3,
		Mix:         .5,
		Alignment:   distillPublicAlignment147(),
	}
	klLoss, klGradient, _, err := d.Loss(teacher, student, -1)
	if err != nil {
		t.Fatalf("public KL loss: %v", err)
	}
	mixedLoss, mixedGradient, _, err := d.Loss(teacher, student, 2)
	if err != nil {
		t.Fatalf("public mixed loss: %v", err)
	}
	if !positiveFiniteDistill147(klLoss) || !positiveFiniteDistill147(mixedLoss) {
		t.Fatalf("losses KL=%.17g mixed=%.17g, want finite positive", klLoss, mixedLoss)
	}
	if !reflect.DeepEqual(studentBefore, student) || !reflect.DeepEqual(teacherBefore, teacher.Probabilities) {
		t.Fatalf("public Loss mutated student or teacher: student before=%v after=%v teacher before=%v after=%v", studentBefore, student, teacherBefore, teacher.Probabilities)
	}
	maxStudent := student[0]
	for _, value := range student[1:] {
		if value > maxStudent {
			maxStudent = value
		}
	}
	normalizer := 0.0
	for _, value := range student {
		normalizer += math.Exp(value - maxStudent)
	}
	for class := range student {
		q := math.Exp(student[class]-maxStudent) / normalizer
		ceGradient := .5 * q
		if class == 2 {
			ceGradient -= .5
		}
		want := klGradient[class] + ceGradient
		if !finiteDistill147(klGradient[class]) || !finiteDistill147(mixedGradient[class]) || !finiteDistill147(want) {
			t.Errorf("class %d non-finite gradients KL=%.17g mixed=%.17g want=%.17g", class, klGradient[class], mixedGradient[class], want)
			continue
		}
		if ulp := ulpDistill147(mixedGradient[class], want); ulp > 8 {
			t.Errorf("class %d mixed gradient=%.17g want KL %.17g + CE %.17g = %.17g, ULP=%d want <=8", class, mixedGradient[class], klGradient[class], ceGradient, want, ulp)
		}
	}
}
