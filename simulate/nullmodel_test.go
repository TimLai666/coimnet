package simulate

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"path/filepath"
	"sort"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/params"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
)

// generatedGraph builds a 40 node, 300 edge graph with no self-loop and no
// duplicate pair, large enough that a degree preserving rewire applies real
// swaps instead of being blocked by the tiny fixtures. Node i has out-degree 7
// (targets i+1..i+7 modulo 40) and the first twenty nodes have one extra edge
// to i+8, which makes the out-degree vector non-uniform on purpose.
func generatedGraph(t *testing.T) *connectome.Graph {
	t.Helper()
	const nodes = 40
	dir := t.TempDir()
	ids := make([]int64, nodes)
	status := make([]string, nodes)
	class := make([]string, nodes)
	for i := range ids {
		ids[i] = int64(101 + i)
		status[i] = "Traced"
		class[i] = "even"
		if i%2 == 1 {
			class[i] = "odd"
		}
	}
	var pre, post, weight []int64
	add := func(source, target int) {
		pre = append(pre, ids[source])
		post = append(post, ids[target])
		weight = append(weight, int64(1+(source*7+target)%5))
	}
	for s := 0; s < nodes; s++ {
		for d := 1; d <= 7; d++ {
			add(s, (s+d)%nodes)
		}
	}
	for s := 0; s < 20; s++ {
		add(s, (s+8)%nodes)
	}
	if len(pre) != 300 {
		t.Fatalf("generated %d edges, want 300", len(pre))
	}
	writeFeather(t, filepath.Join(dir, "annotations.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "class", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues(ids, nil)
		b.Field(1).(*array.StringBuilder).AppendValues(status, nil)
		b.Field(2).(*array.StringBuilder).AppendValues(class, nil)
	})
	writeFeather(t, filepath.Join(dir, "weights.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues(pre, nil)
		b.Field(1).(*array.Int64Builder).AppendValues(post, nil)
		b.Field(2).(*array.Int64Builder).AppendValues(weight, nil)
	})
	writeFeather(t, filepath.Join(dir, "nt.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "consensus_nt", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{ids[0]}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"acetylcholine"}, nil)
	})
	manifest := connectome.DatasetManifest{
		SchemaVersion: connectome.ManifestSchemaVersion,
		Dataset:       "null-model-fixture",
		Namespace:     "simulate-null-fixture-v1",
		SourceVersion: "v1",
		License:       connectome.License{Name: "CC-BY-4.0", URL: "https://creativecommons.org/licenses/by/4.0/"},
		AcquiredAt:    "2026-09-15T00:00:00Z",
		Files: []connectome.SourceFile{
			fixtureSource(t, dir, "weights.feather", connectome.RoleWeights),
			fixtureSource(t, dir, "annotations.feather", connectome.RoleAnnotations),
			fixtureSource(t, dir, "nt.feather", connectome.RoleNeurotransmitters),
		},
		FieldMapping: connectome.FieldMapping{
			Weights:           connectome.WeightsFields{Source: "body_pre", Target: "body_post", Value: "weight"},
			Annotations:       connectome.AnnotationFields{ID: "bodyId", Status: "status", Class: "class"},
			Neurotransmitters: connectome.NeurotransmitterFields{ID: "body", Consensus: "consensus_nt"},
		},
		Identity: connectome.IdentityMapping{
			WeightsEndpointsAreAnnotationIDs:    true,
			NeurotransmitterIDsAreAnnotationIDs: true,
			Evidence:                            "fixture",
		},
		Selection:          connectome.SelectionPredicate{Source: connectome.RoleAnnotations, Field: "status", Equals: "Traced", Label: "engineering selection"},
		DuplicateSemantics: connectome.DuplicateSemanticsUnknown,
		CoordinateUnit:     "unverified",
		TransformHistory:   []connectome.TransformStep{{Step: "generate", Description: "fixture", Version: "test"}},
	}
	result, err := connectome.Build(context.Background(), connectome.BuildRequest{
		Manifest: manifest,
		Limits: connectome.ResourceLimits{
			MaxMemoryBytes: 8 << 20, MaxTempBytes: 8 << 20, MaxRunFiles: 16,
			MaxArrowBytes: 8 << 20, MaxFooterBytes: 1 << 20, MaxRows: 10000,
		},
		TempDir:  t.TempDir(),
		EdgeView: connectome.EdgeViewRows,
	})
	if err != nil {
		t.Fatalf("build generated graph: %v", err)
	}
	if result.Graph.NodeCount() != 40 || result.Graph.EdgeCount() != 300 {
		t.Fatalf("generated graph has %d nodes and %d edges, want 40 and 300", result.Graph.NodeCount(), result.Graph.EdgeCount())
	}
	return result.Graph
}

