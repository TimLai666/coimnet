package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const (
	chemicalRecomputeCheckpointHelperEnv  = "COIMNET_CHEMICAL_RECOMPUTE_CHECKPOINT_HELPER"
	chemicalRecomputeCheckpointSplit      = uint64(1)
	chemicalRecomputeCheckpointTotal      = uint64(7)
	chemicalRecomputeCheckpointSegment    = 3
	chemicalRecomputeCheckpointWindow     = 3
	chemicalRecomputeCheckpointAccumulate = 3
	chemicalRecomputeCheckpointDataSeed   = uint64(0xC0DE0065)
)

type chemicalRecomputeCheckpointChildResult struct {
	Snapshot   learning.TrainingSnapshot `json:"snapshot"`
	Steps      []learning.StepResult     `json:"steps"`
	Prediction []float64                 `json:"prediction"`
}

// TestChemicalRecomputeCheckpointSubprocessResume proves the public episode
// checkpoint boundary for both CPU scalar cores. Each case runs a complete
// eight-row chemical sequence through seven independent Step calls, saves the
// recompute trainer after its first partial accumulation, and resumes that
// actual file in a fresh os.Executable process.
// The caller supplies this fixture's Step offset separately. NextSample keeps
// the checkpoint format's delayed-pulse contract of matching Updates; this test
// does not claim to persist an arbitrary data generator cursor.
func TestChemicalRecomputeCheckpointSubprocessResume(t *testing.T) {
	for _, tc := range []struct {
		name string
		lif  bool
	}{
		{name: "continuous", lif: false},
		{name: "lif", lif: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full := newChemicalRecomputeCheckpointTrainer(t, tc.lif, false)
			defer full.Close()
			recompute := newChemicalRecomputeCheckpointTrainer(t, tc.lif, true)
			defer recompute.Close()

			fullSteps := make([]learning.StepResult, 0, chemicalRecomputeCheckpointTotal)
			for step := uint64(0); step < chemicalRecomputeCheckpointTotal; step++ {
				result, err := full.Step(context.Background(), chemicalRecomputeCheckpointInput(step), chemicalRecomputeCheckpointTarget(step))
				if err != nil {
					t.Fatalf("full-history %s Step %d: %v", tc.name, step, err)
				}
				fullSteps = append(fullSteps, result)
			}

			first, err := recompute.Step(context.Background(), chemicalRecomputeCheckpointInput(0), chemicalRecomputeCheckpointTarget(0))
			if err != nil {
				t.Fatalf("recompute %s split Step 0: %v", tc.name, err)
			}
			assertChemicalRecomputeCheckpointStep(t, tc.name+" split Step", first, fullSteps[0])
			if first.Applied || first.Accumulated != 1 {
				t.Fatalf("recompute %s split result = %+v, want one pending gradient", tc.name, first)
			}
			splitSnapshot := recompute.Snapshot()
			if splitSnapshot.Updates != 0 || splitSnapshot.Accumulator == nil || splitSnapshot.Accumulator.Count != 1 {
				t.Fatalf("recompute %s split snapshot = updates %d accumulator %+v, want updates 0 and count 1", tc.name, splitSnapshot.Updates, splitSnapshot.Accumulator)
			}
			assertChemicalRecomputeCheckpointCore(t, tc.name+" split core", splitSnapshot, tc.lif)
			assertChemicalRecomputeCheckpointBoundary(t, tc.name+" split boundary", splitSnapshot)
			if splitSnapshot.Options.Recompute == nil || splitSnapshot.Options.Recompute.SegmentSteps != chemicalRecomputeCheckpointSegment {
				t.Fatalf("recompute %s split options = %+v, want segment %d", tc.name, splitSnapshot.Options, chemicalRecomputeCheckpointSegment)
			}

			state, err := NewState(splitSnapshot, chemicalRecomputeCheckpointDataSeed, splitSnapshot.Updates)
			if err != nil {
				t.Fatalf("NewState %s split: %v", tc.name, err)
			}
			checkpointPath := filepath.Join(t.TempDir(), tc.name+"-chemical-recompute.json")
			if err := Save(context.Background(), checkpointPath, state); err != nil {
				t.Fatalf("Save %s split checkpoint: %v", tc.name, err)
			}
			loaded, err := Load(context.Background(), checkpointPath)
			if err != nil {
				t.Fatalf("Load %s split checkpoint in parent: %v", tc.name, err)
			}
			assertChemicalRecomputeCheckpointSnapshot(t, tc.name+" loaded split", loaded.Training, splitSnapshot)
			assertChemicalRecomputeCheckpointCore(t, tc.name+" loaded split core", loaded.Training, tc.lif)
			assertChemicalRecomputeCheckpointBoundary(t, tc.name+" loaded split boundary", loaded.Training)
			if loaded.Training.Options.Recompute == nil || loaded.Training.Options.Recompute.SegmentSteps != chemicalRecomputeCheckpointSegment {
				t.Fatalf("loaded %s options = %+v, want segment %d", tc.name, loaded.Training.Options, chemicalRecomputeCheckpointSegment)
			}

			recomputeSteps := []learning.StepResult{first}
			for step := chemicalRecomputeCheckpointSplit; step < chemicalRecomputeCheckpointTotal; step++ {
				result, err := recompute.Step(context.Background(), chemicalRecomputeCheckpointInput(step), chemicalRecomputeCheckpointTarget(step))
				if err != nil {
					t.Fatalf("recompute %s continued Step %d: %v", tc.name, step, err)
				}
				recomputeSteps = append(recomputeSteps, result)
			}
			if len(recomputeSteps) != int(chemicalRecomputeCheckpointTotal) {
				t.Fatalf("recompute %s produced %d Step results, want %d", tc.name, len(recomputeSteps), chemicalRecomputeCheckpointTotal)
			}
			for step := range fullSteps {
				assertChemicalRecomputeCheckpointStep(t, tc.name+" full versus recompute Step "+strconv.Itoa(step), recomputeSteps[step], fullSteps[step])
			}

			fullSnapshot := full.Snapshot()
			recomputeSnapshot := recompute.Snapshot()
			assertChemicalRecomputeCheckpointSnapshot(t, tc.name+" full versus recompute snapshot", recomputeSnapshot, fullSnapshot)
			assertChemicalRecomputeCheckpointBoundary(t, tc.name+" recompute final boundary", recomputeSnapshot)
			if recomputeSnapshot.Options.Recompute == nil || recomputeSnapshot.Options.Recompute.SegmentSteps != chemicalRecomputeCheckpointSegment {
				t.Fatalf("recompute %s final options = %+v, want segment %d", tc.name, recomputeSnapshot.Options, chemicalRecomputeCheckpointSegment)
			}
			if fullSnapshot.Updates != 2 || recomputeSnapshot.Updates != 2 {
				t.Fatalf("%s updates full=%d recompute=%d, want two applied updates", tc.name, fullSnapshot.Updates, recomputeSnapshot.Updates)
			}
			if fullSnapshot.Options.Recompute != nil {
				t.Fatalf("full-history %s options unexpectedly declare recompute: %+v", tc.name, fullSnapshot.Options)
			}
			if fullSnapshot.Accumulator == nil || fullSnapshot.Accumulator.Count != 1 || recomputeSnapshot.Accumulator == nil || recomputeSnapshot.Accumulator.Count != 1 {
				t.Fatalf("%s final accumulators full=%+v recompute=%+v, want count 1", tc.name, fullSnapshot.Accumulator, recomputeSnapshot.Accumulator)
			}

			predictionInput := chemicalRecomputeCheckpointPredictionInput()
			fullPrediction, err := full.Predict(context.Background(), predictionInput)
			if err != nil {
				t.Fatalf("full-history %s Predict: %v", tc.name, err)
			}
			recomputePrediction, err := recompute.Predict(context.Background(), predictionInput)
			if err != nil {
				t.Fatalf("recompute %s Predict: %v", tc.name, err)
			}
			assertChemicalRecomputeCheckpointFloatBits(t, tc.name+" full versus recompute prediction", recomputePrediction, fullPrediction)

			resultPath := filepath.Join(t.TempDir(), tc.name+"-child-result.json")
			executable, err := os.Executable()
			if err != nil {
				t.Fatalf("os.Executable for %s: %v", tc.name, err)
			}
			cmd := exec.Command(executable,
				"-test.run=^TestChemicalRecomputeCheckpointHelperProcess$",
				"--", checkpointPath, resultPath,
				strconv.FormatUint(chemicalRecomputeCheckpointSplit, 10),
				strconv.FormatUint(chemicalRecomputeCheckpointTotal, 10), tc.name,
			)
			cmd.Env = append(os.Environ(), chemicalRecomputeCheckpointHelperEnv+"=1")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s checkpoint helper failed: %v\n%s", tc.name, err, output)
			}
			resultBytes, err := os.ReadFile(resultPath)
			if err != nil {
				t.Fatalf("read %s child result: %v", tc.name, err)
			}
			var child chemicalRecomputeCheckpointChildResult
			if err := json.Unmarshal(resultBytes, &child); err != nil {
				t.Fatalf("decode %s child result: %v", tc.name, err)
			}
			if len(child.Steps) != int(chemicalRecomputeCheckpointTotal-chemicalRecomputeCheckpointSplit) {
				t.Fatalf("%s child returned %d continuation steps, want %d", tc.name, len(child.Steps), chemicalRecomputeCheckpointTotal-chemicalRecomputeCheckpointSplit)
			}
			for index, got := range child.Steps {
				assertChemicalRecomputeCheckpointStep(t, tc.name+" child versus recompute Step "+strconv.Itoa(index+int(chemicalRecomputeCheckpointSplit)), got, recomputeSteps[index+int(chemicalRecomputeCheckpointSplit)])
			}
			assertChemicalRecomputeCheckpointSnapshot(t, tc.name+" child versus recompute snapshot", child.Snapshot, recomputeSnapshot)
			assertChemicalRecomputeCheckpointBoundary(t, tc.name+" child final boundary", child.Snapshot)
			if child.Snapshot.Options.Recompute == nil || child.Snapshot.Options.Recompute.SegmentSteps != chemicalRecomputeCheckpointSegment {
				t.Fatalf("%s child options = %+v, want segment %d", tc.name, child.Snapshot.Options, chemicalRecomputeCheckpointSegment)
			}
			assertChemicalRecomputeCheckpointFloatBits(t, tc.name+" child versus recompute prediction", child.Prediction, recomputePrediction)
		})
	}
}

