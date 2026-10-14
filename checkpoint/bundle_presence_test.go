package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

func rewritePresenceBundle(t *testing.T, dir string, field string, null bool) map[string][]byte {
	t.Helper()
	path := filepath.Join(dir, bundleDocumentFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	keys := strings.Split(field, ".")
	parent := object
	for _, key := range keys[:len(keys)-1] {
		child, ok := parent[key].(map[string]any)
		if !ok {
			t.Fatalf("missing fixture object %q in %s", key, field)
		}
		parent = child
	}
	key := keys[len(keys)-1]
	if _, ok := parent[key]; !ok {
		t.Fatalf("fixture field %s does not exist", field)
	}
	if null {
		parent[key] = nil
	} else {
		delete(parent, key)
	}
	data, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, bundleManifestFile)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest BundleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Document.Bytes = int64(len(data))
	manifest.Document.SHA256 = digestHex(data)
	manifestData, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0600); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, name := range []string{bundleDocumentFile, bundleManifestFile, bundleArraysFile} {
		files[name], err = os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

func assertPresenceBundleUnchanged(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("load changed %s: %v", name, err)
		}
	}
}

func TestBundleRejectsIncompleteIndividualChemicalFields(t *testing.T) {
	for _, field := range []string{"chemical.state.steps", "chemical.resources", "chemical.pending_feedback"} {
		for _, null := range []bool{false, true} {
			t.Run(field+map[bool]string{false: "/missing", true: "/null"}[null], func(t *testing.T) {
				i := newChemicalCheckpointIndividual(t)
				dir := filepath.Join(t.TempDir(), "individual")
				if err := SaveIndividualBundle(context.Background(), dir, i.Snapshot()); err != nil {
					t.Fatal(err)
				}
				files := rewritePresenceBundle(t, dir, field, null)
				got, err := LoadIndividualBundle(context.Background(), dir)
				if err == nil || !reflect.DeepEqual(got, learning.IndividualSnapshot{}) {
					t.Fatalf("accepted incomplete %s or returned partial snapshot: %v", field, err)
				}
				assertPresenceBundleUnchanged(t, dir, files)
			})
		}
	}
}

func TestBundleRejectsIncompleteTrainingFields(t *testing.T) {
	for _, field := range []string{"options.weight_decay", "options.clip_norm", "options.truncation", "options.trainable", "options.trainable.encoder"} {
		for _, null := range []bool{false, true} {
			t.Run(field+map[bool]string{false: "/missing", true: "/null"}[null], func(t *testing.T) {
				c, p := newPackageContinuousModel()
				o := learning.DefaultOptions()
				o.WeightDecay = .2
				o.ClipNorm = 1
				o.Truncation = 1
				trainer, err := learning.NewTrainer(c, p, o)
				if err != nil {
					t.Fatal(err)
				}
				defer trainer.Close()
				dir := filepath.Join(t.TempDir(), "training")
				if err := SaveTrainingBundle(context.Background(), dir, trainer.Snapshot()); err != nil {
					t.Fatal(err)
				}
				files := rewritePresenceBundle(t, dir, field, null)
				got, err := LoadTrainingBundle(context.Background(), dir)
				if err == nil || !reflect.DeepEqual(got, learning.TrainingSnapshot{}) {
					t.Fatalf("accepted incomplete %s or returned partial snapshot: %v", field, err)
				}
				assertPresenceBundleUnchanged(t, dir, files)
			})
		}
	}
}

func TestBundleRejectsIncompleteModelEvidence(t *testing.T) {
	for _, null := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "null"}[null], func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "model")
			if err := SaveModelPackageBundle(context.Background(), dir, newTestModelPackage(t, false)); err != nil {
				t.Fatal(err)
			}
			files := rewritePresenceBundle(t, dir, "evidence_registry", null)
			got, err := LoadModelPackageBundle(context.Background(), dir)
			if err == nil || !reflect.DeepEqual(got, ModelPackage{}) {
				t.Fatalf("accepted incomplete evidence registry or returned partial model: %v", err)
			}
			assertPresenceBundleUnchanged(t, dir, files)
		})
	}
}

func TestBundleTaskChemicalTrainingRoundTrip(t *testing.T) {
	trainer := newChemicalEpisodeCheckpointTrainer(t)
	defer trainer.Close()
	dir := filepath.Join(t.TempDir(), "task")
	const schema = "coimnet-chemical-task-fixture/v1"
	if err := SaveTaskTrainingBundle(context.Background(), dir, schema, []byte(`{"fixture":"chemical"}`), trainer.Snapshot()); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTaskTrainingBundle(context.Background(), dir, schema)
	if err != nil {
		t.Fatalf("saved valid chemical task cannot load: %v", err)
	}
	want := normalizeBundleTrainingSnapshot(trainer.Snapshot())
	assertBundleJSONEqual(t, loaded.Training, want)
	resumed, err := learning.RestoreTrainer(loaded.Training)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	a := trainChemicalEpisodeCheckpoint(t, trainer, 0, 2)
	b := trainChemicalEpisodeCheckpoint(t, resumed, 0, 2)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("restored chemical task training steps differ")
	}
	assertBundleJSONEqual(t, normalizeBundleTrainingSnapshot(resumed.Snapshot()), normalizeBundleTrainingSnapshot(trainer.Snapshot()))
}

