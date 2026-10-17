package experiment

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/experiment/nav2d"
	"github.com/TimLai666/coimnet/learning"
)

func experimentGradientSnapshotSHA147(t *testing.T, snapshot learning.TrainingSnapshot) [32]byte {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func experimentGradientCopyRows147(rows [][]float64) [][]float64 {
	copyRows := make([][]float64, len(rows))
	for i := range rows {
		copyRows[i] = append([]float64(nil), rows[i]...)
	}
	return copyRows
}

func experimentGradientSameFloatBits147(a, b []float64) bool {
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

func experimentGradientSameRows147(a, b [][]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !experimentGradientSameFloatBits147(a[i], b[i]) {
			return false
		}
	}
	return true
}

func experimentGradientFiniteParameters147(p learning.Parameters) bool {
	groups := [][]float64{
		p.Core.Weights, p.Core.Bias, p.Core.LogTau, p.ThetaRaw, p.Encoder, p.Readout,
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

func experimentGradientSameFrozen147(a, b learning.Parameters) bool {
	return experimentGradientSameFloatBits147(a.Core.Weights, b.Core.Weights) &&
		experimentGradientSameFloatBits147(a.Core.Bias, b.Core.Bias) &&
		experimentGradientSameFloatBits147(a.Core.LogTau, b.Core.LogTau) &&
		experimentGradientSameFloatBits147(a.ThetaRaw, b.ThetaRaw) &&
		experimentGradientSameFloatBits147(a.Encoder, b.Encoder)
}

func experimentGradientReadoutColumn147(p learning.Parameters, outputSize, column int) []float64 {
	values := make([]float64, 0)
	for i, value := range p.Readout {
		if i%outputSize == column {
			values = append(values, value)
		}
	}
	return values
}

func experimentGradientSameReadoutColumn147(a, b learning.Parameters, outputSize, column int) bool {
	if outputSize <= 0 || column < 0 || column >= outputSize || len(a.Readout) != len(b.Readout) {
		return false
	}
	for i := range a.Readout {
		if i%outputSize == column && math.Float64bits(a.Readout[i]) != math.Float64bits(b.Readout[i]) {
			return false
		}
	}
	return true
}

func experimentGradientAssertZeroOptimizer147(t *testing.T, snapshot learning.TrainingSnapshot) {
	t.Helper()
	if snapshot.Updates != 0 {
		t.Fatalf("initial updates=%d, want 0", snapshot.Updates)
	}
	if len(snapshot.Optimizer.First) != len(snapshot.Optimizer.Second) || len(snapshot.Optimizer.First) != len(snapshot.Optimizer.Steps) {
		t.Fatalf("optimizer lengths=%d/%d/%d, want equal", len(snapshot.Optimizer.First), len(snapshot.Optimizer.Second), len(snapshot.Optimizer.Steps))
	}
	for i := range snapshot.Optimizer.First {
		if snapshot.Optimizer.First[i] != 0 || snapshot.Optimizer.Second[i] != 0 || snapshot.Optimizer.Steps[i] != 0 {
			t.Fatalf("optimizer[%d]=first %.17g second %.17g steps %d, want zero", i, snapshot.Optimizer.First[i], snapshot.Optimizer.Second[i], snapshot.Optimizer.Steps[i])
		}
	}
}

func experimentGradientNewTrainer147(t *testing.T, inputWidth, classes int) *learning.Trainer {
	t.Helper()
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 1, Sources: []int{0}, Targets: []int{0}, DT: 1, Activation: "tanh",
		},
		InputSize: inputWidth, OutputSize: classes, InputNodes: []int{0}, ReadoutNodes: []int{0}, ReadoutEveryStep: true,
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{0}, Bias: []float64{1}, LogTau: []float64{-10}},
		Encoder: make([]float64, inputWidth), Readout: make([]float64, classes),
	}
	options := learning.DefaultOptions()
	options.LearningRate = .01
	options.WeightDecay = 0
	options.ClipNorm = 0
	options.Trainable = learning.Trainable{Readout: true}
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		t.Fatalf("learning.NewTrainer artificial experiment fixture: %v", err)
	}
	return trainer
}

