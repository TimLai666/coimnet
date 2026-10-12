package checkpoint

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

const (
	chemicalEpisodeCheckpointDataSeed   uint64 = 0xC0DE0012
	chemicalEpisodeCheckpointSourceStep uint64 = 17
	chemicalEpisodeCheckpointStateSteps uint64 = 2
	chemicalEpisodeCheckpointSplit      uint64 = 1
	chemicalEpisodeCheckpointTotal      uint64 = 5
	chemicalEpisodeCheckpointWindow            = 4
	chemicalEpisodeCheckpointFilled            = 2
)

type chemicalEpisodeCheckpointFixture struct {
	path    string
	state   State
	doc     []byte
	raw     envelope
	payload map[string]json.RawMessage
}

type chemicalEpisodeCheckpointChildResult struct {
	Snapshot   learning.TrainingSnapshot `json:"snapshot"`
	Steps      []learning.StepResult     `json:"steps"`
	Prediction []float64                 `json:"prediction"`
}

// TestChemicalEpisodeCheckpointRoundTripAndSubprocessResume is the complete
// episode-training boundary: the chemistry declaration and state are saved in
// NewState, read from a real checkpoint by a new test process, restored into a
// trainer, and continued through a partial accumulation window. The process
// result must be bit-identical to the uninterrupted reference.
func TestChemicalEpisodeCheckpointRoundTripAndSubprocessResume(t *testing.T) {
	fixture := newChemicalEpisodeCheckpointFixture(t)
	assertChemicalEpisodeCheckpointBoundary(t, fixture.state.Training)

	full := newChemicalEpisodeCheckpointTrainer(t)
	defer full.Close()
	fullSteps := trainChemicalEpisodeCheckpoint(t, full, 0, chemicalEpisodeCheckpointTotal)
	fullSnapshot := full.Snapshot()
	fullPrediction, err := full.Predict(context.Background(), chemicalEpisodeCheckpointPredictionInput())
	if err != nil {
		t.Fatalf("reference Predict: %v", err)
	}

	resultPath := filepath.Join(t.TempDir(), "child-result.json")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(executable,
		"-test.run=^TestChemicalEpisodeCheckpointHelperProcess$",
		"--", fixture.path, resultPath,
		strconv.FormatUint(chemicalEpisodeCheckpointSplit, 10),
		strconv.FormatUint(chemicalEpisodeCheckpointTotal, 10),
	)
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkpoint helper failed: %v\n%s", err, output)
	}

	resultBytes, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read child result: %v", err)
	}
	var child chemicalEpisodeCheckpointChildResult
	if err := json.Unmarshal(resultBytes, &child); err != nil {
		t.Fatalf("decode child result: %v", err)
	}
	if !reflect.DeepEqual(child.Snapshot, fullSnapshot) {
		t.Fatalf("subprocess snapshot differs from uninterrupted snapshot:\n got %#v\nwant %#v", child.Snapshot, fullSnapshot)
	}
	if !reflect.DeepEqual(child.Steps, fullSteps[int(chemicalEpisodeCheckpointSplit):]) {
		t.Fatalf("subprocess Step results differ from uninterrupted continuation:\n got %#v\nwant %#v", child.Steps, fullSteps[int(chemicalEpisodeCheckpointSplit):])
	}
	if !reflect.DeepEqual(child.Prediction, fullPrediction) {
		t.Fatalf("subprocess prediction differs from uninterrupted prediction:\n got %#v\nwant %#v", child.Prediction, fullPrediction)
	}
	if child.Snapshot.Accumulator == nil || child.Snapshot.Accumulator.Count != 2 {
		t.Fatalf("continued snapshot lost the partial accumulation boundary: %+v", child.Snapshot.Accumulator)
	}

	restored, err := learning.RestoreTrainer(child.Snapshot)
	if err != nil {
		t.Fatalf("restore child result: %v", err)
	}
	if got := restored.Snapshot(); !reflect.DeepEqual(got, fullSnapshot) {
		t.Fatal("child JSON result did not restore to the uninterrupted snapshot")
	}
}

