package checkpoint_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const taskBundleSchema = "coimnet-task-fixture/v1"

func taskBundleConfig() []byte {
	return []byte(`{"fixture":"task-bundle","seed":7}`)
}

func TestTaskTrainingBundlePreservesMixedTopology(t *testing.T) {
	core := dynamics.MixedConfig{
		Nodes: 3, Sources: []int{0, 1}, Targets: []int{1, 2},
		Delays: []int{0, 1}, DT: math.Ln2, NodeRule: []uint8{0, 1, 0},
		Continuous: dynamics.ContinuousRule{Activation: "tanh"},
		LIF: dynamics.LIFRule{TauSyn: 1, ThetaMin: 0, ThetaMax: 2,
			VReset: -1, RefractorySteps: 1,
			Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}},
	}
	config := learning.Config{Mixed: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
	parameters := learning.Parameters{
		Core:     dynamics.Parameters{Weights: []float64{4, 2}, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}},
		ThetaRaw: []float64{0}, Encoder: []float64{4, 0, 0}, Readout: []float64{1},
	}
	trainer, err := learning.NewTrainer(config, parameters, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "mixed-model")
	if err := checkpoint.SaveTaskTrainingBundle(context.Background(), dir, taskBundleSchema, taskBundleConfig(), trainer.Snapshot()); err != nil {
		t.Fatal(err)
	}
	manifest, err := checkpoint.ReadBundleManifest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	var edges []byte
	for _, node := range []uint32{0, 1, 1, 2} {
		edges = binary.LittleEndian.AppendUint32(edges, node)
	}
	sum := sha256.Sum256(edges)
	if manifest.Topology.Nodes != 3 || manifest.Topology.Edges != 2 || manifest.Topology.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("mixed topology lost: %+v", manifest.Topology)
	}
	loaded, err := checkpoint.LoadTaskTrainingBundle(context.Background(), dir, taskBundleSchema)
	if err != nil {
		t.Fatal(err)
	}
	want := trainer.Snapshot()
	// Bundles canonically store required empty arrays as [] rather than null.
	want.Config.Dynamics.Sources = []int{}
	want.Config.Dynamics.Targets = []int{}
	want.Config.Dynamics.Delays = []int{}
	if !reflect.DeepEqual(loaded.Training, want) {
		t.Fatal("mixed training state changed beyond canonical empty arrays")
	}
}

func TestDecodeTaskConfigRejectsMissingNullUnknownAndDuplicateFields(t *testing.T) {
	type config struct {
		Seed uint64 `json:"seed"`
		Name string `json:"name"`
	}
	want := config{Seed: 0, Name: "valid"}
	var got config
	if err := checkpoint.DecodeTaskConfig([]byte(`{"seed":0,"name":"valid"}`), &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("decoded config: %+v", got)
	}
	for _, data := range []string{`{"name":"valid"}`, `{"seed":null,"name":"valid"}`, `{"seed":0,"name":"valid","extra":1}`, `{"seed":0,"seed":1,"name":"valid"}`, `{"seed":0,"name":"\ud800"}`} {
		got = want
		if err := checkpoint.DecodeTaskConfig([]byte(data), &got); err == nil {
			t.Fatalf("accepted %s", data)
		}
		if got != want {
			t.Fatalf("invalid input changed caller config: %+v", got)
		}
	}
}

func TestTaskTrainingBundleRetainsLargeJSONConfig(t *testing.T) {
	config := []byte(`{"text":"` + strings.Repeat("x", 8192) + `"}`)
	dir := filepath.Join(t.TempDir(), "large-config-model")
	snapshot := taskBundleTrainer(t).Snapshot()
	if err := checkpoint.SaveTaskTrainingBundle(context.Background(), dir, taskBundleSchema, config, snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := checkpoint.LoadTaskTrainingBundle(context.Background(), dir, taskBundleSchema)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Config) != string(config) {
		t.Fatal("large configuration was extracted or changed")
	}
}

func TestTaskTrainingBundleRequiresZeroValuedTrainingFields(t *testing.T) {
	source := filepath.Join(t.TempDir(), "model")
	snapshot := taskBundleTrainer(t).Snapshot()
	if err := checkpoint.SaveTaskTrainingBundle(context.Background(), source, taskBundleSchema, taskBundleConfig(), snapshot); err != nil {
		t.Fatal(err)
	}
	variant := taskBundleCopy(t, source)
	mutateTaskDocument(t, variant, true, func(fields map[string]json.RawMessage) {
		var training map[string]json.RawMessage
		if err := json.Unmarshal(fields["training"], &training); err != nil {
			t.Fatal(err)
		}
		delete(training, "updates")
		data, err := json.Marshal(training)
		if err != nil {
			t.Fatal(err)
		}
		fields["training"] = data
	})
	if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), variant, taskBundleSchema); err == nil {
		t.Fatal("missing zero updates field accepted")
	}
}

