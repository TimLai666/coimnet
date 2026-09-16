package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
	"github.com/TimLai666/coimnet/signal"
)

const (
	individualHelperEnv    = "COIMNET_INDIVIDUAL_CHECKPOINT_HELPER"
	lifIndividualHelperEnv = "COIMNET_LIF_INDIVIDUAL_CHECKPOINT_HELPER"
)

func TestSaveLoadIndividualRoundTripAndAliasIsolation(t *testing.T) {
	individual := newCheckpointIndividual(t, false)
	input := [][]float64{{.7}, {0}, {.2}}
	if _, err := individual.Advance(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	want := individual.Snapshot()
	path := filepath.Join(t.TempDir(), "individual.json")
	if err := SaveIndividual(context.Background(), path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("round trip changed individual snapshot")
	}
	got.Parameters.Core.Weights[0] = 88
	got.Neural.Continuous.History[0][0] = 77
	again, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatal("LoadIndividual returned buffers aliased with a later caller mutation")
	}
	restored, err := learning.RestoreIndividual(again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), again) {
		t.Fatal("loaded snapshot changed during restore")
	}
}

func TestSaveLoadIndividualSupportsZeroEdgeModel(t *testing.T) {
	individual := newCheckpointIndividual(t, true)
	path := filepath.Join(t.TempDir(), "zero-edge.json")
	if err := SaveIndividual(context.Background(), path, individual.Snapshot()); err != nil {
		t.Fatal(err)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, individual.Snapshot()) {
		t.Fatal("zero-edge round trip changed runtime snapshot")
	}
	if _, err := learning.RestoreIndividual(got); err != nil {
		t.Fatal(err)
	}
}

func TestLoadIndividualRejectsOldSchemaAndMalformedDocuments(t *testing.T) {
	individual := newCheckpointIndividual(t, false)
	payload := mustIndividualJSON(t, individual.Snapshot())
	valid := envelopeJSON(IndividualSchemaVersion, payload, checksumHex(payload))
	dir := t.TempDir()
	cases := map[string][]byte{}
	cases["old-envelope"] = envelopeJSON(SchemaVersion, payload, checksumHex(payload))
	cases["wrong-checksum"] = envelopeJSON(IndividualSchemaVersion, payload, strings.Repeat("0", sha256.Size*2))
	cases["truncated"] = valid[:len(valid)/2]

	mutate := func(name string, change func(map[string]json.RawMessage)) {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(payload, &object); err != nil {
			t.Fatal(err)
		}
		change(object)
		changed, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		cases[name] = envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed))
	}
	mutate("wrong-profile", func(object map[string]json.RawMessage) { object["profile"] = json.RawMessage(`"lif/v1"`) })
	mutate("wrong-hash", func(object map[string]json.RawMessage) { object["config_hash"] = json.RawMessage(`"bad"`) })
	mutate("missing-parameters", func(object map[string]json.RawMessage) { delete(object, "parameters") })
	mutate("missing-updates", func(object map[string]json.RawMessage) {
		var optimizer map[string]json.RawMessage
		if err := json.Unmarshal(object["optimizer"], &optimizer); err != nil {
			t.Fatal(err)
		}
		delete(optimizer, "updates")
		encoded, err := json.Marshal(optimizer)
		if err != nil {
			t.Fatal(err)
		}
		object["optimizer"] = encoded
	})
	mutate("null-number", func(object map[string]json.RawMessage) {
		var optimizer map[string]json.RawMessage
		if err := json.Unmarshal(object["optimizer"], &optimizer); err != nil {
			t.Fatal(err)
		}
		var options map[string]json.RawMessage
		if err := json.Unmarshal(optimizer["options"], &options); err != nil {
			t.Fatal(err)
		}
		options["learning_rate"] = json.RawMessage(`null`)
		encodedOptions, err := json.Marshal(options)
		if err != nil {
			t.Fatal(err)
		}
		optimizer["options"] = encodedOptions
		encoded, err := json.Marshal(optimizer)
		if err != nil {
			t.Fatal(err)
		}
		object["optimizer"] = encoded
	})
	mutate("wrong-shape", func(object map[string]json.RawMessage) {
		var parameters map[string]json.RawMessage
		if err := json.Unmarshal(object["parameters"], &parameters); err != nil {
			t.Fatal(err)
		}
		parameters["encoder"] = json.RawMessage(`[1]`)
		encoded, err := json.Marshal(parameters)
		if err != nil {
			t.Fatal(err)
		}
		object["parameters"] = encoded
	})
	mutate("unknown-field", func(object map[string]json.RawMessage) { object["extra"] = json.RawMessage(`true`) })
	mutate("duplicate-field", func(object map[string]json.RawMessage) {})

	for name, document := range cases {
		path := filepath.Join(dir, name+".json")
		if name == "duplicate-field" {
			document = duplicateIndividualPayloadDocument(payload)
		}
		writeRaw(t, path, document)
		if _, err := LoadIndividual(context.Background(), path); err == nil {
			t.Fatalf("accepted malformed individual checkpoint %s", name)
		}
	}
	unicodePayload := bytes.Replace(payload, []byte(`"`+learning.IndividualProfile+`"`), []byte(`"\ud800"`), 1)
	if bytes.Equal(unicodePayload, payload) {
		t.Fatal("fixture profile was not found")
	}
	unicodePath := filepath.Join(dir, "invalid-unicode.json")
	writeRaw(t, unicodePath, envelopeJSON(IndividualSchemaVersion, unicodePayload, checksumHex(unicodePayload)))
	if _, err := LoadIndividual(context.Background(), unicodePath); err == nil || !strings.Contains(err.Error(), "surrogate") {
		t.Fatalf("invalid Unicode error = %v", err)
	}
}

func TestSaveIndividualCancellationNoOverwriteAndConcurrentWinner(t *testing.T) {
	individual := newCheckpointIndividual(t, false)
	snapshot := individual.Snapshot()
	dir := t.TempDir()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledPath := filepath.Join(dir, "canceled.json")
	if err := SaveIndividual(canceled, canceledPath, snapshot); !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveIndividual cancellation error = %v", err)
	}
	assertNoTemps(t, dir, filepath.Base(canceledPath))

	path := filepath.Join(dir, "winner.json")
	if err := SaveIndividual(context.Background(), path, snapshot); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveIndividual(context.Background(), path, snapshot); err == nil {
		t.Fatal("SaveIndividual overwrote an existing checkpoint")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("failed overwrite changed original checkpoint")
	}

	concurrentPath := filepath.Join(dir, "concurrent.json")
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- SaveIndividual(context.Background(), concurrentPath, snapshot)
		}()
	}
	wg.Wait()
	close(results)
	var successes int
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent SaveIndividual calls had %d winners", successes)
	}
	if _, err := LoadIndividual(context.Background(), concurrentPath); err != nil {
		t.Fatalf("concurrent winner is not loadable: %v", err)
	}
	assertNoTemps(t, dir, filepath.Base(concurrentPath))
}

