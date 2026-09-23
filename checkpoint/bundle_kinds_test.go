package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
)

func TestBundleModelPackageRoundTrip(t *testing.T) {
	want := newTestModelPackage(t, false)
	dir := filepath.Join(t.TempDir(), "model")
	if err := SaveModelPackageBundle(context.Background(), dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadModelPackageBundle(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	want = mustCanonicalModelPackage(t, want)
	assertBundleJSONEqual(t, got, want)
}

func TestBundleIndividualRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		new  func(*testing.T) *learning.Individual
	}{
		{name: "continuous", new: func(t *testing.T) *learning.Individual { return newCheckpointIndividual(t, false) }},
		{name: "LIF", new: newCheckpointLIFIndividual},
		{name: "plasticity", new: newPlasticCheckpointIndividual},
		{name: "chemistry", new: newChemicalCheckpointIndividual},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			individual := test.new(t)
			inputs := [][]float64{{.7}, {0}, {.2}}
			if _, err := individual.Advance(context.Background(), inputs); err != nil {
				t.Fatal(err)
			}
			if _, err := individual.TrainEpisode(context.Background(), inputs, []float64{.4}); err != nil {
				t.Fatal(err)
			}
			before := mustNormalizedIndividual(t, individual.Snapshot())
			dir := filepath.Join(t.TempDir(), "individual")
			if err := SaveIndividualBundle(context.Background(), dir, before); err != nil {
				t.Fatal(err)
			}
			got, err := LoadIndividualBundle(context.Background(), dir)
			if err != nil {
				t.Fatal(err)
			}
			assertBundleJSONEqual(t, got, before)
			original, err := learning.RestoreIndividual(before)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := learning.RestoreIndividual(got)
			if err != nil {
				t.Fatal(err)
			}
			wantOutput, err := original.Advance(context.Background(), inputs)
			if err != nil {
				t.Fatal(err)
			}
			gotOutput, err := restored.Advance(context.Background(), inputs)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotOutput, wantOutput) {
				t.Fatal("restored outputs differ after advancing identical rows")
			}
		})
	}
	t.Run("receptor zero gates hebbian rate", func(t *testing.T) {
		individual := newChemicalCheckpointIndividual(t)
		rule := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625}
		receptor := 0
		rule.GateReceptor = &receptor
		rule.GateScale = 1
		if err := individual.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
			t.Fatal(err)
		}
		before := mustNormalizedIndividual(t, individual.Snapshot())
		dir := filepath.Join(t.TempDir(), "individual")
		if err := SaveIndividualBundle(context.Background(), dir, before); err != nil {
			t.Fatal(err)
		}
		after, err := LoadIndividualBundle(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := learning.RestoreIndividual(after); err != nil {
			t.Fatalf("RestoreIndividual rejected the restored receptor-gated rule: %v", err)
		}
		wantJSON, err := json.Marshal(before)
		if err != nil {
			t.Fatal(err)
		}
		gotJSON, err := json.Marshal(after)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotJSON, wantJSON) {
			t.Fatal("receptor-zero individual JSON changed across bundle round trip")
		}
	})
}