// TestChemicalEpisodeCheckpointHelperProcess is launched through os.Executable
// by the parent test. It deliberately reads the checkpoint from disk before
// restoring or stepping, so an in-process copy cannot make this test pass.
func TestChemicalEpisodeCheckpointHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	args := os.Args
	separator := -1
	for i, arg := range args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || len(args)-separator != 5 {
		t.Fatalf("helper arguments = %#v", args)
	}
	checkpointPath, resultPath := args[separator+1], args[separator+2]
	split, err := strconv.ParseUint(args[separator+3], 10, 64)
	if err != nil {
		t.Fatalf("parse split: %v", err)
	}
	total, err := strconv.ParseUint(args[separator+4], 10, 64)
	if err != nil {
		t.Fatalf("parse total: %v", err)
	}

	state, err := Load(context.Background(), checkpointPath)
	if err != nil {
		t.Fatalf("Load in child: %v", err)
	}
	trainer, err := learning.RestoreTrainer(state.Training)
	if err != nil {
		t.Fatalf("RestoreTrainer in child: %v", err)
	}
	defer trainer.Close()
	if state.NextSample != state.Training.Updates {
		t.Fatalf("checkpoint next sample = %d, updates = %d", state.NextSample, state.Training.Updates)
	}
	steps := trainChemicalEpisodeCheckpoint(t, trainer, split, total)
	prediction, err := trainer.Predict(context.Background(), chemicalEpisodeCheckpointPredictionInput())
	if err != nil {
		t.Fatalf("child Predict: %v", err)
	}
	result := chemicalEpisodeCheckpointChildResult{Snapshot: trainer.Snapshot(), Steps: steps, Prediction: prediction}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal child result: %v", err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatalf("write child result: %v", err)
	}
}

// TestNilChemistryEpisodeCheckpointKeepsLegacyFormat makes the nil pointer
// boundary explicit. Existing checkpoints without chemistry remain readable,
// and restoring them still yields the same ordinary trainer snapshot.
func TestNilChemistryEpisodeCheckpointKeepsLegacyFormat(t *testing.T) {
	trainer := newDelayedTrainer(t, 7)
	defer trainer.Close()
	state, err := NewState(trainer.Snapshot(), 1001, 0)
	if err != nil {
		t.Fatalf("NewState without chemistry: %v", err)
	}
	path := filepath.Join(t.TempDir(), "nil-chemistry.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatalf("Save without chemistry: %v", err)
	}
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read nil-chemistry checkpoint: %v", err)
	}
	var raw envelope
	if err := json.Unmarshal(document, &raw); err != nil {
		t.Fatalf("decode nil-chemistry envelope: %v", err)
	}
	if strings.Contains(string(raw.Payload), `"chemistry"`) {
		t.Fatalf("nil chemistry was written into the legacy payload: %s", raw.Payload)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatalf("Load without chemistry: %v", err)
	}
	if !reflect.DeepEqual(loaded, state) {
		t.Fatalf("nil chemistry round trip changed state:\n got %#v\nwant %#v", loaded, state)
	}
	restored, err := learning.RestoreTrainer(loaded.Training)
	if err != nil {
		t.Fatalf("RestoreTrainer without chemistry: %v", err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), state.Training) {
		t.Fatal("restoring the nil-chemistry checkpoint changed the ordinary trainer snapshot")
	}
}

