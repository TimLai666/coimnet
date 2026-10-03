package main

import (
	"context"
	"math"
	"math/rand"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestNewModelContract(t *testing.T) {
	config, parameters, err := newModel(20261003)
	if err != nil {
		t.Fatal(err)
	}
	if config.InputSize != 6 || config.OutputSize != 2 || !config.ReadoutEveryStep {
		t.Fatalf("model dimensions = input %d output %d every=%t", config.InputSize, config.OutputSize, config.ReadoutEveryStep)
	}
	if config.Dynamics.Nodes != 8 || len(config.Dynamics.Sources) != 64 || len(config.Dynamics.Targets) != 64 || len(config.Dynamics.Delays) != 64 {
		t.Fatalf("model topology = nodes %d edges %d/%d delays %d", config.Dynamics.Nodes, len(config.Dynamics.Sources), len(config.Dynamics.Targets), len(config.Dynamics.Delays))
	}
	seen := make(map[[2]int]bool, 64)
	for i := range config.Dynamics.Sources {
		if config.Dynamics.Delays[i] != 0 {
			t.Fatalf("edge %d delay = %d, want zero", i, config.Dynamics.Delays[i])
		}
		key := [2]int{config.Dynamics.Sources[i], config.Dynamics.Targets[i]}
		if seen[key] {
			t.Fatalf("duplicate edge %v", key)
		}
		seen[key] = true
	}
	if len(seen) != 64 || config.InputNodes != nil {
		t.Fatalf("input mapping = %#v, want dense six-to-eight encoder", config.InputNodes)
	}
	wantReadout := []int{0, 1, 2, 3, 4, 5, 6, 7}
	if !reflect.DeepEqual(config.ReadoutNodes, wantReadout) {
		t.Fatalf("readout nodes = %v, want %v", config.ReadoutNodes, wantReadout)
	}
	if len(parameters.Core.Weights) != 64 || len(parameters.Core.Bias) != 8 || len(parameters.Core.LogTau) != 8 || len(parameters.Encoder) != 48 || len(parameters.Readout) != 16 {
		t.Fatalf("parameter shapes = core(%d,%d,%d) encoder=%d readout=%d", len(parameters.Core.Weights), len(parameters.Core.Bias), len(parameters.Core.LogTau), len(parameters.Encoder), len(parameters.Readout))
	}
	wantTau := []float64{2, 4, 8, 16, 64, 256, 1024, 4096}
	for i, tau := range wantTau {
		if got := math.Exp(parameters.Core.LogTau[i]); math.Abs(got-tau) > 1e-12*math.Max(1, tau) {
			t.Fatalf("tau[%d] = %.17g, want %.17g", i, got, tau)
		}
	}
	for i, value := range parameters.Readout {
		if value != 0 {
			t.Fatalf("readout[%d] = %g, want zero", i, value)
		}
	}
	options := trainingOptions()
	if options.LearningRate != .001 || options.ClipNorm != 1 || options.WeightDecay != 0 || options.Truncation != 0 || options.Recompute != nil {
		t.Fatalf("training options = %+v", options)
	}
	if !options.Trainable.Encoder || !options.Trainable.Weights || !options.Trainable.Bias || !options.Trainable.Tau || !options.Trainable.Readout {
		t.Fatalf("trainable groups = %+v", options.Trainable)
	}
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		t.Fatal(err)
	}
	capacity := trainer.Capacity()
	if capacity.Nodes != 8 || capacity.Edges != 64 || capacity.StateDimension != 1 || capacity.ParameterCount != 144 || capacity.FreeParameterCount != 144 || capacity.MultAddsPerStep != 72 {
		t.Fatalf("capacity = %+v", capacity)
	}
}