func experimentGradientPrepare147(t *testing.T, ctx context.Context, input [][]float64, inputWidth, classes, target int, gap float64, excluded int) (public, reference *learning.Trainer, before learning.TrainingSnapshot) {
	t.Helper()
	if len(input) == 0 || inputWidth <= 0 || classes <= 0 || target < 0 || target >= classes || excluded >= classes {
		t.Fatalf("invalid fixture dimensions input=%d width=%d classes=%d target=%d excluded=%d", len(input), inputWidth, classes, target, excluded)
	}
	base := experimentGradientNewTrainer147(t, inputWidth, classes)
	baseSnapshot := base.Snapshot()
	for i := range baseSnapshot.Parameters.Readout {
		baseSnapshot.Parameters.Readout[i] = 1
	}
	baseline, err := learning.NewTrainer(baseSnapshot.Config, baseSnapshot.Parameters, baseSnapshot.Options)
	if err != nil {
		t.Fatalf("baseline learning.NewTrainer: %v", err)
	}
	baselineLogits, err := baseline.PredictAll(ctx, input)
	if err != nil {
		t.Fatalf("baseline PredictAll: %v", err)
	}
	if len(baselineLogits) != len(input) || len(baselineLogits) == 0 || len(baselineLogits[0]) != classes {
		width := 0
		if len(baselineLogits) > 0 {
			width = len(baselineLogits[0])
		}
		t.Fatalf("baseline logits shape=%d x %d, want %d x %d", len(baselineLogits), width, len(input), classes)
	}
	z := baselineLogits[len(baselineLogits)-1][0]
	if math.IsNaN(z) || math.IsInf(z, 0) || z <= 0 {
		t.Fatalf("baseline final z=%.17g, want finite positive", z)
	}
	for row, logits := range baselineLogits {
		for class, value := range logits {
			if math.IsNaN(value) || math.IsInf(value, 0) || value != z {
				t.Fatalf("baseline logits[%d][%d]=%.17g, want constant finite z=%.17g", row, class, value, z)
			}
		}
	}
	if excluded == target {
		t.Fatalf("excluded output %d cannot be target %d", excluded, target)
	}
	blankReadout := 1000 / z
	nonTargetReadout := -gap / z
	if math.IsNaN(blankReadout) || math.IsInf(blankReadout, 0) || math.IsNaN(nonTargetReadout) || math.IsInf(nonTargetReadout, 0) {
		t.Fatalf("scaled readout blank=%.17g non-target=%.17g, want finite", blankReadout, nonTargetReadout)
	}
	tail := baseline.Snapshot()
	experimentGradientAssertZeroOptimizer147(t, tail)
	for i := range tail.Parameters.Readout {
		switch column := i % classes; {
		case column == target:
			tail.Parameters.Readout[i] = 0
		case column == excluded:
			tail.Parameters.Readout[i] = blankReadout
		default:
			tail.Parameters.Readout[i] = nonTargetReadout
		}
	}
	public, err = learning.NewTrainer(tail.Config, tail.Parameters, tail.Options)
	if err != nil {
		t.Fatalf("public fresh Trainer: %v", err)
	}
	reference, err = learning.NewTrainer(tail.Config, tail.Parameters, tail.Options)
	if err != nil {
		t.Fatalf("reference fresh Trainer: %v", err)
	}
	before = public.Snapshot()
	if !reflect.DeepEqual(before, reference.Snapshot()) {
		t.Fatalf("public/reference initial snapshots differ")
	}
	if before.Options.Trainable != (learning.Trainable{Readout: true}) {
		t.Fatalf("trainable groups=%+v, want readout only", before.Options.Trainable)
	}
	experimentGradientAssertZeroOptimizer147(t, before)
	for i, value := range before.Parameters.Readout {
		if i%classes == target && value != 0 {
			t.Fatalf("initial target readout[%d]=%.17g, want exact zero", i, value)
		}
	}
	return public, reference, before
}

func experimentGradientActualRow147(t *testing.T, logits [][]float64, classes int) []float64 {
	t.Helper()
	if len(logits) == 0 || len(logits[0]) != classes {
		t.Fatalf("logits shape rows=%d width=%d, want nonempty width %d", len(logits), func() int {
			if len(logits) == 0 {
				return 0
			}
			return len(logits[0])
		}(), classes)
	}
	first := append([]float64(nil), logits[0]...)
	for row, values := range logits {
		if len(values) != classes {
			t.Fatalf("logits row %d width=%d, want %d", row, len(values), classes)
		}
		for class, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("logits[%d][%d]=%.17g, want finite", row, class, value)
			}
			if math.Float64bits(value) != math.Float64bits(first[class]) {
				t.Fatalf("logits[%d][%d]=%.17g differs from row0 %.17g", row, class, value, first[class])
			}
		}
	}
	return first
}