// uniformNullProtocol is a minimal LIF protocol on the uniform source, used
// where only the parameter construction and the topology matter.
func uniformNullProtocol(steps int) Protocol {
	p := lifProtocol(steps)
	p.Probes = []Probe{{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput}}
	return p
}

func degrees(t *testing.T, nodes int, sources, targets []int) ([]int, []int) {
	t.Helper()
	out := make([]int, nodes)
	in := make([]int, nodes)
	for i, source := range sources {
		out[source]++
		in[targets[i]]++
	}
	return out, in
}

// graphFingerprint streams the graph again and hashes the topology it reports,
// so a test can prove the null model never touched the original.
func graphFingerprint(t *testing.T, g *connectome.Graph) string {
	t.Helper()
	sources, targets, err := streamTopology(context.Background(), g, int(g.NodeCount()), int(g.EdgeCount()))
	if err != nil {
		t.Fatalf("stream topology: %v", err)
	}
	hash, err := topologyHash(sources, targets)
	if err != nil {
		t.Fatalf("topology hash: %v", err)
	}
	return hash
}

// setFingerprint hashes the mutable arrays of a derived parameter set.
func setFingerprint(set *params.Set) string {
	digest := sha256.New()
	buffer := make([]byte, 8)
	for _, sign := range set.EdgeSign {
		digest.Write([]byte{byte(sign)})
	}
	for _, weight := range set.EdgeWeight {
		binary.LittleEndian.PutUint64(buffer, uint64(int64(weight*1e9)))
		digest.Write(buffer)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func TestDegreePreservingRewireKeepsEveryDegreeAndPairUnique(t *testing.T) {
	g := generatedGraph(t)
	protocol := uniformNullProtocol(2)
	protocol.Probes = []Probe{{Name: "all", Nodes: []int{0, 1}, Reduce: ReduceSumOutput}}
	before := graphFingerprint(t, g)

	original, err := OriginalVariant(context.Background(), g, nil, "", protocol, testLimits())
	if err != nil {
		t.Fatalf("original variant: %v", err)
	}
	if original.Name != VariantOriginal || original.Null != nil {
		t.Fatalf("original variant = %q with null model %+v", original.Name, original.Null)
	}
	wantOut, wantIn := degrees(t, 40, original.Sources, original.Targets)

	variant, report, err := DeriveNullModel(context.Background(), g, nil, "", protocol,
		NullModelSpec{Kind: NullDegreePreservingRewire, Seed: 7, SwapFactor: 2}, testLimits())
	if err != nil {
		t.Fatalf("rewire: %v", err)
	}
	if report.Kind != NullDegreePreservingRewire || report.Seed != 7 || report.PRNG != NullPRNG {
		t.Fatalf("report identity = %+v", report)
	}
	// attempts = ceil(2 * 300)
	if report.Attempts != 600 {
		t.Fatalf("attempts = %d, want 600", report.Attempts)
	}
	if report.Applied+report.Rejected.SelfLoop+report.Rejected.Duplicate != report.Attempts {
		t.Fatalf("applied %d + rejected %+v does not add up to %d attempts", report.Applied, report.Rejected, report.Attempts)
	}
	if report.Applied == 0 {
		t.Fatalf("no swap was applied on a 300 edge graph: %+v", report)
	}
	gotOut, gotIn := degrees(t, 40, variant.Sources, variant.Targets)
	for i := range wantOut {
		if gotOut[i] != wantOut[i] || gotIn[i] != wantIn[i] {
			t.Fatalf("node %d degrees = out %d in %d, want out %d in %d", i, gotOut[i], gotIn[i], wantOut[i], wantIn[i])
		}
	}
	seen := map[[2]int]bool{}
	for i, source := range variant.Sources {
		target := variant.Targets[i]
		if source == target {
			t.Fatalf("edge %d is a self loop at node %d", i, source)
		}
		if seen[[2]int{source, target}] {
			t.Fatalf("pair %d->%d appears twice after rewiring", source, target)
		}
		seen[[2]int{source, target}] = true
	}
	// The rewired topology must actually differ from the original.
	if variant.Null == nil || variant.Null.TopologyHash != report.TopologyHash {
		t.Fatalf("variant carries no matching null report: %+v", variant.Null)
	}
	if report.TopologyHash == before {
		t.Fatalf("the rewired topology hash equals the original")
	}
	// Parameters follow the edges: the weight multiset never changes.
	if !sameMultiset(variant.Params.Weights, original.Params.Weights) {
		t.Fatalf("rewiring changed the weight multiset")
	}
	if report.ParameterHash != variant.Params.Hash || variant.Params.Hash != original.Params.Hash {
		t.Fatalf("rewiring changed the parameter hash: %q vs %q", variant.Params.Hash, original.Params.Hash)
	}
	if after := graphFingerprint(t, g); after != before {
		t.Fatalf("the original graph changed: %q -> %q", before, after)
	}
}

func TestDegreePreservingRewireIsReproducibleFromItsSeed(t *testing.T) {
	g := generatedGraph(t)
	protocol := uniformNullProtocol(2)
	protocol.Probes = []Probe{{Name: "all", Nodes: []int{0, 1}, Reduce: ReduceSumOutput}}
	spec := NullModelSpec{Kind: NullDegreePreservingRewire, Seed: 42, SwapFactor: 1}
	first, firstReport, err := DeriveNullModel(context.Background(), g, nil, "", protocol, spec, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	second, secondReport, err := DeriveNullModel(context.Background(), g, nil, "", protocol, spec, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if firstReport != secondReport {
		t.Fatalf("the same seed produced different reports:\n%+v\n%+v", firstReport, secondReport)
	}
	for i := range first.Targets {
		if first.Targets[i] != second.Targets[i] || first.Sources[i] != second.Sources[i] {
			t.Fatalf("the same seed produced a different edge %d", i)
		}
	}
	spec.Seed = 43
	other, otherReport, err := DeriveNullModel(context.Background(), g, nil, "", protocol, spec, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if otherReport.TopologyHash == firstReport.TopologyHash {
		t.Fatal("two different seeds produced the same topology")
	}
	if len(other.Targets) != len(first.Targets) {
		t.Fatalf("seed 43 produced %d edges, want %d", len(other.Targets), len(first.Targets))
	}
}

// TestDegreePreservingRewireRejectsEverySwapOnTheTinyFixture pins the rejection
// counting: the three neuron fixture has exactly the two edges 0->1 and 1->2,
// so the only possible swap makes 0->2 and the self loop 1->1 and is refused.
func TestDegreePreservingRewireRejectsEverySwapOnTheTinyFixture(t *testing.T) {
	g := fixtureGraph(t)
	protocol := uniformNullProtocol(2)
	variant, report, err := DeriveNullModel(context.Background(), g, nil, "", protocol,
		NullModelSpec{Kind: NullDegreePreservingRewire, Seed: 1, SwapFactor: 3}, testLimits())
	if err != nil {
		t.Fatalf("rewire: %v", err)
	}
	if report.Attempts != 6 || report.Applied != 0 || report.Rejected.SelfLoop != 6 || report.Rejected.Duplicate != 0 {
		t.Fatalf("report = %+v; every attempt must be refused as a self loop", report)
	}
	if variant.Sources[0] != 0 || variant.Targets[0] != 1 || variant.Sources[1] != 1 || variant.Targets[1] != 2 {
		t.Fatalf("a refused rewire changed the topology: %v -> %v", variant.Sources, variant.Targets)
	}
}

func TestSignShuffleKeepsTheSignMultisetAndNeedsTheDerivedSource(t *testing.T) {
	set, g := derivedFixtureSet(t)
	protocol := derivedProtocol(CoreLIF, UnknownSignExcitatory, fixtureWeightScale, 3)
	beforeGraph := graphFingerprint(t, g)
	beforeSet := setFingerprint(set)

	original, err := OriginalVariant(context.Background(), g, set, fixtureSetSHA256, protocol, testLimits())
	if err != nil {
		t.Fatalf("original variant: %v", err)
	}
	variant, report, err := DeriveNullModel(context.Background(), g, set, fixtureSetSHA256, protocol,
		NullModelSpec{Kind: NullSignShuffle, Seed: 5}, testLimits())
	if err != nil {
		t.Fatalf("sign shuffle: %v", err)
	}
	if report.Kind != NullSignShuffle || report.Seed != 5 || report.Rejected.SelfLoop != 0 || report.Rejected.Duplicate != 0 {
		t.Fatalf("report = %+v", report)
	}
	if report.TopologyHash != mustTopologyHash(t, original.Sources, original.Targets) {
		t.Fatal("a sign shuffle changed the topology")
	}
	// The fixture has one +1, two -1 and three unknown edges; the shuffle
	// permutes those labels, so the summary counts never move.
	summary := variant.Params.Derived
	if summary == nil || summary.PositiveEdges != 1 || summary.NegativeEdges != 2 || summary.UnknownSignEdges != 3 {
		t.Fatalf("sign counts changed: %+v", summary)
	}
	if report.ParameterHash == original.Params.Hash {
		t.Fatal("the sign shuffle left the parameter hash unchanged")
	}
	if !sameMultiset(absAll(variant.Params.Weights), absAll(original.Params.Weights)) {
		t.Log("magnitudes may move with the sign policy; only the sign labels are permuted")
	}
	if after := graphFingerprint(t, g); after != beforeGraph {
		t.Fatal("the sign shuffle changed the graph")
	}
	if after := setFingerprint(set); after != beforeSet {
		t.Fatal("the sign shuffle changed the caller's parameter set")
	}

	uniform := uniformNullProtocol(3)
	if _, _, err := DeriveNullModel(context.Background(), fixtureGraph(t), nil, "", uniform,
		NullModelSpec{Kind: NullSignShuffle, Seed: 5}, testLimits()); err == nil {
		t.Fatal("a sign shuffle was accepted for the uniform parameter source")
	}
}

func TestWeightShuffleKeepsTheStrengthMultisetOnBothSources(t *testing.T) {
	g := generatedGraph(t)
	protocol := uniformNullProtocol(2)
	protocol.Probes = []Probe{{Name: "all", Nodes: []int{0, 1}, Reduce: ReduceSumOutput}}
	original, err := OriginalVariant(context.Background(), g, nil, "", protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	variant, report, err := DeriveNullModel(context.Background(), g, nil, "", protocol,
		NullModelSpec{Kind: NullWeightShuffle, Seed: 11}, testLimits())
	if err != nil {
		t.Fatalf("weight shuffle: %v", err)
	}
	if report.TopologyHash != mustTopologyHash(t, original.Sources, original.Targets) {
		t.Fatal("a weight shuffle changed the topology")
	}
	if !sameMultiset(variant.Params.Weights, original.Params.Weights) {
		t.Fatal("the weight shuffle changed the strength multiset")
	}
	if equalSeries(variant.Params.Weights, original.Params.Weights) {
		t.Fatal("the weight shuffle returned the original order")
	}

	set, derivedGraph := derivedFixtureSet(t)
	derived := derivedProtocol(CoreLIF, UnknownSignInhibitory, fixtureWeightScale, 3)
	beforeSet := setFingerprint(set)
	derivedOriginal, err := OriginalVariant(context.Background(), derivedGraph, set, fixtureSetSHA256, derived, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	shuffled, _, err := DeriveNullModel(context.Background(), derivedGraph, set, fixtureSetSHA256, derived,
		NullModelSpec{Kind: NullWeightShuffle, Seed: 3}, testLimits())
	if err != nil {
		t.Fatalf("derived weight shuffle: %v", err)
	}
	// Signs are untouched, so every weight keeps the sign of its own edge.
	for i, weight := range shuffled.Params.Weights {
		if sign(weight) != 0 && sign(derivedOriginal.Params.Weights[i]) != 0 && sign(weight) != sign(derivedOriginal.Params.Weights[i]) {
			t.Fatalf("edge %d changed sign: %v -> %v", i, derivedOriginal.Params.Weights[i], weight)
		}
	}
	if !sameMultiset(absAll(shuffled.Params.Weights), absAll(derivedOriginal.Params.Weights)) {
		t.Fatal("the derived weight shuffle changed the magnitude multiset")
	}
	if after := setFingerprint(set); after != beforeSet {
		t.Fatal("the derived weight shuffle changed the caller's parameter set")
	}
}

func TestNullModelSpecValidation(t *testing.T) {
	g := generatedGraph(t)
	protocol := uniformNullProtocol(2)
	protocol.Probes = []Probe{{Name: "all", Nodes: []int{0, 1}, Reduce: ReduceSumOutput}}
	for name, spec := range map[string]NullModelSpec{
		"unknown kind":            {Kind: "rewire", Seed: 1, SwapFactor: 1},
		"empty kind":              {Seed: 1, SwapFactor: 1},
		"rewire without factor":   {Kind: NullDegreePreservingRewire, Seed: 1},
		"negative factor":         {Kind: NullDegreePreservingRewire, Seed: 1, SwapFactor: -1},
		"shuffle with factor":     {Kind: NullWeightShuffle, Seed: 1, SwapFactor: 1},
		"sign shuffle on uniform": {Kind: NullSignShuffle, Seed: 1},
	} {
		if _, _, err := DeriveNullModel(context.Background(), g, nil, "", protocol, spec, testLimits()); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, _, err := DeriveNullModel(context.Background(), g, nil, "", protocol,
		NullModelSpec{Kind: NullDegreePreservingRewire, Seed: 1, SwapFactor: 1}, Limits{MaxMemoryBytes: 1024}); err == nil {
		t.Error("a rewire was accepted under a limit its hash table cannot fit")
	}
}

func TestBuildVariantRunsTheRewiredTopology(t *testing.T) {
	g := generatedGraph(t)
	protocol := uniformNullProtocol(4)
	protocol.Probes = []Probe{{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput}}
	variant, report, err := DeriveNullModel(context.Background(), g, nil, "", protocol,
		NullModelSpec{Kind: NullDegreePreservingRewire, Seed: 9, SwapFactor: 1}, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := BuildVariant(context.Background(), g, variant, protocol, testLimits())
	if err != nil {
		t.Fatalf("build variant: %v", err)
	}
	run, err := runner.Run(context.Background(), runner.Stimulus())
	if err != nil {
		t.Fatalf("run variant: %v", err)
	}
	if run.TopologyHash != report.TopologyHash {
		t.Fatalf("run topology hash %q, variant %q", run.TopologyHash, report.TopologyHash)
	}
	if run.NullModel == nil || *run.NullModel != report {
		t.Fatalf("run report null model = %+v, want %+v", run.NullModel, report)
	}

	// The original path still reports a topology hash and no null model.
	params := mustParameters(t, g, *protocol.Uniform)
	plain, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	plainRun, err := plain.Run(context.Background(), plain.Stimulus())
	if err != nil {
		t.Fatal(err)
	}
	if plainRun.NullModel != nil {
		t.Fatalf("an original run carries a null model: %+v", plainRun.NullModel)
	}
	if plainRun.TopologyHash == "" || plainRun.TopologyHash == run.TopologyHash {
		t.Fatalf("original topology hash %q, rewired %q", plainRun.TopologyHash, run.TopologyHash)
	}
	if plainRun.ProtocolHash != run.ProtocolHash || plainRun.ParameterHash != run.ParameterHash {
		t.Fatal("the rewire changed the protocol or parameter hash")
	}
}

func mustTopologyHash(t *testing.T, sources, targets []int) string {
	t.Helper()
	hash, err := topologyHash(sources, targets)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func sameMultiset(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	a := append([]float64(nil), got...)
	b := append([]float64(nil), want...)
	sort.Float64s(a)
	sort.Float64s(b)
	return equalSeries(a, b)
}

func absAll(values []float64) []float64 {
	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = v
		if v < 0 {
			out[i] = -v
		}
	}
	return out
}

func sign(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// TestPairSetMatchesAMapOracle exercises the open addressing table, and in
// particular the backward shift deletion, against a plain map over a long
// deterministic sequence of adds, removals and lookups.
func TestPairSetMatchesAMapOracle(t *testing.T) {
	const nodes = 24
	set, err := newPairSet(200)
	if err != nil {
		t.Fatal(err)
	}
	oracle := map[uint64]struct{}{}
	state := uint64(0x9e3779b97f4a7c15)
	next := func(n uint64) uint64 {
		state = state*6364136223846793005 + 1442695040888963407
		return (state >> 33) % n
	}
	for step := 0; step < 200000; step++ {
		key := pairKey(int(next(nodes)), int(next(nodes)))
		_, present := oracle[key]
		if set.has(key) != present {
			t.Fatalf("step %d: has(%d) = %v, oracle %v", step, key, set.has(key), present)
		}
		if present {
			if !set.remove(key) {
				t.Fatalf("step %d: remove(%d) reported nothing to delete", step, key)
			}
			delete(oracle, key)
			continue
		}
		if !set.add(key) {
			t.Fatalf("step %d: add(%d) reported a duplicate", step, key)
		}
		oracle[key] = struct{}{}
		if set.count != len(oracle) {
			t.Fatalf("step %d: table holds %d keys, oracle %d", step, set.count, len(oracle))
		}
	}
	for key := range oracle {
		if !set.has(key) {
			t.Fatalf("key %d was lost by the table", key)
		}
	}
}

// TestTopologyHashMatchesItsDocumentedEncoding pins the hash against values
// computed outside this package from the documented encoding: every source
// index followed by every target index, each as a little-endian uint32, hashed
// with SHA-256. The second case crosses the internal block boundary the hash
// writes in, which must not change a single byte of the result.
func TestTopologyHashMatchesItsDocumentedEncoding(t *testing.T) {
	got, err := topologyHash([]int{0, 1}, []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if want := "f75a605a1fcbaea023eb618377547dbff016670bd1b0aeafce56232d837e0d71"; got != want {
		t.Fatalf("topology hash = %q, want %q", got, want)
	}
	sources := make([]int, 3000)
	targets := make([]int, 3000)
	for i := range sources {
		sources[i], targets[i] = i, i+1
	}
	if got, err = topologyHash(sources, targets); err != nil {
		t.Fatal(err)
	}
	if want := "7b5f9ffde5eff20eefaaa58fbf4ddc455594681e06f6ee48b182a9db9517c187"; got != want {
		t.Fatalf("three thousand edge topology hash = %q, want %q", got, want)
	}
	if _, err := topologyHash([]int{0}, []int{0, 1}); err == nil {
		t.Fatal("mismatched array lengths were hashed")
	}
	if _, err := topologyHash([]int{-1}, []int{0}); err == nil {
		t.Fatal("a negative node index was hashed")
	}
}