func TestMaskedMSEUsesOnlyScoredRows(t *testing.T) {
	trial := testHistoryTrial(4)
	trial.Steps[0].Scored = false
	trial.Steps[1].Scored = true
	trial.Steps[2].Scored = false
	trial.Steps[3].Scored = true
	trial.Steps[1].Target = [2]float64{}
	trial.Steps[3].Target = [2]float64{}
	predicted := [][]float64{{100, 100}, {3, 1}, {-100, -100}, {5, 9}}
	loss, upstream, samples, err := maskedMSE(predicted, trial)
	if err != nil {
		t.Fatal(err)
	}
	if samples != 2 {
		t.Fatalf("scored samples = %d, want 2", samples)
	}
	wantLoss := (3*3 + 1*1 + 5*5 + 9*9) / (2 * 2.0)
	if loss != wantLoss {
		t.Fatalf("loss = %.17g, want %.17g", loss, wantLoss)
	}
	want := [][]float64{{0, 0}, {1.5, .5}, {0, 0}, {2.5, 4.5}}
	if !reflect.DeepEqual(upstream, want) {
		t.Fatalf("upstream = %#v, want %#v", upstream, want)
	}
	trial.Steps[0].Target = [2]float64{math.NaN(), math.Inf(1)}
	changedLoss, changedUpstream, changedSamples, err := maskedMSE(predicted, trial)
	if err != nil {
		t.Fatal(err)
	}
	if changedLoss != loss || changedSamples != samples || !reflect.DeepEqual(changedUpstream, upstream) {
		t.Fatalf("unscored target changed mask result: loss %.17g/%g samples %d/%d upstream %#v/%#v", changedLoss, loss, changedSamples, samples, changedUpstream, upstream)
	}
	trial.Steps[1].Scored = false
	trial.Steps[3].Scored = false
	if _, _, _, err := maskedMSE(predicted, trial); err == nil {
		t.Fatal("accepted a trial with no scored rows")
	}
	bad := append([][]float64(nil), predicted...)
	bad[1] = []float64{math.NaN(), 0}
	if _, _, _, err := maskedMSE(bad, testHistoryTrial(4)); err == nil {
		t.Fatal("accepted a non-finite prediction")
	}
}

func TestTrialInputsControlsAreCausalAndDoNotAlias(t *testing.T) {
	trial := testHistoryTrial(40)
	for i := range trial.Steps {
		trial.Steps[i].Input[5] = float64((i / 3) % 2)
	}
	delivered, err := trialInputs(trial, "delivered", 77)
	if err != nil {
		t.Fatal(err)
	}
	if len(delivered) != len(trial.Steps) || delivered[0][5] != trial.Steps[0].Input[5] {
		t.Fatalf("delivered inputs do not preserve rows")
	}
	delivered[0][0] = 999
	if trial.Steps[0].Input[0] == 999 {
		t.Fatal("delivered input aliases source row")
	}
	noStimulus, err := trialInputs(trial, "no_stimulus", 77)
	if err != nil {
		t.Fatal(err)
	}
	for i := range noStimulus {
		for j := 0; j < 5; j++ {
			if noStimulus[i][j] != trial.Steps[i].Input[j] {
				t.Fatalf("no-stimulus changed feature row=%d col=%d", i, j)
			}
		}
		if noStimulus[i][5] != 0 {
			t.Fatalf("no-stimulus[%d][5] = %g", i, noStimulus[i][5])
		}
	}
	shuffled, err := trialInputs(trial, "shuffled_stimulus", 77)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		if shuffled[i][5] != 0 {
			t.Fatalf("first shuffled block[%d][5] = %g", i, shuffled[i][5])
		}
	}
	wantBlock := append([]float64(nil), trialStimulusBlock(trial, 0)...)
	shuffleWithSeed(wantBlock, 78)
	for i, want := range wantBlock {
		if shuffled[16+i][5] != want {
			t.Fatalf("shuffled block[%d][5] = %g, want %g", i, shuffled[16+i][5], want)
		}
	}
	mutated := trial
	mutated.Steps = append([]historyStep(nil), trial.Steps...)
	mutated.Steps[32].Input[0] += 1000
	mutated.Steps[32].Input[5] = 1 - mutated.Steps[32].Input[5]
	mutatedInputs, err := trialInputs(mutated, "shuffled_stimulus", 77)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mutatedInputs[:32], shuffled[:32]) {
		t.Fatal("future source rows changed a shuffled prefix")
	}
	config, parameters, err := newModel(77)
	if err != nil {
		t.Fatal(err)
	}
	for i := range parameters.Readout {
		parameters.Readout[i] = .3
	}
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix, err := network.PredictAll(context.Background(), parameters, shuffled)
	if err != nil {
		t.Fatal(err)
	}
	gotPrefix, err := network.PredictAll(context.Background(), parameters, mutatedInputs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wantPrefix[:32], gotPrefix[:32]) {
		t.Fatal("future source rows changed a prediction prefix")
	}
}