func experimentGradientCheckTail147(t *testing.T, logits [][]float64, classes, target, excluded int, gap float64) float64 {
	t.Helper()
	row := experimentGradientActualRow147(t, logits, classes)
	if row[target] != 0 {
		t.Fatalf("target logits[%d]=%.17g, want exact 0; row=%v", target, row[target], row)
	}
	nonTarget := -1
	for class := 0; class < classes; class++ {
		if class != target && class != excluded {
			nonTarget = class
			break
		}
	}
	if nonTarget < 0 {
		t.Fatalf("no non-target class remains in width %d", classes)
	}
	d := row[nonTarget]
	if d >= 0 || math.Abs(d+gap) > 1e-4 {
		t.Fatalf("non-target d=%.17g, want finite negative near -%.17g; row=%v", d, gap, row)
	}
	for class, value := range row {
		if class == target {
			continue
		}
		if class == excluded {
			if math.Abs(value-1000) > 1e-3 {
				t.Fatalf("excluded output[%d]=%.17g, want near 1000; row=%v", class, value, row)
			}
			continue
		}
		if value != d {
			t.Fatalf("non-target output[%d]=%.17g, want exactly d=%.17g; row=%v", class, value, d, row)
		}
	}
	return d
}

func experimentGradientIndependent147(t *testing.T, row []float64, classes, target int, factor float64) ([]float64, float64) {
	t.Helper()
	if len(row) < classes || target < 0 || target >= classes || !finite(factor) || factor <= 0 {
		t.Fatalf("invalid analytic row/classes/target/factor: width=%d classes=%d target=%d factor=%.17g", len(row), classes, target, factor)
	}
	high := row[0]
	for _, value := range row[:classes] {
		if value > high {
			high = value
		}
	}
	exps := make([]float64, classes)
	sum := 0.0
	for class, value := range row[:classes] {
		exps[class] = math.Exp(value - high)
		sum += exps[class]
	}
	if !(sum > 0) || math.IsNaN(sum) || math.IsInf(sum, 0) {
		t.Fatalf("analytic exp sum=%.17g, want finite positive", sum)
	}
	otherSum := 0.0
	for class, value := range exps {
		if class != target {
			otherSum += value
		}
	}
	gradient := make([]float64, classes)
	for class, value := range exps {
		if class == target {
			gradient[class] = -otherSum / sum
		} else {
			gradient[class] = value / sum
		}
		gradient[class] *= factor
	}
	for class, value := range gradient {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("analytic gradient[%d]=%.17g, want finite", class, value)
		}
		if class == target && value >= 0 {
			t.Fatalf("analytic target gradient=%.17g, want finite negative", value)
		}
		if class != target && value <= 0 {
			t.Fatalf("analytic non-target gradient[%d]=%.17g, want finite positive", class, value)
		}
	}
	return gradient, gradient[target]
}

func experimentGradientBuildUpstream147(t *testing.T, logits [][]float64, width, classes int, rows, targets []int, factors []float64) ([][]float64, []float64) {
	t.Helper()
	if len(rows) != len(targets) || len(rows) != len(factors) {
		t.Fatalf("reference row/target/factor lengths=%d/%d/%d", len(rows), len(targets), len(factors))
	}
	upstream := make([][]float64, len(logits))
	for row := range upstream {
		upstream[row] = make([]float64, width)
	}
	targetGradients := make([]float64, 0, len(rows))
	for i, row := range rows {
		if row < 0 || row >= len(logits) {
			t.Fatalf("reference scored row %d outside logits rows %d", row, len(logits))
		}
		gradient, targetGradient := experimentGradientIndependent147(t, logits[row], classes, targets[i], factors[i])
		copy(upstream[row], gradient)
		targetGradients = append(targetGradients, targetGradient)
	}
	return upstream, targetGradients
}

func experimentGradientCompareVector147(t *testing.T, label string, public, reference []float64) {
	t.Helper()
	if len(public) != len(reference) {
		t.Fatalf("%s lengths=%d/%d, want equal", label, len(public), len(reference))
	}
	for i, want := range reference {
		got := public[i]
		if math.IsNaN(got) || math.IsInf(got, 0) || math.IsNaN(want) || math.IsInf(want, 0) {
			t.Errorf("%s[%d] public=%.17g reference=%.17g, want finite", label, i, got, want)
			continue
		}
		if want == 0 {
			if got != 0 {
				t.Errorf("%s[%d] public=%.17g reference=0, want exact zero", label, i, got)
			}
			continue
		}
		if got == 0 {
			t.Errorf("%s[%d] became zero, reference=%.17g", label, i, want)
			continue
		}
		relative := math.Abs(got-want) / math.Abs(want)
		if relative > 2e-6 {
			t.Errorf("%s[%d] public=%.17g reference=%.17g relative=%.17g, want <=2e-6", label, i, got, want, relative)
		}
	}
}

