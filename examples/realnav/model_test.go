package main

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestFitSamplesPerformsTrueGradientUpdates(t *testing.T) {
	samples := syntheticSamples("train", 96)
	result, err := fitSamples(context.Background(), samples, nil, 8)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updates == 0 {
		t.Fatal("fit performed no optimizer updates")
	}
	if reflect.DeepEqual(result.Before.Parameters.Core.Weights, result.After.Parameters.Core.Weights) {
		t.Fatal("core weights did not change")
	}
	if reflect.DeepEqual(result.Before.Parameters.Encoder, result.After.Parameters.Encoder) {
		t.Fatal("encoder did not change")
	}
	if result.AfterMSE.Train.Samples != len(samples) {
		t.Fatalf("train metric samples = %d, want %d", result.AfterMSE.Train.Samples, len(samples))
	}
	if !(result.AfterMSE.Train.MSE < result.BeforeMSE.Train.MSE) {
		t.Fatalf("training MSE did not improve: before=%g after=%g", result.BeforeMSE.Train.MSE, result.AfterMSE.Train.MSE)
	}
}

func TestDirectionBaselineUsesOnlyPreviousDisplacement(t *testing.T) {
	sample := causalSample{Input: []float64{100, 100, 1.5, 2, .1}, Target: [2]float64{9, 12}}
	baseline := directionBaseline(sample, 2)
	if baseline != ([2]float64{1.2, 1.6}) {
		t.Fatalf("direction baseline = %v", baseline)
	}
	if got := zeroBaseline(sample); got != ([2]float64{}) {
		t.Fatalf("zero baseline = %v", got)
	}
	if got := lastDisplacementBaseline(sample); got != ([2]float64{3, 4}) {
		t.Fatalf("last displacement baseline = %v", got)
	}
}

func TestSnapshotFingerprintAndFreshTrainerPredictionsMatch(t *testing.T) {
	samples := syntheticSamples("train", 16)
	result, err := fitSamples(context.Background(), samples, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotFingerprint(result.After) == "" || snapshotFingerprint(result.Before) == "" {
		t.Fatal("snapshot fingerprint is empty")
	}
	restored, err := restoreTrainer(result.After)
	if err != nil {
		t.Fatal(err)
	}
	input, _ := sampleRows(samples)
	want, err := restored.PredictAll(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	network, err := learning.NewNetwork(result.After.Config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := network.PredictAll(context.Background(), result.After.Parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("fresh network predictions differ from restored trainer")
	}
}

func TestPredictionsAddCausalDirectionBeforeLearnedResidual(t *testing.T) {
	sample := causalSample{Input: []float64{0, 0, .2, -.4, .1}}
	got, err := addModelBaseline([][]float64{{.5, -.25}}, []causalSample{sample}, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]float64{{0.9472135954999579, -1.1444271909999158}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("residual prediction = %v, want %v", got, want)
	}
}

func TestRestoredSnapshotInferenceMatchesFreshProcess(t *testing.T) {
	samples := syntheticSamples("heldout", 8)
	result, err := fitSamples(context.Background(), syntheticSamples("train", 24), samples, 2)
	if err != nil {
		t.Fatal(err)
	}
	input, _ := sampleRows(samples)
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "snapshot.json")
	inputPath := filepath.Join(dir, "input.json")
	outputPath := filepath.Join(dir, "output.json")
	writeTestJSON(t, snapshotPath, result.After)
	writeTestJSON(t, inputPath, input)
	cmd := exec.Command(os.Args[0], "-test.run=TestRealnavInferenceHelper", "-test.v=false")
	cmd.Env = append(os.Environ(),
		"REALNAV_INFERENCE_HELPER=1",
		"REALNAV_SNAPSHOT_PATH="+snapshotPath,
		"REALNAV_INPUT_PATH="+inputPath,
		"REALNAV_OUTPUT_PATH="+outputPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fresh inference process: %v\n%s", err, output)
	}
	var want, got [][]float64
	wantTrainer, err := restoreTrainer(result.After)
	if err != nil {
		t.Fatal(err)
	}
	want, err = wantTrainer.PredictAll(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	readTestJSON(t, outputPath, &got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fresh-process predictions differ: got %v want %v", got, want)
	}
}

func TestInferenceEvaluationDoesNotAdvanceTrainer(t *testing.T) {
	result, err := fitSamples(context.Background(), syntheticSamples("train", 24), nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	trainer, err := restoreTrainer(result.After)
	if err != nil {
		t.Fatal(err)
	}
	before := trainer.Snapshot()
	heldout := syntheticSamples("heldout", 8)
	if _, err := evaluateTrainer(context.Background(), trainer, heldout, meanTargetDisplacement(heldout)); err != nil {
		t.Fatal(err)
	}
	after := trainer.Snapshot()
	if after.Updates != before.Updates || snapshotFingerprint(after) != snapshotFingerprint(before) {
		t.Fatalf("inference changed trainer state: before updates=%d after updates=%d", before.Updates, after.Updates)
	}
}

func TestRealnavInferenceHelper(t *testing.T) {
	if os.Getenv("REALNAV_INFERENCE_HELPER") != "1" {
		return
	}
	var snapshot learning.TrainingSnapshot
	readTestJSON(t, os.Getenv("REALNAV_SNAPSHOT_PATH"), &snapshot)
	var input [][]float64
	readTestJSON(t, os.Getenv("REALNAV_INPUT_PATH"), &input)
	trainer, err := restoreTrainer(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	predicted, err := trainer.PredictAll(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, os.Getenv("REALNAV_OUTPUT_PATH"), predicted)
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(strings.NewReader(string(data))).Decode(value); err != nil {
		t.Fatal(err)
	}
}

func syntheticSamples(trial string, count int) []causalSample {
	samples := make([]causalSample, count)
	for i := range samples {
		x := float64(i) * 0.1
		y := math.Sin(float64(i) * 0.07)
		previousDX := 0.1
		previousDY := math.Cos(float64(i)*0.07) * 0.07
		samples[i] = causalSample{
			TrialID: trial, Condition: "rewarded", Timestamp: float64(i) * .1, DeltaSeconds: .1,
			Input:  []float64{x, y, previousDX, previousDY, .1},
			Target: [2]float64{0.1, previousDY},
		}
	}
	return samples
}