func TestEarlierStimulusChangesLaterOutput(t *testing.T) {
	config, parameters, err := newModel(20261003)
	if err != nil {
		t.Fatal(err)
	}
	for i := range parameters.Core.Weights {
		parameters.Core.Weights[i] = .05
	}
	for i := range parameters.Encoder {
		parameters.Encoder[i] = .2
	}
	for i := range parameters.Readout {
		parameters.Readout[i] = .3
	}
	trial := testHistoryTrial(8)
	trial.Steps[0].Input[5] = 0
	changed := trial
	changed.Steps = append([]historyStep(nil), trial.Steps...)
	changed.Steps[0].Input[5] = 1
	wantInput, err := trialInputs(trial, "delivered", 0)
	if err != nil {
		t.Fatal(err)
	}
	gotInput, err := trialInputs(changed, "delivered", 0)
	if err != nil {
		t.Fatal(err)
	}
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	want, err := network.PredictAll(context.Background(), parameters, wantInput)
	if err != nil {
		t.Fatal(err)
	}
	got, err := network.PredictAll(context.Background(), parameters, gotInput)
	if err != nil {
		t.Fatal(err)
	}
	changedLater := false
	for i := 1; i < len(want); i++ {
		if !reflect.DeepEqual(want[i], got[i]) {
			changedLater = true
			break
		}
	}
	if !changedLater {
		t.Fatal("earlier stimulus did not change a later output")
	}
}