func TestTaskTrainingBundleRequiresZeroValuedManifestFields(t *testing.T) {
	source := filepath.Join(t.TempDir(), "model")
	if err := checkpoint.SaveTaskTrainingBundle(context.Background(), source, taskBundleSchema, taskBundleConfig(), taskBundleTrainer(t).Snapshot()); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"entries", "arrays.bytes"} {
		t.Run(field, func(t *testing.T) {
			variant := taskBundleCopy(t, source)
			mutateTaskManifest(t, variant, func(fields map[string]json.RawMessage) {
				if field == "entries" {
					delete(fields, "entries")
					return
				}
				var arrays map[string]json.RawMessage
				if err := json.Unmarshal(fields["arrays"], &arrays); err != nil {
					t.Fatal(err)
				}
				delete(arrays, "bytes")
				data, err := json.Marshal(arrays)
				if err != nil {
					t.Fatal(err)
				}
				fields["arrays"] = data
			})
			if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), variant, taskBundleSchema); err == nil {
				t.Fatal("missing required manifest field accepted")
			}
		})
	}
}

func taskBundleTrainer(t *testing.T) *learning.Trainer {
	t.Helper()
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      2,
			Sources:    []int{0},
			Targets:    []int{1},
			Delays:     []int{0},
			DT:         0.5,
			Activation: "tanh",
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{0.3},
			Bias:    []float64{0.1, -0.1},
			LogTau:  []float64{0.2, -0.05},
		},
		Encoder: []float64{0.7, -0.2},
		Readout: []float64{0.8},
	}
	options := learning.DefaultOptions()
	options.LearningRate = 0.03
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		t.Fatalf("NewTrainer: %v", err)
	}
	return trainer
}

func taskBundleLargeTrainer(t *testing.T) *learning.Trainer {
	t.Helper()
	const nodes = 64
	sources := make([]int, 0, nodes*nodes)
	targets := make([]int, 0, nodes*nodes)
	delays := make([]int, 0, nodes*nodes)
	for source := 0; source < nodes; source++ {
		for target := 0; target < nodes; target++ {
			sources = append(sources, source)
			targets = append(targets, target)
			delays = append(delays, 0)
		}
	}
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      nodes,
			Sources:    sources,
			Targets:    targets,
			Delays:     delays,
			DT:         0.5,
			Activation: "tanh",
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{nodes - 1},
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: make([]float64, len(sources)),
			Bias:    make([]float64, nodes),
			LogTau:  make([]float64, nodes),
		},
		Encoder: make([]float64, nodes),
		Readout: []float64{0.1},
	}
	for i := range parameters.Core.Weights {
		parameters.Core.Weights[i] = 0.001
	}
	for i := range parameters.Core.LogTau {
		parameters.Core.LogTau[i] = 0.1
	}
	parameters.Encoder[0] = 1
	options := learning.DefaultOptions()
	options.LearningRate = 0.01
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		t.Fatalf("NewTrainer(large): %v", err)
	}
	return trainer
}

func trainTaskBundleStep(t *testing.T, trainer *learning.Trainer) {
	t.Helper()
	result, err := trainer.Step(context.Background(), [][]float64{{0.5}, {-0.2}}, []float64{0.4})
	if err != nil {
		t.Fatalf("Trainer.Step: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Trainer.Step did not apply an update: %+v", result)
	}
}

func taskBundleFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("bundle contains nested directory %q", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", entry.Name(), err)
		}
		files[entry.Name()] = data
	}
	return files
}

func taskBundleCopy(t *testing.T, source string) string {
	t.Helper()
	destination := filepath.Join(t.TempDir(), "copy")
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatalf("copy bundle: %v", err)
	}
	return destination
}

func mutateTaskManifest(t *testing.T, dir string, mutate func(map[string]json.RawMessage)) {
	t.Helper()
	path := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(manifest.json): %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("decode manifest.json: %v", err)
	}
	mutate(fields)
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode manifest.json: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
}

