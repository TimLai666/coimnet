package checkpoint_test

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/internal/delayedfixture"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

const (
	plasticChemicalHelperEnv = "COIMNET_PLASTIC_CHEMICAL_RESUME_HELPER"
	// plasticChemicalSeed fixes both halves of the reference run: the core
	// weight the fixture starts from and the delayed-pulse stream the
	// episodes are drawn from.
	plasticChemicalSeed     uint64 = 11
	plasticChemicalEpisodes        = 8
	plasticChemicalBreak           = 4
)

// episodeCursor is the sidecar the caller keeps beside a checkpoint: the index
// of the next episode of the training stream. The snapshot holds the model,
// not the position in the data, so an exact resume needs both.
type episodeCursor struct {
	NextEpisode int `json:"next_episode"`
}

// TestPlasticChemicalIndividualSubprocessResumeMatchesUninterrupted runs the
// same eight episodes twice: once straight through, and once broken after four
// with the second half replayed in a new process from the checkpoint and its
// episode-cursor sidecar. Both halves have to agree on every persistent part
// at once, and the wrong-cursor subtest shows the sidecar is part of the
// resume rather than bookkeeping beside it.
func TestPlasticChemicalIndividualSubprocessResumeMatchesUninterrupted(t *testing.T) {
	if os.Getenv(plasticChemicalHelperEnv) == "1" {
		t.Skip("helper is tested separately")
	}
	ctx := context.Background()
	dir := t.TempDir()

	uninterrupted := newPlasticChemicalIndividual(t)
	wantOutputs, err := runPlasticChemicalEpisodes(ctx, uninterrupted, 0, plasticChemicalEpisodes)
	if err != nil {
		t.Fatal(err)
	}
	want := uninterrupted.Snapshot()

	interrupted := newPlasticChemicalIndividual(t)
	if _, err := runPlasticChemicalEpisodes(ctx, interrupted, 0, plasticChemicalBreak); err != nil {
		t.Fatal(err)
	}
	midPath := filepath.Join(dir, "mid.json")
	if err := checkpoint.SaveIndividual(ctx, midPath, interrupted.Snapshot()); err != nil {
		t.Fatal(err)
	}
	cursorPath := filepath.Join(dir, "mid.cursor.json")
	writeEpisodeCursor(t, cursorPath, plasticChemicalBreak)

	finalPath := filepath.Join(dir, "final.json")
	outputPath := filepath.Join(dir, "outputs.json")
	runPlasticChemicalHelper(t, midPath, cursorPath, finalPath, outputPath)

	resumed, err := checkpoint.LoadIndividual(ctx, finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed, want) {
		t.Fatal("resumed individual state differs from the uninterrupted state")
	}
	assertOutputsIdentical(t, readEpisodeOutputs(t, outputPath), wantOutputs[plasticChemicalBreak:])

	if resumed.Plastic == nil {
		t.Fatal("resumed snapshot lost the local plasticity layer")
	}
	if resumed.Chemical == nil {
		t.Fatal("resumed snapshot lost the chemical modulation layer")
	}
	if !anyNonZero(resumed.Plastic.State) {
		t.Fatal("fast weight state is all zero, so the resume proves nothing about it")
	}
	if resumed.Optimizer.Updates != plasticChemicalEpisodes {
		t.Fatalf("optimizer updates = %d, want %d", resumed.Optimizer.Updates, plasticChemicalEpisodes)
	}

	t.Run("WrongCursorReplaysAnEpisode", func(t *testing.T) {
		wrongCursorPath := filepath.Join(dir, "wrong.cursor.json")
		writeEpisodeCursor(t, wrongCursorPath, plasticChemicalBreak-1)
		wrongFinalPath := filepath.Join(dir, "wrong-final.json")
		wrongOutputPath := filepath.Join(dir, "wrong-outputs.json")
		runPlasticChemicalHelper(t, midPath, wrongCursorPath, wrongFinalPath, wrongOutputPath)

		replayed, err := checkpoint.LoadIndividual(ctx, wrongFinalPath)
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(replayed, want) {
			t.Fatal("resuming from the wrong episode still reproduced the uninterrupted state, so the data cursor is not part of the resume")
		}
	})
}