func TestIndividualCheckpointSubprocessResume(t *testing.T) {
	if os.Getenv(individualHelperEnv) == "1" {
		t.Skip("helper is tested separately")
	}
	dir := t.TempDir()
	midPath := filepath.Join(dir, "mid.json")
	finalPath := filepath.Join(dir, "final.json")
	full := newCheckpointIndividual(t, false)
	part := newCheckpointIndividual(t, false)
	first := [][]float64{{.7}, {0}}
	second := [][]float64{{.2}, {-.1}, {0}}
	if _, err := full.Advance(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	fullSuffix, err := full.Advance(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Advance(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := SaveIndividual(context.Background(), midPath, part.Snapshot()); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "outputs.json")
	cmd := exec.Command(os.Args[0], "-test.run=^TestIndividualCheckpointHelperProcess$", "--", midPath, finalPath, outputPath)
	cmd.Env = append(os.Environ(), individualHelperEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("individual helper failed: %v\n%s", err, output)
	}
	resumed, err := LoadIndividual(context.Background(), finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed, full.Snapshot()) {
		t.Fatal("new-process persistent state differs from uninterrupted state")
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var resumedSuffix [][]float64
	if err := json.Unmarshal(data, &resumedSuffix); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumedSuffix, fullSuffix) {
		t.Fatalf("new-process outputs differ: got %v want %v", resumedSuffix, fullSuffix)
	}
}

func TestIndividualCheckpointHelperProcess(t *testing.T) {
	if os.Getenv(individualHelperEnv) != "1" {
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
	if separator < 0 || len(args)-separator != 4 {
		t.Fatalf("helper arguments = %#v", args)
	}
	loaded, err := LoadIndividual(context.Background(), args[separator+1])
	if err != nil {
		t.Fatal(err)
	}
	individual, err := learning.RestoreIndividual(loaded)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := individual.Advance(context.Background(), [][]float64{{.2}, {-.1}, {0}})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveIndividual(context.Background(), args[separator+2], individual.Snapshot()); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(outputs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(args[separator+3], data, 0600); err != nil {
		t.Fatal(err)
	}
}

func newCheckpointIndividual(t *testing.T, zeroEdges bool) *learning.Individual {
	t.Helper()
	c := learning.Config{Dynamics: dynamics.Config{Nodes: 2, DT: .5, Activation: "tanh"}, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}}
	p := learning.Parameters{Core: dynamics.Parameters{Bias: []float64{.1, -.1}, LogTau: []float64{0, 0}}, Encoder: []float64{.5, .2}, Readout: []float64{.8}}
	if zeroEdges {
		p.Core.Weights = nil
	} else {
		c.Dynamics.Sources = []int{0}
		c.Dynamics.Targets = []int{1}
		c.Dynamics.Delays = []int{1}
		p.Core.Weights = []float64{.25}
	}
	individual, err := learning.NewIndividual(c, p, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	return individual
}

func mustIndividualJSON(t *testing.T, value learning.IndividualSnapshot) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func duplicateIndividualPayloadDocument(payload []byte) []byte {
	sum := sha256.Sum256(payload)
	return []byte(`{"schema_version":"` + IndividualSchemaVersion + `","schema_version":"` + IndividualSchemaVersion + `","payload":` + string(payload) + `,"checksum":"` + hex.EncodeToString(sum[:]) + `"}`)
}

// newCheckpointLIFIndividual is a three neuron spiking fixture with one delayed
// edge and the slow stabiliser on, so its persistent state carries every part
// the union has to survive: voltage, synaptic history, adaptation, refractory
// counters, rate estimate and threshold offset.
func newCheckpointLIFIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	core := dynamics.LIFConfig{
		Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2}, Delays: []int{0, 1, 0},
		DT: 1, TauSyn: 1, ThetaMin: .05, ThetaMax: 1, VReset: -.5, RefractorySteps: 1,
		Adaptation:  dynamics.LIFAdaptation{Enabled: true, TauAdapt: 2, Beta: .3},
		Homeostasis: &dynamics.LIFHomeostasis{Enabled: true, TauRate: 2, TargetRate: .3, Eta: 1.5, HMax: .4},
		Surrogate:   dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	c := learning.Config{LIF: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
	p := learning.Parameters{
		Core:     dynamics.Parameters{Weights: []float64{.65, .25, .7}, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}},
		ThetaRaw: []float64{-1, -1, -1},
		Encoder:  []float64{1, 0, 0},
		Readout:  []float64{1},
	}
	individual, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, core.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	return individual
}

func TestSaveLoadLIFIndividualRoundTrip(t *testing.T) {
	individual := newCheckpointLIFIndividual(t)
	if _, err := individual.Advance(context.Background(), [][]float64{{1}, {0}, {1}, {0}}); err != nil {
		t.Fatal(err)
	}
	want := individual.Snapshot()
	if want.Profile != learning.IndividualProfileLIF {
		t.Fatalf("profile = %q", want.Profile)
	}
	path := filepath.Join(t.TempDir(), "lif-individual.json")
	if err := SaveIndividual(context.Background(), path, want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"neural":{"core":"lif","lif":{`)) {
		t.Fatalf("saved document does not carry the declared union: %s", raw)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("round trip changed the spiking individual snapshot")
	}
	if got.Neural.LIF == nil || len(got.Neural.LIF.Homeostasis) != 3 || len(got.Neural.LIF.Rate) != 3 {
		t.Fatalf("round trip lost the slow stabiliser state: %+v", got.Neural.LIF)
	}
	got.Neural.LIF.Voltage[0] = 88
	again, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatal("LoadIndividual returned buffers aliased with a later caller mutation")
	}
	restored, err := learning.RestoreIndividual(again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), again) {
		t.Fatal("loaded spiking snapshot changed during restore")
	}
}

// TestLoadIndividualReadsPreUnionContinuousCheckpoint reads a committed file
// written by the released code before IndividualSnapshot.Neural became a union,
// where "neural" is the continuous dynamics.State itself.
func TestLoadIndividualReadsPreUnionContinuousCheckpoint(t *testing.T) {
	const path = "testdata/continuous-individual-pre-union.json"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"neural":{"schema_version"`)) || bytes.Contains(raw, []byte(`"neural":{"core"`)) {
		t.Fatal("the fixture is not a pre-union document")
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatalf("a pre-union continuous checkpoint must stay readable: %v", err)
	}
	if got.Profile != learning.IndividualProfile {
		t.Fatalf("profile = %q", got.Profile)
	}
	if got.Neural.Core != learning.NeuralCoreContinuous || got.Neural.Continuous == nil || got.Neural.LIF != nil {
		t.Fatalf("pre-union document did not become a continuous union: %+v", got.Neural)
	}
	// The fixture was produced by three Advance steps of the same model the
	// other tests build, so a fresh individual must reach the same state.
	fresh := newCheckpointIndividual(t, false)
	if _, err := fresh.Advance(context.Background(), [][]float64{{.7}, {0}, {.2}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, fresh.Snapshot()) {
		t.Fatal("the pre-union document did not restore the recorded trajectory")
	}
	// Resaving it publishes the union, and that file reads back identically.
	path2 := filepath.Join(t.TempDir(), "upgraded.json")
	if err := SaveIndividual(context.Background(), path2, got); err != nil {
		t.Fatal(err)
	}
	upgraded, err := LoadIndividual(context.Background(), path2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(upgraded, got) {
		t.Fatal("republishing a pre-union checkpoint changed it")
	}
}

// TestLoadIndividualRejectsInconsistentNeuralUnion covers the union rules a
// strict decoder alone cannot enforce.
func TestLoadIndividualRejectsInconsistentNeuralUnion(t *testing.T) {
	lifPayload := mustIndividualJSON(t, newCheckpointLIFIndividual(t).Snapshot())
	continuousPayload := mustIndividualJSON(t, newCheckpointIndividual(t, false).Snapshot())
	// A model whose longest delay is zero keeps exactly one history row at every
	// step, so an omitted step count would decode as a plausible step-0 state
	// instead of being rejected. That is the defect only the presence check
	// catches; every other case below is also refused further down.
	undelayedConfig := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: .5, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
	undelayed, err := learning.NewIndividual(undelayedConfig, learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.25}, Bias: []float64{.1, -.1}, LogTau: []float64{0, 0}},
		Encoder: []float64{.5, .2},
		Readout: []float64{.8},
	}, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := undelayed.Advance(context.Background(), [][]float64{{.4}, {0}}); err != nil {
		t.Fatal(err)
	}
	undelayedPayload := mustIndividualJSON(t, undelayed.Snapshot())
	preUnion, err := os.ReadFile("testdata/continuous-individual-pre-union.json")
	if err != nil {
		t.Fatal(err)
	}

	var preUnionEnvelope struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(preUnion, &preUnionEnvelope); err != nil {
		t.Fatal(err)
	}
	rewrite := func(payload []byte, change func(map[string]json.RawMessage)) []byte {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(payload, &object); err != nil {
			t.Fatal(err)
		}
		change(object)
		changed, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		return envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed))
	}
	cases := map[string][]byte{
		"lif neural without a lif configuration": rewrite(lifPayload, func(object map[string]json.RawMessage) {
			var config map[string]json.RawMessage
			if err := json.Unmarshal(object["config"], &config); err != nil {
				t.Fatal(err)
			}
			delete(config, "lif")
			encoded, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			object["config"] = encoded
		}),
		"continuous core against a lif configuration": rewrite(lifPayload, func(object map[string]json.RawMessage) {
			object["neural"] = json.RawMessage(`{"core":"continuous","continuous":{"schema_version":"` + dynamics.ContinuousStateVersion + `","config_hash":"x","steps":0,"voltage":[0,0,0],"history":[[0,0,0]]}}`)
		}),
		"unknown core": rewrite(continuousPayload, func(object map[string]json.RawMessage) {
			var neural map[string]json.RawMessage
			if err := json.Unmarshal(object["neural"], &neural); err != nil {
				t.Fatal(err)
			}
			neural["core"] = json.RawMessage(`"spiking"`)
			encoded, err := json.Marshal(neural)
			if err != nil {
				t.Fatal(err)
			}
			object["neural"] = encoded
		}),
		"both halves present": rewrite(continuousPayload, func(object map[string]json.RawMessage) {
			var neural map[string]json.RawMessage
			if err := json.Unmarshal(object["neural"], &neural); err != nil {
				t.Fatal(err)
			}
			neural["lif"] = json.RawMessage(`{"schema_version":"` + dynamics.LIFStateVersion + `","config_hash":"x","steps":0,"voltage":[0],"history":[[0]],"adaptation":[0],"refractory":[0]}`)
			encoded, err := json.Marshal(neural)
			if err != nil {
				t.Fatal(err)
			}
			object["neural"] = encoded
		}),
		"continuous state without a step count": rewrite(undelayedPayload, func(object map[string]json.RawMessage) {
			var neural, state map[string]json.RawMessage
			if err := json.Unmarshal(object["neural"], &neural); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(neural["continuous"], &state); err != nil {
				t.Fatal(err)
			}
			delete(state, "steps")
			encodedState, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			neural["continuous"] = encodedState
			encoded, err := json.Marshal(neural)
			if err != nil {
				t.Fatal(err)
			}
			object["neural"] = encoded
		}),
		"missing declared half": rewrite(continuousPayload, func(object map[string]json.RawMessage) {
			object["neural"] = json.RawMessage(`{"core":"continuous"}`)
		}),
		"pre-union shape with a lif configuration": rewrite(lifPayload, func(object map[string]json.RawMessage) {
			object["neural"] = preUnionEnvelope.Payload
		}),
		"lif state missing its refractory counters": rewrite(lifPayload, func(object map[string]json.RawMessage) {
			var neural, state map[string]json.RawMessage
			if err := json.Unmarshal(object["neural"], &neural); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(neural["lif"], &state); err != nil {
				t.Fatal(err)
			}
			delete(state, "refractory")
			encodedState, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			neural["lif"] = encodedState
			encoded, err := json.Marshal(neural)
			if err != nil {
				t.Fatal(err)
			}
			object["neural"] = encoded
		}),
		"lif configuration missing its edge sources": rewrite(lifPayload, func(object map[string]json.RawMessage) {
			var config, lif map[string]json.RawMessage
			if err := json.Unmarshal(object["config"], &config); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(config["lif"], &lif); err != nil {
				t.Fatal(err)
			}
			delete(lif, "sources")
			encodedLIF, err := json.Marshal(lif)
			if err != nil {
				t.Fatal(err)
			}
			config["lif"] = encodedLIF
			encoded, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			object["config"] = encoded
		}),
	}
	dir := t.TempDir()
	index := 0
	for name, document := range cases {
		index++
		path := filepath.Join(dir, fmt.Sprintf("case-%d.json", index))
		writeRaw(t, path, document)
		if _, err := LoadIndividual(context.Background(), path); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
}

func TestLIFIndividualCheckpointSubprocessResume(t *testing.T) {
	if os.Getenv(lifIndividualHelperEnv) == "1" {
		t.Skip("helper is tested separately")
	}
	dir := t.TempDir()
	midPath := filepath.Join(dir, "mid.json")
	finalPath := filepath.Join(dir, "final.json")
	outputPath := filepath.Join(dir, "outputs.json")
	full := newCheckpointLIFIndividual(t)
	part := newCheckpointLIFIndividual(t)
	first := [][]float64{{1}, {0}}
	second := [][]float64{{1}, {0}, {0}}
	if _, err := full.Advance(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	fullSuffix, err := full.Advance(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Advance(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := SaveIndividual(context.Background(), midPath, part.Snapshot()); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLIFIndividualCheckpointHelperProcess$", "--", midPath, finalPath, outputPath)
	cmd.Env = append(os.Environ(), lifIndividualHelperEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("LIF individual helper failed: %v\n%s", err, output)
	}
	resumed, err := LoadIndividual(context.Background(), finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed, full.Snapshot()) {
		t.Fatal("new-process spiking state differs from uninterrupted state")
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var resumedSuffix [][]float64
	if err := json.Unmarshal(data, &resumedSuffix); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumedSuffix, fullSuffix) {
		t.Fatalf("new-process outputs differ: got %v want %v", resumedSuffix, fullSuffix)
	}
	var fired bool
	for _, row := range fullSuffix {
		if row[0] != 0 {
			fired = true
		}
	}
	if !fired {
		t.Fatal("the resumed window produced no activity, the comparison would be vacuous")
	}
}

func TestLIFIndividualCheckpointHelperProcess(t *testing.T) {
	if os.Getenv(lifIndividualHelperEnv) != "1" {
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
	if separator < 0 || len(args)-separator != 4 {
		t.Fatalf("helper arguments = %#v", args)
	}
	loaded, err := LoadIndividual(context.Background(), args[separator+1])
	if err != nil {
		t.Fatal(err)
	}
	individual, err := learning.RestoreIndividual(loaded)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := individual.Advance(context.Background(), [][]float64{{1}, {0}, {0}})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveIndividual(context.Background(), args[separator+2], individual.Snapshot()); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(outputs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(args[separator+3], data, 0600); err != nil {
		t.Fatal(err)
	}
}

// newPlasticCheckpointIndividual is the spiking fixture with local plasticity
// enabled on two of its three edges and a few gated steps behind it, so the
// saved document carries a fast state that is not all zeros.
func newPlasticCheckpointIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	individual := newCheckpointLIFIndividual(t)
	rule := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625}
	if err := individual.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0, 2}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := individual.AdvanceGated(context.Background(), [][]float64{{1}, {0}, {1}, {0}}, []float64{1, 1, 1, 1}); err != nil {
		t.Fatal(err)
	}
	return individual
}

// savedIndividualPayload returns the payload SaveIndividual actually writes,
// which is the normalized document a loader has to accept.
func savedIndividualPayload(t *testing.T, s learning.IndividualSnapshot) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.json")
	if err := SaveIndividual(context.Background(), path, s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw testEnvelope
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw.Payload
}

// TestSaveLoadIndividualCarriesThePlasticPart is the optional-part contract:
// absent while the mechanism was never enabled, complete and bit-identical
// once it was.
func TestSaveLoadIndividualCarriesThePlasticPart(t *testing.T) {
	plain := newCheckpointLIFIndividual(t).Snapshot()
	if bytes.Contains(savedIndividualPayload(t, plain), []byte(`"plastic"`)) {
		t.Fatal("an individual that never enabled plasticity wrote a plastic part")
	}
	want := newPlasticCheckpointIndividual(t).Snapshot()
	if want.Plastic == nil {
		t.Fatal("the fixture did not enable plasticity")
	}
	payload := savedIndividualPayload(t, want)
	if !bytes.Contains(payload, []byte(`"plastic":{"config":{"rule":{"kind":"hebbian_rate"`)) {
		t.Fatalf("saved payload does not carry the declared rule: %s", payload)
	}
	if bytes.Contains(payload, []byte(`"pre_trace"`)) {
		t.Fatal("a rate rule wrote the pair traces")
	}
	path := filepath.Join(t.TempDir(), "plastic-individual.json")
	if err := SaveIndividual(context.Background(), path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the plastic part:\n got %+v\nwant %+v", got.Plastic, want.Plastic)
	}
	got.Plastic.State.Plastic[0] = 77
	again, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatal("LoadIndividual returned a plastic part aliased with a later caller mutation")
	}
	restored, err := learning.RestoreIndividual(again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), again) {
		t.Fatal("loaded plastic part changed during restore")
	}
	// A document that carries a plastic part is still an individual snapshot
	// and nothing else: the model package loader must keep refusing it.
	if _, err := LoadModelPackage(context.Background(), path); err == nil {
		t.Fatal("the model package loader accepted an individual checkpoint")
	}
}

// slowCheckpointConfig drives one hypothesized receptor on node 0 with a single
// timeline pulse, so two gated rows leave an occupancy well above any small
// threshold the consolidation gate can be declared with.
func slowCheckpointConfig() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: .5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "consol-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
}

// newSlowCheckpointIndividual is the continuous fixture with local plasticity on
// its edge, the chemical layer and a consolidation layer that has already taken
// one of its two budget slots during one completed training episode, so the
// saved document carries a non-empty slow layer and the episode clock its
// per-episode write rule reads.
func newSlowCheckpointIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	individual := newCheckpointIndividual(t, false)
	rule := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625}
	if err := individual.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	if err := individual.EnableChemistry(slowCheckpointConfig()); err != nil {
		t.Fatal(err)
	}
	if err := individual.EnableConsolidation(2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := individual.AdvanceGated(context.Background(), [][]float64{{1}, {1}}, []float64{1, 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := individual.TrainEpisode(context.Background(), [][]float64{{1}, {0}}, []float64{.4}); err != nil {
		t.Fatal(err)
	}
	if _, err := individual.Consolidate(context.Background(), learning.ConsolidationTrigger{
		Receptor: 0, Threshold: .01, Rate: .5, Retain: .25,
	}); err != nil {
		t.Fatal(err)
	}
	return individual
}

// TestSaveLoadIndividualCarriesConsolidationSlowAndEpisodes is the
// optional-part contract of the consolidation layer and the episode clock:
// absent while never enabled or never started, complete and bit-identical once
// it was.
func TestSaveLoadIndividualCarriesConsolidationSlowAndEpisodes(t *testing.T) {
	plain := newPlasticCheckpointIndividual(t).Snapshot()
	if bytes.Contains(savedIndividualPayload(t, plain), []byte(`"slow"`)) {
		t.Fatal("a plastic individual that never enabled consolidation wrote a slow layer")
	}
	want := newSlowCheckpointIndividual(t).Snapshot()
	if want.Plastic == nil || want.Plastic.Slow == nil {
		t.Fatal("the fixture did not enable consolidation")
	}
	if want.Plastic.Slow.Values[0] == 0 {
		t.Fatal("the fixture produced an empty write, the payload presence check would be vacuous")
	}
	if want.Optimizer.Episodes != 1 {
		t.Fatalf("episodes = %d, want 1", want.Optimizer.Episodes)
	}
	payload := savedIndividualPayload(t, want)
	if !bytes.Contains(payload, []byte(`"slow":{"values":[`)) {
		t.Fatalf("saved payload does not carry the slow layer: %s", payload)
	}
	if !bytes.Contains(payload, []byte(`"episodes":1`)) {
		t.Fatalf("saved payload does not carry the episode clock: %s", payload)
	}
	if bytes.Contains(payload, []byte(`null`)) {
		t.Fatalf("saved payload carries a null: %s", payload)
	}
	path := filepath.Join(t.TempDir(), "slow-individual.json")
	if err := SaveIndividual(context.Background(), path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the consolidation part:\n got %+v\nwant %+v", got.Plastic.Slow, want.Plastic.Slow)
	}
	got.Plastic.Slow.Values[0] = 77
	again, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatal("LoadIndividual returned a slow layer aliased with a later caller mutation")
	}
	restored, err := learning.RestoreIndividual(again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), again) {
		t.Fatal("loaded consolidation part changed during restore")
	}
}

// TestLoadIndividualRejectsMalformedPlasticParts keeps a hand-edited plastic
// block from becoming a silently different mechanism. Every case replaces the
// whole "plastic" value, so the rejected shapes are written out in full.
func TestLoadIndividualRejectsMalformedPlasticParts(t *testing.T) {
	want := newPlasticCheckpointIndividual(t).Snapshot()
	payload := savedIndividualPayload(t, want)
	const (
		rule  = `{"kind":"hebbian_rate","decay_e":0.5,"decay_p":0.5,"plastic_max":8,"w_min":0.0625}`
		state = `{"eligibility":[0.5,0.25],"plastic":[0.5,0.25]}`
	)
	for name, part := range map[string]string{
		"null part":            `null`,
		"missing config":       `{"state":` + state + `}`,
		"missing state":        `{"config":{"rule":` + rule + `,"edges":[0,2]}}`,
		"missing edges":        `{"config":{"rule":` + rule + `},"state":` + state + `}`,
		"missing decay_e":      `{"config":{"rule":{"kind":"hebbian_rate","decay_p":0.5,"plastic_max":8,"w_min":0.0625},"edges":[0,2]},"state":` + state + `}`,
		"missing decay_p":      `{"config":{"rule":{"kind":"hebbian_rate","decay_e":0.5,"plastic_max":8,"w_min":0.0625},"edges":[0,2]},"state":` + state + `}`,
		"missing w_min":        `{"config":{"rule":{"kind":"hebbian_rate","decay_e":0.5,"decay_p":0.5,"plastic_max":8},"edges":[0,2]},"state":` + state + `}`,
		"missing kind":         `{"config":{"rule":{"decay_e":0.5,"decay_p":0.5,"plastic_max":8,"w_min":0.0625},"edges":[0,2]},"state":` + state + `}`,
		"unknown kind":         `{"config":{"rule":{"kind":"oja","decay_e":0.5,"decay_p":0.5,"plastic_max":8,"w_min":0.0625},"edges":[0,2]},"state":` + state + `}`,
		"null decay_e":         `{"config":{"rule":{"kind":"hebbian_rate","decay_e":null,"decay_p":0.5,"plastic_max":8,"w_min":0.0625},"edges":[0,2]},"state":` + state + `}`,
		"missing eligibility":  `{"config":{"rule":` + rule + `,"edges":[0,2]},"state":{"plastic":[0.5,0.25]}}`,
		"null eligibility":     `{"config":{"rule":` + rule + `,"edges":[0,2]},"state":{"eligibility":null,"plastic":[0.5,0.25]}}`,
		"null inside plastic":  `{"config":{"rule":` + rule + `,"edges":[0,2]},"state":{"eligibility":[0.5,0.25],"plastic":[null,0.25]}}`,
		"wrong state length":   `{"config":{"rule":` + rule + `,"edges":[0,2]},"state":{"eligibility":[0.5,0.25,0],"plastic":[0.5,0.25,0]}}`,
		"pair trace on a rate": `{"config":{"rule":` + rule + `,"edges":[0,2]},"state":{"eligibility":[0.5,0.25],"plastic":[0.5,0.25],"pre_trace":[0,0]}}`,
		"edge out of range":    `{"config":{"rule":` + rule + `,"edges":[0,9]},"state":` + state + `}`,
		"descending edges":     `{"config":{"rule":` + rule + `,"edges":[2,0]},"state":` + state + `}`,
		"empty edges":          `{"config":{"rule":` + rule + `,"edges":[]},"state":{"eligibility":[],"plastic":[]}}`,
		"unknown rule field":   `{"config":{"rule":{"kind":"hebbian_rate","decay_e":0.5,"decay_p":0.5,"plastic_max":8,"w_min":0.0625,"decay_w":0.5},"edges":[0,2]},"state":` + state + `}`,
	} {
		t.Run(name, func(t *testing.T) {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(payload, &object); err != nil {
				t.Fatal(err)
			}
			object["plastic"] = json.RawMessage(part)
			changed, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "bad.json")
			writeRaw(t, path, envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed)))
			if _, err := LoadIndividual(context.Background(), path); err == nil {
				t.Fatal("accepted a malformed plastic part")
			}
		})
	}
}