func TestBundlePresencePreservesLargeArraysAndLegacyOmissions(t *testing.T) {
	const nodes = 4097
	c := learning.Config{Dynamics: dynamics.Config{Nodes: nodes, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, Activation: "tanh"}, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{}, Bias: make([]float64, nodes), LogTau: make([]float64, nodes)}, Encoder: make([]float64, nodes), Readout: []float64{1}}
	p.Encoder[0] = 1
	trainer, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer trainer.Close()
	individual, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, nodes))
	if err != nil {
		t.Fatal(err)
	}
	chem := modulation.ChemistryConfig{Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}}, Sources: []modulation.SourceSpec{{Kind: modulation.SourceController, Channel: 0, Controller: &modulation.Controller{Inputs: modulation.ControllerInputs{SummaryWindow: 2}, Nodes: nodes, Hidden: 1, Channel: 0, Parameters: []float64{.6, .2, .05, .8, -.1}}}}, Receptors: modulation.Receptors{}, Regions: modulation.RegionAssignment{NodeRegion: make([]int, nodes)}}
	if err := individual.EnableChemistry(chem); err != nil {
		t.Fatal(err)
	}
	if _, err := individual.Advance(context.Background(), [][]float64{{.7}, {.2}}); err != nil {
		t.Fatal(err)
	}
	pkg, err := NewModelPackage(c, p, packageUnits(), []string{"fixture"})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"individual", "training", "model", "task"} {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), kind)
			switch kind {
			case "individual":
				err = SaveIndividualBundle(context.Background(), dir, individual.Snapshot())
			case "training":
				err = SaveTrainingBundle(context.Background(), dir, trainer.Snapshot())
			case "model":
				err = SaveModelPackageBundle(context.Background(), dir, pkg)
			case "task":
				err = SaveTaskTrainingBundle(context.Background(), dir, "coimnet-large-task-fixture/v1", []byte(`{"fixture":"large"}`), trainer.Snapshot())
			}
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := ReadBundleManifest(context.Background(), dir)
			if err != nil || len(manifest.Entries) == 0 {
				t.Fatalf("fixture did not extract numeric arrays: %v", err)
			}
			switch kind {
			case "individual":
				got, err := LoadIndividualBundle(context.Background(), dir)
				if err != nil {
					t.Fatal(err)
				}
				assertBundleJSONEqual(t, got, mustNormalizedIndividual(t, individual.Snapshot()))
			case "training":
				got, err := LoadTrainingBundle(context.Background(), dir)
				if err != nil {
					t.Fatal(err)
				}
				assertBundleJSONEqual(t, got, normalizeBundleTrainingSnapshot(trainer.Snapshot()))
			case "model":
				got, err := LoadModelPackageBundle(context.Background(), dir)
				if err != nil {
					t.Fatal(err)
				}
				assertBundleJSONEqual(t, got, mustCanonicalModelPackage(t, pkg))
			case "task":
				got, err := LoadTaskTrainingBundle(context.Background(), dir, "coimnet-large-task-fixture/v1")
				if err != nil {
					t.Fatal(err)
				}
				assertBundleJSONEqual(t, got.Training, normalizeBundleTrainingSnapshot(trainer.Snapshot()))
			}
		})
	}
	// A complete zero-state remains valid. Omitted legacy optional fields retain their declared defaults.
	t.Run("legacy model", func(t *testing.T) {
		legacyConfig := learning.Config{Dynamics: dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1, Activation: "tanh"}, InputSize: 1, OutputSize: 1, InputNodes: []int{0, 1}, ReadoutNodes: []int{1}}
		legacyParameters := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{.3}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}, Encoder: []float64{1, 0}, Readout: []float64{1}}
		legacy, err := NewModelPackage(legacyConfig, legacyParameters, packageUnits(), []string{"fixture"})
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(t.TempDir(), "legacy")
		if err := SaveModelPackageBundle(context.Background(), dir, legacy); err != nil {
			t.Fatal(err)
		}
		rewritePresenceBundle(t, dir, "capacity", false)
		rewritePresenceBundle(t, dir, "config.input_nodes", false)
		rewritePresenceBundle(t, dir, "config.dynamics.delays", false)
		if _, err := LoadModelPackageBundle(context.Background(), dir); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("legacy sources", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "legacy")
		if err := SaveIndividualBundle(context.Background(), dir, individual.Snapshot()); err != nil {
			t.Fatal(err)
		}
		files := rewritePresenceBundle(t, dir, "chemical.source_states", false)
		got, err := LoadIndividualBundle(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if got.Chemical.State.Steps != individual.Snapshot().Chemical.State.Steps || !reflect.DeepEqual(got.Neural, individual.Snapshot().Neural) {
			t.Fatal("legacy source omission reset chemical/neural clock")
		}
		if got.Chemical.SourceStates[0].Controller.Filled != 0 {
			t.Fatal("legacy sources are not fresh")
		}
		assertPresenceBundleUnchanged(t, dir, files)
	})
}
