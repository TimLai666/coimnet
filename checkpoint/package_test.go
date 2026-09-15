package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const packageHelperEnv = "COIMNET_MODEL_PACKAGE_HELPER"

// newPackageContinuousModel is the same two neuron continuous fixture the
// individual tests use, expressed as a declaration a model package can carry.
func newPackageContinuousModel() (learning.Config, learning.Parameters) {
	c := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1}, DT: .5, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.25}, Bias: []float64{.1, -.1}, LogTau: []float64{0, 0}},
		Encoder: []float64{.5, .2},
		Readout: []float64{.8},
	}
	return c, p
}

// newPackageLIFModel is a three neuron spiking declaration with one delayed
// edge, short term adaptation and the slow stabiliser on, so an individual
// seeded from it carries every persistent field the union has to survive.
func newPackageLIFModel() (learning.Config, learning.Parameters) {
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
	return c, p
}

func packageUnits() Units {
	return Units{TimeStep: "model_step", TimeConstant: "model_step"}
}

func newTestModelPackage(t *testing.T, lif bool) ModelPackage {
	t.Helper()
	c, p := newPackageContinuousModel()
	if lif {
		c, p = newPackageLIFModel()
	}
	pkg, err := NewModelPackage(c, p, packageUnits(), []string{"evidence/STA-01/verification.json"})
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

// referenceTopologyDigest re-derives the fingerprint the straightforward way:
// every source index followed by every target index, each as a little-endian
// uint32, hashed with SHA-256. It is the independent reference for the shared
// encoding simulate's topology hash also uses.
func referenceTopologyDigest(sources, targets []int) string {
	var block []byte
	for _, group := range [][]int{sources, targets} {
		for _, node := range group {
			var four [4]byte
			binary.LittleEndian.PutUint32(four[:], uint32(node))
			block = append(block, four[:]...)
		}
	}
	sum := sha256.Sum256(block)
	return hex.EncodeToString(sum[:])
}

func TestModelPackageTopologyFingerprintUsesTheSharedEncoding(t *testing.T) {
	// A one edge graph pins the encoding against a literal computed outside Go:
	// SHA-256 of 00000000 01000000 is the digest below.
	const oneEdge = "01acecb507abfe1a354aa8064f4af5d3f1acd019e37db3c11c97523b71c76e9d"
	continuous := newTestModelPackage(t, false)
	if continuous.Topology.Nodes != 2 || continuous.Topology.Edges != 1 {
		t.Fatalf("continuous topology = %+v", continuous.Topology)
	}
	if continuous.Topology.SHA256 != oneEdge {
		t.Fatalf("continuous topology sha256 = %q, want %q", continuous.Topology.SHA256, oneEdge)
	}
	if got := referenceTopologyDigest([]int{0}, []int{1}); got != continuous.Topology.SHA256 {
		t.Fatalf("independent recomputation = %q", got)
	}

	spiking := newTestModelPackage(t, true)
	if spiking.Topology.Nodes != 3 || spiking.Topology.Edges != 3 {
		t.Fatalf("spiking topology = %+v", spiking.Topology)
	}
	want := referenceTopologyDigest([]int{0, 1, 1}, []int{1, 1, 2})
	if spiking.Topology.SHA256 != want {
		t.Fatalf("spiking topology sha256 = %q, want %q", spiking.Topology.SHA256, want)
	}
	// Reordering the same edge set changes the fingerprint: it identifies the
	// stored edge arrays, not the abstract graph.
	reordered, p := newPackageLIFModel()
	reordered.LIF.Sources = []int{1, 0, 1}
	reordered.LIF.Targets = []int{1, 1, 2}
	other, err := NewModelPackage(reordered, p, packageUnits(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if other.Topology.SHA256 == spiking.Topology.SHA256 {
		t.Fatal("a different edge order produced the same fingerprint")
	}
}

func TestModelPackageRoundTripAndByteIdenticalSave(t *testing.T) {
	for _, lif := range []bool{false, true} {
		name := "continuous"
		if lif {
			name = "lif"
		}
		t.Run(name, func(t *testing.T) {
			want := newTestModelPackage(t, lif)
			if want.SchemaVersion != ModelPackageSchemaVersion {
				t.Fatalf("schema = %q", want.SchemaVersion)
			}
			if !reflect.DeepEqual(want.CompatibleVersions, CompatibleVersions{
				Individual: []string{IndividualSchemaVersion},
				Training:   []string{"coimnet-episode-training/v1"},
			}) {
				t.Fatalf("compatible versions = %+v", want.CompatibleVersions)
			}
			dir := t.TempDir()
			first := filepath.Join(dir, "model.json")
			second := filepath.Join(dir, "model-again.json")
			if err := SaveModelPackage(context.Background(), first, want); err != nil {
				t.Fatal(err)
			}
			if err := SaveModelPackage(context.Background(), second, want); err != nil {
				t.Fatal(err)
			}
			a, err := os.ReadFile(first)
			if err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(second)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(a, b) {
				t.Fatal("two saves of the same package produced different bytes")
			}
			if bytes.Contains(a, []byte("null")) {
				t.Fatalf("published package contains a null value: %s", a)
			}
			got, err := LoadModelPackage(context.Background(), first)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip changed the package:\n got %+v\nwant %+v", got, want)
			}
			// The loader must not hand back buffers it keeps or shares.
			got.Parameters.Core.Bias[0] = 88
			got.EvidenceRegistry[0] = "tampered"
			again, err := LoadModelPackage(context.Background(), first)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(again, want) {
				t.Fatal("LoadModelPackage returned buffers aliased with a later caller mutation")
			}
			// Saving what was loaded reproduces the same document.
			third := filepath.Join(dir, "model-resaved.json")
			if err := SaveModelPackage(context.Background(), third, again); err != nil {
				t.Fatal(err)
			}
			c, err := os.ReadFile(third)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(a, c) {
				t.Fatal("resaving a loaded package changed its bytes")
			}
		})
	}
}

func TestLoadModelPackageRejectsTamperedDocuments(t *testing.T) {
	pkg := newTestModelPackage(t, true)
	payload, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	valid := envelopeJSON(ModelPackageSchemaVersion, payload, checksumHex(payload))

	rewrite := func(change func(map[string]json.RawMessage)) []byte {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(payload, &object); err != nil {
			t.Fatal(err)
		}
		change(object)
		changed, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		return envelopeJSON(ModelPackageSchemaVersion, changed, checksumHex(changed))
	}
	nested := func(field string, change func(map[string]json.RawMessage)) []byte {
		return rewrite(func(object map[string]json.RawMessage) {
			var inner map[string]json.RawMessage
			if err := json.Unmarshal(object[field], &inner); err != nil {
				t.Fatal(err)
			}
			change(inner)
			encoded, err := json.Marshal(inner)
			if err != nil {
				t.Fatal(err)
			}
			object[field] = encoded
		})
	}

	flipped := append([]byte(nil), valid...)
	index := bytes.Index(flipped, []byte(`"fast_sigmoid"`))
	if index < 0 {
		t.Fatal("fixture surrogate was not found")
	}
	flipped[index+1] = 'F'

	cases := map[string][]byte{
		"flipped payload byte": flipped,
		"wrong checksum":       envelopeJSON(ModelPackageSchemaVersion, payload, strings.Repeat("0", sha256.Size*2)),
		"short checksum":       envelopeJSON(ModelPackageSchemaVersion, payload, "abcd"),
		"truncated":            valid[:len(valid)/2],
		"future schema":        envelopeJSON("coimnet-model-package/v2", payload, checksumHex(payload)),
		"payload schema": rewrite(func(object map[string]json.RawMessage) {
			object["schema_version"] = json.RawMessage(`"coimnet-model-package/v2"`)
		}),
		"fingerprint digest": nested("topology", func(inner map[string]json.RawMessage) {
			inner["sha256"] = json.RawMessage(`"` + strings.Repeat("a", sha256.Size*2) + `"`)
		}),
		"fingerprint edge count": nested("topology", func(inner map[string]json.RawMessage) {
			inner["edges"] = json.RawMessage(`2`)
		}),
		"fingerprint node count": nested("topology", func(inner map[string]json.RawMessage) {
			inner["nodes"] = json.RawMessage(`4`)
		}),
		"missing units": rewrite(func(object map[string]json.RawMessage) { delete(object, "units") }),
		"missing time constant": nested("units", func(inner map[string]json.RawMessage) {
			delete(inner, "time_constant")
		}),
		"empty time step": nested("units", func(inner map[string]json.RawMessage) {
			inner["time_step"] = json.RawMessage(`""`)
		}),
		"missing evidence registry": rewrite(func(object map[string]json.RawMessage) {
			delete(object, "evidence_registry")
		}),
		"missing compatible versions": rewrite(func(object map[string]json.RawMessage) {
			delete(object, "compatible_versions")
		}),
		"unknown compatible individual version": nested("compatible_versions", func(inner map[string]json.RawMessage) {
			inner["individual"] = json.RawMessage(`["coimnet-individual-checkpoint/v9"]`)
		}),
		"absolute evidence path": rewrite(func(object map[string]json.RawMessage) {
			object["evidence_registry"] = json.RawMessage(`["/etc/passwd"]`)
		}),
		"escaping evidence path": rewrite(func(object map[string]json.RawMessage) {
			object["evidence_registry"] = json.RawMessage(`["../../etc/passwd"]`)
		}),
		"null learning rate": nested("parameters", func(inner map[string]json.RawMessage) {
			inner["encoder"] = json.RawMessage(`[null,0,0]`)
		}),
		"missing parameters": rewrite(func(object map[string]json.RawMessage) { delete(object, "parameters") }),
		"parameter shape": nested("parameters", func(inner map[string]json.RawMessage) {
			inner["readout"] = json.RawMessage(`[1,1]`)
		}),
		"unknown field": rewrite(func(object map[string]json.RawMessage) {
			object["extra"] = json.RawMessage(`true`)
		}),
		"duplicate key": []byte(`{"schema_version":"` + ModelPackageSchemaVersion + `","schema_version":"` +
			ModelPackageSchemaVersion + `","payload":` + string(payload) + `,"checksum":"` + checksumHex(payload) + `"}`),
		"trailing data": append(append([]byte(nil), valid...), ' ', '{', '}'),
	}

	dir := t.TempDir()
	i := 0
	for name, document := range cases {
		i++
		path := filepath.Join(dir, fmt.Sprintf("case-%d.json", i))
		writeRaw(t, path, document)
		if _, err := LoadModelPackage(context.Background(), path); err == nil {
			t.Fatalf("accepted a tampered model package: %s", name)
		}
	}

	// The unmodified document must still load, so the cases above fail for the
	// reason under test rather than because the fixture is broken.
	good := filepath.Join(dir, "valid.json")
	writeRaw(t, good, valid)
	if _, err := LoadModelPackage(context.Background(), good); err != nil {
		t.Fatalf("the untampered fixture must load: %v", err)
	}
}

func TestSaveModelPackageCancellationNoOverwriteAndInvalidInput(t *testing.T) {
	pkg := newTestModelPackage(t, false)
	dir := t.TempDir()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledPath := filepath.Join(dir, "canceled.json")
	if err := SaveModelPackage(canceled, canceledPath, pkg); !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveModelPackage cancellation error = %v", err)
	}
	if _, err := os.Stat(canceledPath); !os.IsNotExist(err) {
		t.Fatalf("a canceled save published a file: %v", err)
	}
	assertNoTemps(t, dir, filepath.Base(canceledPath))

	path := filepath.Join(dir, "model.json")
	if err := SaveModelPackage(context.Background(), path, pkg); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveModelPackage(context.Background(), path, pkg); err == nil {
		t.Fatal("SaveModelPackage overwrote an existing package")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("failed overwrite changed the published package")
	}
	assertNoTemps(t, dir, filepath.Base(path))

	if err := SaveModelPackage(context.Background(), "", pkg); err == nil {
		t.Fatal("SaveModelPackage accepted an empty path")
	}
	if err := SaveModelPackage(context.Background(), filepath.Join(dir, "zero.json"), ModelPackage{}); err == nil {
		t.Fatal("SaveModelPackage accepted a zero-value package")
	}
	broken := newTestModelPackage(t, false)
	broken.Topology.SHA256 = strings.Repeat("b", sha256.Size*2)
	if err := SaveModelPackage(context.Background(), filepath.Join(dir, "broken.json"), broken); err == nil {
		t.Fatal("SaveModelPackage accepted a fingerprint that disagrees with the configuration")
	}
}

func TestNewModelPackageRejectsInvalidDeclarations(t *testing.T) {
	c, p := newPackageContinuousModel()
	if _, err := NewModelPackage(c, p, Units{TimeConstant: "model_step"}, nil); err == nil {
		t.Fatal("accepted a package without a declared time step unit")
	}
	if _, err := NewModelPackage(c, p, Units{TimeStep: "model_step"}, nil); err == nil {
		t.Fatal("accepted a package without a declared time constant unit")
	}
	if _, err := NewModelPackage(c, p, packageUnits(), []string{""}); err == nil {
		t.Fatal("accepted an empty evidence path")
	}
	if _, err := NewModelPackage(c, p, packageUnits(), []string{"/absolute/path.json"}); err == nil {
		t.Fatal("accepted an absolute evidence path")
	}
	if _, err := NewModelPackage(c, p, packageUnits(), []string{"evidence/../../secret"}); err == nil {
		t.Fatal("accepted an escaping evidence path")
	}
	bad := p
	bad.Readout = []float64{1, 2}
	if _, err := NewModelPackage(c, bad, packageUnits(), nil); err == nil {
		t.Fatal("accepted parameters whose shapes do not match the configuration")
	}
	both := c
	both.LIF = &dynamics.LIFConfig{Nodes: 2}
	if _, err := NewModelPackage(both, p, packageUnits(), nil); err == nil {
		t.Fatal("accepted a configuration that declares two cores")
	}
	// Caller-owned slices must not be retained.
	evidence := []string{"evidence/STA-01/verification.json"}
	pkg, err := NewModelPackage(c, p, packageUnits(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	evidence[0] = "changed"
	c.ReadoutNodes[0] = 0
	p.Core.Bias[0] = 99
	if pkg.EvidenceRegistry[0] != "evidence/STA-01/verification.json" || pkg.Config.ReadoutNodes[0] != 1 || pkg.Parameters.Core.Bias[0] != .1 {
		t.Fatalf("NewModelPackage retained caller-owned buffers: %+v", pkg)
	}
}

// TestArtefactKindsRefuseEachOther covers all six cross loads of the three
// artefact kinds. Only the model package can be mistaken for a complete
// recovery source, so its refusal message has to say what it is and what it is
// not; the other four name the kind they found.
func TestArtefactKindsRefuseEachOther(t *testing.T) {
	dir := t.TempDir()
	packagePath := filepath.Join(dir, "model.json")
	individualPath := filepath.Join(dir, "individual.json")
	episodePath := filepath.Join(dir, "episode.json")

	if err := SaveModelPackage(context.Background(), packagePath, newTestModelPackage(t, true)); err != nil {
		t.Fatal(err)
	}
	if err := SaveIndividual(context.Background(), individualPath, newCheckpointLIFIndividual(t).Snapshot()); err != nil {
		t.Fatal(err)
	}
	c, p := newPackageContinuousModel()
	trainer, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	episode, err := NewState(trainer.Snapshot(), 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(context.Background(), episodePath, episode); err != nil {
		t.Fatal(err)
	}

	_, err = LoadIndividual(context.Background(), packagePath)
	if err == nil {
		t.Fatal("LoadIndividual accepted a model package")
	}
	for _, want := range []string{"model package", "not an individual snapshot"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("LoadIndividual on a model package = %v, want it to contain %q", err, want)
		}
	}
	if _, err := LoadIndividual(context.Background(), episodePath); err == nil ||
		!strings.Contains(err.Error(), SchemaVersion) {
		t.Fatalf("LoadIndividual on an episode checkpoint = %v", err)
	}
	if _, err := LoadModelPackage(context.Background(), individualPath); err == nil ||
		!strings.Contains(err.Error(), IndividualSchemaVersion) {
		t.Fatalf("LoadModelPackage on an individual snapshot = %v", err)
	}
	if _, err := LoadModelPackage(context.Background(), episodePath); err == nil ||
		!strings.Contains(err.Error(), SchemaVersion) {
		t.Fatalf("LoadModelPackage on an episode checkpoint = %v", err)
	}
	if _, err := Load(context.Background(), packagePath); err == nil ||
		!strings.Contains(err.Error(), ModelPackageSchemaVersion) {
		t.Fatalf("Load on a model package = %v", err)
	}
	if _, err := Load(context.Background(), individualPath); err == nil ||
		!strings.Contains(err.Error(), IndividualSchemaVersion) {
		t.Fatalf("Load on an individual snapshot = %v", err)
	}
}

func TestNewIndividualFromPackageEqualsNewIndividual(t *testing.T) {
	for _, lif := range []bool{false, true} {
		name := "continuous"
		if lif {
			name = "lif"
		}
		t.Run(name, func(t *testing.T) {
			c, p := newPackageContinuousModel()
			input := [][]float64{{.7}, {0}, {.2}}
			if lif {
				c, p = newPackageLIFModel()
				input = [][]float64{{1}, {0}, {1}, {0}}
			}
			options := learning.DefaultOptions()
			options.Trainable.Theta = lif
			initial := make([]float64, 3)
			if !lif {
				initial = make([]float64, 2)
			}
			pkg := newTestModelPackage(t, lif)
			seeded, err := NewIndividualFromPackage(pkg, options, initial)
			if err != nil {
				t.Fatal(err)
			}
			direct, err := learning.NewIndividual(c, p, options, initial)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(seeded.Snapshot(), direct.Snapshot()) {
				t.Fatal("an individual seeded from a package differs from a directly created one")
			}
			seededOut, err := seeded.Advance(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			directOut, err := direct.Advance(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(seededOut, directOut) {
				t.Fatalf("Advance outputs differ: %v vs %v", seededOut, directOut)
			}
			if !reflect.DeepEqual(seeded.Snapshot(), direct.Snapshot()) {
				t.Fatal("state after Advance differs between the two individuals")
			}
			snapshot := seeded.Snapshot()
			if lif {
				if snapshot.Profile != learning.IndividualProfileLIF {
					t.Fatalf("profile = %q", snapshot.Profile)
				}
				if snapshot.Neural.LIF == nil || len(snapshot.Neural.LIF.Homeostasis) != 3 || len(snapshot.Neural.LIF.Rate) != 3 {
					t.Fatalf("the seeded spiking individual has no slow stabiliser state: %+v", snapshot.Neural.LIF)
				}
			} else if snapshot.Profile != learning.IndividualProfile {
				t.Fatalf("profile = %q", snapshot.Profile)
			}
			// A package with a fingerprint that no longer matches its
			// configuration must not seed anything.
			broken := pkg
			broken.Topology.Nodes++
			if _, err := NewIndividualFromPackage(broken, options, initial); err == nil {
				t.Fatal("seeded an individual from a package whose fingerprint disagrees with its configuration")
			}
			// The individual must not alias the package it was seeded from.
			second, err := NewIndividualFromPackage(pkg, options, initial)
			if err != nil {
				t.Fatal(err)
			}
			pkg.Parameters.Core.Bias[0] = 55
			pkg.Config.ReadoutNodes[0] = 0
			untouched, err := learning.NewIndividual(c, p, options, initial)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(second.Snapshot(), untouched.Snapshot()) {
				t.Fatal("mutating the package after seeding changed the individual it produced")
			}
		})
	}
}

// TestLIFIndividualFromPackageSubprocessResume trains a spiking individual
// seeded from a model package, snapshots it, and continues in a genuinely new
// process. The resumed run has to match the uninterrupted one in every part:
// neural state including the rate estimate and threshold offset, parameters,
// Adam moments and the update count.
func TestLIFIndividualFromPackageSubprocessResume(t *testing.T) {
	if os.Getenv(packageHelperEnv) == "1" {
		t.Skip("helper is tested separately")
	}
	dir := t.TempDir()
	packagePath := filepath.Join(dir, "model.json")
	midPath := filepath.Join(dir, "mid.json")
	finalPath := filepath.Join(dir, "final.json")
	outputPath := filepath.Join(dir, "outputs.json")
	if err := SaveModelPackage(context.Background(), packagePath, newTestModelPackage(t, true)); err != nil {
		t.Fatal(err)
	}
	pkg, err := LoadModelPackage(context.Background(), packagePath)
	if err != nil {
		t.Fatal(err)
	}
	options := learning.DefaultOptions()
	options.Trainable.Theta = true

	run := func() *learning.Individual {
		individual, err := NewIndividualFromPackage(pkg, options, make([]float64, 3))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := individual.Advance(context.Background(), packageWarmup); err != nil {
			t.Fatal(err)
		}
		for _, episode := range packageEpisodes {
			if _, err := individual.TrainEpisode(context.Background(), episode, []float64{.5}); err != nil {
				t.Fatal(err)
			}
		}
		return individual
	}
	full, part := run(), run()
	fullSuffix, err := full.Advance(context.Background(), packageTail)
	if err != nil {
		t.Fatal(err)
	}
	trained := part.Snapshot()
	if trained.Optimizer.Updates != uint64(len(packageEpisodes)) {
		t.Fatalf("updates = %d", trained.Optimizer.Updates)
	}
	fresh := newTestModelPackage(t, true)
	if reflect.DeepEqual(trained.Parameters, fresh.Parameters) {
		t.Fatal("training changed no parameter, the resume comparison would be vacuous")
	}
	var moved bool
	for _, v := range trained.Optimizer.State.First {
		if v != 0 {
			moved = true
		}
	}
	if !moved {
		t.Fatal("the optimizer carries no moment, the resume comparison would be vacuous")
	}
	if err := SaveIndividual(context.Background(), midPath, trained); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestModelPackageHelperProcess$", "--", midPath, finalPath, outputPath)
	cmd.Env = append(os.Environ(), packageHelperEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("model package helper failed: %v\n%s", err, output)
	}
	resumed, err := LoadIndividual(context.Background(), finalPath)
	if err != nil {
		t.Fatal(err)
	}
	want := full.Snapshot()
	if !reflect.DeepEqual(resumed, want) {
		t.Fatal("new-process state differs from the uninterrupted run")
	}
	if resumed.Neural.LIF == nil || len(resumed.Neural.LIF.Rate) != 3 || len(resumed.Neural.LIF.Homeostasis) != 3 {
		t.Fatalf("the resumed state lost the slow stabiliser: %+v", resumed.Neural.LIF)
	}
	var stabilised bool
	for _, h := range resumed.Neural.LIF.Homeostasis {
		if h != 0 {
			stabilised = true
		}
	}
	if !stabilised {
		t.Fatal("the threshold offset stayed at zero, the comparison would not cover it")
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

// The persistent windows keep driving the spiking fixture so the slow
// stabiliser is still holding a nonzero threshold offset at the point where the
// resumed run and the uninterrupted run are compared. A window that ends in
// silence would let the offset fall back to zero and the comparison would no
// longer cover it.
var (
	packageWarmup   = [][]float64{{1}, {0}, {1}, {0}}
	packageTail     = [][]float64{{1}, {0}, {1}, {1}, {0}}
	packageEpisodes = [][][]float64{
		{{1}, {0}, {1}, {0}, {0}},
		{{0}, {1}, {0}, {1}, {0}},
	}
)

func TestModelPackageHelperProcess(t *testing.T) {
	if os.Getenv(packageHelperEnv) != "1" {
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
	outputs, err := individual.Advance(context.Background(), packageTail)
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