func experimentGradientAssertStepResult147(t *testing.T, result learning.StepResult, wantUpdates uint64) {
	t.Helper()
	if !result.Applied {
		t.Errorf("reference StepFrom Applied=%v, want true", result.Applied)
	}
	if result.Updates != wantUpdates {
		t.Errorf("reference StepFrom updates=%d, want %d", result.Updates, wantUpdates)
	}
	if result.LossKnown || result.Loss != 0 {
		t.Errorf("reference StepFrom loss=(%.17g, known=%v), want zero and unknown", result.Loss, result.LossKnown)
	}
	if math.IsNaN(result.GradientNorm) || math.IsInf(result.GradientNorm, 0) || result.GradientNorm <= 0 {
		t.Errorf("reference StepFrom gradient norm=%.17g, want finite positive", result.GradientNorm)
	}
}

func experimentGradientFinish147(t *testing.T, label string, before, publicAfter, referenceAfter learning.TrainingSnapshot, publicUpdates, referenceUpdates, wantUpdates uint64, input, inputBefore [][]float64, target, excluded int, high bool) {
	t.Helper()
	if publicUpdates != wantUpdates || referenceUpdates != wantUpdates {
		t.Errorf("%s update returns public=%d reference=%d, want %d", label, publicUpdates, referenceUpdates, wantUpdates)
	}
	if publicAfter.Updates != wantUpdates || referenceAfter.Updates != wantUpdates {
		t.Errorf("%s snapshot updates public=%d reference=%d, want %d", label, publicAfter.Updates, referenceAfter.Updates, wantUpdates)
	}
	if !experimentGradientFiniteParameters147(publicAfter.Parameters) || !experimentGradientFiniteParameters147(referenceAfter.Parameters) {
		t.Errorf("%s public/reference parameters contain non-finite values", label)
	}
	if !experimentGradientSameFrozen147(before.Parameters, publicAfter.Parameters) || !experimentGradientSameFrozen147(before.Parameters, referenceAfter.Parameters) {
		t.Errorf("%s core or encoder parameters changed", label)
	}
	if excluded >= 0 && (!experimentGradientSameReadoutColumn147(before.Parameters, publicAfter.Parameters, before.Config.OutputSize, excluded) || !experimentGradientSameReadoutColumn147(before.Parameters, referenceAfter.Parameters, before.Config.OutputSize, excluded)) {
		t.Errorf("%s excluded readout column %d changed", label, excluded)
	}
	if !experimentGradientSameRows147(input, inputBefore) {
		t.Errorf("%s supplied input rows changed", label)
	}
	experimentGradientCompareVector147(t, label+" readout", publicAfter.Parameters.Readout, referenceAfter.Parameters.Readout)
	experimentGradientCompareVector147(t, label+" adam_first", publicAfter.Optimizer.First, referenceAfter.Optimizer.First)
	experimentGradientCompareVector147(t, label+" adam_second", publicAfter.Optimizer.Second, referenceAfter.Optimizer.Second)
	if !reflect.DeepEqual(publicAfter.Optimizer.Steps, referenceAfter.Optimizer.Steps) {
		t.Errorf("%s optimizer steps differ", label)
	}
	if publicAfter.Updates != referenceAfter.Updates {
		t.Errorf("%s update counts differ public=%d reference=%d", label, publicAfter.Updates, referenceAfter.Updates)
	}
	publicTarget := experimentGradientReadoutColumn147(publicAfter.Parameters, before.Config.OutputSize, target)
	referenceTarget := experimentGradientReadoutColumn147(referenceAfter.Parameters, before.Config.OutputSize, target)
	if high {
		nonzero := false
		for _, value := range referenceTarget {
			if value != 0 {
				nonzero = true
				break
			}
		}
		if !nonzero {
			t.Errorf("%s high-confidence reference target readout is all zero", label)
		}
	}
	experimentGradientCompareVector147(t, label+" target_readout", publicTarget, referenceTarget)
}

func experimentGradientZeroRows147(rows, width int) [][]float64 {
	input := make([][]float64, rows)
	for i := range input {
		input[i] = make([]float64, width)
	}
	return input
}

func experimentGradientRepeatRows147(obs [][]float64, settle int) [][]float64 {
	rows := make([][]float64, 0, len(obs)*settle)
	for _, observation := range obs {
		for i := 0; i < settle; i++ {
			rows = append(rows, append([]float64(nil), observation...))
		}
	}
	return rows
}

