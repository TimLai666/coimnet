package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// onlineChemicalWeightClamp is the public score contract written out in the
// test so the expected target never comes from OnlineLearner's private helper.
func onlineChemicalWeightClamp(score float64) float64 {
	return math.Max(0, math.Min(1, score))
}

// onlineChemicalWeightPrediction deliberately uses an independent public
// prediction. The chemical branch goes through the existing test helper that
// constructs an explicit ChemicalEpisode from the persistent boundary; it does
// not call Individual's private episodePredict path.
func onlineChemicalWeightPrediction(t *testing.T, before learning.IndividualSnapshot, input [][]float64) []float64 {
	t.Helper()
	if before.Chemical == nil {
		network, err := learning.NewNetwork(before.Config)
		if err != nil {
			t.Fatalf("NewNetwork() ordinary control error = %v", err)
		}
		prediction, err := network.Predict(context.Background(), before.Parameters, input)
		if err != nil {
			t.Fatalf("ordinary independent Predict() error = %v", err)
		}
		return prediction
	}
	reference := individualChemicalTrainingReference(t, before)
	defer func() { _ = reference.Close() }()
	prediction, err := reference.Predict(context.Background(), input)
	if err != nil {
		t.Fatalf("explicit ChemicalEpisode Predict() error = %v", err)
	}
	return prediction
}

func onlineChemicalWeightPlainPrediction(t *testing.T, before learning.IndividualSnapshot, input [][]float64) []float64 {
	t.Helper()
	// Config.Chemistry is nil on a persistent Individual. Keeping the same
	// parameters and using this config is the no-chemistry control for the
	// explicit ChemicalEpisode prediction above.
	network, err := learning.NewNetwork(before.Config)
	if err != nil {
		t.Fatalf("plain NewNetwork() error = %v", err)
	}
	prediction, err := network.Predict(context.Background(), before.Parameters, input)
	if err != nil {
		t.Fatalf("plain independent Predict() error = %v", err)
	}
	return prediction
}

func onlineChemicalWeightWithoutExpressionPrediction(t *testing.T, before learning.IndividualSnapshot, input [][]float64) []float64 {
	t.Helper()
	withoutExpression, err := learning.RestoreIndividual(before)
	if err != nil {
		t.Fatalf("RestoreIndividual() expression control error = %v", err)
	}
	withoutExpression.ClearExpressionGain()
	control := individualChemicalTrainingReference(t, withoutExpression.Snapshot())
	defer func() { _ = control.Close() }()
	prediction, err := control.Predict(context.Background(), input)
	if err != nil {
		t.Fatalf("expression-free ChemicalEpisode Predict() error = %v", err)
	}
	return prediction
}

func onlineChemicalWeightPredictionsDiffer(a, b []float64) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-12 {
			return true
		}
	}
	return false
}

func assertOnlineChemicalWeightActiveBoundary(t *testing.T, snapshot learning.IndividualSnapshot) {
	t.Helper()
	if snapshot.Chemical == nil || snapshot.Chemical.Expression == nil {
		t.Fatalf("expected a persistent chemistry and expression boundary: %+v", snapshot.Chemical)
	}
	if len(snapshot.Chemical.SourceStates) == 0 || snapshot.Chemical.SourceStates[0].Controller == nil {
		t.Fatalf("expected a saved controller source state: %+v", snapshot.Chemical.SourceStates)
	}
	active := false
	for _, row := range snapshot.Chemical.State.Concentration {
		for _, value := range row {
			active = active || value != 0
		}
	}
	if !active {
		t.Fatal("expected nonzero chemistry concentration in the online boundary")
	}
}

func assertOnlineChemicalWeightFreshOptimizer(t *testing.T, snapshot learning.IndividualSnapshot) {
	t.Helper()
	if snapshot.Optimizer.Updates != 0 || snapshot.Optimizer.Episodes != 0 || snapshot.Optimizer.Accumulator != nil || snapshot.Optimizer.Options.WeightDecay != 0 {
		t.Fatalf("fixture is not a fresh zero-decay optimizer: %+v", snapshot.Optimizer)
	}
}