// newAccumulatingCheckpointIndividual is a two neuron fixture whose optimizer
// averages three gradients per update and has taken exactly one of them, so its
// snapshot sits in the middle of an accumulation window.
func newAccumulatingCheckpointIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	c := learning.Config{Dynamics: dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1}, DT: .5, Activation: "tanh"}, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{.25}, Bias: []float64{.1, -.1}, LogTau: []float64{0, 0}}, Encoder: []float64{.5, .2}, Readout: []float64{.8}}
	o := learning.DefaultOptions()
	o.AccumulateSteps = 3
	individual, err := learning.NewIndividual(c, p, o, []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	result, err := individual.TrainEpisode(context.Background(), [][]float64{{.7}, {0}, {0}}, []float64{.4})
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied || result.Accumulated != 1 {
		t.Fatalf("the fixture step applied %v with %d accumulated", result.Applied, result.Accumulated)
	}
	return individual
}

// TestSaveLoadIndividualCarriesTheAccumulatorPart is the optional-part contract
// of the open accumulation window: absent while no window is open, complete and
// bit-identical once one is.
func TestSaveLoadIndividualCarriesTheAccumulatorPart(t *testing.T) {
	plain := newCheckpointIndividual(t, false).Snapshot()
	if bytes.Contains(savedIndividualPayload(t, plain), []byte(`"accumulator"`)) {
		t.Fatal("an individual with no open window wrote an accumulator part")
	}
	want := newAccumulatingCheckpointIndividual(t).Snapshot()
	if want.Optimizer.Accumulator == nil {
		t.Fatal("the fixture did not open an accumulation window")
	}
	payload := savedIndividualPayload(t, want)
	if !bytes.Contains(payload, []byte(`"accumulator":{"sum":[`)) {
		t.Fatalf("saved payload does not carry the window: %s", payload)
	}
	path := filepath.Join(t.TempDir(), "accumulating-individual.json")
	if err := SaveIndividual(context.Background(), path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the window:\n got %+v\nwant %+v", got.Optimizer.Accumulator, want.Optimizer.Accumulator)
	}
	got.Optimizer.Accumulator.Sum[0] = 77
	again, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatal("LoadIndividual returned an accumulator aliased with a later caller mutation")
	}
	restored, err := learning.RestoreIndividual(again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), again) {
		t.Fatal("the loaded window changed during restore")
	}
}