// TestChemicalRecomputeCheckpointHelperProcess is launched by the parent test
// with a private marker. It only loads the checkpoint and derives the input
// sequence from the explicit step number; it never rebuilds a fixture.
func TestChemicalRecomputeCheckpointHelperProcess(t *testing.T) {
	if os.Getenv(chemicalRecomputeCheckpointHelperEnv) != "1" {
		return
	}
	args := os.Args
	separator := -1
	for index, arg := range args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || len(args)-separator != 6 {
		t.Fatalf("chemical recompute helper arguments = %#v", args)
	}
	checkpointPath, resultPath := args[separator+1], args[separator+2]
	split, err := strconv.ParseUint(args[separator+3], 10, 64)
	if err != nil {
		t.Fatalf("parse chemical recompute helper split %q: %v", args[separator+3], err)
	}
	total, err := strconv.ParseUint(args[separator+4], 10, 64)
	if err != nil {
		t.Fatalf("parse chemical recompute helper total %q: %v", args[separator+4], err)
	}
	if split != chemicalRecomputeCheckpointSplit || total != chemicalRecomputeCheckpointTotal || split >= total {
		t.Fatalf("chemical recompute helper range split=%d total=%d, want split=%d total=%d", split, total, chemicalRecomputeCheckpointSplit, chemicalRecomputeCheckpointTotal)
	}
	kind := args[separator+5]
	if kind != "continuous" && kind != "lif" {
		t.Fatalf("chemical recompute helper core kind = %q", kind)
	}

	state, err := Load(context.Background(), checkpointPath)
	if err != nil {
		t.Fatalf("Load chemical recompute checkpoint in child: %v", err)
	}
	if state.Training.Options.Recompute == nil || state.Training.Options.Recompute.SegmentSteps != chemicalRecomputeCheckpointSegment {
		t.Fatalf("child restored recompute options = %+v, want segment %d", state.Training.Options, chemicalRecomputeCheckpointSegment)
	}
	if state.Training.Options.AccumulateSteps != chemicalRecomputeCheckpointAccumulate {
		t.Fatalf("child restored accumulate_steps = %d, want %d", state.Training.Options.AccumulateSteps, chemicalRecomputeCheckpointAccumulate)
	}
	if state.Training.Accumulator == nil || state.Training.Accumulator.Count != 1 || state.Training.Updates != 0 {
		t.Fatalf("child restored split state updates=%d accumulator=%+v, want updates 0 and count 1", state.Training.Updates, state.Training.Accumulator)
	}
	if state.DataSeed != chemicalRecomputeCheckpointDataSeed {
		t.Fatalf("child restored data seed=%d, want %d", state.DataSeed, chemicalRecomputeCheckpointDataSeed)
	}
	if state.NextSample != state.Training.Updates {
		t.Fatalf("child restored next sample=%d, updates=%d", state.NextSample, state.Training.Updates)
	}
	if (state.Training.Config.LIF != nil) != (kind == "lif") {
		t.Fatalf("child restored core lif=%t, want %q", state.Training.Config.LIF != nil, kind)
	}
	assertChemicalRecomputeCheckpointCore(t, "child loaded core "+kind, state.Training, kind == "lif")
	assertChemicalRecomputeCheckpointBoundary(t, "child loaded boundary", state.Training)

	trainer, err := learning.RestoreTrainer(state.Training)
	if err != nil {
		t.Fatalf("RestoreTrainer chemical recompute child: %v", err)
	}
	defer trainer.Close()
	steps := make([]learning.StepResult, 0, total-split)
	for step := split; step < total; step++ {
		result, err := trainer.Step(context.Background(), chemicalRecomputeCheckpointInput(step), chemicalRecomputeCheckpointTarget(step))
		if err != nil {
			t.Fatalf("child continued %s Step %d: %v", kind, step, err)
		}
		steps = append(steps, result)
	}
	final := trainer.Snapshot()
	if final.Updates != 2 || final.Accumulator == nil || final.Accumulator.Count != 1 {
		t.Fatalf("child final %s state updates=%d accumulator=%+v, want updates 2 and count 1", kind, final.Updates, final.Accumulator)
	}
	prediction, err := trainer.Predict(context.Background(), chemicalRecomputeCheckpointPredictionInput())
	if err != nil {
		t.Fatalf("child Predict %s: %v", kind, err)
	}
	encoded, err := json.Marshal(chemicalRecomputeCheckpointChildResult{Snapshot: final, Steps: steps, Prediction: prediction})
	if err != nil {
		t.Fatalf("marshal child %s result: %v", kind, err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatalf("write child %s result: %v", kind, err)
	}
}

func newChemicalRecomputeCheckpointTrainer(t *testing.T, lif, recompute bool) *learning.Trainer {
	t.Helper()
	base := newChemicalEpisodeCheckpointTrainer(t)
	baseSnapshot := base.Snapshot()
	if err := base.Close(); err != nil {
		t.Fatalf("close chemistry base trainer: %v", err)
	}
	config, parameters := baseSnapshot.Config, baseSnapshot.Parameters
	if config.Chemistry == nil || len(config.Chemistry.Part.PendingFeedback) != 1 {
		t.Fatalf("base chemistry boundary = %#v, want one pending feedback", config.Chemistry)
	}
	// Make the feedback available during the eight-row sequence while keeping
	// its source and produced timestamps from the shared checkpoint fixture.
	config.Chemistry.Part.PendingFeedback[0].AvailableAt.Value = int64(config.Chemistry.SourceStep + 2)
	if lif {
		config.Dynamics = dynamics.Config{}
		config.LIF = &dynamics.LIFConfig{
			Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1},
			DT: 0.7, TauSyn: 1.1, ThetaMin: 0.1, ThetaMax: 1.5, VReset: -0.5,
			RefractorySteps: 2,
			Adaptation:      dynamics.LIFAdaptation{Enabled: true, TauAdapt: 1.8, Beta: 0.25},
			Homeostasis:     &dynamics.LIFHomeostasis{Enabled: true, TauRate: 2.5, TargetRate: 0.25, Eta: 0.2, HMax: 0.3},
			Surrogate:       dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.8},
		}
		parameters.Core = dynamics.Parameters{Weights: []float64{0.35}, Bias: []float64{0.15, 0.08}, LogTau: []float64{0.05, -0.1}}
		parameters.ThetaRaw = []float64{0.2, -0.1}
		parameters.Encoder = []float64{1.1, 0.9}
		parameters.Readout = []float64{0.7, -0.4}
	} else {
		config.LIF = nil
		parameters.ThetaRaw = nil
	}
	options := learning.DefaultOptions()
	options.Truncation = chemicalRecomputeCheckpointWindow
	options.AccumulateSteps = chemicalRecomputeCheckpointAccumulate
	if lif {
		options.Trainable.Theta = true
	}
	if recompute {
		options.Recompute = &learning.Recompute{SegmentSteps: chemicalRecomputeCheckpointSegment}
	}
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		t.Fatalf("NewTrainer lif=%t recompute=%t: %v", lif, recompute, err)
	}
	return trainer
}