// onlineChemicalWeightFrozenPrediction follows only public Individual APIs.
// ResetNeural supplies the same zero neural episode boundary that TrainEpisode
// documents, and FreezeChemistry makes both the public probe and TrainEpisode
// skip source releases/kinetics while retaining occupancy, effects and
// expression. The probe is a restored twin, so its Advance cannot alter the
// online individual's persistent state.
func onlineChemicalWeightFrozenPrediction(t *testing.T, before learning.IndividualSnapshot, input [][]float64) []float64 {
	t.Helper()
	probe, err := learning.RestoreIndividual(before)
	if err != nil {
		t.Fatalf("RestoreIndividual() frozen probe error = %v", err)
	}
	if err := probe.ResetNeural(context.Background(), make([]float64, individualChemicalTrainingNodes(before.Config))); err != nil {
		t.Fatalf("ResetNeural() frozen probe error = %v", err)
	}
	if err := probe.FreezeChemistry(true); err != nil {
		t.Fatalf("FreezeChemistry() frozen probe error = %v", err)
	}
	outputs, err := probe.Advance(context.Background(), input)
	if err != nil {
		t.Fatalf("frozen public Advance() error = %v", err)
	}
	if len(outputs) == 0 {
		t.Fatal("frozen public Advance() returned no readout")
	}
	afterProbe := probe.Snapshot()
	if !reflect.DeepEqual(afterProbe.Chemical.State, before.Chemical.State) || !reflect.DeepEqual(afterProbe.Chemical.SourceStates, before.Chemical.SourceStates) {
		t.Fatal("frozen public probe advanced concentration or controller source state")
	}
	if !probe.ChemistryReport().Frozen {
		t.Fatal("frozen public probe lost its FreezeChemistry flag")
	}
	return append([]float64(nil), outputs[len(outputs)-1]...)
}

func assertOnlineChemicalWeightPersistentBoundary(t *testing.T, before, after learning.IndividualSnapshot) {
	t.Helper()
	if before.Profile != after.Profile || before.ConfigHash != after.ConfigHash || !reflect.DeepEqual(before.Config, after.Config) {
		t.Fatal("online gradient update changed the immutable individual configuration")
	}
	if !reflect.DeepEqual(before.Neural, after.Neural) {
		t.Fatal("online gradient update advanced persistent neural state")
	}
	if !reflect.DeepEqual(before.Plastic, after.Plastic) {
		t.Fatal("online gradient update changed persistent plastic state")
	}
	if !reflect.DeepEqual(before.Chemical, after.Chemical) {
		t.Fatal("online gradient update changed persistent chemistry, source, resource, feedback, or expression state")
	}
}