// TestLoadIndividualReadsAnAbsentAccumulatorAsNoWindow keeps documents written
// before this part existed readable: no key is the declared "no window is
// open". An explicit null is not a second spelling of it, because this document
// format permits no null anywhere; the plastic part is refused the same way.
func TestLoadIndividualReadsAnAbsentAccumulatorAsNoWindow(t *testing.T) {
	want := newCheckpointIndividual(t, false).Snapshot()
	payload := savedIndividualPayload(t, want)
	if bytes.Contains(payload, []byte(`"accumulator"`)) {
		t.Fatalf("the fixture already carries a window: %s", payload)
	}
	path := filepath.Join(t.TempDir(), "no-window.json")
	writeRaw(t, path, envelopeJSON(IndividualSchemaVersion, payload, checksumHex(payload)))
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Optimizer.Accumulator != nil {
		t.Fatalf("an absent accumulator loaded as an open window: %+v", got.Optimizer.Accumulator)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("a document without a window did not round trip")
	}
}

// TestLoadIndividualRejectsMalformedAccumulatorParts keeps a hand-edited window
// from decoding as a valid partial sum. Every case replaces the whole
// "accumulator" value, so the rejected shapes are written out in full.
func TestLoadIndividualRejectsMalformedAccumulatorParts(t *testing.T) {
	want := newAccumulatingCheckpointIndividual(t).Snapshot()
	payload := savedIndividualPayload(t, want)
	sum, err := json.Marshal(want.Optimizer.Accumulator.Sum)
	if err != nil {
		t.Fatal(err)
	}
	for name, part := range map[string]string{
		"null accumulator":        `null`,
		"missing sum":             `{"count":1}`,
		"missing count":           `{"sum":` + string(sum) + `}`,
		"empty object":            `{}`,
		"null sum":                `{"sum":null,"count":1}`,
		"null count":              `{"sum":` + string(sum) + `,"count":null}`,
		"null inside sum":         `{"sum":[null,0,0,0,0,0,0,0],"count":1}`,
		"short sum":               `{"sum":[0,0,0],"count":1}`,
		"count at the window":     `{"sum":` + string(sum) + `,"count":3}`,
		"negative count":          `{"sum":` + string(sum) + `,"count":-1}`,
		"gradient without count":  `{"sum":` + string(sum) + `,"count":0}`,
		"unknown accumulator key": `{"sum":` + string(sum) + `,"count":1,"mean":[0]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(payload, &object); err != nil {
				t.Fatal(err)
			}
			var optimizer map[string]json.RawMessage
			if err := json.Unmarshal(object["optimizer"], &optimizer); err != nil {
				t.Fatal(err)
			}
			optimizer["accumulator"] = json.RawMessage(part)
			encoded, err := json.Marshal(optimizer)
			if err != nil {
				t.Fatal(err)
			}
			object["optimizer"] = encoded
			changed, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "bad-window.json")
			writeRaw(t, path, envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed)))
			if _, err := LoadIndividual(context.Background(), path); err == nil {
				t.Fatal("accepted a malformed accumulation window")
			}
		})
	}
}

// newChemicalCheckpointIndividual is the spiking fixture with a chemical layer
// enabled: two regions, one channel fed by a declared timeline, one
// hypothesized and one unresponsive receptor, a threshold effect, a declared
// resource and one feedback still queued. A few steps have run, so the saved
// document carries a concentration that is not zero.
func newChemicalCheckpointIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	individual := newCheckpointLIFIndividual(t)
	if err := individual.EnableChemistry(chemicalCheckpointConfig()); err != nil {
		t.Fatal(err)
	}
	if err := individual.SetResource("energy", 1.25); err != nil {
		t.Fatal(err)
	}
	if err := individual.OfferFeedback(chemicalCheckpointFeedback(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := individual.Advance(context.Background(), [][]float64{{1}, {0}, {1}}); err != nil {
		t.Fatal(err)
	}
	return individual
}

func chemicalCheckpointConfig() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 2, Channels: 1, DT: 1, Tau: []float64{2}, Units: modulation.UnitNormalized},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 0, Channel: 0, Rate: 1.5}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{0, 1}, CellType: "fixture", Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: .5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
			{Cells: []int{2}, Signal: "octopamine", Channel: 0, Status: modulation.StatusUnresponsive,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
		}, Mix: modulation.MixSum},
		Effects: []modulation.Effect{{Kind: modulation.EffectThreshold, Receptor: 0, ThetaScale: .25, ThetaAbsMax: 1}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0, 1}},
	}
}

func chemicalCheckpointFeedback(t *testing.T) signal.Feedback {
	t.Helper()
	f, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(),
		ExperienceID:  "exp-chem",
		ActionID:      "act-chem",
		ProducedAt:    signal.Timestamp{Value: 1, Unit: signal.TimeUnitModelStep},
		AvailableAt:   signal.Timestamp{Value: 9, Unit: signal.TimeUnitModelStep},
		Source:        "teacher",
		Score:         .5,
		ModelVersion:  signal.CurrentSchemaVersion(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestSaveLoadIndividualCarriesTheChemicalPart is the optional-part contract of
// the chemical layer: absent while it was never enabled, complete and
// bit-identical once it was.
func TestSaveLoadIndividualCarriesTheChemicalPart(t *testing.T) {
	plain := newCheckpointLIFIndividual(t).Snapshot()
	if bytes.Contains(savedIndividualPayload(t, plain), []byte(`"chemical"`)) {
		t.Fatal("an individual that never enabled chemistry wrote a chemical part")
	}
	want := newChemicalCheckpointIndividual(t).Snapshot()
	if want.Chemical == nil {
		t.Fatal("the fixture did not enable chemistry")
	}
	payload := savedIndividualPayload(t, want)
	for _, key := range []string{`"chemical"`, `"engineering_kd"`, `"node_region"`, `"pending_feedback"`, `"resources"`, `"external_timeline"`} {
		if !bytes.Contains(payload, []byte(key)) {
			t.Fatalf("saved payload does not carry %s: %s", key, payload)
		}
	}
	if bytes.Contains(payload, []byte(`null`)) {
		t.Fatalf("saved payload carries a null: %s", payload)
	}
	path := filepath.Join(t.TempDir(), "chemical-individual.json")
	if err := SaveIndividual(context.Background(), path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the chemical part:\n got %+v\nwant %+v", got.Chemical, want.Chemical)
	}
	got.Chemical.State.Concentration[0][0] = 77
	again, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatal("LoadIndividual returned a chemical part aliased with a later caller mutation")
	}
	restored, err := learning.RestoreIndividual(again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), again) {
		t.Fatal("loaded chemical part changed during restore")
	}
	if _, err := LoadModelPackage(context.Background(), path); err == nil {
		t.Fatal("the model package loader accepted an individual checkpoint")
	}
}

// TestLoadIndividualRejectsMalformedChemicalParts keeps a hand-edited chemical
// block from decoding as a silently different declaration. Every case replaces
// the whole "chemical" value, so the rejected shapes are written out in full.
func TestLoadIndividualRejectsMalformedChemicalParts(t *testing.T) {
	want := newChemicalCheckpointIndividual(t).Snapshot()
	payload := savedIndividualPayload(t, want)
	var intact map[string]json.RawMessage
	if err := json.Unmarshal(payload, &intact); err != nil {
		t.Fatal(err)
	}
	var part map[string]json.RawMessage
	if err := json.Unmarshal(intact["chemical"], &part); err != nil {
		t.Fatal(err)
	}
	config, state := string(part["config"]), string(part["state"])
	resources, pending := string(part["resources"]), string(part["pending_feedback"])
	full := func(fields ...string) string { return "{" + strings.Join(fields, ",") + "}" }
	for name, block := range map[string]string{
		"null part":              `null`,
		"missing config":         full(`"state":`+state, `"resources":`+resources, `"pending_feedback":`+pending),
		"missing state":          full(`"config":`+config, `"resources":`+resources, `"pending_feedback":`+pending),
		"missing resources":      full(`"config":`+config, `"state":`+state, `"pending_feedback":`+pending),
		"missing pending":        full(`"config":`+config, `"state":`+state, `"resources":`+resources),
		"null state":             full(`"config":`+config, `"state":null`, `"resources":`+resources, `"pending_feedback":`+pending),
		"missing concentration":  full(`"config":`+config, `"state":{"steps":3}`, `"resources":`+resources, `"pending_feedback":`+pending),
		"missing steps":          full(`"config":`+config, `"state":{"concentration":[[0.5],[0.5]]}`, `"resources":`+resources, `"pending_feedback":`+pending),
		"one region":             full(`"config":`+config, `"state":{"concentration":[[0.5]],"steps":3}`, `"resources":`+resources, `"pending_feedback":`+pending),
		"negative concentration": full(`"config":`+config, `"state":{"concentration":[[-0.5],[0.5]],"steps":3}`, `"resources":`+resources, `"pending_feedback":`+pending),
		"null inside concentration": full(`"config":`+config, `"state":{"concentration":[[null],[0.5]],"steps":3}`,
			`"resources":`+resources, `"pending_feedback":`+pending),
		"config has no chemistry": full(`"config":{"sources":[],"receptors":{"records":[],"allow_assumed_coefficients":false},"effects":[],"regions":{"node_region":[0,0,1]}}`,
			`"state":`+state, `"resources":`+resources, `"pending_feedback":`+pending),
		"config has no regions": full(`"config":`+strings.Replace(config, `,"regions"`, `,"unused_regions"`, 1),
			`"state":`+state, `"resources":`+resources, `"pending_feedback":`+pending),
		"receptor without mapping_version": full(`"config":`+strings.Replace(config, `"mapping_version":"chem-fixture/v1"`, `"mapping_version_typo":"chem-fixture/v1"`, 1),
			`"state":`+state, `"resources":`+resources, `"pending_feedback":`+pending),
		"feedback without source": full(`"config":`+config, `"state":`+state, `"resources":`+resources,
			`"pending_feedback":[{"schema_version":{"major":1,"minor":0},"experience_id":"exp-chem","action_id":"act-chem","produced_at":{"value":1,"unit":"model_step"},"available_at":{"value":9,"unit":"model_step"},"score":0.5,"model_version":{"major":1,"minor":0}}]`),
		"feedback on another clock": full(`"config":`+config, `"state":`+state, `"resources":`+resources,
			`"pending_feedback":[{"schema_version":{"major":1,"minor":0},"experience_id":"exp-chem","action_id":"act-chem","produced_at":{"value":1,"unit":"ms"},"available_at":{"value":9,"unit":"ms"},"source":"teacher","score":0.5,"model_version":{"major":1,"minor":0}}]`),
		"resource is not finite": full(`"config":`+config, `"state":`+state, `"resources":{"energy":1e999}`, `"pending_feedback":`+pending),
	} {
		t.Run(name, func(t *testing.T) {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(payload, &object); err != nil {
				t.Fatal(err)
			}
			object["chemical"] = json.RawMessage(block)
			changed, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "bad.json")
			writeRaw(t, path, envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed)))
			if _, err := LoadIndividual(context.Background(), path); err == nil {
				t.Fatal("accepted a malformed chemical part")
			}
		})
	}
}

// TestLoadIndividualReadsAnAbsentChemicalPartAsDisabled keeps documents written
// before this part existed readable: no key is the declared "the layer was
// never enabled".
func TestLoadIndividualReadsAnAbsentChemicalPartAsDisabled(t *testing.T) {
	want := newCheckpointLIFIndividual(t).Snapshot()
	payload := savedIndividualPayload(t, want)
	path := filepath.Join(t.TempDir(), "no-chemical.json")
	writeRaw(t, path, envelopeJSON(IndividualSchemaVersion, payload, checksumHex(payload)))
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Chemical != nil {
		t.Fatalf("an absent chemical part loaded as %+v", got.Chemical)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("an absent chemical part changed the rest of the snapshot")
	}
}

func newCheckpointMixedIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	core := dynamics.MixedConfig{
		Nodes: 3, Sources: []int{0, 1}, Targets: []int{1, 2}, Delays: []int{0, 1},
		DT: 1, NodeRule: []uint8{0, 1, 0},
		Continuous: dynamics.ContinuousRule{Activation: "tanh"},
		LIF: dynamics.LIFRule{
			TauSyn: 1, ThetaMin: .05, ThetaMax: 1, VReset: -.5, RefractorySteps: 1,
			Adaptation:  dynamics.LIFAdaptation{Enabled: true, TauAdapt: 2, Beta: .3},
			Homeostasis: &dynamics.LIFHomeostasis{Enabled: true, TauRate: 2, TargetRate: .3, Eta: 1.5, HMax: .4},
			Surrogate:   dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
	}
	c := learning.Config{Mixed: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
	p := learning.Parameters{
		Core:     dynamics.Parameters{Weights: []float64{.65, .7}, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}},
		ThetaRaw: []float64{-1},
		Encoder:  []float64{1, 0, 0},
		Readout:  []float64{1},
	}
	individual, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, core.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	return individual
}

func TestSaveLoadMixedIndividualRoundTrip(t *testing.T) {
	individual := newCheckpointMixedIndividual(t)
	if _, err := individual.Advance(context.Background(), [][]float64{{1}, {0}, {1}, {0}}); err != nil {
		t.Fatal(err)
	}
	want := individual.Snapshot()
	if want.Profile != learning.IndividualProfileMixed {
		t.Fatalf("profile = %q", want.Profile)
	}
	path := filepath.Join(t.TempDir(), "mixed-individual.json")
	if err := SaveIndividual(context.Background(), path, want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"neural":{"core":"mixed","mixed":{`)) {
		t.Fatalf("saved document does not carry the declared union: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"lif_index":[1]`)) {
		t.Fatalf("saved document does not carry the theta index: %s", raw)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("round trip changed the mixed individual snapshot")
	}
	if got.Neural.Mixed == nil || got.Neural.Continuous != nil || got.Neural.LIF != nil {
		t.Fatalf("round trip changed the neural union: %+v", got.Neural)
	}
	if len(got.Neural.Mixed.Index.LIFNodes) != 1 || got.Neural.Mixed.Index.LIFNodes[0] != 1 {
		t.Fatalf("round trip lost the node map: %+v", got.Neural.Mixed.Index)
	}
	if len(got.Neural.Mixed.LIF.Homeostasis) != 1 || len(got.Neural.Mixed.LIF.Rate) != 1 {
		t.Fatalf("round trip lost the slow stabiliser state: %+v", got.Neural.Mixed.LIF)
	}
	restored, err := learning.RestoreIndividual(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), want) {
		t.Fatal("loaded mixed snapshot changed during restore")
	}
}