func chemicalRecomputeCheckpointInput(step uint64) [][]float64 {
	pattern := [...]float64{0.64, 0.82, 0.71, 0.93, 0.68, 0.87, 0.76, 0.9}
	input := make([][]float64, len(pattern))
	for row := range input {
		input[row] = []float64{pattern[row] + 0.01*float64(step%3)}
	}
	return input
}

func chemicalRecomputeCheckpointTarget(step uint64) []float64 {
	return []float64{0.18 + 0.02*float64(step%4)}
}

func chemicalRecomputeCheckpointPredictionInput() [][]float64 {
	return chemicalRecomputeCheckpointInput(chemicalRecomputeCheckpointTotal + 1)
}

func assertChemicalRecomputeCheckpointCore(t *testing.T, label string, snapshot learning.TrainingSnapshot, lif bool) {
	t.Helper()
	if (snapshot.Config.LIF != nil) != lif {
		t.Fatalf("%s LIF presence=%t, want %t", label, snapshot.Config.LIF != nil, lif)
	}
	if lif {
		config := snapshot.Config.LIF
		if config == nil || !config.Adaptation.Enabled || config.Homeostasis == nil || !config.Homeostasis.Enabled || config.RefractorySteps != 2 {
			t.Fatalf("%s LIF mechanisms=%+v, want adaptation, homeostasis and refractory steps 2", label, config)
		}
		assertChemicalRecomputeCheckpointFloatBits(t, label+" theta raw", snapshot.Parameters.ThetaRaw, []float64{0.2, -0.1})
		if !snapshot.Options.Trainable.Theta {
			t.Fatalf("%s Trainable.Theta=%t, want true", label, snapshot.Options.Trainable.Theta)
		}
		return
	}
	if len(snapshot.Parameters.ThetaRaw) != 0 || snapshot.Options.Trainable.Theta {
		t.Fatalf("%s continuous threshold state theta=%v trainable=%t, want empty and false", label, snapshot.Parameters.ThetaRaw, snapshot.Options.Trainable.Theta)
	}
}