func mutateTaskDocument(t *testing.T, dir string, updateManifest bool, mutate func(map[string]json.RawMessage)) {
	t.Helper()
	path := filepath.Join(dir, "document.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(document.json): %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("decode document.json: %v", err)
	}
	mutate(fields)
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode document.json: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write document.json: %v", err)
	}
	if updateManifest {
		digest := sha256.Sum256(data)
		mutateTaskManifest(t, dir, func(fields map[string]json.RawMessage) {
			var document map[string]json.RawMessage
			if err := json.Unmarshal(fields["document"], &document); err != nil {
				t.Fatalf("decode document metadata: %v", err)
			}
			document["bytes"] = json.RawMessage([]byte(stringifyTaskInt(int64(len(data)))))
			document["sha256"] = json.RawMessage(`"` + hex.EncodeToString(digest[:]) + `"`)
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatalf("encode document metadata: %v", err)
			}
			fields["document"] = encoded
		})
	}
}

func stringifyTaskInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func mutateTaskDocumentWithCanonicalMetadata(t *testing.T, dir string, mutate func(map[string]json.RawMessage)) {
	t.Helper()
	mutateTaskDocument(t, dir, true, mutate)
}

func TestTaskTrainingBundleRoundTripOwnsCompleteTraining(t *testing.T) {
	trainer := taskBundleTrainer(t)
	trainTaskBundleStep(t, trainer)
	wantTraining := trainer.Snapshot()
	dir := filepath.Join(t.TempDir(), "training-model")
	if err := checkpoint.SaveTaskTrainingBundle(context.Background(), dir, taskBundleSchema, taskBundleConfig(), wantTraining); err != nil {
		t.Fatalf("SaveTaskTrainingBundle: %v", err)
	}
	got, err := checkpoint.LoadTaskTrainingBundle(context.Background(), dir, taskBundleSchema)
	if err != nil {
		t.Fatalf("LoadTaskTrainingBundle: %v", err)
	}
	if got.TaskSchema != taskBundleSchema {
		t.Fatalf("task schema = %q, want %q", got.TaskSchema, taskBundleSchema)
	}
	manifest, err := checkpoint.ReadBundleManifest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	if got.ManifestSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("load receipt does not identify the verified manifest")
	}
	var gotConfig, wantConfig any
	if err := json.Unmarshal(got.Config, &gotConfig); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(taskBundleConfig(), &wantConfig); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotConfig, wantConfig) {
		t.Fatalf("config = %#v, want %#v", gotConfig, wantConfig)
	}
	if !reflect.DeepEqual(got.Training, wantTraining) {
		t.Fatalf("training snapshot changed:\n got=%#v\nwant=%#v", got.Training, wantTraining)
	}
	got.Config[0] = 'x'
	got.Training.Parameters.Core.Weights[0] = 99
	got.Training.Optimizer.First[0] = 88
	again, err := checkpoint.LoadTaskTrainingBundle(context.Background(), dir, taskBundleSchema)
	if err != nil {
		t.Fatalf("LoadTaskTrainingBundle after caller mutation: %v", err)
	}
	if !reflect.DeepEqual(again.Training, wantTraining) {
		t.Fatal("loaded training snapshot aliases caller-owned buffers")
	}
}