func experimentGradientSameExperienceRows147(obs [][]float64) [][]float64 {
	rows := 5
	if n := len(obs) * 3; n > rows {
		rows = n
	}
	input := make([][]float64, rows)
	for i := range input {
		input[i] = make([]float64, 5)
	}
	for k, observation := range obs {
		for r := k * 3; r < (k+1)*3; r++ {
			copy(input[r][:4], observation)
		}
	}
	return input
}

func experimentGradientLabelWeights147(t *testing.T, actions []int) []float64 {
	t.Helper()
	counts := make([]int, nav2d.Actions)
	distinct := 0
	for _, action := range actions {
		if action < 0 || action >= nav2d.Actions {
			t.Fatalf("action %d outside [0,%d)", action, nav2d.Actions)
		}
		if counts[action] == 0 {
			distinct++
		}
		counts[action]++
	}
	if len(actions) == 0 || distinct == 0 {
		t.Fatalf("empty action labels")
	}
	weights := make([]float64, nav2d.Actions)
	for action, count := range counts {
		if count != 0 {
			weights[action] = float64(len(actions)) / (float64(distinct) * float64(count))
		}
	}
	return weights
}

func experimentGradientRunImitation147(t *testing.T, gap float64, actions []int) {
	t.Helper()
	ctx := context.Background()
	input := experimentGradientZeroRows147(len(actions), 1)
	inputBefore := experimentGradientCopyRows147(input)
	public, reference, before := experimentGradientPrepare147(t, ctx, input, 1, gridnav.Actions, 0, gap, -1)
	publicLogits, err := public.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	referenceInput := experimentGradientCopyRows147(input)
	referenceLogits, err := reference.PredictAll(ctx, referenceInput)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(publicLogits, referenceLogits) {
		t.Fatalf("imitation public/reference initial logits differ")
	}
	actualD := experimentGradientCheckTail147(t, publicLogits, gridnav.Actions, 0, -1, gap)
	scoredRows := make([]int, len(actions))
	factors := make([]float64, len(actions))
	for i := range scoredRows {
		scoredRows[i] = i
		factors[i] = 1 / float64(len(actions))
	}
	upstream, targetGradients := experimentGradientBuildUpstream147(t, referenceLogits, gridnav.Actions, gridnav.Actions, scoredRows, actions, factors)
	publicUpdates, publicErr := imitationStep(ctx, public, input, actions)
	publicAfter := public.Snapshot()
	referenceResult, referenceErr := reference.StepFrom(ctx, referenceInput, upstream)
	referenceAfter := reference.Snapshot()
	beforeHash := experimentGradientSnapshotSHA147(t, before)
	publicHash := experimentGradientSnapshotSHA147(t, publicAfter)
	referenceHash := experimentGradientSnapshotSHA147(t, referenceAfter)
	t.Logf("imitation gap=%.17g actual_d=%.17g target_gradients=%v public_target_readout=%v reference_target_readout=%v before_snapshot_json_sha256=%x public_after_snapshot_json_sha256=%x reference_after_snapshot_json_sha256=%x public_updates=%d reference_result=%+v", gap, actualD, targetGradients, experimentGradientReadoutColumn147(publicAfter.Parameters, gridnav.Actions, 0), experimentGradientReadoutColumn147(referenceAfter.Parameters, gridnav.Actions, 0), beforeHash, publicHash, referenceHash, publicUpdates, referenceResult)
	if publicErr != nil {
		t.Errorf("imitationStep: %v", publicErr)
	}
	if referenceErr != nil {
		t.Errorf("imitation reference StepFrom: %v", referenceErr)
	}
	if publicErr != nil || referenceErr != nil {
		return
	}
	experimentGradientAssertStepResult147(t, referenceResult, 1)
	experimentGradientFinish147(t, "imitation", before, publicAfter, referenceAfter, publicUpdates, referenceResult.Updates, 1, input, inputBefore, 0, -1, gap == 40)
	if !experimentGradientSameRows147(referenceInput, inputBefore) {
		t.Errorf("imitation reference input rows changed")
	}
}

func experimentGradientCorridorRows147(obs [][]float64) [][]float64 {
	input := make([][]float64, 0, len(obs)*3)
	for _, observation := range obs {
		for i := 0; i < 3; i++ {
			row := make([]float64, 5)
			copy(row[:4], observation)
			input = append(input, row)
		}
	}
	return input
}