func assertChemicalRecomputeCheckpointBoundary(t *testing.T, label string, snapshot learning.TrainingSnapshot) {
	t.Helper()
	assertChemicalEpisodeCheckpointBoundary(t, snapshot)
	chemistry := snapshot.Config.Chemistry
	if chemistry == nil || chemistry.SourceStep == 0 || chemistry.Part.State.Steps == 0 || chemistry.SourceStep == chemistry.Part.State.Steps {
		t.Fatalf("%s has invalid distinct clocks: %#v", label, chemistry)
	}
	if len(chemistry.Part.State.Concentration) != 2 || len(chemistry.Part.State.Concentration[0]) != 1 || len(chemistry.Part.State.Concentration[1]) != 1 {
		t.Fatalf("%s concentration shape = %#v, want two nonzero regions", label, chemistry.Part.State.Concentration)
	}
	if chemistry.Part.State.Concentration[0][0] == 0 || chemistry.Part.State.Concentration[1][0] == 0 || chemistry.Part.State.Concentration[0][0] == chemistry.Part.State.Concentration[1][0] {
		t.Fatalf("%s concentration = %#v, want nonzero distinct region values", label, chemistry.Part.State.Concentration)
	}
	feedback := chemistry.Part.PendingFeedback[0]
	if feedback.AvailableAt.Value != int64(chemistry.SourceStep+2) || feedback.AvailableAt.Value < int64(chemistry.SourceStep) || feedback.AvailableAt.Value > int64(chemistry.SourceStep)+int64(len(chemicalRecomputeCheckpointInput(0))-1) {
		t.Fatalf("%s pending feedback availability = %+v, want within the eight-row source sequence", label, feedback.AvailableAt)
	}
	if source := chemistry.Part.SourceStates[0].Controller; source == nil || source.Window[2][0] == source.Window[3][0] || source.Window[2][1] == source.Window[3][1] {
		t.Fatalf("%s controller window lost distinct nonzero boundary rows: %#v", label, source)
	}
	if chemistry.Part.Resources["energy"] == 0 || chemistry.Part.Resources["water"] == 0 {
		t.Fatalf("%s resources = %#v, want nonzero energy and water", label, chemistry.Part.Resources)
	}
	if location := chemistry.Part.Config.Receptors.Records[0].Location; location == nil || location.Region != 1 || location.Compartment == "" || location.MappingVersion == "" {
		t.Fatalf("%s receptor location = %#v, want persisted location metadata", label, location)
	}
	if len(chemistry.InitialActivity) != 2 || chemistry.InitialActivity[0] == 0 || chemistry.InitialActivity[1] == 0 || chemistry.InitialActivity[0] == chemistry.InitialActivity[1] {
		t.Fatalf("%s initial activity = %#v, want distinct nonzero values", label, chemistry.InitialActivity)
	}
	if chemistry.Part.Expression == nil || chemistry.Part.Expression.Scale == 0 || len(chemistry.Part.Expression.Nodes) != 1 {
		t.Fatalf("%s expression = %#v, want a nonzero expression gain", label, chemistry.Part.Expression)
	}
}

