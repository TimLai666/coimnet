package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

func TestRolloutLineIntersectionHandCalculated(t *testing.T) {
	tests := []struct {
		name        string
		from, to    [2]float64
		wantHit     bool
		wantNearest float64
	}{
		{name: "crosses", from: [2]float64{-4, 0}, to: [2]float64{4, 0}, wantHit: true, wantNearest: 0},
		{name: "tangent", from: [2]float64{-4, 2}, to: [2]float64{4, 2}, wantHit: true, wantNearest: 2},
		{name: "misses", from: [2]float64{-4, 2.0001}, to: [2]float64{4, 2.0001}, wantHit: false, wantNearest: 2.0001},
		{name: "zero", from: [2]float64{3, 4}, to: [2]float64{3, 4}, wantHit: false, wantNearest: 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hit, nearest := rolloutSegmentHitsGoal(test.from, test.to)
			if hit != test.wantHit || math.Abs(nearest-test.wantNearest) > 1e-12 {
				t.Fatalf("hit=%v nearest=%g, want hit=%v nearest=%g", hit, nearest, test.wantHit, test.wantNearest)
			}
		})
	}
}

func TestRolloutInitialSuccessIsSeparateFromLearnedSteps(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a"})
	start := rolloutStart{TrialID: "trial-a", Condition: "rewarded", XCM: 1, YCM: 1, PreviousDXCM: 1, PreviousDYCM: 0, PreviousDT: .2, Target: rolloutTarget{TrialID: "trial-a", XCM: 1, YCM: 1}}
	got, err := simulateRolloutTrial(context.Background(), model, start, 20, rolloutStrategyPersistent)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Success || !got.StartSuccess || got.HitStep != 0 || got.Steps != 0 {
		t.Fatalf("initial success = %+v", got)
	}
}

func TestRolloutScoresThePerTrialTargetCenterAndKeepsItOutOfPolicyInput(t *testing.T) {
	start := rolloutStart{TrialID: "trial-a", Condition: "rewarded", XCM: 5, YCM: 0, PreviousDXCM: 0, PreviousDYCM: 0, PreviousDT: .2, Target: rolloutTarget{TrialID: "trial-a", XCM: 5, YCM: 0}}
	model := rolloutTestModel(t, []string{"trial-a"})
	inside, err := simulateRolloutTrial(context.Background(), model, start, 3, rolloutStrategyZero)
	if err != nil {
		t.Fatal(err)
	}
	start.Target = rolloutTarget{TrialID: "trial-a", XCM: 0, YCM: 0}
	outside, err := simulateRolloutTrial(context.Background(), model, start, 3, rolloutStrategyZero)
	if err != nil {
		t.Fatal(err)
	}
	if !inside.Success || !inside.StartSuccess || inside.HitStep != 0 {
		t.Fatalf("target-centered initial success = %+v", inside)
	}
	if outside.Success || outside.StartSuccess || outside.Steps != 3 {
		t.Fatalf("origin target incorrectly affected zero policy = %+v", outside)
	}
	hit, nearest := rolloutSegmentHitsTarget(rolloutTarget{XCM: 10, YCM: 0}, [2]float64{6, 2}, [2]float64{14, 2})
	if !hit || math.Abs(nearest-2) > 1e-12 {
		t.Fatalf("off-origin tangent = hit %v nearest %g", hit, nearest)
	}
}

func TestRolloutCollisionUsesRealizedZeroForNextObservation(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a"})
	start := rolloutStart{TrialID: "trial-a", Condition: "rewarded", XCM: 29, YCM: 0, PreviousDXCM: 1, PreviousDYCM: 0, PreviousDT: .2, Target: rolloutTarget{TrialID: "trial-a", XCM: 0, YCM: 0}}
	trace, err := simulateRolloutTrace(context.Background(), model, start, 2, rolloutStrategyPersistent)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace) != 2 || trace[0].Collision != 1 || trace[0].RealizedDXCM != 0 || trace[0].RealizedDYCM != 0 {
		t.Fatalf("collision trace = %+v", trace)
	}
	if trace[1].Input.PreviousDXCM != 0 || trace[1].Input.PreviousDYCM != 0 {
		t.Fatalf("next input reused rejected action: %+v", trace[1].Input)
	}
}