// TestChemicalEpisodeCheckpointRejectsMissingOrNullRequiredFields first reads
// a legal chemistry checkpoint. Every mutation below therefore exercises the
// strict new envelope rather than a checksum-only or already-invalid document.
func TestChemicalEpisodeCheckpointRejectsMissingOrNullRequiredFields(t *testing.T) {
	fixture := newChemicalEpisodeCheckpointFixture(t)
	cases := []struct {
		name   string
		mutate func(*testing.T, map[string]json.RawMessage)
		want   string
	}{
		{"missing part", mutateChemical(func(chemistry map[string]json.RawMessage) { delete(chemistry, "part") }), "part"},
		{"null part", mutateChemical(func(chemistry map[string]json.RawMessage) { chemistry["part"] = json.RawMessage(`null`) }), "part"},
		{"missing source_step", mutateChemical(func(chemistry map[string]json.RawMessage) { delete(chemistry, "source_step") }), "source_step"},
		{"null source_step", mutateChemical(func(chemistry map[string]json.RawMessage) { chemistry["source_step"] = json.RawMessage(`null`) }), "source_step"},
		{"missing resources", mutateChemicalPart(func(part map[string]json.RawMessage) { delete(part, "resources") }), "resources"},
		{"null resources", mutateChemicalPart(func(part map[string]json.RawMessage) { part["resources"] = json.RawMessage(`null`) }), "resources"},
		{"missing pending_feedback", mutateChemicalPart(func(part map[string]json.RawMessage) { delete(part, "pending_feedback") }), "pending_feedback"},
		{"null pending_feedback", mutateChemicalPart(func(part map[string]json.RawMessage) { part["pending_feedback"] = json.RawMessage(`null`) }), "pending_feedback"},
		{"missing controller window", mutateChemicalController(func(controller map[string]json.RawMessage) { delete(controller, "window") }), "window"},
		{"null controller window", mutateChemicalController(func(controller map[string]json.RawMessage) { controller["window"] = json.RawMessage(`null`) }), "window"},
		{"missing controller filled", mutateChemicalController(func(controller map[string]json.RawMessage) { delete(controller, "filled") }), "filled"},
		{"null controller filled", mutateChemicalController(func(controller map[string]json.RawMessage) { controller["filled"] = json.RawMessage(`null`) }), "filled"},
		{"missing receptor location compartment", mutateChemicalLocation(func(location map[string]json.RawMessage) { delete(location, "compartment") }), "compartment"},
		{"null receptor location region", mutateChemicalLocation(func(location map[string]json.RawMessage) { location["region"] = json.RawMessage(`null`) }), "region"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := chemicalEpisodeCheckpointMutation(t, fixture.doc, tc.mutate)
			if _, err := loadChemicalEpisodeMutation(t, bad); err == nil {
				t.Fatalf("accepted malformed checkpoint for %s", tc.name)
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name %s", err, tc.want)
			}
		})
	}
}

// TestChemicalEpisodeCheckpointRejectsUnknownAndMisalignedBoundaries covers
// strict object decoding and semantic source-state alignment after checksum
// verification. The fixture's legal round trip is performed before mutation.
func TestChemicalEpisodeCheckpointRejectsUnknownAndMisalignedBoundaries(t *testing.T) {
	fixture := newChemicalEpisodeCheckpointFixture(t)
	cases := []struct {
		name   string
		mutate func(*testing.T, map[string]json.RawMessage)
		want   string
	}{
		{"unknown chemistry field", mutateChemical(func(chemistry map[string]json.RawMessage) { chemistry["unexpected"] = json.RawMessage(`true`) }), "unexpected"},
		{"unknown controller field", mutateChemicalController(func(controller map[string]json.RawMessage) { controller["unexpected"] = json.RawMessage(`true`) }), "unexpected"},
		{"unknown location field", mutateChemicalLocation(func(location map[string]json.RawMessage) { location["unexpected"] = json.RawMessage(`true`) }), "unexpected"},
		{"source state count mismatch", mutateChemicalPart(func(part map[string]json.RawMessage) { part["source_states"] = json.RawMessage(`[]`) }), "source state"},
		{"source controller alignment mismatch", mutateChemicalPart(func(part map[string]json.RawMessage) {
			states := rawArray(t, part["source_states"])
			state := rawObject(t, states[0])
			state["controller"] = json.RawMessage(`null`)
			states[0] = mustRawJSON(t, state)
			part["source_states"] = mustRawJSON(t, states)
		}), "controller"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := chemicalEpisodeCheckpointMutation(t, fixture.doc, tc.mutate)
			if _, err := loadChemicalEpisodeMutation(t, bad); err == nil {
				t.Fatalf("accepted malformed checkpoint for %s", tc.name)
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name %s", err, tc.want)
			}
		})
	}
}