func assertChemicalRecomputeCheckpointSnapshot(t *testing.T, label string, got, want learning.TrainingSnapshot) {
	t.Helper()
	if got.SchemaVersion != want.SchemaVersion || got.Updates != want.Updates {
		t.Fatalf("%s schema/update state got schema=%q updates=%d want schema=%q updates=%d", label, got.SchemaVersion, got.Updates, want.SchemaVersion, want.Updates)
	}
	gotConfig, err := json.Marshal(got.Config)
	if err != nil {
		t.Fatalf("%s marshal got config: %v", label, err)
	}
	wantConfig, err := json.Marshal(want.Config)
	if err != nil {
		t.Fatalf("%s marshal want config: %v", label, err)
	}
	if !bytes.Equal(gotConfig, wantConfig) {
		t.Fatalf("%s config differs", label)
	}
	gotOptions := got.Options
	wantOptions := want.Options
	gotOptions.Recompute = nil
	wantOptions.Recompute = nil
	gotOptionsJSON, err := json.Marshal(gotOptions)
	if err != nil {
		t.Fatalf("%s marshal got options: %v", label, err)
	}
	wantOptionsJSON, err := json.Marshal(wantOptions)
	if err != nil {
		t.Fatalf("%s marshal want options: %v", label, err)
	}
	if !bytes.Equal(gotOptionsJSON, wantOptionsJSON) {
		t.Fatalf("%s options differ outside Recompute", label)
	}
	assertChemicalRecomputeCheckpointFloatBits(t, label+" core weights", got.Parameters.Core.Weights, want.Parameters.Core.Weights)
	assertChemicalRecomputeCheckpointFloatBits(t, label+" core bias", got.Parameters.Core.Bias, want.Parameters.Core.Bias)
	assertChemicalRecomputeCheckpointFloatBits(t, label+" core log tau", got.Parameters.Core.LogTau, want.Parameters.Core.LogTau)
	assertChemicalRecomputeCheckpointFloatBits(t, label+" theta raw", got.Parameters.ThetaRaw, want.Parameters.ThetaRaw)
	assertChemicalRecomputeCheckpointFloatBits(t, label+" encoder", got.Parameters.Encoder, want.Parameters.Encoder)
	assertChemicalRecomputeCheckpointFloatBits(t, label+" readout", got.Parameters.Readout, want.Parameters.Readout)
	assertChemicalRecomputeCheckpointFloatBits(t, label+" Adam first", got.Optimizer.First, want.Optimizer.First)
	assertChemicalRecomputeCheckpointFloatBits(t, label+" Adam second", got.Optimizer.Second, want.Optimizer.Second)
	if len(got.Optimizer.Steps) != len(want.Optimizer.Steps) {
		t.Fatalf("%s Adam step count length got %d want %d", label, len(got.Optimizer.Steps), len(want.Optimizer.Steps))
	}
	for index := range want.Optimizer.Steps {
		if got.Optimizer.Steps[index] != want.Optimizer.Steps[index] {
			t.Fatalf("%s Adam update count[%d] got %d want %d", label, index, got.Optimizer.Steps[index], want.Optimizer.Steps[index])
		}
	}
	if (got.Accumulator == nil) != (want.Accumulator == nil) {
		t.Fatalf("%s accumulator presence got=%+v want=%+v", label, got.Accumulator, want.Accumulator)
	}
	if got.Accumulator != nil {
		if got.Accumulator.Count != want.Accumulator.Count {
			t.Fatalf("%s accumulator count got %d want %d", label, got.Accumulator.Count, want.Accumulator.Count)
		}
		assertChemicalRecomputeCheckpointFloatBits(t, label+" accumulator sum", got.Accumulator.Sum, want.Accumulator.Sum)
	}
}