func TestFullHistoryStepFromReachesEarlyStimulus(t *testing.T) {
	config, parameters, err := newModel(20261003)
	if err != nil {
		t.Fatal(err)
	}
	for i := range parameters.Core.Weights {
		parameters.Core.Weights[i] = .05
	}
	for i := range parameters.Encoder {
		parameters.Encoder[i] = .2
	}
	for i := range parameters.Readout {
		parameters.Readout[i] = .3
	}
	trial := testHistoryTrial(300)
	for i := range trial.Steps {
		trial.Steps[i].Scored = i >= 288
	}
	input, err := trialInputs(trial, "delivered", 0)
	if err != nil {
		t.Fatal(err)
	}
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	predicted, err := network.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	_, upstream, _, err := maskedMSE(predicted, trial)
	if err != nil {
		t.Fatal(err)
	}
	gradient, err := network.LossGradientFrom(context.Background(), parameters, input, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(gradient.Inputs) != len(input) {
		t.Fatalf("input gradient rows = %d, want %d", len(gradient.Inputs), len(input))
	}
	if math.Abs(gradient.Inputs[0][5]) < 1e-12 {
		t.Fatalf("early stimulus input gradient = %.17g, want nonzero", gradient.Inputs[0][5])
	}
	trainer, err := learning.NewTrainer(config, parameters, trainingOptions())
	if err != nil {
		t.Fatal(err)
	}
	result, err := trainer.StepFrom(context.Background(), input, upstream)
	if err != nil {
		t.Fatal(err)
	}
	if result.GradientHorizonSteps != len(input) {
		t.Fatalf("gradient horizon = %d, want %d", result.GradientHorizonSteps, len(input))
	}
	if !result.Applied || result.Updates != 1 {
		t.Fatalf("StepFrom result = %+v", result)
	}
}

func TestEarlyStimulusGradientMatchesFiniteDifference(t *testing.T) {
	config, parameters, err := newModel(20261003)
	if err != nil {
		t.Fatal(err)
	}
	for i := range parameters.Core.Weights {
		parameters.Core.Weights[i] = .05
	}
	for i := range parameters.Encoder {
		parameters.Encoder[i] = .2
	}
	for i := range parameters.Readout {
		parameters.Readout[i] = .3
	}
	trial := testHistoryTrial(8)
	for i := range trial.Steps {
		trial.Steps[i].Scored = i == len(trial.Steps)-1
	}
	input, err := trialInputs(trial, "delivered", 0)
	if err != nil {
		t.Fatal(err)
	}
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	predicted, err := network.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	_, upstream, _, err := maskedMSE(predicted, trial)
	if err != nil {
		t.Fatal(err)
	}
	gradient, err := network.LossGradientFrom(context.Background(), parameters, input, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	const epsilon = 2e-3
	plus := cloneInputRows(input)
	minus := cloneInputRows(input)
	plus[0][5] += epsilon
	minus[0][5] -= epsilon
	plusLoss := maskedObjective(t, network, parameters, plus, trial)
	minusLoss := maskedObjective(t, network, parameters, minus, trial)
	finiteDifference := (plusLoss - minusLoss) / (2 * epsilon)
	got := gradient.Inputs[0][5]
	if math.Abs(got) < 1e-12 {
		t.Fatalf("early stimulus gradient = %.17g, want nonzero", got)
	}
	if delta := math.Abs(got - finiteDifference); delta > 1e-5+1e-4*math.Abs(finiteDifference) {
		t.Fatalf("early stimulus gradient = %.17g, finite difference %.17g, delta %.17g", got, finiteDifference, delta)
	}
}

func TestFitModelAndEvaluateAllControlsUseMatchedUpdates(t *testing.T) {
	controls := []string{"delivered", "no_stimulus", "shuffled_stimulus"}
	trial := testHistoryTrial(32)
	for i := range trial.Steps {
		trial.Steps[i].Scored = i >= 16
	}
	var referenceConfig learning.Config
	var referenceOptions learning.Options
	var referenceParameters learning.Parameters
	for controlIndex, control := range controls {
		t.Run(control, func(t *testing.T) {
			before, after, curve, err := fitModel(context.Background(), []historyTrial{trial}, control, 20261003, 2)
			if err != nil {
				t.Fatal(err)
			}
			if before.Updates != 0 || after.Updates != 2 || len(curve) != 2 {
				t.Fatalf("snapshots/curve = before %d after %d curve %v", before.Updates, after.Updates, curve)
			}
			for i, value := range curve {
				if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
					t.Fatalf("curve[%d] = %g", i, value)
				}
			}
			got, err := evaluateModel(context.Background(), after, []historyTrial{trial}, control, 20261003)
			if err != nil {
				t.Fatal(err)
			}
			if got.Samples != 16 || math.IsNaN(got.MSE) || math.IsInf(got.MSE, 0) {
				t.Fatalf("evaluation metric = %+v", got)
			}
			if controlIndex == 0 {
				referenceConfig, referenceOptions, referenceParameters = before.Config, before.Options, before.Parameters
			} else {
				if !reflect.DeepEqual(before.Config, referenceConfig) || !reflect.DeepEqual(before.Options, referenceOptions) || !reflect.DeepEqual(before.Parameters, referenceParameters) {
					t.Fatal("control initialization or capacity differs")
				}
			}
		})
	}
}

func TestFitModelUsesLexicalTrialOrder(t *testing.T) {
	first := testHistoryTrial(32)
	first.ID = "a-trial"
	for i := range first.Steps {
		first.Steps[i].Scored = i >= 16
	}
	second := testHistoryTrial(32)
	second.ID = "z-trial"
	for i := range second.Steps {
		second.Steps[i].Input[0] += .3
		second.Steps[i].Scored = i >= 16
	}
	_, orderedAfter, orderedCurve, err := fitModel(context.Background(), []historyTrial{first, second}, "delivered", 20261003, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, reversedAfter, reversedCurve, err := fitModel(context.Background(), []historyTrial{second, first}, "delivered", 20261003, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(orderedAfter, reversedAfter) || !reflect.DeepEqual(orderedCurve, reversedCurve) {
		t.Fatal("trial input order changed lexical training result")
	}
}

func TestTrainingAndEvaluationRespectCancellation(t *testing.T) {
	trial := testHistoryTrial(32)
	for i := range trial.Steps {
		trial.Steps[i].Scored = i >= 16
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := fitModel(ctx, []historyTrial{trial}, "delivered", 1, 1); err == nil {
		t.Fatal("fitModel accepted canceled context")
	}
	config, parameters, err := newModel(1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := learning.TrainingSnapshot{SchemaVersion: "coimnet-episode-training/v1", Config: config, Parameters: parameters, Options: trainingOptions()}
	if _, err := evaluateModel(ctx, snapshot, []historyTrial{trial}, "delivered", 1); err == nil {
		t.Fatal("evaluateModel accepted canceled context")
	}
}

func testHistoryTrial(rows int) historyTrial {
	steps := make([]historyStep, rows)
	for i := range steps {
		steps[i].Input = [6]float64{float64(i) / 30, math.Sin(float64(i)*.07) / 30, .1, .02, .1, float64((i / 3) % 2)}
		steps[i].Target = [2]float64{.1, .02}
		steps[i].Scored = true
		steps[i].T = float64(i) * .1
	}
	return historyTrial{ID: "trial-a", Condition: "rewarded", Steps: steps, RolloutIndex: 16}
}

func trialStimulusBlock(trial historyTrial, block int) []float64 {
	start := block * 16
	values := make([]float64, 16)
	for i := range values {
		values[i] = trial.Steps[start+i].Input[5]
	}
	return values
}

func shuffleWithSeed(values []float64, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for i := len(values) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		values[i], values[j] = values[j], values[i]
	}
}

func cloneInputRows(input [][]float64) [][]float64 {
	clone := make([][]float64, len(input))
	for i := range input {
		clone[i] = append([]float64(nil), input[i]...)
	}
	return clone
}

func maskedObjective(t *testing.T, network *learning.Network, parameters learning.Parameters, input [][]float64, trial historyTrial) float64 {
	t.Helper()
	predicted, err := network.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	loss, _, _, err := maskedMSE(predicted, trial)
	if err != nil {
		t.Fatal(err)
	}
	return loss
}