func newChemicalEpisodeCheckpointFixture(t *testing.T) chemicalEpisodeCheckpointFixture {
	t.Helper()
	trainer := newChemicalEpisodeCheckpointTrainer(t)
	defer trainer.Close()
	first, err := trainer.Step(context.Background(), chemicalEpisodeCheckpointInput(0), chemicalEpisodeCheckpointTarget(0))
	if err != nil {
		t.Fatalf("partial chemistry Step: %v", err)
	}
	if first.Applied || first.Accumulated != 1 {
		t.Fatalf("first chemistry Step did not leave one accumulated gradient: %+v", first)
	}
	snapshot := trainer.Snapshot()
	if snapshot.Accumulator == nil || snapshot.Accumulator.Count != 1 {
		t.Fatalf("partial accumulator missing from snapshot: %+v", snapshot.Accumulator)
	}
	state, err := NewState(snapshot, chemicalEpisodeCheckpointDataSeed, snapshot.Updates)
	if err != nil {
		t.Fatalf("NewState with chemistry: %v", err)
	}
	if state.NextSample != state.Training.Updates {
		t.Fatalf("NewState cursor = %d, updates = %d", state.NextSample, state.Training.Updates)
	}
	// NewState must own every nested source boundary. Mutating the snapshot
	// supplied by the caller must not alter the state that will be saved.
	snapshot.Config.Chemistry.Part.Config.Sources[0].Controller.Parameters[0] = 9
	snapshot.Config.Chemistry.Part.SourceStates[0].Controller.Window[3][0] = 9
	snapshot.Config.Chemistry.Part.Resources["energy"] = 9
	snapshot.Config.Chemistry.InitialActivity[0] = 9
	assertChemicalEpisodeCheckpointBoundary(t, state.Training)
	if state.Training.Config.Chemistry.Part.Config.Sources[0].Controller.Parameters[0] == 9 ||
		state.Training.Config.Chemistry.Part.SourceStates[0].Controller.Window[3][0] == 9 ||
		state.Training.Config.Chemistry.Part.Resources["energy"] == 9 ||
		state.Training.Config.Chemistry.InitialActivity[0] == 9 {
		t.Fatal("NewState retained caller-owned chemistry source or boundary data")
	}

	path := filepath.Join(t.TempDir(), "chemical-episode.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatalf("Save chemistry checkpoint: %v", err)
	}
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read chemistry checkpoint: %v", err)
	}
	var raw envelope
	if err := json.Unmarshal(document, &raw); err != nil {
		t.Fatalf("decode chemistry envelope: %v", err)
	}
	payload := rawObject(t, raw.Payload)
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatalf("Load legal chemistry checkpoint: %v", err)
	}
	if !reflect.DeepEqual(loaded, state) {
		t.Fatalf("legal chemistry round trip changed state:\n got %#v\nwant %#v", loaded, state)
	}
	assertChemicalEpisodeCheckpointBoundary(t, loaded.Training)

	// A newly returned boundary must also be independent of later callers.
	loaded.Training.Config.Chemistry.Part.Config.Sources[0].Controller.Parameters[0] = 8
	loaded.Training.Config.Chemistry.Part.SourceStates[0].Controller.Window[3][0] = 8
	reloaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatalf("reload chemistry checkpoint: %v", err)
	}
	if reloaded.Training.Config.Chemistry.Part.Config.Sources[0].Controller.Parameters[0] == 8 ||
		reloaded.Training.Config.Chemistry.Part.SourceStates[0].Controller.Window[3][0] == 8 {
		t.Fatal("Load returned chemistry source data aliased with a later caller mutation")
	}
	return chemicalEpisodeCheckpointFixture{path: path, state: state, doc: document, raw: raw, payload: payload}
}