// TestOnlineChemicalWeightUsesTheExplicitEpisodePrediction checks the public
// Act -> SetTargets -> Receive -> Update boundary for the ordinary control and
// for both CPU continuous and CPU LIF chemistry. The contract is:
//   - online.go:450-479 maps score to y + clamp(score, 0, 1)*(target-y);
//   - online.go:132-145 and 331-347 make Update the only learning boundary;
//   - individual.go:823-828 makes TrainEpisode an isolated zero-neural-state
//     episode that retains persistent state;
//   - chemical_episode.go:14-22 defines the explicit chemistry boundary.
//
// The twin is trained with the independently predicted effective target. A
// regression where episodePredict ignores chemistry therefore mismatches the
// twin for the chemical cases while the ordinary control remains valid.
func TestOnlineChemicalWeightUsesTheExplicitEpisodePrediction(t *testing.T) {
	scores := []struct {
		name  string
		score float64
	}{
		{name: "zero", score: 0},
		{name: "half", score: .5},
		{name: "one", score: 1},
		{name: "negative clamps zero", score: -1},
		{name: "above one clamps one", score: 2},
	}
	cases := []struct {
		name     string
		build    func(*testing.T) (*learning.Individual, [][]float64, []float64)
		chemical bool
	}{
		{
			name: "ordinary control",
			build: func(t *testing.T) (*learning.Individual, [][]float64, []float64) {
				return onlineIndividual(t), onlineInput(), []float64{.4}
			},
		},
		{
			name: "continuous chemistry expression",
			build: func(t *testing.T) (*learning.Individual, [][]float64, []float64) {
				profile := individualChemicalTrainingProfiles()[0]
				individual, _, _ := prepareIndividualChemicalTrainingBoundary(t, profile)
				return individual, profile.input, profile.target
			},
			chemical: true,
		},
		{
			name: "lif chemistry expression",
			build: func(t *testing.T) (*learning.Individual, [][]float64, []float64) {
				profile := individualChemicalTrainingProfiles()[1]
				individual, _, _ := prepareIndividualChemicalTrainingBoundary(t, profile)
				return individual, profile.input, profile.target
			},
			chemical: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, score := range scores {
				score := score
				t.Run(score.name, func(t *testing.T) {
					individual, input, declared := tc.build(t)
					learner := newLearner(t, individual, onlinePolicy(), nil)
					record := act(t, learner, input)
					before := individual.Snapshot()
					assertOnlineChemicalWeightFreshOptimizer(t, before)
					if tc.chemical {
						assertOnlineChemicalWeightActiveBoundary(t, before)
					}
					beforeReport := individual.ChemistryReport()

					if err := learner.SetTargets(map[string][]float64{record.ActionID: declared}); err != nil {
						t.Fatalf("SetTargets() error = %v", err)
					}
					if err := learner.Receive(context.Background(), feedback(t, record.ActionID, "teacher", int64(record.Step), int64(record.Step), score.score)); err != nil {
						t.Fatalf("Receive() error = %v", err)
					}
					if got := individual.Snapshot(); !reflect.DeepEqual(got, before) {
						t.Fatal("SetTargets/Receive changed the individual before Update")
					}

					twin, err := learning.RestoreIndividual(before)
					if err != nil {
						t.Fatalf("RestoreIndividual() twin error = %v", err)
					}
					prediction := onlineChemicalWeightPrediction(t, before, input)
					if tc.chemical {
						plain := onlineChemicalWeightPlainPrediction(t, before, input)
						if !onlineChemicalWeightPredictionsDiffer(prediction, plain) {
							t.Fatal("chemical prediction did not differ from the same-parameter plain NewNetwork prediction")
						}
						withoutExpression := onlineChemicalWeightWithoutExpressionPrediction(t, before, input)
						if !onlineChemicalWeightPredictionsDiffer(prediction, withoutExpression) {
							t.Fatal("expression gain did not change the explicit chemical prediction")
						}
					}
					weight := onlineChemicalWeightClamp(score.score)
					effective := make([]float64, len(declared))
					for i := range declared {
						effective[i] = prediction[i] + weight*(declared[i]-prediction[i])
					}
					if _, err := twin.TrainEpisode(context.Background(), input, effective); err != nil {
						t.Fatalf("twin TrainEpisode() error = %v", err)
					}

					report := update(t, learner, int64(record.Step))
					if report.Consumed != 1 || report.GradientSteps != 1 {
						t.Fatalf("Update() report = %+v, want one gradient step", report)
					}
					after, want := individual.Snapshot(), twin.Snapshot()
					if !reflect.DeepEqual(after, want) {
						t.Fatalf("online TrainEpisode snapshot differs from twin at score %g:\n got  %+v\n want %+v", score.score, after, want)
					}
					assertOnlineChemicalWeightPersistentBoundary(t, before, after)
					if !reflect.DeepEqual(beforeReport, individual.ChemistryReport()) {
						t.Fatal("online gradient update changed ChemistryReport")
					}
					if weight == 0 && !reflect.DeepEqual(after.Parameters, before.Parameters) {
						t.Fatal("zero loss weight moved parameters with WeightDecay=0")
					}
				})
			}
		})
	}
}