func TestBundleTrainingRoundTrip(t *testing.T) {
	individual := newCheckpointIndividual(t, false)
	if _, err := individual.TrainEpisode(context.Background(), [][]float64{{.7}, {0}, {.2}}, []float64{.4}); err != nil {
		t.Fatal(err)
	}
	snapshot := individual.Snapshot()
	training := learning.TrainingSnapshot{
		SchemaVersion: TrainingSchemaVersion,
		Config:        snapshot.Config,
		Parameters:    snapshot.Parameters,
		Options:       snapshot.Optimizer.Options,
		Optimizer:     snapshot.Optimizer.State,
		Updates:       snapshot.Optimizer.Updates,
		Accumulator:   snapshot.Optimizer.Accumulator,
	}
	restored, err := learning.RestoreTrainer(training)
	if err != nil {
		t.Fatal(err)
	}
	want := normalizeBundleTrainingSnapshot(restored.Snapshot())
	dir := filepath.Join(t.TempDir(), "training")
	if err := SaveTrainingBundle(context.Background(), dir, training); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTrainingBundle(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	assertBundleJSONEqual(t, got, want)
}

func TestBundleKindsRefuseEachOther(t *testing.T) {
	individualDir := filepath.Join(t.TempDir(), "individual")
	if err := SaveIndividualBundle(context.Background(), individualDir, newCheckpointIndividual(t, false).Snapshot()); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadModelPackageBundle(context.Background(), individualDir); err == nil || !strings.Contains(err.Error(), BundleIndividual) {
		t.Fatalf("LoadModelPackageBundle error = %v, want kind %q", err, BundleIndividual)
	}
	if _, err := LoadTrainingBundle(context.Background(), individualDir); err == nil || !strings.Contains(err.Error(), BundleIndividual) {
		t.Fatalf("LoadTrainingBundle error = %v, want kind %q", err, BundleIndividual)
	}
	modelDir := filepath.Join(t.TempDir(), "model")
	if err := SaveModelPackageBundle(context.Background(), modelDir, newTestModelPackage(t, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIndividualBundle(context.Background(), modelDir); err == nil || !strings.Contains(err.Error(), BundleModelPackage) {
		t.Fatalf("LoadIndividualBundle error = %v, want kind %q", err, BundleModelPackage)
	}
}

func TestBundleReadManifestOnly(t *testing.T) {
	ctx := context.Background()
	makeBundle := func(t *testing.T) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "individual")
		if err := SaveIndividualBundle(ctx, dir, newCheckpointIndividual(t, false).Snapshot()); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	t.Run("document and arrays metadata", func(t *testing.T) {
		dir := makeBundle(t)
		manifest, err := ReadBundleManifest(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range []BundleFile{manifest.Document, manifest.Arrays} {
			data, err := os.ReadFile(filepath.Join(dir, file.Name))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if file.Bytes != int64(len(data)) || file.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatalf("manifest metadata = %#v, actual = (%d, %x)", file, len(data), sum)
			}
		}
	})
	t.Run("missing arrays file", func(t *testing.T) {
		dir := makeBundle(t)
		if err := os.Remove(filepath.Join(dir, "arrays.bin")); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadBundleManifest(ctx, dir); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("missing manifest", func(t *testing.T) {
		dir := makeBundle(t)
		if err := os.Remove(filepath.Join(dir, bundleManifestFile)); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadBundleManifest(ctx, dir); err == nil || !strings.Contains(err.Error(), "incomplete bundle") {
			t.Fatalf("ReadBundleManifest error = %v, want incomplete bundle", err)
		}
	})
}

func TestBundleRejectsTamperedTopology(t *testing.T) {
	t.Run("individual", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "individual")
		if err := SaveIndividualBundle(context.Background(), dir, newCheckpointIndividual(t, false).Snapshot()); err != nil {
			t.Fatal(err)
		}
		manifest, err := ReadBundleManifest(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		manifest.Topology.SHA256 = strings.Repeat("0", sha256.Size*2)
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, bundleManifestFile), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadIndividualBundle(context.Background(), dir); err == nil {
			t.Fatal("accepted a manifest with a different topology fingerprint")
		}
	})
	t.Run("training", func(t *testing.T) {
		individual := newCheckpointIndividual(t, false)
		snapshot := individual.Snapshot()
		training := learning.TrainingSnapshot{
			SchemaVersion: TrainingSchemaVersion,
			Config:        snapshot.Config,
			Parameters:    snapshot.Parameters,
			Options:       snapshot.Optimizer.Options,
			Optimizer:     snapshot.Optimizer.State,
			Updates:       snapshot.Optimizer.Updates,
			Accumulator:   snapshot.Optimizer.Accumulator,
		}
		dir := filepath.Join(t.TempDir(), "training")
		if err := SaveTrainingBundle(context.Background(), dir, training); err != nil {
			t.Fatal(err)
		}
		manifest, err := ReadBundleManifest(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		manifest.Topology.SHA256 = strings.Repeat("0", sha256.Size*2)
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, bundleManifestFile), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadTrainingBundle(context.Background(), dir); err == nil ||
			!strings.Contains(err.Error(), "manifest topology") || !strings.Contains(err.Error(), "snapshot topology") {
			t.Fatalf("LoadTrainingBundle error = %v, want both manifest and snapshot topology", err)
		}
	})
}

func TestBundleLargerThanSingleFileLimit(t *testing.T) {
	if bundleRaceEnabled {
		t.Skip("race instrumentation makes the large bundle fixture too memory intensive")
	}
	const nodes, edges = 1000, 3_000_000
	sources := make([]int, edges)
	targets := make([]int, edges)
	weights := make([]float64, edges)
	for edge := range sources {
		sources[edge] = edge % nodes
		targets[edge] = (edge*17 + 3) % nodes
		weights[edge] = .0001
	}
	const inputSize = 3072
	config := learning.Config{
		Dynamics:  dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: .5, Activation: "tanh"},
		InputSize: inputSize, OutputSize: 1, ReadoutNodes: []int{nodes - 1},
	}
	encoder := make([]float64, nodes*inputSize)
	for i := range encoder {
		encoder[i] = .0001
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: weights, Bias: make([]float64, nodes), LogTau: make([]float64, nodes)},
		Encoder: encoder, Readout: []float64{1},
	}
	individual, err := learning.NewIndividual(config, parameters, learning.DefaultOptions(), make([]float64, nodes))
	if err != nil {
		t.Fatal(err)
	}
	input := make([]float64, inputSize)
	for i := range input {
		input[i] = .5
	}
	if _, err := individual.TrainEpisode(context.Background(), [][]float64{input}, []float64{.25}); err != nil {
		t.Fatal(err)
	}
	want := mustNormalizedIndividual(t, individual.Snapshot())
	dir := filepath.Join(t.TempDir(), "large")
	if err := SaveIndividualBundle(context.Background(), dir, want); err != nil {
		t.Fatal(err)
	}
	manifest, err := ReadBundleManifest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("document_bytes=%d arrays_bytes=%d", manifest.Document.Bytes, manifest.Arrays.Bytes)
	if manifest.Arrays.Bytes <= 64<<20 {
		t.Fatalf("arrays_bytes=%d, want > %d", manifest.Arrays.Bytes, 64<<20)
	}
	got, err := LoadIndividualBundle(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Config.Dynamics.Sources, want.Config.Dynamics.Sources) || !reflect.DeepEqual(got.Config.Dynamics.Targets, want.Config.Dynamics.Targets) ||
		!reflect.DeepEqual(got.Parameters, want.Parameters) || !reflect.DeepEqual(got.Optimizer, want.Optimizer) {
		t.Fatal("large bundle changed config edge arrays, parameters, or optimizer values")
	}
}

func mustCanonicalModelPackage(t *testing.T, pkg ModelPackage) ModelPackage {
	t.Helper()
	got, err := canonicalModelPackage(pkg)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func mustNormalizedIndividual(t *testing.T, snapshot learning.IndividualSnapshot) learning.IndividualSnapshot {
	t.Helper()
	individual, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return normalizeIndividualSnapshot(individual.Snapshot())
}

func assertBundleJSONEqual(t *testing.T, got, want any) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatalf("JSON bytes differ:\n got %s\nwant %s", gotJSON, wantJSON)
	}
}