func newChemicalEpisodeCheckpointTrainer(t *testing.T) *learning.Trainer {
	t.Helper()
	controller := &modulation.Controller{
		Inputs: modulation.ControllerInputs{SummaryWindow: chemicalEpisodeCheckpointWindow, UseFeedback: true, Resources: []string{"energy"}},
		Nodes:  2, Hidden: 1, Channel: 0,
		Parameters: []float64{0.2, 0.1, 0.05, 0.3, 0.7, -0.2, 0.1},
	}
	episode := &learning.ChemicalEpisode{
		Part: learning.ChemicalPart{
			Config: modulation.ChemistryConfig{
				Chemistry: modulation.Chemistry{
					Regions: 2, Channels: 1, DT: 1, Tau: []float64{2}, Units: modulation.UnitNormalized,
					Transport: &modulation.Transport{Fraction: [][]float64{{0, 0.2}, {0.1, 0}}},
				},
				Sources: []modulation.SourceSpec{{Kind: modulation.SourceController, Channel: 0, Controller: controller}},
				Receptors: modulation.Receptors{Records: []modulation.Receptor{{
					Cells: []int{0, 1}, Signal: "fixture", Channel: 0, Status: modulation.StatusHypothesized,
					Kd: 0.5, N: 1, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-checkpoint/v1",
					Location: &modulation.ReceptorLocation{
						Region: 1, Compartment: "postsynaptic", AnatomicalLocation: "fixture-region-1",
						Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-checkpoint/location-v1",
					},
				}}},
				Effects: []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 0.8, BetaScale: 0.1}},
				Regions: modulation.RegionAssignment{NodeRegion: []int{0, 1}},
			},
			State: modulation.ChemistryState{
				Concentration: [][]float64{{0.25}, {0.4}}, Steps: chemicalEpisodeCheckpointStateSteps,
			},
			Resources:       map[string]float64{"energy": 1.25, "water": 0.75},
			PendingFeedback: []signal.FeedbackSpec{chemicalEpisodeCheckpointFeedback()},
			SourceStates: []learning.ChemicalSourceState{{Controller: &modulation.ControllerState{
				Window: [][]float64{{0, 0}, {0, 0}, {0.15, -0.1}, {0.25, -0.2}}, Filled: chemicalEpisodeCheckpointFilled,
			}}},
			Expression: &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 0.7, Min: 0.5, Max: 2},
		},
		SourceStep:      chemicalEpisodeCheckpointSourceStep,
		InitialActivity: []float64{0.21, -0.13},
	}
	config := learning.Config{
		Dynamics:  dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1}, DT: 0.7, Activation: "tanh"},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0, 1}, InputNodes: []int{0, 1}, Chemistry: episode,
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{0.25}, Bias: []float64{0.05, -0.03}, LogTau: []float64{0.1, -0.05}},
		Encoder: []float64{0.45, -0.35}, Readout: []float64{0.6, -0.4},
	}
	options := learning.DefaultOptions()
	options.AccumulateSteps = 3
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		t.Fatalf("NewTrainer with chemistry: %v", err)
	}
	return trainer
}

func chemicalEpisodeCheckpointFeedback() signal.FeedbackSpec {
	version := signal.CurrentSchemaVersion()
	return signal.FeedbackSpec{
		SchemaVersion: version, ExperienceID: "chem-checkpoint", ActionID: "action",
		ProducedAt:  signal.Timestamp{Value: 17, Unit: signal.TimeUnitModelStep},
		AvailableAt: signal.Timestamp{Value: 40, Unit: signal.TimeUnitModelStep},
		Source:      "fixture", Score: 0.4, ModelVersion: version,
	}
}

func chemicalEpisodeCheckpointInput(step uint64) [][]float64 {
	inputs := [][][]float64{
		{{0.45}, {0.25}, {-0.1}},
		{{0.35}, {-0.2}, {0.3}},
		{{-0.4}, {0.1}, {0.2}},
		{{0.2}, {0.4}, {-0.3}},
		{{-0.15}, {0.05}, {0.35}},
	}
	return inputs[int(step)%len(inputs)]
}

func chemicalEpisodeCheckpointTarget(step uint64) []float64 {
	targets := [][]float64{{0.15}, {-0.2}, {0.3}, {-0.1}, {0.25}}
	return targets[int(step)%len(targets)]
}