func TestLoadIndividualRejectsMixedNeuralUnionDefects(t *testing.T) {
	mixed := newCheckpointMixedIndividual(t)
	if _, err := mixed.Advance(context.Background(), [][]float64{{1}}); err != nil {
		t.Fatal(err)
	}
	mixedPayload := mustIndividualJSON(t, mixed.Snapshot())
	continuousPayload := mustIndividualJSON(t, newCheckpointIndividual(t, false).Snapshot())
	lifPayload := mustIndividualJSON(t, newCheckpointLIFIndividual(t).Snapshot())

	rewrite := func(payload []byte, change func(map[string]json.RawMessage)) []byte {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(payload, &object); err != nil {
			t.Fatal(err)
		}
		change(object)
		changed, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		return envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed))
	}
	editConfig := func(object map[string]json.RawMessage, change func(map[string]json.RawMessage)) {
		var config map[string]json.RawMessage
		if err := json.Unmarshal(object["config"], &config); err != nil {
			t.Fatal(err)
		}
		change(config)
		encoded, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		object["config"] = encoded
	}
	editNeural := func(object map[string]json.RawMessage, change func(map[string]json.RawMessage)) {
		var neural map[string]json.RawMessage
		if err := json.Unmarshal(object["neural"], &neural); err != nil {
			t.Fatal(err)
		}
		change(neural)
		encoded, err := json.Marshal(neural)
		if err != nil {
			t.Fatal(err)
		}
		object["neural"] = encoded
	}
	mixedConfigOf := func(payload []byte) json.RawMessage {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(payload, &object); err != nil {
			t.Fatal(err)
		}
		var config map[string]json.RawMessage
		if err := json.Unmarshal(object["config"], &config); err != nil {
			t.Fatal(err)
		}
		return config["mixed"]
	}

	cases := map[string][]byte{
		"mixed neural without a mixed configuration": rewrite(mixedPayload, func(object map[string]json.RawMessage) {
			editConfig(object, func(config map[string]json.RawMessage) { delete(config, "mixed") })
		}),
		"continuous core against a mixed configuration": rewrite(mixedPayload, func(object map[string]json.RawMessage) {
			object["neural"] = json.RawMessage(`{"core":"continuous","continuous":{"schema_version":"` + dynamics.ContinuousStateVersion + `","config_hash":"x","steps":0,"voltage":[0,0,0],"history":[[0,0,0]]}}`)
		}),
		"mixed core also carrying a continuous half": rewrite(mixedPayload, func(object map[string]json.RawMessage) {
			editNeural(object, func(neural map[string]json.RawMessage) {
				neural["continuous"] = json.RawMessage(`{"schema_version":"` + dynamics.ContinuousStateVersion + `","config_hash":"x","steps":0,"voltage":[0,0,0],"history":[[0,0,0]]}`)
			})
		}),
		"mixed core also carrying a lif half": rewrite(mixedPayload, func(object map[string]json.RawMessage) {
			editNeural(object, func(neural map[string]json.RawMessage) {
				neural["lif"] = json.RawMessage(`{"schema_version":"` + dynamics.LIFStateVersion + `","config_hash":"x","steps":0,"voltage":[0,0,0],"history":[[0,0,0]],"adaptation":[0,0,0],"refractory":[0,0,0]}`)
			})
		}),
		"mixed core without a mixed half": rewrite(mixedPayload, func(object map[string]json.RawMessage) {
			editNeural(object, func(neural map[string]json.RawMessage) { delete(neural, "mixed") })
		}),
		"mixed neural against a continuous configuration": rewrite(continuousPayload, func(object map[string]json.RawMessage) {
			editNeural(object, func(neural map[string]json.RawMessage) { neural["core"] = json.RawMessage(`"mixed"`) })
		}),
		"lif neural against a mixed configuration": rewrite(lifPayload, func(object map[string]json.RawMessage) {
			editConfig(object, func(config map[string]json.RawMessage) {
				config["mixed"] = mixedConfigOf(mixedPayload)
			})
		}),
		"configuration declaring two cores": rewrite(mixedPayload, func(object map[string]json.RawMessage) {
			editConfig(object, func(config map[string]json.RawMessage) {
				var lifConfig map[string]json.RawMessage
				if err := json.Unmarshal(lifPayload, &lifConfig); err != nil {
					t.Fatal(err)
				}
				var inner map[string]json.RawMessage
				if err := json.Unmarshal(lifConfig["config"], &inner); err != nil {
					t.Fatal(err)
				}
				config["lif"] = inner["lif"]
			})
		}),
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "union.json")
			if err := os.WriteFile(path, document, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadIndividual(context.Background(), path); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}

// TestMixedModelPackageIsRefused pins the boundary of ticket 25 stage one: the
// persistent individual snapshot carries the mixed core, the model package does
// not. The package's topology fingerprint reads only the continuous and the
// spiking configuration, so accepting a mixed one would publish a file that
// claims no nodes and no edges and cannot be read back.
func TestMixedModelPackageIsRefused(t *testing.T) {
	s := newCheckpointMixedIndividual(t).Snapshot()
	units := Units{TimeStep: "model_step", TimeConstant: "model_step"}
	if _, err := NewModelPackage(s.Config, s.Parameters, units, nil); err == nil {
		t.Fatal("accepted a model package carrying the mixed core")
	}
}
