package checkpoint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/signal"
)

func TestSelectedProjectionSubprocessResume(t *testing.T) {
	candidates := []signal.NeuronCandidate{
		{ID: signal.NeuronID{Namespace: "fixture", ExternalID: "a"}},
		{ID: signal.NeuronID{Namespace: "fixture", ExternalID: "b"}},
		{ID: signal.NeuronID{Namespace: "fixture", ExternalID: "c"}},
	}
	projection := func(direction signal.ProjectionDirection, ids []signal.NeuronID, weights []float64) signal.Projection {
		t.Helper()
		p, err := signal.NewProjection(signal.ProjectionSpec{SchemaVersion: signal.CurrentSchemaVersion(), Direction: direction, Source: "artificial-checkpoint-fixture/v1", Artificial: true, ChannelShape: []int{1}, Selection: signal.NeuronSelection{Mode: signal.SelectExplicit, Neurons: ids}, Weights: weights}, candidates)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	initial := newDelayedTrainer(t, 7).Snapshot()
	input := projection(signal.ProjectionInput, []signal.NeuronID{candidates[2].ID, candidates[0].ID}, []float64{.2, .7})
	output := projection(signal.ProjectionOutput, []signal.NeuronID{candidates[1].ID}, []float64{.8})
	config, parameters, err := learning.BindProjections(initial.Config, initial.Parameters, candidates, input, output)
	if err != nil {
		t.Fatal(err)
	}
	newTrainer := func() *learning.Trainer {
		t.Helper()
		trainer, err := learning.NewTrainer(config, parameters, learning.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		return trainer
	}
	full, part := newTrainer(), newTrainer()
	const seed = uint64(1001)
	train(t, full, seed, 0, 9)
	train(t, part, seed, 0, 4)
	midpoint, err := NewState(part.Snapshot(), seed, 4)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path, resumedPath := filepath.Join(dir, "midpoint.json"), filepath.Join(dir, "resumed.json")
	if err := Save(context.Background(), path, midpoint); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCheckpointHelperProcess$", "--", path, resumedPath, "9")
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper: %v\n%s", err, output)
	}
	state, err := Load(context.Background(), resumedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state.Training, full.Snapshot()) || state.NextSample != 9 {
		t.Fatal("selected-input snapshot or optimizer differs after subprocess resume")
	}
	resumed, err := learning.RestoreTrainer(state.Training)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(predictions(t, resumed, 1003, 5), predictions(t, full, 1003, 5)) {
		t.Fatal("selected-input predictions differ after subprocess resume")
	}
}