func chemicalEpisodeCheckpointPredictionInput() [][]float64 {
	return [][]float64{{0.11}, {-0.08}, {0.19}}
}

func trainChemicalEpisodeCheckpoint(t *testing.T, trainer *learning.Trainer, start, end uint64) []learning.StepResult {
	t.Helper()
	results := make([]learning.StepResult, 0, int(end-start))
	for step := start; step < end; step++ {
		result, err := trainer.Step(context.Background(), chemicalEpisodeCheckpointInput(step), chemicalEpisodeCheckpointTarget(step))
		if err != nil {
			t.Fatalf("chemistry Step %d: %v", step, err)
		}
		results = append(results, result)
	}
	return results
}

func assertChemicalEpisodeCheckpointBoundary(t *testing.T, snapshot learning.TrainingSnapshot) {
	t.Helper()
	chemistry := snapshot.Config.Chemistry
	if chemistry == nil {
		t.Fatal("training snapshot lost its ChemicalEpisode")
	}
	if chemistry.SourceStep == chemistry.Part.State.Steps {
		t.Fatalf("source_step %d unexpectedly equals chemistry state steps %d", chemistry.SourceStep, chemistry.Part.State.Steps)
	}
	if chemistry.SourceStep != chemicalEpisodeCheckpointSourceStep || chemistry.Part.State.Steps != chemicalEpisodeCheckpointStateSteps {
		t.Fatalf("source/state clock boundary changed: source_step=%d state_steps=%d", chemistry.SourceStep, chemistry.Part.State.Steps)
	}
	if len(chemistry.Part.Resources) != 2 || chemistry.Part.Resources["energy"] != 1.25 || chemistry.Part.Resources["water"] != 0.75 {
		t.Fatalf("resource map changed: %#v", chemistry.Part.Resources)
	}
	if len(chemistry.Part.PendingFeedback) != 1 {
		t.Fatalf("pending feedback changed: %#v", chemistry.Part.PendingFeedback)
	}
	if chemistry.Part.Expression == nil || !reflect.DeepEqual(chemistry.Part.Expression.Nodes, []int{0}) || chemistry.Part.Expression.Scale != 0.7 {
		t.Fatalf("expression boundary changed: %#v", chemistry.Part.Expression)
	}
	if !reflect.DeepEqual(chemistry.InitialActivity, []float64{0.21, -0.13}) {
		t.Fatalf("initial activity changed: %#v", chemistry.InitialActivity)
	}
	location := chemistry.Part.Config.Receptors.Records[0].Location
	if location == nil || location.Region != 1 || location.Compartment != "postsynaptic" || location.MappingVersion != "chem-checkpoint/location-v1" {
		t.Fatalf("receptor position changed: %#v", location)
	}
	if len(chemistry.Part.SourceStates) != 1 || chemistry.Part.SourceStates[0].Controller == nil {
		t.Fatalf("controller source state missing: %#v", chemistry.Part.SourceStates)
	}
	controllerState := chemistry.Part.SourceStates[0].Controller
	if len(controllerState.Window) != chemicalEpisodeCheckpointWindow || controllerState.Filled != chemicalEpisodeCheckpointFilled {
		t.Fatalf("controller window boundary changed: %#v", controllerState)
	}
	maxDelay := 0
	for _, delay := range snapshot.Config.Dynamics.Delays {
		if delay > maxDelay {
			maxDelay = delay
		}
	}
	if len(controllerState.Window) <= maxDelay {
		t.Fatalf("controller window width %d is not greater than max delay %d", len(controllerState.Window), maxDelay)
	}
	if snapshot.Options.AccumulateSteps != 3 || snapshot.Accumulator == nil || snapshot.Accumulator.Count < 1 || snapshot.Accumulator.Count >= snapshot.Options.AccumulateSteps {
		t.Fatalf("partial accumulation boundary changed: options=%+v accumulator=%+v", snapshot.Options, snapshot.Accumulator)
	}
}