// TestOnlineChemicalWeightUsesFrozenPublicEpisodePrediction covers the
// evaluation posture separately. The production online prediction must retain
// the individual's frozen chemistry flag as well as its saved concentration
// and controller window. Scores 0/.5/1 cover the zero, partial and full
// residual paths without mixing a thawed source into the oracle.
func TestOnlineChemicalWeightUsesFrozenPublicEpisodePrediction(t *testing.T) {
	for _, score := range []struct {
		name  string
		score float64
	}{
		{name: "zero", score: 0},
		{name: "half", score: .5},
		{name: "one", score: 1},
	} {
		score := score
		t.Run(score.name, func(t *testing.T) {
			profile := individualChemicalTrainingProfiles()[0]
			individual, _, _ := prepareIndividualChemicalTrainingBoundary(t, profile)
			if err := individual.FreezeChemistry(true); err != nil {
				t.Fatalf("online FreezeChemistry(true) error = %v", err)
			}
			learner := newLearner(t, individual, onlinePolicy(), nil)
			record := act(t, learner, profile.input)
			before := individual.Snapshot()
			beforeReport := individual.ChemistryReport()
			assertOnlineChemicalWeightFreshOptimizer(t, before)
			assertOnlineChemicalWeightActiveBoundary(t, before)
			if !beforeReport.Frozen {
				t.Fatal("online chemistry report is not frozen before Update")
			}

			if err := learner.SetTargets(map[string][]float64{record.ActionID: profile.target}); err != nil {
				t.Fatalf("SetTargets() error = %v", err)
			}
			if err := learner.Receive(context.Background(), feedback(t, record.ActionID, "teacher", int64(record.Step), int64(record.Step), score.score)); err != nil {
				t.Fatalf("Receive() error = %v", err)
			}

			twin, err := learning.RestoreIndividual(before)
			if err != nil {
				t.Fatalf("RestoreIndividual() twin error = %v", err)
			}
			if err := twin.FreezeChemistry(true); err != nil {
				t.Fatalf("twin FreezeChemistry(true) error = %v", err)
			}
			prediction := onlineChemicalWeightFrozenPrediction(t, before, profile.input)
			weight := onlineChemicalWeightClamp(score.score)
			effective := make([]float64, len(profile.target))
			for i := range profile.target {
				effective[i] = prediction[i] + weight*(profile.target[i]-prediction[i])
			}
			if _, err := twin.TrainEpisode(context.Background(), profile.input, effective); err != nil {
				t.Fatalf("frozen twin TrainEpisode() error = %v", err)
			}

			report := update(t, learner, int64(record.Step))
			if report.Consumed != 1 || report.GradientSteps != 1 {
				t.Fatalf("frozen Update() report = %+v, want one gradient step", report)
			}
			after, want := individual.Snapshot(), twin.Snapshot()
			if !reflect.DeepEqual(after, want) {
				t.Fatalf("frozen online snapshot differs from twin at score %g:\n got  %+v\n want %+v", score.score, after, want)
			}
			assertOnlineChemicalWeightPersistentBoundary(t, before, after)
			if !reflect.DeepEqual(before.Chemical.State, after.Chemical.State) || !reflect.DeepEqual(before.Chemical.SourceStates, after.Chemical.SourceStates) {
				t.Fatal("frozen online Update changed concentration or controller source state")
			}
			if !reflect.DeepEqual(beforeReport, individual.ChemistryReport()) || !individual.ChemistryReport().Frozen {
				t.Fatal("frozen online Update changed ChemistryReport or thawed chemistry")
			}
			if weight == 0 && !reflect.DeepEqual(after.Parameters, before.Parameters) {
				t.Fatal("zero frozen loss weight moved parameters with WeightDecay=0")
			}
		})
	}
}