func TestRolloutClipsLongActionAndCountsZeroActionSeparately(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a"})
	start := rolloutStart{TrialID: "trial-a", Condition: "non-rewarded", XCM: 0, YCM: 0, PreviousDXCM: 0, PreviousDYCM: 0, PreviousDT: .2, Target: rolloutTarget{TrialID: "trial-a", XCM: 10, YCM: 10}}
	trace, err := simulateRolloutTrace(context.Background(), model, start, 1, rolloutStrategyFixedAction, [2]float64{6, 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(trace) != 1 || trace[0].Clipped != 1 || math.Abs(trace[0].RealizedDXCM-3) > 1e-12 || math.Abs(trace[0].RealizedDYCM-4) > 1e-12 {
		t.Fatalf("clipped action trace = %+v", trace)
	}
	trace, err = simulateRolloutTrace(context.Background(), model, start, 1, rolloutStrategyFixedAction, [2]float64{})
	if err != nil {
		t.Fatal(err)
	}
	if trace[0].Clipped != 0 || trace[0].RealizedDXCM != 0 || trace[0].RealizedDYCM != 0 {
		t.Fatalf("zero action trace = %+v", trace)
	}
}

func TestRolloutModelAdvanceMatchesExistingPredictAllResidual(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a"})
	inputs := [][]float64{
		{0, 0, .5, 0, .2},
		{.1, 0, .5, .1, .1},
		{.2, .1, 0, 0, .1},
	}
	individual, err := learning.NewIndividual(model.Snapshot.Config, model.Snapshot.Parameters, model.Snapshot.Options, rolloutZeroNeuralState(model.Snapshot.Config))
	if err != nil {
		t.Fatal(err)
	}
	got := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		rows, err := individual.Advance(context.Background(), [][]float64{input})
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, append([]float64(nil), rows[0]...))
	}
	trainer, err := restoreTrainer(model.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	want, err := trainer.PredictAll(context.Background(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("one-row individual residuals differ from PredictAll: got=%v want=%v", got, want)
	}
}

func TestRolloutResetsNeuralEveryTrainingChunkWithoutOptimizerUpdate(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a"})
	start := rolloutStart{TrialID: "trial-a", Condition: "rewarded", XCM: 0, YCM: 0, PreviousDXCM: 1, PreviousDYCM: 0, PreviousDT: .2, Target: rolloutTarget{TrialID: "trial-a", XCM: 10, YCM: 10}}
	before := model.Snapshot
	got, err := simulateRolloutTrial(context.Background(), model, start, trainingChunk+1, rolloutStrategyModel)
	if err != nil {
		t.Fatal(err)
	}
	if got.Steps == 0 {
		t.Fatal("model rollout unexpectedly stopped before reset boundary")
	}
	if !reflect.DeepEqual(model.Snapshot.Parameters, before.Parameters) || !reflect.DeepEqual(model.Snapshot.Options, before.Options) || !reflect.DeepEqual(model.Snapshot.Optimizer, before.Optimizer) {
		t.Fatal("rollout mutated saved model parameters, options, or optimizer")
	}
}

func TestRolloutTargetAndFutureTrajectoryCannotAffectModelAction(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a"})
	base := rolloutStart{TrialID: "trial-a", Condition: "rewarded", XCM: 0, YCM: 0, PreviousDXCM: 1, PreviousDYCM: 0, PreviousDT: .2, Target: rolloutTarget{TrialID: "trial-a", XCM: 1, YCM: 1}}
	mutated := base
	mutated.Target = rolloutTarget{TrialID: "trial-a", XCM: -29, YCM: 0}
	a, err := modelFirstAction(context.Background(), model, base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := modelFirstAction(context.Background(), model, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("evaluation-only target changed action: a=%v b=%v", a, b)
	}

	dataset := rolloutDataset([]string{"trial-a"}, false)
	startsA, _, err := buildRolloutStarts(dataset, []string{"trial-a"}, []rolloutTarget{base.Target})
	if err != nil {
		t.Fatal(err)
	}
	mutatedDataset := dataset
	mutatedDataset.Rows = append([]trajectory.Point(nil), dataset.Rows...)
	mutatedDataset.Rows[len(mutatedDataset.Rows)-1].XCM = 28
	mutatedDataset.Rows[len(mutatedDataset.Rows)-1].YCM = -28
	startsB, _, err := buildRolloutStarts(mutatedDataset, []string{"trial-a"}, []rolloutTarget{base.Target})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(startsA, startsB) {
		t.Fatalf("future source row changed initial observed pair: A=%+v B=%+v", startsA, startsB)
	}
}

func TestRolloutTrialOrderAndStrategyIsolationAreDeterministic(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a", "trial-b"})
	dataset := rolloutDataset([]string{"trial-a", "trial-b"}, false)
	targets := []rolloutTarget{{TrialID: "trial-a", XCM: 10, YCM: 0}, {TrialID: "trial-b", XCM: 0, YCM: 10}}
	one, err := runRolloutEngine(context.Background(), model, dataset, targets, 5)
	if err != nil {
		t.Fatal(err)
	}
	shuffled := rolloutDataset([]string{"trial-b", "trial-a"}, false)
	two, err := runRolloutEngine(context.Background(), model, shuffled, targets, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one.Trials, two.Trials) || !reflect.DeepEqual(one.Aggregates, two.Aggregates) {
		t.Fatalf("trial order changed results:\none=%+v\ntwo=%+v", one.Trials, two.Trials)
	}
	if len(one.Trials) != 2 || len(one.Trials[0].Strategies) != len(rolloutStrategyNames) {
		t.Fatalf("strategy isolation report = %+v", one.Trials)
	}
	starts, _, err := buildRolloutStarts(dataset, model.Split.Test, targets)
	if err != nil {
		t.Fatal(err)
	}
	random, err := simulateRolloutTrial(context.Background(), model, starts[0], 5, rolloutStrategyRandom)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one.Trials[0].Strategies[rolloutStrategyRandom], random) {
		t.Fatal("other strategies changed the independently simulated random result")
	}
}

func TestRolloutRejectsInvalidLimitsContextAndTargetMetadata(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a"})
	dataset := rolloutDataset([]string{"trial-a"}, false)
	validTarget := []rolloutTarget{{TrialID: "trial-a", XCM: 0, YCM: 0}}
	for _, steps := range []int{0, maxRolloutSteps + 1} {
		if _, err := runRolloutEngine(context.Background(), model, dataset, validTarget, steps); err == nil {
			t.Fatalf("steps=%d accepted", steps)
		}
	}
	if _, err := runRolloutEngine(nil, model, dataset, validTarget, 1); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := runRolloutEngine(context.Background(), model, dataset, nil, 1); err == nil {
		t.Fatal("missing target metadata accepted")
	}
	if _, err := runRolloutEngine(context.Background(), model, dataset, []rolloutTarget{{TrialID: "trial-a", XCM: 0, YCM: 0}, {TrialID: "trial-a", XCM: 1, YCM: 1}}, 1); err == nil {
		t.Fatal("duplicate target IDs accepted")
	}
	for _, target := range []rolloutTarget{{TrialID: "trial-a", XCM: math.NaN()}, {TrialID: "trial-a", XCM: 31, YCM: 0}} {
		if _, err := runRolloutEngine(context.Background(), model, dataset, []rolloutTarget{target}, 1); err == nil {
			t.Fatalf("invalid target %+v accepted", target)
		}
	}
	if _, err := runRolloutEngine(context.Background(), model, dataset, []rolloutTarget{{TrialID: "missing", XCM: 0, YCM: 0}}, 1); err == nil {
		t.Fatal("target without selected test trial accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runRolloutEngine(canceled, model, dataset, validTarget, 1); err == nil {
		t.Fatal("canceled rollout accepted")
	}
	if _, err := runRolloutEngine(context.Background(), model, trajectory.Dataset{Source: modelSource()}, validTarget, 1); err == nil {
		t.Fatal("empty dataset accepted")
	}
	mismatch := model
	mismatch.SourceSHA256 = "different"
	if _, err := runRolloutEngine(context.Background(), mismatch, dataset, validTarget, 1); err == nil {
		t.Fatal("source mismatch accepted")
	}
	overlap := model
	overlap.Split.Train = []string{"trial-a"}
	if _, err := runRolloutEngine(context.Background(), overlap, dataset, validTarget, 1); err == nil {
		t.Fatal("overlapping train/test split accepted")
	}
}

func TestRolloutCLIHelpAndArgumentErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"rollout", "--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Usage:", "--data", "--snapshot", "--out", "--steps", "200", "4096", "evaluation-only"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("rollout help missing %q: %s", want, stderr.String())
		}
	}
	if stdout.Len() != 0 {
		t.Fatalf("rollout help wrote stdout: %s", stdout.String())
	}
	for _, args := range [][]string{{"rollout", "--unknown"}, {"rollout", "positional", "--data", "d", "--snapshot", "s", "--out", "o"}} {
		stdout.Reset()
		stderr.Reset()
		if err := run(context.Background(), args, &stdout, &stderr); err == nil {
			t.Fatalf("run(%v) succeeded", args)
		}
		if stdout.Len() != 0 {
			t.Fatalf("run(%v) wrote stdout: %s", args, stdout.String())
		}
	}
}