func chemicalEpisodeCheckpointMutation(t *testing.T, document []byte, mutate func(*testing.T, map[string]json.RawMessage)) []byte {
	t.Helper()
	var raw envelope
	if err := json.Unmarshal(document, &raw); err != nil {
		t.Fatalf("decode mutation envelope: %v", err)
	}
	payload := rawObject(t, raw.Payload)
	mutate(t, payload)
	changed := mustRawJSON(t, payload)
	return envelopeJSON(raw.SchemaVersion, changed, checksumHex(changed))
}

func mutateChemical(mutate func(map[string]json.RawMessage)) func(*testing.T, map[string]json.RawMessage) {
	return func(t *testing.T, payload map[string]json.RawMessage) {
		training, config, chemistry := chemicalEpisodeCheckpointMaps(t, payload)
		mutate(chemistry)
		config["chemistry"] = mustRawJSON(t, chemistry)
		training["config"] = mustRawJSON(t, config)
		payload["training"] = mustRawJSON(t, training)
	}
}

func mutateChemicalPart(mutate func(map[string]json.RawMessage)) func(*testing.T, map[string]json.RawMessage) {
	return func(t *testing.T, payload map[string]json.RawMessage) {
		training, config, chemistry := chemicalEpisodeCheckpointMaps(t, payload)
		part := rawObject(t, chemistry["part"])
		mutate(part)
		chemistry["part"] = mustRawJSON(t, part)
		config["chemistry"] = mustRawJSON(t, chemistry)
		training["config"] = mustRawJSON(t, config)
		payload["training"] = mustRawJSON(t, training)
	}
}

func mutateChemicalController(mutate func(map[string]json.RawMessage)) func(*testing.T, map[string]json.RawMessage) {
	return func(t *testing.T, payload map[string]json.RawMessage) {
		mutateChemicalPart(func(part map[string]json.RawMessage) {
			states := rawArray(t, part["source_states"])
			state := rawObject(t, states[0])
			controller := rawObject(t, state["controller"])
			mutate(controller)
			state["controller"] = mustRawJSON(t, controller)
			states[0] = mustRawJSON(t, state)
			part["source_states"] = mustRawJSON(t, states)
		})(t, payload)
	}
}

func mutateChemicalRecord(mutate func(map[string]json.RawMessage)) func(*testing.T, map[string]json.RawMessage) {
	return func(t *testing.T, payload map[string]json.RawMessage) {
		mutateChemicalPart(func(part map[string]json.RawMessage) {
			config := rawObject(t, part["config"])
			receptors := rawObject(t, config["receptors"])
			var records []json.RawMessage
			if err := json.Unmarshal(receptors["records"], &records); err != nil {
				t.Fatalf("decode receptor records: %v", err)
			}
			record := rawObject(t, records[0])
			mutate(record)
			records[0] = mustRawJSON(t, record)
			receptors["records"] = mustRawJSON(t, records)
			config["receptors"] = mustRawJSON(t, receptors)
			part["config"] = mustRawJSON(t, config)
		})(t, payload)
	}
}

func mutateChemicalLocation(mutate func(map[string]json.RawMessage)) func(*testing.T, map[string]json.RawMessage) {
	return func(t *testing.T, payload map[string]json.RawMessage) {
		mutateChemicalRecord(func(record map[string]json.RawMessage) {
			location := rawObject(t, record["location"])
			mutate(location)
			record["location"] = mustRawJSON(t, location)
		})(t, payload)
	}
}

func chemicalEpisodeCheckpointMaps(t *testing.T, payload map[string]json.RawMessage) (map[string]json.RawMessage, map[string]json.RawMessage, map[string]json.RawMessage) {
	t.Helper()
	training := rawObject(t, payload["training"])
	config := rawObject(t, training["config"])
	chemistry := rawObject(t, config["chemistry"])
	return training, config, chemistry
}

func rawArray(t *testing.T, raw json.RawMessage) []json.RawMessage {
	t.Helper()
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatalf("decode JSON array: %v", err)
	}
	return values
}

func loadChemicalEpisodeMutation(t *testing.T, document []byte) (State, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mutated-chemical.json")
	writeRaw(t, path, document)
	return Load(context.Background(), path)
}