func TestPlasticChemicalIndividualHelperProcess(t *testing.T) {
	if os.Getenv(plasticChemicalHelperEnv) != "1" {
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
	midPath, cursorPath := args[separator+1], args[separator+2]
	finalPath, outputPath := args[separator+3], args[separator+4]

	ctx := context.Background()
	loaded, err := checkpoint.LoadIndividual(ctx, midPath)
	if err != nil {
		t.Fatal(err)
	}
	individual, err := learning.RestoreIndividual(loaded)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := runPlasticChemicalEpisodes(ctx, individual, readEpisodeCursor(t, cursorPath), plasticChemicalEpisodes)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.SaveIndividual(ctx, finalPath, individual.Snapshot()); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(outputs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// newPlasticChemicalIndividual is the three-neuron delayed-pulse chain of the
// fixed_decay ablation group carrying every part an exact resume has to
// restore at once: trainable core weights with their optimizer state, the
// chemical layer, and the fast weights the layer gates through receptor 0.
// The gate is what makes the fast weight move under an unmodulated Advance;
// without it the eligibility trace still runs but the plastic term stays zero,
// so a resume would prove nothing about fast weights.
func newPlasticChemicalIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2},
			DT: 1, Activation: "tanh",
		},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2},
	}
	params := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.15 + float64(mixSeed(plasticChemicalSeed)%100)/1000, .2, .2},
			Bias:    []float64{0, 0, 0},
			LogTau:  []float64{math.Log(2), math.Log(2), math.Log(2)},
		},
		Encoder: []float64{1, 0, 0},
		Readout: []float64{1},
	}
	options := learning.DefaultOptions()
	options.LearningRate = .02
	options.Trainable = learning.Trainable{Weights: true}
	individual, err := learning.NewIndividual(config, params, options, make([]float64, config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	if err := individual.EnableChemistry(plasticChemicalDeclaration()); err != nil {
		t.Fatal(err)
	}
	receptor := 0
	rule := plasticity.Rule{
		Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .01,
		GateReceptor: &receptor, GateScale: 1,
	}
	if err := individual.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{1}}); err != nil {
		t.Fatal(err)
	}
	return individual
}

// plasticChemicalDeclaration is the continual-control chemistry declaration:
// one region holding all three nodes, one channel, an external timeline that
// releases 1 on row 1 of every advance and one hypothesized receptor on node 2.
func plasticChemicalDeclaration() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{2}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0, 0}},
	}
}

// runPlasticChemicalEpisodes walks episodes [from, to) of the delayed-pulse
// stream: the unmodulated advance over the whole episode first, then the
// isolated gradient update of the same episode. It returns the last output row
// of every episode it ran.
func runPlasticChemicalEpisodes(ctx context.Context, individual *learning.Individual, from, to int) ([][]float64, error) {
	outputs := make([][]float64, 0, to-from)
	for e := from; e < to; e++ {
		ep := delayedfixture.DelayedEpisode(plasticChemicalSeed, uint64(e))
		out, err := individual.Advance(ctx, ep.Input)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, append([]float64(nil), out[len(out)-1]...))
		if _, err := individual.TrainEpisode(ctx, ep.Input, ep.Target); err != nil {
			return nil, err
		}
	}
	return outputs, nil
}

func runPlasticChemicalHelper(t *testing.T, paths ...string) {
	t.Helper()
	args := append([]string{"-test.run=^TestPlasticChemicalIndividualHelperProcess$", "--"}, paths...)
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), plasticChemicalHelperEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("plastic chemical helper failed: %v\n%s", err, output)
	}
}

func writeEpisodeCursor(t *testing.T, path string, next int) {
	t.Helper()
	data, err := json.Marshal(episodeCursor{NextEpisode: next})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readEpisodeCursor(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cursor episodeCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		t.Fatal(err)
	}
	return cursor.NextEpisode
}

func readEpisodeOutputs(t *testing.T, path string) [][]float64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var outputs [][]float64
	if err := json.Unmarshal(data, &outputs); err != nil {
		t.Fatal(err)
	}
	return outputs
}

// assertOutputsIdentical compares bit patterns, because a resumed run that is
// merely close to the uninterrupted one is not an exact resume.
func assertOutputsIdentical(t *testing.T, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("resumed %d episode outputs, want %d", len(got), len(want))
	}
	for e := range want {
		if len(got[e]) != len(want[e]) {
			t.Fatalf("episode %d output width = %d, want %d", plasticChemicalBreak+e, len(got[e]), len(want[e]))
		}
		for i := range want[e] {
			if math.Float64bits(got[e][i]) != math.Float64bits(want[e][i]) {
				t.Fatalf("episode %d output %d = %v, want %v", plasticChemicalBreak+e, i, got[e][i], want[e][i])
			}
		}
	}
}

func anyNonZero(state plasticity.State) bool {
	for _, values := range [][]float64{state.Eligibility, state.Plastic, state.PreTrace, state.PostTrace} {
		for _, v := range values {
			if v != 0 {
				return true
			}
		}
	}
	return false
}

// mixSeed restates the counter-based SplitMix64 finalizer the delayed-pulse
// fixtures place their first core weight with. The experiment package keeps it
// unexported, so the rule is repeated here rather than its resulting constant.
func mixSeed(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