func experimentGradientRunCorridor147(t *testing.T, gap float64, target int, obs [][]float64, actions []int) {
	t.Helper()
	ctx := context.Background()
	input := experimentGradientCorridorRows147(obs)
	inputBefore := experimentGradientCopyRows147(input)
	public, reference, before := experimentGradientPrepare147(t, ctx, input, 5, 4, target, gap, 3)
	publicLogits, err := public.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	referenceInput := experimentGradientCopyRows147(input)
	referenceLogits, err := reference.PredictAll(ctx, referenceInput)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(publicLogits, referenceLogits) {
		t.Fatalf("corridor public/reference initial logits differ")
	}
	actualD := experimentGradientCheckTail147(t, publicLogits, 4, target, 3, gap)
	scoredRows := make([]int, len(actions))
	factors := make([]float64, len(actions))
	for i := range scoredRows {
		scoredRows[i] = (i+1)*3 - 1
		factors[i] = 3.25 / float64(len(obs))
	}
	upstream, targetGradients := experimentGradientBuildUpstream147(t, referenceLogits, 4, gridnav.Actions, scoredRows, actions, factors)
	publicModel := &multiTaskModel{trainer: public, hidden: 1, built: 1}
	publicObs := experimentGradientCopyRows147(obs)
	publicErr := publicModel.corridorStep(ctx, publicObs, actions, 3.25)
	publicAfter := public.Snapshot()
	referenceResult, referenceErr := reference.StepFrom(ctx, referenceInput, upstream)
	referenceAfter := reference.Snapshot()
	beforeHash := experimentGradientSnapshotSHA147(t, before)
	publicHash := experimentGradientSnapshotSHA147(t, publicAfter)
	referenceHash := experimentGradientSnapshotSHA147(t, referenceAfter)
	t.Logf("corridor gap=%.17g actual_d=%.17g target=%d target_gradients=%v public_target_readout=%v reference_target_readout=%v before_snapshot_json_sha256=%x public_after_snapshot_json_sha256=%x reference_after_snapshot_json_sha256=%x reference_result=%+v", gap, actualD, target, targetGradients, experimentGradientReadoutColumn147(publicAfter.Parameters, 4, target), experimentGradientReadoutColumn147(referenceAfter.Parameters, 4, target), beforeHash, publicHash, referenceHash, referenceResult)
	if publicErr != nil {
		t.Errorf("corridorStep: %v", publicErr)
	}
	if referenceErr != nil {
		t.Errorf("corridor reference StepFrom: %v", referenceErr)
	}
	if publicErr != nil || referenceErr != nil {
		return
	}
	experimentGradientAssertStepResult147(t, referenceResult, 1)
	experimentGradientFinish147(t, "corridor", before, publicAfter, referenceAfter, publicAfter.Updates, referenceResult.Updates, 1, referenceInput, inputBefore, target, 3, gap == 40)
	if !experimentGradientSameRows147(publicObs, obs) {
		t.Errorf("corridorStep mutated observations")
	}
}