func assertChemicalRecomputeCheckpointStep(t *testing.T, label string, got, want learning.StepResult) {
	t.Helper()
	assertChemicalRecomputeCheckpointFinite(t, label+" loss", got.Loss)
	assertChemicalRecomputeCheckpointFinite(t, label+" want loss", want.Loss)
	assertChemicalRecomputeCheckpointFinite(t, label+" gradient norm", got.GradientNorm)
	assertChemicalRecomputeCheckpointFinite(t, label+" want gradient norm", want.GradientNorm)
	assertChemicalRecomputeCheckpointFinite(t, label+" update norm", got.UpdateNorm)
	assertChemicalRecomputeCheckpointFinite(t, label+" want update norm", want.UpdateNorm)
	assertChemicalRecomputeCheckpointFinite(t, label+" learning rate", got.LearningRate)
	assertChemicalRecomputeCheckpointFinite(t, label+" want learning rate", want.LearningRate)
	if math.Float64bits(got.Loss) != math.Float64bits(want.Loss) || got.LossKnown != want.LossKnown || math.Float64bits(got.GradientNorm) != math.Float64bits(want.GradientNorm) || math.Float64bits(got.UpdateNorm) != math.Float64bits(want.UpdateNorm) || math.Float64bits(got.LearningRate) != math.Float64bits(want.LearningRate) || got.Updates != want.Updates || got.Applied != want.Applied || got.Accumulated != want.Accumulated || got.GradientHorizonSteps != want.GradientHorizonSteps {
		t.Fatalf("%s differs: got=%+v want=%+v", label, got, want)
	}
	if (got.Projected == nil) != (want.Projected == nil) || len(got.Projected) != len(want.Projected) {
		t.Fatalf("%s projected map differs: got=%#v want=%#v", label, got.Projected, want.Projected)
	}
	for key, wantValue := range want.Projected {
		if gotValue, ok := got.Projected[key]; !ok || gotValue != wantValue {
			t.Fatalf("%s projected[%q] got %d/%t want %d", label, key, gotValue, ok, wantValue)
		}
	}
}

func assertChemicalRecomputeCheckpointFloatBits(t *testing.T, label string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length got %d want %d", label, len(got), len(want))
	}
	for index := range want {
		assertChemicalRecomputeCheckpointFinite(t, label+"["+strconv.Itoa(index)+"]", got[index])
		assertChemicalRecomputeCheckpointFinite(t, label+" want["+strconv.Itoa(index)+"]", want[index])
		if math.Float64bits(got[index]) != math.Float64bits(want[index]) {
			t.Fatalf("%s[%d] got %.17g want %.17g", label, index, got[index], want[index])
		}
	}
}

func assertChemicalRecomputeCheckpointFinite(t *testing.T, label string, value float64) {
	t.Helper()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		t.Fatalf("%s is non-finite: %v", label, value)
	}
}
