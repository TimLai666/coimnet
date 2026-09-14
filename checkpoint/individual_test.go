package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const individualHelperEnv = "COIMNET_INDIVIDUAL_CHECKPOINT_HELPER"

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
	got.Neural.History[0][0] = 77
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