func TestTaskTrainingBundleRejectsInvalidArgumentsAndPreservesExisting(t *testing.T) {
	trainer := taskBundleTrainer(t)
	snapshot := trainer.Snapshot()
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		schema string
		config []byte
	}{
		{name: "empty schema", schema: "", config: taskBundleConfig()},
		{name: "nil config", schema: taskBundleSchema, config: nil},
		{name: "null config", schema: taskBundleSchema, config: []byte("null")},
		{name: "array config", schema: taskBundleSchema, config: []byte("[]")},
		{name: "malformed config", schema: taskBundleSchema, config: []byte("{")},
		{name: "duplicate config key", schema: taskBundleSchema, config: []byte(`{"x":1,"x":2}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "invalid-model")
			if err := checkpoint.SaveTaskTrainingBundle(ctx, dir, tc.schema, tc.config, snapshot); err == nil {
				t.Fatal("SaveTaskTrainingBundle accepted invalid input")
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid Save left a path: %v", err)
			}
		})
	}

	dir := filepath.Join(t.TempDir(), "existing-model")
	if err := checkpoint.SaveTaskTrainingBundle(ctx, dir, taskBundleSchema, taskBundleConfig(), snapshot); err != nil {
		t.Fatalf("first SaveTaskTrainingBundle: %v", err)
	}
	original := taskBundleFiles(t, dir)
	if err := checkpoint.SaveTaskTrainingBundle(ctx, dir, taskBundleSchema, taskBundleConfig(), snapshot); err == nil {
		t.Fatal("SaveTaskTrainingBundle overwrote an existing bundle")
	}
	if got := taskBundleFiles(t, dir); !reflect.DeepEqual(got, original) {
		t.Fatal("failed save changed the existing bundle")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	canceledDir := filepath.Join(t.TempDir(), "canceled-model")
	if err := checkpoint.SaveTaskTrainingBundle(canceled, canceledDir, taskBundleSchema, taskBundleConfig(), snapshot); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SaveTaskTrainingBundle = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(canceledDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled Save left a path: %v", err)
	}
	if _, err := checkpoint.LoadTaskTrainingBundle(nil, dir, taskBundleSchema); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted a nil context")
	}
	if _, err := checkpoint.LoadTaskTrainingBundle(ctx, "", taskBundleSchema); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted an empty path")
	}
}

func TestTaskTrainingBundleRejectsDocumentManifestArrayAndTopologyVariants(t *testing.T) {
	trainer := taskBundleLargeTrainer(t)
	snapshot := trainer.Snapshot()
	source := filepath.Join(t.TempDir(), "valid-model")
	if err := checkpoint.SaveTaskTrainingBundle(context.Background(), source, taskBundleSchema, taskBundleConfig(), snapshot); err != nil {
		t.Fatalf("SaveTaskTrainingBundle: %v", err)
	}
	manifest, err := checkpoint.ReadBundleManifest(context.Background(), source)
	if err != nil {
		t.Fatalf("ReadBundleManifest: %v", err)
	}
	if manifest.Arrays.Bytes == 0 || len(manifest.Entries) == 0 {
		t.Fatal("large fixture did not produce an arrays.bin entry")
	}

	unknownManifest := taskBundleCopy(t, source)
	mutateTaskManifest(t, unknownManifest, func(fields map[string]json.RawMessage) {
		fields["unexpected"] = json.RawMessage(`true`)
	})
	if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), unknownManifest, taskBundleSchema); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted an unknown manifest field")
	}

	schemaManifest := taskBundleCopy(t, source)
	mutateTaskManifest(t, schemaManifest, func(fields map[string]json.RawMessage) {
		fields["schema_version"] = json.RawMessage(`"coimnet-wrong-bundle/v1"`)
	})
	if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), schemaManifest, taskBundleSchema); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted an unsupported bundle schema")
	}
	if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), source, "coimnet-other-task/v1"); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted a mismatched task schema")
	}

	topologyVariant := taskBundleCopy(t, source)
	mutateTaskManifest(t, topologyVariant, func(fields map[string]json.RawMessage) {
		var topology map[string]json.RawMessage
		if err := json.Unmarshal(fields["topology"], &topology); err != nil {
			t.Fatal(err)
		}
		topology["nodes"] = json.RawMessage(`999`)
		encoded, err := json.Marshal(topology)
		if err != nil {
			t.Fatal(err)
		}
		fields["topology"] = encoded
	})
	if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), topologyVariant, taskBundleSchema); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted a topology mismatch")
	}

	geometryVariant := taskBundleCopy(t, source)
	mutateTaskManifest(t, geometryVariant, func(fields map[string]json.RawMessage) {
		var entries []map[string]json.RawMessage
		if err := json.Unmarshal(fields["entries"], &entries); err != nil {
			t.Fatal(err)
		}
		var length int64
		if err := json.Unmarshal(entries[0]["length"], &length); err != nil {
			t.Fatal(err)
		}
		entries[0]["length"] = json.RawMessage(strconv.FormatInt(length+1, 10))
		encoded, err := json.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		fields["entries"] = encoded
	})
	if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), geometryVariant, taskBundleSchema); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted a bundle geometry mismatch")
	}

	documentVariant := taskBundleCopy(t, source)
	documentPath := filepath.Join(documentVariant, "document.json")
	document, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	document[0] ^= 1
	if err := os.WriteFile(documentPath, document, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), documentVariant, taskBundleSchema); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted a corrupt document checksum")
	}

	arraysVariant := taskBundleCopy(t, source)
	arraysPath := filepath.Join(arraysVariant, "arrays.bin")
	arrays, err := os.ReadFile(arraysPath)
	if err != nil {
		t.Fatal(err)
	}
	arrays[0] ^= 1
	if err := os.WriteFile(arraysPath, arrays, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), arraysVariant, taskBundleSchema); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted a corrupt arrays checksum")
	}

	missingFieldVariant := taskBundleCopy(t, source)
	mutateTaskDocumentWithCanonicalMetadata(t, missingFieldVariant, func(fields map[string]json.RawMessage) {
		delete(fields, "task_schema")
	})
	if _, err := checkpoint.LoadTaskTrainingBundle(context.Background(), missingFieldVariant, taskBundleSchema); err == nil {
		t.Fatal("LoadTaskTrainingBundle accepted a document with a missing required field")
	}
}