func experimentGradientRunSameExperience147(t *testing.T, gap float64) {
	t.Helper()
	ctx := context.Background()
	corridor := gridnav.Config{Length: 5, TimeLimit: 6, StepPenalty: .01, GoalReward: 1}
	obs, actions, err := imitationExpertEpisode(corridor, imitationTrainSeed(17, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) < 2 || len(actions) != len(obs) {
		t.Fatalf("same-experience expert rows=%d actions=%d, want at least 2 equal counts", len(obs), len(actions))
	}
	for i, action := range actions {
		if action != actions[0] {
			t.Fatalf("same-experience action[%d]=%d differs from action0=%d", i, action, actions[0])
		}
		if action != gridnav.ActionLeft {
			t.Fatalf("same-experience action[%d]=%d, want winner action 0", i, action)
		}
	}
	input := experimentGradientSameExperienceRows147(obs)
	if len(input) < 5 {
		t.Fatalf("same-experience input rows=%d, want at least 5", len(input))
	}
	inputBefore := experimentGradientCopyRows147(input)
	public, reference, before := experimentGradientPrepare147(t, ctx, input, 5, 4, 0, gap, 3)
	publicLogits, err := public.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	referenceInput := experimentGradientCopyRows147(input)
	referenceLogits, err := reference.PredictAll(ctx, referenceInput)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(publicLogits, referenceLogits) {
		t.Fatalf("same-experience public/reference initial logits differ")
	}
	actualD := experimentGradientCheckTail147(t, publicLogits, 4, 0, 3, gap)
	scoredRows := make([]int, len(actions))
	targets := append([]int(nil), actions...)
	factors := make([]float64, len(actions))
	for i := range scoredRows {
		scoredRows[i] = (i+1)*3 - 1
		factors[i] = 2 * 3 / float64(len(obs))
	}
	upstream, targetGradients := experimentGradientBuildUpstream147(t, referenceLogits, 4, gridnav.Actions, scoredRows, targets, factors)
	tasks := []TaskBinding{
		{Name: MultiTaskCorridor, LossScale: 3, MixWeight: 2, SamplingRatio: 1, UpdateEvery: 1},
		{Name: MultiTaskDelayed, LossScale: 1, MixWeight: 1, SamplingRatio: 1, UpdateEvery: 1},
	}
	publicModel := &multiTaskModel{trainer: public, hidden: 1, built: 1}
	publicObs := experimentGradientCopyRows147(obs)
	publicErr := runSameExperience(ctx, publicModel, tasks, []int{0}, corridor, 17, []uint64{0, 0})
	publicAfter := public.Snapshot()
	referenceResult, referenceErr := reference.StepFrom(ctx, referenceInput, upstream)
	referenceAfter := reference.Snapshot()
	beforeHash := experimentGradientSnapshotSHA147(t, before)
	publicHash := experimentGradientSnapshotSHA147(t, publicAfter)
	referenceHash := experimentGradientSnapshotSHA147(t, referenceAfter)
	t.Logf("same_experience gap=%.17g actual_d=%.17g obs=%d target_gradients=%v public_target_readout=%v reference_target_readout=%v before_snapshot_json_sha256=%x public_after_snapshot_json_sha256=%x reference_after_snapshot_json_sha256=%x reference_result=%+v", gap, actualD, len(obs), targetGradients, experimentGradientReadoutColumn147(publicAfter.Parameters, 4, 0), experimentGradientReadoutColumn147(referenceAfter.Parameters, 4, 0), beforeHash, publicHash, referenceHash, referenceResult)
	if publicErr != nil {
		t.Errorf("runSameExperience: %v", publicErr)
	}
	if referenceErr != nil {
		t.Errorf("same-experience reference StepFrom: %v", referenceErr)
	}
	if publicErr != nil || referenceErr != nil {
		return
	}
	experimentGradientAssertStepResult147(t, referenceResult, 1)
	experimentGradientFinish147(t, "same_experience", before, publicAfter, referenceAfter, publicAfter.Updates, referenceResult.Updates, 1, referenceInput, inputBefore, 0, 3, gap == 40)
	if !experimentGradientSameRows147(publicObs, obs) {
		t.Errorf("runSameExperience mutated observations")
	}
}

func experimentGradientRunNav147(t *testing.T, kind string, gap float64, actions []int) {
	t.Helper()
	ctx := context.Background()
	obs := experimentGradientZeroRows147(len(actions), 2)
	input := experimentGradientRepeatRows147(obs, 3)
	inputBefore := experimentGradientCopyRows147(input)
	public, reference, before := experimentGradientPrepare147(t, ctx, input, 2, nav2d.Actions, 0, gap, -1)
	publicLogits, err := public.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	referenceInput := experimentGradientCopyRows147(input)
	referenceLogits, err := reference.PredictAll(ctx, referenceInput)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(publicLogits, referenceLogits) {
		t.Fatalf("nav %s public/reference initial logits differ", kind)
	}
	actualD := experimentGradientCheckTail147(t, publicLogits, nav2d.Actions, 0, -1, gap)
	weights := experimentGradientLabelWeights147(t, actions)
	publicPolicy := &nav2dPolicy{kind: kind, trainer: public, settle: 3}
	publicObs := experimentGradientCopyRows147(obs)
	var publicUpdates uint64
	var publicErr error
	var referenceUpdates uint64
	var referenceResult learning.StepResult
	var referenceErr error
	var targetGradients []float64
	if kind == Nav2DRecurrent || kind == Nav2DRewired {
		scoredRows := make([]int, len(actions))
		targets := append([]int(nil), actions...)
		factors := make([]float64, len(actions))
		for i := range scoredRows {
			scoredRows[i] = (i+1)*3 - 1
			factors[i] = weights[actions[i]] / float64(len(actions))
		}
		upstream, gradients := experimentGradientBuildUpstream147(t, referenceLogits, nav2d.Actions, nav2d.Actions, scoredRows, targets, factors)
		targetGradients = gradients
		publicUpdates, publicErr = publicPolicy.train(ctx, publicObs, actions)
		referenceResult, referenceErr = reference.StepFrom(ctx, referenceInput, upstream)
		referenceUpdates = referenceResult.Updates
	} else {
		publicUpdates, publicErr = publicPolicy.train(ctx, publicObs, actions)
		for i, observation := range obs {
			stepInput := experimentGradientRepeatRows147([][]float64{observation}, 3)
			stepLogits, err := reference.PredictAll(ctx, stepInput)
			if err != nil {
				referenceErr = err
				break
			}
			row := experimentGradientActualRow147(t, stepLogits, nav2d.Actions)
			if i == 0 && !experimentGradientSameFloatBits147(row, publicLogits[0]) {
				t.Fatalf("nav feedforward first-step logits differ from initial tail row")
			}
			gradient, targetGradient := experimentGradientIndependent147(t, row, nav2d.Actions, actions[i], weights[actions[i]])
			targetGradients = append(targetGradients, targetGradient)
			upstream := make([][]float64, 3)
			for row := range upstream {
				upstream[row] = make([]float64, nav2d.Actions)
			}
			copy(upstream[2], gradient)
			referenceResult, referenceErr = reference.StepFrom(ctx, stepInput, upstream)
			if referenceErr != nil {
				break
			}
			referenceUpdates = referenceResult.Updates
			experimentGradientAssertStepResult147(t, referenceResult, uint64(i+1))
		}
	}
	publicAfter := public.Snapshot()
	referenceAfter := reference.Snapshot()
	beforeHash := experimentGradientSnapshotSHA147(t, before)
	publicHash := experimentGradientSnapshotSHA147(t, publicAfter)
	referenceHash := experimentGradientSnapshotSHA147(t, referenceAfter)
	t.Logf("nav kind=%s gap=%.17g actual_d=%.17g target_gradients=%v public_target_readout=%v reference_target_readout=%v before_snapshot_json_sha256=%x public_after_snapshot_json_sha256=%x reference_after_snapshot_json_sha256=%x public_updates=%d reference_result=%+v", kind, gap, actualD, targetGradients, experimentGradientReadoutColumn147(publicAfter.Parameters, nav2d.Actions, 0), experimentGradientReadoutColumn147(referenceAfter.Parameters, nav2d.Actions, 0), beforeHash, publicHash, referenceHash, publicUpdates, referenceResult)
	if publicErr != nil {
		t.Errorf("nav %s train: %v", kind, publicErr)
	}
	if referenceErr != nil {
		t.Errorf("nav %s reference StepFrom: %v", kind, referenceErr)
	}
	if publicErr != nil || referenceErr != nil {
		return
	}
	wantUpdates := uint64(1)
	if kind == Nav2DFeedforward {
		wantUpdates = uint64(len(actions))
	} else {
		experimentGradientAssertStepResult147(t, referenceResult, 1)
	}
	experimentGradientFinish147(t, "nav_"+kind, before, publicAfter, referenceAfter, publicUpdates, referenceUpdates, wantUpdates, referenceInput, inputBefore, 0, -1, gap == 40)
	if !experimentGradientSameRows147(publicObs, obs) {
		t.Errorf("nav %s mutated observations", kind)
	}
}

func TestExperimentCategoricalGradient147(t *testing.T) {
	t.Run("imitation/high_gap40", func(t *testing.T) {
		experimentGradientRunImitation147(t, 40, []int{0, 0})
	})
	t.Run("imitation/normal_gap2", func(t *testing.T) {
		experimentGradientRunImitation147(t, 2, []int{0, 0, 1})
	})
	t.Run("corridor_step/high_gap40", func(t *testing.T) {
		experimentGradientRunCorridor147(t, 40, 1, [][]float64{{0, 0, 0, 0}, {0, 0, 0, 0}}, []int{1, 1})
	})
	t.Run("corridor_step/normal_gap2", func(t *testing.T) {
		experimentGradientRunCorridor147(t, 2, 0, [][]float64{{0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}, []int{0, 1, 0})
	})
	t.Run("same_experience/high_gap40", func(t *testing.T) {
		experimentGradientRunSameExperience147(t, 40)
	})
	t.Run("same_experience/normal_gap2", func(t *testing.T) {
		experimentGradientRunSameExperience147(t, 2)
	})
	for _, kind := range []string{Nav2DRecurrent, Nav2DRewired, Nav2DFeedforward} {
		kind := kind
		t.Run("nav_"+kind+"/high_gap40", func(t *testing.T) {
			experimentGradientRunNav147(t, kind, 40, []int{0, 0})
		})
		t.Run("nav_"+kind+"/normal_gap2", func(t *testing.T) {
			experimentGradientRunNav147(t, kind, 2, []int{0, 0, 1})
		})
	}
}