func TestRolloutExcludesTrialWithoutLegalPair(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a"})
	dataset := trajectory.Dataset{Schema: trajectory.SchemaVersion, Source: modelSource(), Rows: []trajectory.Point{
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: 0, XCM: -1, YCM: 0},
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: .1, XCM: 0, YCM: 0},
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: .2, XCM: 0, YCM: 1},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "before_relocation", T: 0, XCM: 0, YCM: 0},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 1, XCM: 40, YCM: 0},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 1.1, XCM: 41, YCM: 0},
	}}
	if _, err := runRolloutEngine(context.Background(), model, dataset, []rolloutTarget{{TrialID: "trial-a", XCM: 0, YCM: 0}}, 1); err == nil {
		t.Fatal("all-excluded rollout produced a success result")
	}
}

func TestRolloutReportsValidPairWhenHeldOutSampleHasNoThirdRow(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a", "trial-b"})
	dataset := trajectory.Dataset{Schema: trajectory.SchemaVersion, Source: modelSource(), Rows: []trajectory.Point{
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: 0, XCM: -1, YCM: 0},
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: .1, XCM: 0, YCM: 0},
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: .2, XCM: 0, YCM: 1},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 0, XCM: 0, YCM: 0},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: .2, XCM: 1, YCM: 0},
		{TrialID: "trial-b", Condition: "non-rewarded", Segment: "after_relocation", T: 0, XCM: 4, YCM: 0},
		{TrialID: "trial-b", Condition: "non-rewarded", Segment: "after_relocation", T: 1, XCM: 5, YCM: 0},
	}}
	report, err := runRolloutEngine(context.Background(), model, dataset, []rolloutTarget{{TrialID: "trial-a", XCM: 10, YCM: 0}, {TrialID: "trial-b", XCM: 10, YCM: 0}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Trials) != 1 || report.Trials[0].TrialID != "trial-a" {
		t.Fatalf("trials = %+v", report.Trials)
	}
	if len(report.Excluded) != 1 || report.Excluded[0].TrialID != "trial-b" {
		t.Fatalf("excluded = %+v", report.Excluded)
	}
	if got := report.Split.TestSampleCounts["rewarded"]; got != 0 {
		t.Fatalf("test sample count = %d, want 0 for two-row held-out trial", got)
	}
}

func rolloutTestModel(t *testing.T, testIDs []string) savedModel {
	t.Helper()
	config, parameters, err := newModel()
	if err != nil {
		t.Fatal(err)
	}
	trainer, err := learning.NewTrainer(config, parameters, defaultTrainingOptions())
	if err != nil {
		t.Fatal(err)
	}
	return savedModel{
		SchemaVersion:         realnavSchemaVersion,
		SourceSHA256:          modelSource().SHA256,
		FeatureRule:           featureRuleVersion,
		Preprocessing:         currentPreprocessingContract(),
		TrainMeanDisplacement: 2,
		Split:                 splitTrials{Train: []string{"train"}, Test: append([]string(nil), testIDs...)},
		Snapshot:              trainer.Snapshot(),
	}
}

func modelSource() trajectory.Source {
	return trajectory.Source{ID: "fixture", URL: "https://example.invalid/fixture", SHA256: "fixture-sha", License: trajectory.License{Holder: "fixture", Terms: "test", Source: "https://example.invalid/license"}}
}

func rolloutDataset(trials []string, shuffleRows bool) trajectory.Dataset {
	rows := make([]trajectory.Point, 0, len(trials)*3)
	for _, trial := range trials {
		condition, base := "rewarded", 0.0
		if trial == "trial-b" {
			condition, base = "non-rewarded", 4
		}
		rows = append(rows,
			trajectory.Point{TrialID: trial, Condition: condition, Segment: "after_relocation", T: 0, XCM: base, YCM: 0},
			trajectory.Point{TrialID: trial, Condition: condition, Segment: "after_relocation", T: .2, XCM: base + 1, YCM: 0},
			trajectory.Point{TrialID: trial, Condition: condition, Segment: "after_relocation", T: .3, XCM: base + 1, YCM: 1},
		)
	}
	rows = append([]trajectory.Point{
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: 0, XCM: -1, YCM: 0},
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: .1, XCM: 0, YCM: 0},
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: .2, XCM: 0, YCM: 1},
	}, rows...)
	if shuffleRows {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].TrialID < rows[j].TrialID })
	}
	return trajectory.Dataset{Schema: trajectory.SchemaVersion, Source: modelSource(), Rows: rows}
}

func TestRolloutProtocolHashRoundTrips(t *testing.T) {
	protocol := currentRolloutProtocol(200)
	hash, err := rolloutProtocolHash(protocol)
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" || hash != rolloutProtocolHashMust(protocol) {
		t.Fatalf("protocol hash = %q", hash)
	}
	encoded, err := json.Marshal(protocol)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("protocol JSON = %s err=%v", encoded, err)
	}
}
